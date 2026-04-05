from fastapi import APIRouter, Depends, BackgroundTasks, HTTPException
from sqlalchemy import select, func
from sqlalchemy.ext.asyncio import AsyncSession

from app.database import get_db, AsyncSessionLocal
from app.config import settings
from app.services.library import scan_library, auto_scrape_missing
from app.api.auth import get_current_user
from app.models import User, Track, Artist, Album

router = APIRouter()


@router.get("/stats/overview")
async def stats_overview(
    db: AsyncSession = Depends(get_db),
):
    """获取音乐库统计信息"""
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


@router.post("/scan")
async def trigger_scan(
    background_tasks: BackgroundTasks,
    current_user: User = Depends(get_current_user),
):
    if not current_user.is_admin:
        raise HTTPException(status_code=403, detail="Admin only")

    # Run scan synchronously
    async with AsyncSessionLocal() as session:
        result, new_album_ids, new_artist_ids = await scan_library(session, settings.music_paths_list)
    
    # Schedule auto-scrape as background task
    if new_album_ids or new_artist_ids:
        background_tasks.add_task(auto_scrape_background, new_album_ids, new_artist_ids)
    
    return {"status": "completed", "result": result, "paths": settings.music_paths_list}


async def auto_scrape_background(album_ids: list, artist_ids: list):
    """Background task to scrape missing metadata - runs after response is sent"""
    from app.services.library import auto_scrape_missing
    try:
        await auto_scrape_missing(album_ids, artist_ids)
    except Exception as e:
        print(f"Background scrape failed: {e}")
