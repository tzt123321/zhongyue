"""
MusicBrainz 刮削器 - 核心元数据源
"""
import asyncio
import aiohttp
from typing import Optional, Dict, Any
from .base import BaseScraper


class MusicBrainzScraper(BaseScraper):
    """MusicBrainz 刮削器，精确匹配专辑和艺术家"""
    
    BASE_URL = "https://musicbrainz.org/ws/2"
    USER_AGENT = "HarmonyBox/2.0 (https://github.com/harmonybox)"
    
    def __init__(self, cache_ttl: int = 86400 * 7):  # MusicBrainz 数据相对稳定，缓存7天
        super().__init__(cache_ttl)
        self._timeout = 15  # MusicBrainz 可能较慢
    
    @property
    def name(self) -> str:
        return "musicbrainz"
    
    @property
    def order(self) -> int:
        return 1  # 最高优先级
    
    async def search_recording(self, artist: str, title: str, limit: int = 5) -> list:
        """搜索录音（歌曲）"""
        query = f'artist:"{artist}" AND recording:"{title}"'
        return await self._search("recording", query, limit)
    
    async def search_release(self, artist: str, album: str, limit: int = 5) -> list:
        """搜索发行版本（专辑）"""
        query = f'artist:"{artist}" AND release:"{album}"'
        return await self._search("release", query, limit)
    
    async def search_album_by_name(self, album: str, limit: int = 10) -> list:
        """仅用专辑名搜索（更灵活）"""
        query = f'release:"{album}"'
        return await self._search("release", query, limit)
    
    async def _search(self, entity_type: str, query: str, limit: int) -> list:
        """执行搜索请求"""
        cache_key = self._get_cache_key("search", entity_type, query)
        cached = await self._get_from_cache(cache_key)
        if cached:
            return cached
        
        url = f"{self.BASE_URL}/{entity_type}/"
        params = {"query": query, "limit": limit, "fmt": "json"}
        headers = {"User-Agent": self.USER_AGENT}
        
        async def _do_fetch():
            async with aiohttp.ClientSession() as session:
                async with session.get(url, params=params, headers=headers, 
                                     timeout=aiohttp.ClientTimeout(total=self._timeout)) as resp:
                    if resp.status == 200:
                        data = await resp.json()
                        return data.get(f"{entity_type}-list", [])
                    return []
        
        result = await self._fetch_with_timeout(_do_fetch())
        if result:
            await self._set_cache(cache_key, result)
        return result or []
    
    async def get_release_details(self, release_id: str) -> Optional[Dict]:
        """获取发行版本详细信息"""
        cache_key = self._get_cache_key("release", release_id)
        cached = await self._get_from_cache(cache_key)
        if cached:
            return cached
        
        url = f"{self.BASE_URL}/release/{release_id}"
        params = {"inc": "recordings+artist-credits+release-groups", "fmt": "json"}
        headers = {"User-Agent": self.USER_AGENT}
        
        async def _do_fetch():
            async with aiohttp.ClientSession() as session:
                async with session.get(url, params=params, headers=headers,
                                     timeout=aiohttp.ClientTimeout(total=self._timeout)) as resp:
                    if resp.status == 200:
                        return await resp.json()
                    return None
        
        result = await self._fetch_with_timeout(_do_fetch())
        if result:
            await self._set_cache(cache_key, result)
        return result
    
    async def search_artist(self, artist: str, limit: int = 5) -> list:
        """搜索艺术家"""
        cache_key = self._get_cache_key("artist", artist)
        cached = await self._get_from_cache(cache_key)
        if cached:
            return cached
        
        url = f"{self.BASE_URL}/artist/"
        params = {"query": f'artist:"{artist}"', "limit": limit, "fmt": "json"}
        headers = {"User-Agent": self.USER_AGENT}
        
        async def _do_fetch():
            async with aiohttp.ClientSession() as session:
                async with session.get(url, params=params, headers=headers,
                                     timeout=aiohttp.ClientTimeout(total=self._timeout)) as resp:
                    if resp.status == 200:
                        data = await resp.json()
                        return data.get("artists", [])
                    return []
        
        result = await self._fetch_with_timeout(_do_fetch())
        if result:
            await self._set_cache(cache_key, result)
        return result or []
    
    async def get_artist_details(self, artist_mbid: str) -> Optional[Dict]:
        """获取艺术家详细信息"""
        cache_key = self._get_cache_key("artist_detail", artist_mbid)
        cached = await self._get_from_cache(cache_key)
        if cached:
            return cached
        
        url = f"{self.BASE_URL}/artist/{artist_mbid}"
        params = {"inc": "url-rels+release-groups+works", "fmt": "json"}
        headers = {"User-Agent": self.USER_AGENT}
        
        async def _do_fetch():
            async with aiohttp.ClientSession() as session:
                async with session.get(url, params=params, headers=headers,
                                     timeout=aiohttp.ClientTimeout(total=self._timeout)) as resp:
                    if resp.status == 200:
                        return await resp.json()
                    return None
        
        result = await self._fetch_with_timeout(_do_fetch())
        if result:
            await self._set_cache(cache_key, result)
        return result
