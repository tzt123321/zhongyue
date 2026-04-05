import os
from datetime import datetime
from typing import List, Dict, Any
from sqlalchemy import select, update, func
from sqlalchemy.ext.asyncio import AsyncSession
import asyncio

from app.models import Artist, Album, Track, TrackArtist
from app.services.metadata import extract_metadata
from app.utils.file import find_audio_files


async def scan_library(db: AsyncSession, music_paths: List[str]) -> Dict[str, Any]:
    """Scan music paths and update the database."""
    audio_files = find_audio_files(music_paths)
    audio_file_set = set(audio_files)
    stats = {"scanned": 0, "added": 0, "updated": 0, "errors": 0, "removed": 0}

    # Get existing tracks for incremental scan
    result = await db.execute(select(Track.file_path))
    existing_paths = {row[0] for row in result.fetchall()}

    # Remove orphaned tracks (files that no longer exist on disk)
    orphaned_paths = existing_paths - audio_file_set
    if orphaned_paths:
        for path in orphaned_paths:
            result = await db.execute(select(Track).where(Track.file_path == path))
            track = result.scalar_one_or_none()
            if track:
                await db.delete(track)
                stats["removed"] += 1

    # Clean up orphaned albums (no tracks left)
    result = await db.execute(
        select(Album).where(
            ~Album.id.in_(select(Track.album_id).where(Track.album_id.isnot(None)))
        )
    )
    for orphan_album in result.scalars().all():
        await db.delete(orphan_album)
        stats["removed"] += 1

    # Clean up orphaned artists (no tracks and no albums left)
    result = await db.execute(
        select(Artist).where(
            ~Artist.id.in_(select(Track.artist_id).where(Track.artist_id.isnot(None))),
            ~Artist.id.in_(select(Album.artist_id).where(Album.artist_id.isnot(None)))
        )
    )
    for orphan_artist in result.scalars().all():
        await db.delete(orphan_artist)

    # Get existing artists and albums
    result = await db.execute(select(Artist))
    artists = {a.name: a for a in result.scalars().all()}
    result = await db.execute(select(Album))
    albums = {(a.name, a.artist_id): a for a in result.scalars().all()}

    # Track IDs that need scraping (new albums/artists without covers)
    new_album_ids = []
    new_artist_ids = []

    # Process each file
    for file_path in audio_files:
        try:
            metadata = extract_metadata(file_path)
            if not metadata:
                stats["errors"] += 1
                continue

            # Get or create artist (use first split artist as primary)
            artist_names = metadata.get("artist_names", [metadata.get("artist", "Unknown Artist")])
            primary_artist_name = artist_names[0]
            if primary_artist_name not in artists:
                artist = Artist(name=primary_artist_name)
                db.add(artist)
                await db.flush()
                artists[primary_artist_name] = artist
                new_artist_ids.append(artist.id)
            artist = artists[primary_artist_name]

            # Ensure all split artists exist in DB
            for aname in artist_names[1:]:
                aname = aname.strip()
                if aname and aname not in artists:
                    a = Artist(name=aname)
                    db.add(a)
                    await db.flush()
                    artists[aname] = a
                    new_artist_ids.append(a.id)

            # Get or create album
            album_name = metadata.get("album", "Unknown Album")
            album_key = (album_name, artist.id)
            if album_key not in albums:
                album = Album(
                    name=album_name,
                    artist_id=artist.id,
                    year=metadata.get("year"),
                    total_tracks=0,
                )
                db.add(album)
                await db.flush()
                albums[album_key] = album
                new_album_ids.append(album.id)
            album = albums[album_key]

            # Check if track exists
            if file_path in existing_paths:
                # Use UPDATE statement instead of ORM modification
                await db.execute(
                    update(Track)
                    .where(Track.file_path == file_path)
                    .values(
                        title=metadata.get("title"),
                        artist_id=artist.id,
                        album_id=album.id,
                        track_number=metadata.get("track_number"),
                        disc_number=metadata.get("disc_number"),
                        duration=metadata.get("duration"),
                        bitrate=metadata.get("bitrate"),
                        format=metadata.get("format"),
                        file_size=metadata.get("file_size"),
                        file_mtime=metadata.get("file_mtime"),
                        updated_at=datetime.utcnow(),
                    )
                )
                # Update track_artists for existing track
                result = await db.execute(select(Track).where(Track.file_path == file_path))
                existing_track = result.scalar_one()
                # Delete old associations
                from app.models.track_artist import TrackArtist as TA
                await db.execute(TA.__table__.delete().where(TA.track_id == existing_track.id))
                # Rebuild associations
                for idx, aname in enumerate(artist_names):
                    aname = aname.strip()
                    if aname and aname in artists:
                        ta = TA(track_id=existing_track.id, artist_id=artists[aname].id, is_primary=(idx == 0))
                        db.add(ta)
                stats["updated"] += 1
            else:
                track = Track(
                    title=metadata.get("title"),
                    artist_id=artist.id,
                    album_id=album.id,
                    track_number=metadata.get("track_number"),
                    disc_number=metadata.get("disc_number"),
                    duration=metadata.get("duration"),
                    bitrate=metadata.get("bitrate"),
                    format=metadata.get("format"),
                    file_path=file_path,
                    file_size=metadata.get("file_size"),
                    file_mtime=metadata.get("file_mtime"),
                )
                db.add(track)
                await db.flush()
                # All collaborative artists (is_primary=True for first)
                for idx, aname in enumerate(artist_names):
                    aname = aname.strip()
                    if aname and aname in artists:
                        track_artist = TrackArtist(
                            track_id=track.id,
                            artist_id=artists[aname].id,
                            is_primary=(idx == 0),
                        )
                        db.add(track_artist)
                stats["added"] += 1

            stats["scanned"] += 1
        except Exception:
            stats["errors"] += 1

    # Update album track counts using COUNT
    for album in albums.values():
        result = await db.execute(
            select(func.count(Track.id)).where(Track.album_id == album.id)
        )
        album.total_tracks = result.scalar() or 0

    await db.commit()
    
    print(f"SCAN_LIBRARY RETURNING: {stats}, new_album_ids={new_album_ids}, new_artist_ids={new_artist_ids}", flush=True)
    return stats, new_album_ids, new_artist_ids


