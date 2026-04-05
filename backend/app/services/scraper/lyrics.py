"""
歌词刮削器 - LRCLIB
"""
import aiohttp
from typing import Optional
from .base import BaseScraper


class LyricsScraper(BaseScraper):
    """歌词刮削器，主要使用 LRCLib（免费开源）"""
    
    LRCLIB_API = "https://lrclib.net/api"
    
    def __init__(self, cache_ttl: int = 86400 * 365):  # 歌词几乎不变，缓存1年
        super().__init__(cache_ttl)
        self._timeout = 10
    
    @property
    def name(self) -> str:
        return "lrclib"
    
    @property
    def order(self) -> int:
        return 1  # 歌词优先级高
    
    async def get_lyrics(self, artist: str, title: str) -> Optional[str]:
        """获取歌词"""
        cache_key = self._get_cache_key("lyrics", artist, title)
        cached = await self._get_from_cache(cache_key)
        if cached:
            return cached
        
        # 方法1: 精确搜索
        lyrics = await self._search_exact(artist, title)
        if lyrics:
            await self._set_cache(cache_key, lyrics)
            return lyrics
        
        # 方法2: 模糊搜索
        lyrics = await self._search_fuzzy(artist, title)
        if lyrics:
            await self._set_cache(cache_key, lyrics)
            return lyrics
        
        return None
    
    async def _search_exact(self, artist: str, title: str) -> Optional[str]:
        """精确搜索歌词"""
        url = f"{self.LRCLIB_API}/get"
        params = {
            "artist_name": artist,
            "track_name": title
        }
        
        async def _do_fetch():
            async with aiohttp.ClientSession() as session:
                async with session.get(
                    url,
                    params=params,
                    timeout=aiohttp.ClientTimeout(total=self._timeout)
                ) as resp:
                    if resp.status == 200:
                        data = await resp.json()
                        # 优先返回同步歌词，其次是纯文本
                        return data.get("syncedLyrics") or data.get("plainLyrics")
                    return None
        
        return await self._fetch_with_timeout(_do_fetch())
    
    async def _search_fuzzy(self, artist: str, title: str) -> Optional[str]:
        """模糊搜索歌词"""
        url = f"{self.LRCLIB_API}/search"
        params = {
            "artist_name": artist,
            "track_name": title,
            "limit": 5
        }
        
        async def _do_fetch():
            async with aiohttp.ClientSession() as session:
                async with session.get(
                    url,
                    params=params,
                    timeout=aiohttp.ClientTimeout(total=self._timeout)
                ) as resp:
                    if resp.status == 200:
                        results = await resp.json()
                        if results:
                            # 返回第一个结果的歌词
                            result = results[0]
                            return result.get("syncedLyrics") or result.get("plainLyrics")
                    return None
        
        return await self._fetch_with_timeout(_do_fetch())
