"""
Cover Art Archive 刮削器 - 获取专辑封面
"""
import aiohttp
from typing import Optional
from pathlib import Path
from .base import BaseScraper


class CoverArtScraper(BaseScraper):
    """Cover Art Archive 刮削器，根据 MusicBrainz Release ID 获取封面"""
    
    BASE_URL = "https://coverartarchive.org"
    
    def __init__(self, cache_ttl: int = 86400 * 30):  # 封面很少变化，缓存30天
        super().__init__(cache_ttl)
        self._timeout = 8
    
    @property
    def name(self) -> str:
        return "coverartarchive"
    
    @property
    def order(self) -> int:
        return 2  # 第二优先级
    
    async def get_cover(self, musicbrainz_id: str) -> Optional[str]:
        """
        根据 MusicBrainz Release ID 获取封面 URL
        优先尝试直接获取 front 图片，如果失败再尝试 API
        """
        cache_key = self._get_cache_key("cover", musicbrainz_id)
        cached = await self._get_from_cache(cache_key)
        if cached:
            return cached
        
        # 方法1: 直接获取 front 图片（最常用）
        cover_url = await self._try_front_image(musicbrainz_id)
        if cover_url:
            await self._set_cache(cache_key, cover_url)
            return cover_url
        
        # 方法2: 通过 API 获取（返回重定向到 archive.org）
        cover_url = await self._try_api(musicbrainz_id)
        if cover_url:
            await self._set_cache(cache_key, cover_url)
            return cover_url
        
        return None
    
    async def _try_front_image(self, release_id: str) -> Optional[str]:
        """尝试直接获取 front 图片"""
        url = f"{self.BASE_URL}/release/{release_id}/front"
        
        async def _do_fetch():
            async with aiohttp.ClientSession() as session:
                async with session.get(
                    url, 
                    timeout=aiohttp.ClientTimeout(total=self._timeout),
                    allow_redirects=False  # 不跟随重定向，我们自己处理
                ) as resp:
                    if resp.status == 200:
                        # 检查是否是图片（通过 Content-Type）
                        content_type = resp.headers.get("Content-Type", "")
                        if "image" in content_type or resp.headers.get("Content-Length", "0") != "0":
                            return url
                    elif resp.status == 307:
                        # 重定向，检查是否指向有效图片
                        location = resp.headers.get("Location", "")
                        if location and "archive.org" in location:
                            # archive.org 可能被屏蔽，返回重定向URL
                            return location
                    return None
        
        return await self._fetch_with_timeout(_do_fetch())
    
    async def _try_api(self, release_id: str) -> Optional[str]:
        """通过 API 获取封面列表"""
        url = f"{self.BASE_URL}/release/{release_id}"
        
        async def _do_fetch():
            async with aiohttp.ClientSession() as session:
                async with session.get(
                    url,
                    timeout=aiohttp.ClientTimeout(total=self._timeout),
                    allow_redirects=True
                ) as resp:
                    if resp.status == 200:
                        data = await resp.json()
                        images = data.get("images", [])
                        # 优先找 front 封面
                        for img in images:
                            if img.get("front"):
                                return img.get("image")
                        # 没有 front，返回第一张
                        if images:
                            return images[0].get("image")
                    return None
        
        return await self._fetch_with_timeout(_do_fetch())
