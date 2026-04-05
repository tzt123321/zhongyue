"""
Last.fm 刮削器 - 获取艺术家信息和标签
"""
import aiohttp
from typing import Optional, Dict, List
from .base import BaseScraper


class LastFMScraper(BaseScraper):
    """Last.fm 刮削器，获取艺术家信息和标签"""
    
    BASE_URL = "https://ws.audioscrobbler.com/2.0"
    
    def __init__(self, api_key: str = "", cache_ttl: int = 86400 * 7):
        super().__init__(cache_ttl)
        self.api_key = api_key or "demo"  # 允许不带 key 的受限查询
        self._timeout = 8
    
    @property
    def name(self) -> str:
        return "lastfm"
    
    @property
    def order(self) -> int:
        return 3
    
    async def get_artist_info(self, artist: str) -> Optional[Dict]:
        """获取艺术家详细信息"""
        if not self.api_key or self.api_key == "demo":
            return None
        
        cache_key = self._get_cache_key("artist", artist)
        cached = await self._get_from_cache(cache_key)
        if cached:
            return cached
        
        params = {
            "method": "artist.getinfo",
            "artist": artist,
            "api_key": self.api_key,
            "format": "json"
        }
        
        async def _do_fetch():
            async with aiohttp.ClientSession() as session:
                async with session.get(
                    self.BASE_URL,
                    params=params,
                    timeout=aiohttp.ClientTimeout(total=self._timeout)
                ) as resp:
                    if resp.status == 200:
                        data = await resp.json()
                        artist_data = data.get("artist", {})
                        if artist_data:
                            return {
                                "biography": artist_data.get("bio", {}).get("content", ""),
                                "tags": [tag["name"] for tag in artist_data.get("tags", {}).get("tag", [])],
                                "similar_artists": [a["name"] for a in artist_data.get("similar", {}).get("artist", [])],
                                "cover_url": artist_data.get("image", [{}])[-1].get("#text", "") if artist_data.get("image") else "",
                                "url": artist_data.get("url", "")
                            }
                    return None
        
        result = await self._fetch_with_timeout(_do_fetch())
        if result:
            await self._set_cache(cache_key, result)
        return result
    
    async def get_artist_cover(self, artist: str) -> Optional[str]:
        """获取艺术家封面图片"""
        if not self.api_key or self.api_key == "demo":
            return None
        
        params = {
            "method": "artist.getinfo",
            "artist": artist,
            "api_key": self.api_key,
            "format": "json"
        }
        
        async def _do_fetch():
            async with aiohttp.ClientSession() as session:
                async with session.get(
                    self.BASE_URL,
                    params=params,
                    timeout=aiohttp.ClientTimeout(total=self._timeout)
                ) as resp:
                    if resp.status == 200:
                        data = await resp.json()
                        images = data.get("artist", {}).get("image", [])
                        if images:
                            # 优先获取最大尺寸图片
                            for img in reversed(images):
                                url = img.get("#text", "")
                                if url:
                                    return url
                    return None
        
        return await self._fetch_with_timeout(_do_fetch())
