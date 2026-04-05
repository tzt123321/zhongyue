from fastapi import APIRouter, Depends, Query
from sqlalchemy import select, func
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy.orm import selectinload

from app.database import get_db
from app.models import Artist, Album, Track, TrackArtist
from app.schemas.artist import ArtistResponse, ArtistListResponse
from app.schemas.album import AlbumListResponse
from app.schemas.track import TrackListResponse

router = APIRouter()


@router.get("", response_model=ArtistListResponse)
async def list_artists(
    skip: int = Query(0, ge=0),
    limit: int = Query(50, ge=1, le=200),
    db: AsyncSession = Depends(get_db),
):
    count_result = await db.execute(select(func.count(Artist.id)))
    total = count_result.scalar()

    result = await db.execute(select(Artist).offset(skip).limit(limit))
    items = result.scalars().all()

    return ArtistListResponse(total=total, items=items)


@router.get("/{artist_id}", response_model=ArtistResponse)
async def get_artist(artist_id: int, db: AsyncSession = Depends(get_db)):
    result = await db.execute(select(Artist).where(Artist.id == artist_id))
    artist = result.scalar_one_or_none()
    if not artist:
        from fastapi import HTTPException
        raise HTTPException(status_code=404, detail="Artist not found")
    return artist


@router.get("/{artist_id}/albums", response_model=AlbumListResponse)
async def artist_albums(artist_id: int, db: AsyncSession = Depends(get_db)):
    result = await db.execute(
        select(Album).options(selectinload(Album.artist)).where(Album.artist_id == artist_id)
    )
    albums = result.scalars().all()
    return AlbumListResponse(total=len(albums), items=albums)


@router.get("/{artist_id}/tracks", response_model=TrackListResponse)
async def artist_tracks(artist_id: int, db: AsyncSession = Depends(get_db)):
    """Get all tracks where this artist is a collaborator (primary or guest)."""
    result = await db.execute(
        select(Track)
        .join(TrackArtist, TrackArtist.track_id == Track.id)
        .where(TrackArtist.artist_id == artist_id)
        .options(selectinload(Track.artist), selectinload(Track.album))
    )
    tracks = result.scalars().all()
    return TrackListResponse(total=len(tracks), items=tracks)
