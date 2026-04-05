"""
Apple Music 刮削器 - iTunes Search API
"""
import aiohttp
import json
from typing import Optional, Dict
from .base import BaseScraper


class AppleMusicScraper(BaseScraper):
    """Apple Music / iTunes 刮削器，支持专辑和艺术家搜索"""
    
    BASE_URL = "https://itunes.apple.com"
    
    def __init__(self, cache_ttl: int = 86400 * 30):
        super().__init__(cache_ttl)
        self._timeout = 12
    
    @property
    def name(self) -> str:
        return "applemusic"
    
    @property
    def order(self) -> int:
        return 1  # 高优先级
    
    def _get_cover_url(self, artwork: str, size: int = 300) -> str:
        """转换 Apple Music 封面 URL 到指定尺寸"""
        # artworkUrl 格式: https://is1-ssl.mzstatic.com/.../100x100bb.jpg
        # 转换为更大尺寸: 300x300, 600x600
        if not artwork:
            return None
        # 替换尺寸参数
        return artwork.replace("/100x100bb.jpg", f"/{size}x{size}bb.jpg")
    
    async def search_album(self, artist: str, album: str) -> Optional[Dict]:
        """搜索专辑"""
        cache_key = self._get_cache_key("album", artist, album)
        cached = await self._get_from_cache(cache_key)
        if cached:
            return cached
        
        # 搜索专辑
        params = {
            "term": f"{artist} {album}",
            "media": "music",
            "entity": "album",
            "limit": 10,
        }
        
        try:
            async with aiohttp.ClientSession() as session:
                async with session.get(
                    self.BASE_URL + "/search",
                    params=params,
                    timeout=aiohttp.ClientTimeout(total=self._timeout)
                ) as resp:
                    if resp.status == 200:
                        text = await resp.text()
                        # 处理 JSONP 响应
                        if text.startswith("("):
                            text = text[1:-1]
                        data = json.loads(text)
                        results = data.get("results", [])
                        
                        # 精确匹配
                        for r in results:
                            collection_name = r.get("collectionName", "").lower()
                            artist_name = r.get("artistName", "").lower()
                            if album.lower() in collection_name and artist.lower() in artist_name:
                                result = {
                                    "album_name": r.get("collectionName"),
                                    "artist_name": r.get("artistName"),
                                    "cover_url": self._get_cover_url(r.get("artworkUrl100"), 600),
                                    "collection_id": r.get("collectionId"),
                                }
                                await self._set_cache(cache_key, result)
                                return result
                        
                        # 如果没精确匹配，返回第一个
                        if results:
                            r = results[0]
                            result = {
                                "album_name": r.get("collectionName"),
                                "artist_name": r.get("artistName"),
                                "cover_url": self._get_cover_url(r.get("artworkUrl100"), 600),
                                "collection_id": r.get("collectionId"),
                            }
                            await self._set_cache(cache_key, result)
                            return result
        except Exception as e:
            print(f"Apple Music album search error: {e}")
        
        return None
    
    async def get_cover(self, collection_id: int) -> Optional[str]:
        """通过专辑 ID 获取封面"""
        params = {
            "id": collection_id,
            "entity": "album",
            "limit": 1,
        }
        
        try:
            async with aiohttp.ClientSession() as session:
                async with session.get(
                    self.BASE_URL + "/lookup",
                    params=params,
                    timeout=aiohttp.ClientTimeout(total=self._timeout)
                ) as resp:
                    if resp.status == 200:
                        text = await resp.text()
                        if text.startswith("("):
                            text = text[1:-1]
                        data = json.loads(text)
                        results = data.get("results", [])
                        if results:
                            return self._get_cover_url(results[0].get("artworkUrl100"), 600)
        except Exception as e:
            print(f"Apple Music get cover error: {e}")
        
        return None
    
    async def search_artist(self, artist: str) -> Optional[Dict]:
        """搜索艺术家"""
        cache_key = self._get_cache_key("artist", artist)
        cached = await self._get_from_cache(cache_key)
        if cached:
            return cached
        
        params = {
            "term": artist,
            "media": "music",
            "entity": "musicArtist",
            "limit": 5,
        }
        
        try:
            async with aiohttp.ClientSession() as session:
                async with session.get(
                    self.BASE_URL + "/search",
                    params=params,
                    timeout=aiohttp.ClientTimeout(total=self._timeout)
                ) as resp:
                    if resp.status == 200:
                        text = await resp.text()
                        if text.startswith("("):
                            text = text[1:-1]
                        data = json.loads(text)
                        results = data.get("results", [])
                        
                        # 精确匹配
                        for r in results:
                            if artist.lower() in r.get("artistName", "").lower():
                                result = {
                                    "name": r.get("artistName"),
                                    "artist_id": r.get("artistId"),
                                    "cover_url": r.get("artworkUrl100"),
                                }
                                await self._set_cache(cache_key, result)
                                return result
                        
                        if results:
                            r = results[0]
                            result = {
                                "name": r.get("artistName"),
                                "artist_id": r.get("artistId"),
                                "cover_url": r.get("artworkUrl100"),
                            }
                            await self._set_cache(cache_key, result)
                            return result
        except Exception as e:
            print(f"Apple Music artist search error: {e}")
        
        return None
    
    async def get_artist_image(self, artist: str) -> Optional[str]:
        """获取艺术家图片"""
        result = await self.search_artist(artist)
        if result and result.get("cover_url"):
            return self._get_cover_url(result["cover_url"], 600)
        return None
