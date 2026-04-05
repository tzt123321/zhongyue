"""
网易云音乐刮削器 - 国内可访问的封面源
"""
import aiohttp
from typing import Optional, Dict
from .base import BaseScraper


class NetEaseScraper(BaseScraper):
    """网易云音乐刮削器，国内可用"""
    
    SEARCH_API = "https://music.163.com/api/search/get/web"
    ALBUM_API = "https://music.163.com/api/album/"
    
    def __init__(self, cache_ttl: int = 86400 * 30):
        super().__init__(cache_ttl)
        self._timeout = 10
    
    @property
    def name(self) -> str:
        return "netease"
    
    @property
    def order(self) -> int:
        return 1  # 高优先级
    
    async def search_album(self, artist: str, album: str) -> Optional[Dict]:
        """搜索专辑"""
        cache_key = self._get_cache_key("album", artist, album)
        cached = await self._get_from_cache(cache_key)
        if cached:
            return cached
        
        # 搜索专辑
        data = await self._search_album(album)
        if not data:
            return None
        
        albums = data.get("result", {}).get("albums", [])
        if albums:
            album_info = albums[0]
            return {
                "album_id": album_info.get("id"),
                "album_name": album_info.get("name"),
                "cover_url": album_info.get("picUrl"),
                "artist_name": album_info.get("artist", {}).get("name", ""),
            }
        
        return None
    
    async def search_artist(self, artist: str) -> Optional[Dict]:
        """搜索艺术家"""
        cache_key = self._get_cache_key("artist", artist)
        cached = await self._get_from_cache(cache_key)
        if cached:
            return cached
        
        data = await self._search_artist(artist)
        if not data:
            return None
        
        artists = data.get("result", {}).get("artists", [])
        if artists:
            artist_info = artists[0]
            return {
                "id": artist_info.get("id"),
                "name": artist_info.get("name"),
                "pic_url": artist_info.get("picUrl"),
                "alias": artist_info.get("alias", []),
                "album_size": artist_info.get("albumSize", 0),
            }
        
        return None
    
    async def get_artist_image(self, artist: str) -> Optional[str]:
        """获取艺术家图片"""
        result = await self.search_artist(artist)
        if result and result.get("pic_url"):
            return result["pic_url"]
        return None
    
    async def get_cover(self, album_id: int) -> Optional[str]:
        """根据专辑 ID 获取封面"""
        cache_key = self._get_cache_key("cover", album_id)
        cached = await self._get_from_cache(cache_key)
        if cached:
            return cached
        
        url = f"{self.ALBUM_API}{album_id}"
        
        try:
            async with aiohttp.ClientSession() as session:
                async with session.get(
                    url,
                    headers={
                        "Referer": "https://music.163.com",
                        "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"
                    },
                    timeout=aiohttp.ClientTimeout(total=self._timeout)
                ) as resp:
                    if resp.status == 200:
                        text = await resp.text()
                        import json
                        data = json.loads(text)
                        album = data.get("album", {})
                        cover = album.get("picUrl")
                        if cover:
                            await self._set_cache(cache_key, cover)
                            return cover
        except Exception as e:
            print(f"NetEase get_cover error: {e}")
        
        return None
    
    async def _search_album(self, keyword: str) -> Optional[Dict]:
        """搜索专辑"""
        try:
            async with aiohttp.ClientSession() as session:
                async with session.post(
                    self.SEARCH_API,
                    data={
                        "s": keyword,
                        "type": "10",  # 专辑类型
                        "offset": "0",
                        "total": "true",
                        "limit": "5",
                    },
                    headers={"Referer": "https://music.163.com/"},
                    timeout=aiohttp.ClientTimeout(total=self._timeout)
                ) as resp:
                    if resp.status == 200:
                        text = await resp.text()
                        import json
                        return json.loads(text)
        except Exception as e:
            print(f"NetEase search error: {e}")
        
        return None
    
    async def _search_artist(self, keyword: str) -> Optional[Dict]:
        """搜索艺术家"""
        try:
            async with aiohttp.ClientSession() as session:
                async with session.post(
                    self.SEARCH_API,
                    data={
                        "s": keyword,
                        "type": "100",  # 艺术家类型
                        "offset": "0",
                        "total": "true",
                        "limit": "5",
                    },
                    headers={"Referer": "https://music.163.com/"},
                    timeout=aiohttp.ClientTimeout(total=self._timeout)
                ) as resp:
                    if resp.status == 200:
                        text = await resp.text()
                        import json
                        return json.loads(text)
        except Exception as e:
            print(f"NetEase artist search error: {e}")
        
        return None