async def auto_scrape_missing(album_ids: List[int], artist_ids: List[int]):
    """Background task to scrape missing metadata"""
    from app.database import AsyncSessionLocal
    try:
        # Import here to avoid circular imports
        from app.services.music_metadata import scrape_album_cover, scrape_artist_image
        from sqlalchemy.orm import selectinload
        
        async with AsyncSessionLocal() as db:
            # Get album and artist info - use selectinload to eagerly load relationships
            for album_id in album_ids:
                result = await db.execute(
                    select(Album).options(selectinload(Album.artist)).where(Album.id == album_id)
                )
                album = result.scalar_one_or_none()
                if album and not album.cover_path:
                    try:
                        artist_name = album.artist.name if album.artist else "Unknown"
                        path = await scrape_album_cover(album.name, artist_name)
                        album.cover_path = path
                        await db.commit()
                        print(f"Auto scraped cover for album {album_id}: {path}", flush=True)
                    except Exception as e:
                        print(f"Auto scrape album {album_id} failed: {e}", flush=True)
            
            for artist_id in artist_ids:
                result = await db.execute(select(Artist).where(Artist.id == artist_id))
                artist = result.scalar_one_or_none()
                if artist and not artist.cover_path:
                    try:
                        path = await scrape_artist_image(artist.name)
                        artist.cover_path = path
                        await db.commit()
                        print(f"Auto scraped image for artist {artist_id}: {path}", flush=True)
                    except Exception as e:
                        print(f"Auto scrape artist {artist_id} failed: {e}", flush=True)
    except Exception as e:
        print(f"Auto scrape failed: {e}", flush=True)
