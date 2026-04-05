"""
元数据刮削 API
"""
from fastapi import APIRouter, Depends, HTTPException
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy.orm import selectinload
from pydantic import BaseModel
from typing import Optional

from app.database import get_db
from app.models import Track, Album, Artist
from app.services.music_metadata import (
    scrape_all_for_track,
    scrape_album_cover,
    scrape_artist_image,
)


router = APIRouter(prefix="/scrape", tags=["scraper"])


class ScrapeTrackResponse(BaseModel):
    success: bool
    cover_path: Optional[str]
    artist_cover_path: Optional[str]
    lyrics_path: Optional[str]
    message: str


class ScrapeAlbumResponse(BaseModel):
    success: bool
    cover_path: Optional[str]
    message: str


class ScrapeArtistResponse(BaseModel):
    success: bool
    cover_path: Optional[str]
    message: str


@router.post("/track/{track_id}", response_model=ScrapeTrackResponse)
async def scrape_track_metadata(track_id: int, db: AsyncSession = Depends(get_db)):
    """
    为指定曲目刮削所有可用元数据：
    - 专辑封面
    - 艺术家图片  
    - 歌词
    """
    # 获取曲目信息
    result = await db.execute(
        select(Track)
        .options(selectinload(Track.artist), selectinload(Track.album))
        .where(Track.id == track_id)
    )
    track = result.scalar_one_or_none()
    if not track:
        raise HTTPException(status_code=404, detail="Track not found")
    
    # 获取专辑和艺术家信息
    album_name = "Unknown Album"
    artist_name = "Unknown Artist"
    album_id = None
    artist_id = None
    
    if track.album_id:
        album_result = await db.execute(select(Album).where(Album.id == track.album_id))
        album = album_result.scalar_one_or_none()
        if album:
            album_name = album.name
            album_id = album.id
            artist_name = album.artist.name if album.artist else artist_name
    
    if track.artist_id:
        artist_result = await db.execute(select(Artist).where(Artist.id == track.artist_id))
        artist = artist_result.scalar_one_or_none()
        if artist:
            artist_name = artist.name
            artist_id = artist.id
    
    # 执行刮削
    results = await scrape_all_for_track(
        track_title=track.title,
        artist_name=artist_name,
        album_name=album_name,
        track_id=track.id,
        album_id=album_id,
        artist_id=artist_id,
    )
    
    # 更新数据库中的封面路径
    if results["cover_path"] and album_id:
        await db.execute(
            select(Album).where(Album.id == album_id)
        )
        album_result = await db.execute(select(Album).where(Album.id == album_id))
        album = album_result.scalar_one_or_none()
        if album:
            album.cover_path = results["cover_path"]
    
    if results["artist_cover_path"] and artist_id:
        artist_result = await db.execute(select(Artist).where(Artist.id == artist_id))
        artist = artist_result.scalar_one_or_none()
        if artist:
            artist.cover_path = results["artist_cover_path"]
    
    await db.commit()
    
    message_parts = []
    if results["cover_path"]:
        message_parts.append("专辑封面")
    if results["artist_cover_path"]:
        message_parts.append("艺术家图片")
    if results["lyrics_path"]:
        message_parts.append("歌词")
    
    message = f"成功刮削: {', '.join(message_parts)}" if message_parts else "未找到额外元数据"
    
    return ScrapeTrackResponse(
        success=bool(results["cover_path"] or results["artist_cover_path"] or results["lyrics_path"]),
        cover_path=results["cover_path"],
        artist_cover_path=results["artist_cover_path"],
        lyrics_path=results["lyrics_path"],
        message=message,
    )


@router.post("/album/{album_id}", response_model=ScrapeAlbumResponse)
async def scrape_album_metadata(album_id: int, db: AsyncSession = Depends(get_db)):
    """
    为指定专辑刮削封面
    """
    result = await db.execute(select(Album).where(Album.id == album_id))
    album = result.scalar_one_or_none()
    if not album:
        raise HTTPException(status_code=404, detail="Album not found")
    
    artist_name = album.artist.name if album.artist else "Unknown Artist"
    
    cover_path = await scrape_album_cover(album.name, artist_name)
    
    if cover_path:
        album.cover_path = cover_path
        await db.commit()
    
    return ScrapeAlbumResponse(
        success=bool(cover_path),
        cover_path=cover_path,
        message="成功获取专辑封面" if cover_path else "未找到专辑封面",
    )


@router.post("/artist/{artist_id}", response_model=ScrapeArtistResponse)
async def scrape_artist_metadata(artist_id: int, db: AsyncSession = Depends(get_db)):
    """
    为指定艺术家刮削图片
    """
    result = await db.execute(select(Artist).where(Artist.id == artist_id))
    artist = result.scalar_one_or_none()
    if not artist:
        raise HTTPException(status_code=404, detail="Artist not found")
    
    cover_path = await scrape_artist_image(artist.name)
    
    if cover_path:
        artist.cover_path = cover_path
        await db.commit()
    
    return ScrapeArtistResponse(
        success=bool(cover_path),
        cover_path=cover_path,
        message="成功获取艺术家图片" if cover_path else "未找到艺术家图片",
    )


@router.post("/library/scrape-all")
async def scrape_library(db: AsyncSession = Depends(get_db)):
    """
    扫描音乐库，刮削所有缺失封面的专辑和艺术家
    返回需要刮削的数量统计
    """
    # 统计缺失封面的专辑
    albums_without_cover = await db.execute(
        select(Album).where(Album.cover_path == None)
    )
    albums_list = albums_without_cover.scalars().all()
    
    # 统计缺失图片的艺术家
    artists_without_cover = await db.execute(
        select(Artist).where(Artist.cover_path == None)
    )
    artists_list = artists_without_cover.scalars().all()
    
    # 统计缺失歌词的曲目
    # 需要先检查 lyrics 文件是否存在
    
    return {
        "albums_need_cover": len(albums_list),
        "artists_need_image": len(artists_list),
        "message": "使用 POST /scrape/album/{id} 或 /scrape/artist/{id} 单独刮削",
    }
