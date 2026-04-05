import os
import aiofiles
from fastapi import HTTPException, Request
from fastapi.responses import StreamingResponse
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from app.database import get_db
from app.models import Track
from app.utils.range import get_range_info
from app.config import settings


async def stream_track(track_id: int, request: Request, db: AsyncSession):
    """Stream audio file with HTTP Range support."""
    result = await db.execute(select(Track).where(Track.id == track_id))
    track = result.scalar_one_or_none()
    if not track:
        raise HTTPException(status_code=404, detail="Track not found")

    file_path = track.file_path
    if not os.path.exists(file_path):
        raise HTTPException(status_code=404, detail="File not found")

    file_size = os.path.getsize(file_path)
    start, end, total = get_range_info(request, file_size)
    content_length = end - start + 1

    # Determine content type
    content_types = {
        ".mp3": "audio/mpeg",
        ".flac": "audio/flac",
        ".wav": "audio/wav",
        ".m4a": "audio/mp4",
        ".ogg": "audio/ogg",
        ".opus": "audio/opus",
        ".aac": "audio/aac",
    }
    ext = os.path.splitext(file_path.lower())[1]
    content_type = content_types.get(ext, "application/octet-stream")

    async def iterfile():
        async with aiofiles.open(file_path, "rb") as f:
            await f.seek(start)
            remaining = content_length
            chunk_size = settings.BUFFER_SIZE
            while remaining > 0:
                chunk = await f.read(min(chunk_size, remaining))
                if not chunk:
                    break
                remaining -= len(chunk)
                yield chunk

    headers = {
        "Content-Type": content_type,
        "Content-Length": str(content_length),
        "Accept-Ranges": "bytes",
        "Content-Range": f"bytes {start}-{end}/{total}",
    }

    return StreamingResponse(iterfile(), status_code=206, headers=headers)
