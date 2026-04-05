from fastapi import APIRouter, Depends, Query
from sqlalchemy import select, func
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy.orm import selectinload

from app.database import get_db
from app.models import Album, Track
from app.schemas.album import AlbumResponse, AlbumListResponse
from app.schemas.track import TrackResponse, TrackListResponse

router = APIRouter()


@router.get("", response_model=AlbumListResponse)
async def list_albums(
    skip: int = Query(0, ge=0),
    limit: int = Query(50, ge=1, le=200),
    db: AsyncSession = Depends(get_db),
):
    count_result = await db.execute(select(func.count(Album.id)))
    total = count_result.scalar()

    result = await db.execute(
        select(Album).options(selectinload(Album.artist)).offset(skip).limit(limit)
    )
    items = result.scalars().all()

    return AlbumListResponse(total=total, items=items)


@router.get("/{album_id}", response_model=AlbumResponse)
async def get_album(album_id: int, db: AsyncSession = Depends(get_db)):
    result = await db.execute(
        select(Album).options(selectinload(Album.artist)).where(Album.id == album_id)
    )
    album = result.scalar_one_or_none()
    if not album:
        from fastapi import HTTPException
        raise HTTPException(status_code=404, detail="Album not found")
    return album


@router.get("/{album_id}/tracks", response_model=TrackListResponse)
async def album_tracks(album_id: int, db: AsyncSession = Depends(get_db)):
    result = await db.execute(
        select(Track)
        .options(selectinload(Track.artist), selectinload(Track.album))
        .where(Track.album_id == album_id)
        .order_by(Track.disc_number, Track.track_number)
    )
    tracks = result.scalars().all()
    return TrackListResponse(total=len(tracks), items=tracks)
