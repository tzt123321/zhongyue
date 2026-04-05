import logging
import os
import re
import shutil
import traceback
import uuid
from pathlib import Path
from typing import List, Optional

from fastapi import APIRouter, Depends, HTTPException, UploadFile, File, Form
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession

from app.database import get_db
from app.models.user import User
from app.models.track import Track
from app.models.album import Album
from app.models.artist import Artist
from app.api.auth import get_current_user
from app.services.metadata import extract_metadata

logger = logging.getLogger(__name__)

router = APIRouter()

# ── Input sanitization ──────────────────────────────────────────────────

INJECT_CHARS_RE = re.compile(r'[<>&\'";\\]|(\.\.)|[\/\\\\]')

def sanitize_track_input(value: str, field_name: str, max_len: int = 255) -> str:
    """防止 XSS / 路径穿越 / SQL 注入"""
    if not value:
        return value
    if INJECT_CHARS_RE.search(value):
        raise HTTPException(status_code=400, detail=f"{field_name}含有非法字符")
    return value.strip()[:max_len]

UPLOAD_DIR = Path("/app/backend/uploads")
UPLOAD_DIR.mkdir(exist_ok=True)

ALLOWED_EXTENSIONS = {".mp3", ".flac", ".wav", ".m4a", ".ogg", ".aac", ".opus"}
MAX_FILE_SIZE = 500 * 1024 * 1024  # 500MB


def split_artists(name: str) -> List[str]:
    """Split artist names by common separators."""
    for sep in ["&", "/", ",", ";", "+"]:
        if sep in name:
            return [a.strip() for a in name.split(sep) if a.strip()]
    return [name.strip()]


@router.post("/upload")
async def upload_tracks(
    title: str = Form(...),
    artist_name: str = Form("未知艺术家"),
    album_name: str = Form("未知专辑"),
    year: Optional[int] = Form(None),
    genre: Optional[str] = Form(None),
    db: AsyncSession = Depends(get_db),
    current_user: User = Depends(get_current_user),
):
    """批量上传音乐文件（仅乐主和管理员）。"""
    # 权限检查
    if not (current_user.is_admin or current_user.is_musician):
        raise HTTPException(status_code=403, detail="只有乐主或管理员可以上传音乐")

    uploaded_files: List[UploadFile] = []
    # 接受多个文件字段
    files_raw = File(default=None)
    # FastAPI 处理多文件需要特殊方式
    return {"message": "see multipart endpoint"}


@router.post("/upload/batch")
async def upload_batch(
    files: List[UploadFile],
    title: Optional[str] = Form(None),
    artist_name: Optional[str] = Form(None),
    album_name: Optional[str] = Form(None),
    year: Optional[int] = Form(None),
    genre: Optional[str] = Form(None),
    db: AsyncSession = Depends(get_db),
    current_user: User = Depends(get_current_user),
):
    """批量上传多个音乐文件（仅乐主和管理员）。
    
    每个文件作为一个独立曲目入库。
    """
    if not (current_user.is_admin or current_user.is_musician):
        raise HTTPException(status_code=403, detail="只有乐主或管理员可以上传音乐")

    if not files:
        raise HTTPException(status_code=400, detail="请选择至少一个文件")

    results = []
    errors = []

    for file in files:
        if not file.filename:
            continue
        ext = os.path.splitext(file.filename)[1].lower()
        if ext not in ALLOWED_EXTENSIONS:
            errors.append(f"{file.filename}: 不支持的格式 ({ext})，仅支持 MP3/FLAC/WAV/M4A/OGG/AAC/OPUS")
            continue

        # 文件大小校验
        content = file.file.read()
        file.file.seek(0)  # 重置指针，不丢失内容
        if len(content) > MAX_FILE_SIZE:
            errors.append(f"{file.filename}: 文件超过500MB限制")
            continue

        # 生成唯一文件名
        safe_name = f"{uuid.uuid4().hex}{ext}"
        file_path = UPLOAD_DIR / safe_name

        try:
            # 保存文件
            with file_path.open("wb") as f:
                shutil.copyfileobj(file.file, f)

            # 提取元数据（带兜底fallback）
            filename_title = file.filename.rsplit(".", 1)[0].strip()
            try:
                metadata = extract_metadata(str(file_path)) or {}
            except Exception as e:
                logger.warning(f"[UPLOAD] extract_metadata failed for {file.filename}: {e}\n{traceback.format_exc()}")
                metadata = {}

            track_title = sanitize_track_input(title or metadata.get("title") or filename_title or "未知曲目", "曲目标题")
            track_artist = sanitize_track_input(artist_name or metadata.get("artist") or metadata.get("albumartist") or "未知艺术家", "艺术家名")
            track_album = sanitize_track_input(album_name or metadata.get("album") or "未知专辑", "专辑名")
            track_year = year or metadata.get("year")
            track_genre = genre or metadata.get("genre")

            # 确保艺术家存在（取第一个为主艺术家）
            artist_names = split_artists(track_artist)
            primary_artist = None
            for aname in artist_names:
                result = await db.execute(select(Artist).where(Artist.name == aname))
                artist = result.scalar_one_or_none()
                if not artist:
                    artist = Artist(name=aname)
                    db.add(artist)
                    await db.flush()
                if primary_artist is None:
                    primary_artist = artist

            # 确保专辑存在（按专辑名唯一）
            result = await db.execute(
                select(Album).where(Album.name == track_album).limit(1)
            )
            album = result.scalar_one_or_none()
            if not album:
                album_artist_name = artist_names[0] if artist_names else (primary_artist.name if primary_artist else "未知艺术家")
                result = await db.execute(select(Artist).where(Artist.name == album_artist_name))
                album_artist = result.scalar_one_or_none()
                if not album_artist:
                    album_artist = Artist(name=album_artist_name)
                    db.add(album_artist)
                    await db.flush()
                album = Album(name=track_album, artist_id=album_artist.id, year=track_year)
                db.add(album)
                await db.flush()

            # 创建曲目记录
            track = Track(
                title=track_title,
                file_path=str(file_path),
                duration=metadata.get("duration"),
                file_size=file_path.stat().st_size,
                album_id=album.id,
            )
            db.add(track)
            await db.flush()

            # 关联艺术家（仅第一个为主艺术家）
            if primary_artist:
                track.artist_id = primary_artist.id

            await db.commit()
            results.append({"filename": file.filename, "track_id": track.id, "title": track_title})

        except Exception as e:
            errors.append(f"{file.filename}: 元数据解析失败: {str(e)[:100]}")
            # 清理失败的文件
            if file_path.exists():
                file_path.unlink()

    return {
        "uploaded": results,
        "errors": errors,
        "summary": f"成功 {len(results)} 个，失败 {len(errors)} 个"
    }
