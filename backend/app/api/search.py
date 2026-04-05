from fastapi import APIRouter, Depends, Query
from sqlalchemy import select, or_
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy.orm import selectinload

from app.database import get_db
from app.models import Track, Artist, Album
from app.schemas.track import TrackResponse, TrackListResponse
from app.schemas.artist import ArtistResponse, ArtistListResponse
from app.schemas.album import AlbumResponse, AlbumListResponse

router = APIRouter()


@router.get("")
async def search(
    q: str = Query(..., min_length=1),
    type: str = Query("all"),  # all, tracks, artists, albums
    db: AsyncSession = Depends(get_db),
):
    results = {}
    q_lower = q.lower()

    if type in ("all", "tracks"):
        result = await db.execute(
            select(Track)
            .options(selectinload(Track.artist), selectinload(Track.album))
            .where(Track.title.ilike(f"%{q}%"))
            .limit(20)
        )
        tracks = result.scalars().all()
        results["tracks"] = {"total": len(tracks), "items": tracks}

    if type in ("all", "artists"):
        result = await db.execute(
            select(Artist).where(Artist.name.ilike(f"%{q}%")).limit(20)
        )
        artists = result.scalars().all()
        results["artists"] = {"total": len(artists), "items": artists}

    if type in ("all", "albums"):
        result = await db.execute(
            select(Album).where(Album.name.ilike(f"%{q}%")).limit(20)
        )
        albums = result.scalars().all()
        results["albums"] = {"total": len(albums), "items": albums}

    return results
