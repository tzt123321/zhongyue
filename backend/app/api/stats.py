from fastapi import APIRouter, Depends
from sqlalchemy import select, func
from sqlalchemy.ext.asyncio import AsyncSession

from app.database import get_db
from app.models import Track, Artist, Album, User
from app.api.auth import get_current_user
from app.models import User

router = APIRouter()


@router.get("/overview")
async def stats_overview(
    db: AsyncSession = Depends(get_db),
    current_user: User = Depends(get_current_user),
):
    tracks_count = await db.scalar(select(func.count(Track.id)))
    artists_count = await db.scalar(select(func.count(Artist.id)))
    albums_count = await db.scalar(select(func.count(Album.id)))

    total_duration = await db.scalar(select(func.sum(Track.duration)))
    total_size = await db.scalar(select(func.sum(Track.file_size)))

    return {
        "tracks": tracks_count or 0,
        "artists": artists_count or 0,
        "albums": albums_count or 0,
        "total_duration": total_duration or 0,
        "total_size": total_size or 0,
    }
