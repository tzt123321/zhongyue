from fastapi import APIRouter, Depends, HTTPException, Query
from sqlalchemy import select, func, desc, or_
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy.orm import selectinload
from typing import Optional
from datetime import datetime, timedelta
import hashlib

from app.database import get_db
from app.models import Track, Artist, TrackArtist, Album
from app.schemas.track import TrackResponse, TrackListResponse
from app.state import get_daily_recommend_cache

router = APIRouter()


@router.get("", response_model=TrackListResponse)
async def list_tracks(
    skip: int = Query(0, ge=0),
    limit: int = Query(50, ge=1, le=200),
    sort: str = Query("id"),
    order: str = Query("desc"),
    search: Optional[str] = Query(None, description="搜索曲目标题/艺术家"),
    db: AsyncSession = Depends(get_db),
):
    # Base query with eager loading
    base_query = select(Track).options(selectinload(Track.artist), selectinload(Track.album))

    # Apply search filter
    if search:
        search_pattern = f"%{search}%"
        # 主艺术家匹配（原有）
        artist_filter = Artist.name.ilike(search_pattern)
        
        # 也匹配 secondary artists（通过 track_artists 表）
        secondary_filter = (
            Track.id.in_(
                select(TrackArtist.track_id)
                .join(Artist, TrackArtist.artist_id == Artist.id)
                .where(Artist.name.ilike(search_pattern))
            )
        )
        
        base_query = base_query.join(Track.artist).where(
            or_(
                Track.title.ilike(search_pattern),
                artist_filter,
                secondary_filter
            )
        )

    # Count total
    count_query = select(func.count(Track.id))
    if search:
        search_pattern = f"%{search}%"
        count_query = count_query.select_from(Track).join(Track.artist).where(
            or_(
                Track.title.ilike(search_pattern),
                Artist.name.ilike(search_pattern),
                Track.id.in_(
                    select(TrackArtist.track_id)
                    .join(Artist, TrackArtist.artist_id == Artist.id)
                    .where(Artist.name.ilike(search_pattern))
                )
            )
        )
    count_result = await db.execute(count_query)
    total = count_result.scalar()

    # Fetch items
    sort_column = getattr(Track, sort, Track.id)
    if order == "asc":
        query = (
            base_query
            .order_by(sort_column.asc())
            .offset(skip)
            .limit(limit)
        )
    else:
        query = (
            base_query
            .order_by(sort_column.desc())
            .offset(skip)
            .limit(limit)
        )

    result = await db.execute(query)
    items = result.scalars().all()

    return TrackListResponse(total=total, items=items)


@router.get("/recent", response_model=TrackListResponse)
async def recent_tracks(
    limit: int = Query(20, ge=1, le=50),
    db: AsyncSession = Depends(get_db),
):
    result = await db.execute(
        select(Track)
        .options(selectinload(Track.artist), selectinload(Track.album))
        .order_by(Track.created_at.desc())
        .limit(limit)
    )
    items = result.scalars().all()
    return TrackListResponse(total=len(items), items=items)


@router.get("/popular", response_model=TrackListResponse)
async def popular_tracks(
    limit: int = Query(20, ge=1, le=50),
    db: AsyncSession = Depends(get_db),
):
    result = await db.execute(
        select(Track)
        .options(selectinload(Track.artist), selectinload(Track.album))
        .where(Track.play_count > 0)
        .order_by(Track.play_count.desc())
        .limit(limit)
    )
    items = result.scalars().all()
    return TrackListResponse(total=len(items), items=items)


@router.get("/recommend", response_model=TrackResponse)
async def daily_recommend(db: AsyncSession = Depends(get_db)):
    """
    今日推荐：从后端缓存返回，北京时间每日 08:00 自动刷新。
    如果缓存的 track 已被删除，则自动从现有曲库中选一首替代。
    """
    cache = get_daily_recommend_cache()
    track = cache.get("track")
    today = datetime.utcnow().strftime("%Y-%m-%d")

    # If cache is stale (new day), or cached track no longer exists, refresh
    if not track or cache.get("date") != today:
        need_refresh = True
    else:
        # Cached track may have been deleted; verify it still exists in DB
        check = await db.execute(select(Track).where(Track.id == track.id))
        need_refresh = check.scalar_one_or_none() is None

    if need_refresh:
        seed = int(hashlib.md5(today.encode()).hexdigest()[:8], 16)
        result = await db.execute(select(func.count(Track.id)))
        total = result.scalar() or 1
        offset_idx = seed % total
        result = await db.execute(
            select(Track)
            .options(selectinload(Track.artist), selectinload(Track.album))
            .offset(offset_idx)
            .limit(1)
        )
        track = result.scalar_one_or_none()
        cache["track"] = track
        cache["date"] = today

    if not track:
        raise HTTPException(status_code=404, detail="No tracks available")
    return track


@router.get("/{track_id}", response_model=TrackResponse)
async def get_track(track_id: int, db: AsyncSession = Depends(get_db)):
    result = await db.execute(
        select(Track)
        .options(selectinload(Track.artist), selectinload(Track.album))
        .where(Track.id == track_id)
    )
    track = result.scalar_one_or_none()
    if not track:
        from fastapi import HTTPException
        raise HTTPException(status_code=404, detail="Track not found")
    return track


@router.delete("/{track_id}", status_code=204)
async def delete_track(track_id: int, db: AsyncSession = Depends(get_db)):
    result = await db.execute(select(Track).where(Track.id == track_id))
    track = result.scalar_one_or_none()
    if not track:
        raise HTTPException(status_code=404, detail="Track not found")

    album_id = track.album_id
    artist_id = track.artist_id  # capture primary artist before delete
    await db.delete(track)
    await db.flush()

    # Cascade: delete empty album
    if album_id:
        track_count_result = await db.execute(
            select(func.count(Track.id)).where(Track.album_id == album_id)
        )
        if track_count_result.scalar() == 0:
            # Album is now empty — delete it and check if artist is orphaned
            album_result = await db.execute(select(Album).where(Album.id == album_id))
            album = album_result.scalar_one_or_none()
            if album:
                await db.delete(album)
                await db.flush()

            # Check if primary artist has no remaining albums
            if artist_id:
                artist_album_count = await db.execute(
                    select(func.count(Album.id)).where(Album.artist_id == artist_id)
                )
                if artist_album_count.scalar() == 0:
                    artist_result = await db.execute(select(Artist).where(Artist.id == artist_id))
                    artist = artist_result.scalar_one_or_none()
                    if artist:
                        await db.delete(artist)

    await db.commit()
