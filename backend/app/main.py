from fastapi import FastAPI, Request, Depends
from fastapi.middleware.cors import CORSMiddleware
from fastapi.staticfiles import StaticFiles
from fastapi.responses import JSONResponse, FileResponse
from contextlib import asynccontextmanager
import os
from pathlib import Path
import asyncio

from app.database import get_db, AsyncSessionLocal
from app.api import api_router
from app.services.stream import stream_track
from app.config import settings

# Daily recommendation cache
from app.state import _daily_recommend_cache, get_daily_recommend_cache


async def _refresh_daily_recommend():
    """Compute and cache today's recommended track."""
    from sqlalchemy import select, func
    from sqlalchemy.orm import selectinload
    from app.models import Track
    import hashlib
    from datetime import datetime

    today = datetime.utcnow().strftime("%Y-%m-%d")
    seed = int(hashlib.md5(today.encode()).hexdigest()[:8], 16)

    try:
        async with AsyncSessionLocal() as session:
            result = await session.execute(select(func.count(Track.id)))
            total = result.scalar() or 1
            offset_idx = seed % total
            result = await session.execute(
                select(Track)
                .options(selectinload(Track.artist), selectinload(Track.album))
                .offset(offset_idx)
                .limit(1)
            )
            track = result.scalar_one_or_none()
            _daily_recommend_cache["track"] = track
            _daily_recommend_cache["date"] = today
            print(f"[DailyRecommend] Refreshed for {today}: {track.title if track else 'none'}")
    except Exception as e:
        print(f"[DailyRecommend] Refresh failed: {e}")


async def _scheduled_refresh():
    """Called by the scheduler at 8am Beijing time each day."""
    await _refresh_daily_recommend()


def _start_scheduler():
    """Start APScheduler in a background thread."""
    from apscheduler.schedulers.background import BackgroundScheduler
    from apscheduler.triggers.cron import CronTrigger

    scheduler = BackgroundScheduler(timezone="Asia/Shanghai")
    # Run at 08:00 Beijing time every day
    scheduler.add_job(
        _scheduled_refresh,
        CronTrigger(hour=8, minute=0, timezone="Asia/Shanghai"),
        id="daily_recommend_refresh",
        replace_existing=True,
    )
    scheduler.start()
    print("[Scheduler] Started, daily refresh at 08:00 Asia/Shanghai")


@asynccontextmanager
async def lifespan(app: FastAPI):
    # Startup
    os.makedirs("./data", exist_ok=True)

    # Create default admin if no users
    from sqlalchemy import select
    from app.models import User
    from app.utils.password import hash_password
    async with AsyncSessionLocal() as session:
        result = await session.execute(select(User).limit(1))
        if not result.scalar_one_or_none():
            admin = User(
                username="admin",
                password_hash=hash_password("admin"),
                is_admin=True,
            )
            session.add(admin)
            await session.commit()

    # Pre-compute today's recommendation
    await _refresh_daily_recommend()

    # Start background scheduler
    loop = asyncio.get_event_loop()
    loop.run_in_executor(None, _start_scheduler)

    yield

    # Shutdown
    from apscheduler.schedulers.background import BackgroundScheduler
    for job in BackgroundScheduler().get_jobs():
        job.remove()


# Expose cache to endpoints
def get_daily_recommend_cache():
    return _daily_recommend_cache


app = FastAPI(
    title="众乐",
    description="自托管音乐服务器 - 众乐",
    version="0.8.0",
    lifespan=lifespan,
)


@app.middleware("http")
async def limit_upload_size(request: Request, call_next):
    """FastAPI 请求体大小限制（兜底），520MB 阈值返回 413."""
    content_length = request.headers.get("content-length")
    if content_length:
        size = int(content_length)
        if size > 520 * 1024 * 1024:  # 520MB，留余量
            return JSONResponse(status_code=413, content={"detail": "文件过大，最大支持500MB"})
    response = await call_next(request)
    return response

# CORS
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

# Include API routes
app.include_router(api_router, prefix="/api")


# Streaming endpoint - uses Depends for proper async session lifecycle
@app.get("/stream/{track_id}")
async def stream(track_id: int, request: Request, db = Depends(get_db)):
    return await stream_track(track_id, request, db)


# Serve scraped metadata (covers, artist images) - MUST be before SPA mount
DATA_DIR = Path(__file__).parent.parent / "data"
app.mount("/data/covers", StaticFiles(directory=str(DATA_DIR / "covers")), name="covers")
app.mount("/data/artists", StaticFiles(directory=str(DATA_DIR / "artists")), name="artists")
app.mount("/data/lyrics", StaticFiles(directory=str(DATA_DIR / "lyrics")), name="lyrics")

# Serve frontend static files
# Mount at "/" so /assets/ paths from Vue build work correctly
frontend_dist = os.path.join(os.path.dirname(__file__), "..", "..", "frontend", "dist")
if os.path.exists(frontend_dist):
    app.mount("/", StaticFiles(directory=frontend_dist, html=True), name="frontend")

    # SPA fallback — only for non-API, non-stream, non-data paths
    @app.exception_handler(404)
    async def spa_fallback(request: Request, exc):
        path = request.url.path
        if path.startswith("/api") or path.startswith("/stream") or path.startswith("/data"):
            from fastapi.responses import JSONResponse
            return JSONResponse(status_code=404, content={"detail": "Not found"})
        return FileResponse(os.path.join(frontend_dist, "index.html"))



