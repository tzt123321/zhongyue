"""
Wikipedia刮削器 - 获取艺术家/百科图片
"""
import aiohttp
from typing import Optional, Dict
from .base import BaseScraper


class WikipediaScraper(BaseScraper):
    """Wikipedia 刮削器，可获取艺术家图片"""
    
    # 多语言Wikipedia
    BASE_URLS = [
        "https://zh.wikipedia.org/w/api.php",      # 中文
        "https://en.wikipedia.org/w/api.php",      # 英文
        "https://ja.wikipedia.org/w/api.php",      # 日文
    ]
    
    def __init__(self, cache_ttl: int = 86400 * 30):
        super().__init__(cache_ttl)
        self._timeout = 12
    
    @property
    def name(self) -> str:
        return "wikipedia"
    
    @property
    def order(self) -> int:
        return 2  # 第二优先级
    
    async def get_artist_image(self, artist: str) -> Optional[str]:
        """从 Wikipedia 获取艺术家图片"""
        # 尝试多个语言版本
        for base_url in self.BASE_URLS:
            image_url = await self._search_wiki(base_url, artist)
            if image_url:
                return image_url
        return None
    
    async def _search_wiki(self, api_url: str, artist: str) -> Optional[str]:
        """搜索 Wikipedia"""
        cache_key = self._get_cache_key("wiki", api_url.split("/")[2], artist)
        cached = await self._get_from_cache(cache_key)
        if cached:
            return cached
        
        params = {
            "action": "query",
            "titles": artist,
            "prop": "pageimages",
            "piprop": "thumbnail",
            "pithumbsize": 500,
            "format": "json",
            "origin": "*",
        }
        
        try:
            async with aiohttp.ClientSession() as session:
                async with session.get(
                    api_url,
                    params=params,
                    timeout=aiohttp.ClientTimeout(total=self._timeout)
                ) as resp:
                    if resp.status == 200:
                        data = await resp.json()
                        pages = data.get("query", {}).get("pages", {})
                        for page_id, page in pages.items():
                            if "thumbnail" in page:
                                image_url = page["thumbnail"]["source"]
                                await self._set_cache(cache_key, image_url)
                                return image_url
        except Exception as e:
            print(f"Wikipedia search error ({api_url}): {e}")
        
        return None
    
    async def get_artist_info(self, artist: str) -> Optional[Dict]:
        """获取艺术家详细信息"""
        info = {}
        
        for base_url in self.BASE_URLS:
            result = await self._get_artist_info(base_url, artist)
            if result:
                info.update(result)
                if info.get("image"):
                    break
        
        return info if info else None
    
    async def _get_artist_info(self, api_url: str, artist: str) -> Optional[Dict]:
        """获取艺术家信息"""
        params = {
            "action": "query",
            "titles": artist,
            "prop": "extracts|pageimages|info",
            "exintro": True,
            "explaintext": True,
            "piprop": "thumbnail",
            "pithumbsize": 500,
            "format": "json",
            "origin": "*",
        }
        
        try:
            async with aiohttp.ClientSession() as session:
                async with session.get(
                    api_url,
                    params=params,
                    timeout=aiohttp.ClientTimeout(total=self._timeout)
                ) as resp:
                    if resp.status == 200:
                        data = await resp.json()
                        pages = data.get("query", {}).get("pages", {})
                        for page_id, page in pages.items():
                            if page_id != "-1":
                                return {
                                    "name": page.get("title"),
                                    "biography": page.get("extract", ""),
                                    "image": page.get("thumbnail", {}).get("source"),
                                    "url": f"{api_url.replace('/w/api.php', '')}/wiki/{page.get('title', '')}",
                                }
        except Exception as e:
            print(f"Wikipedia info error: {e}")
        
        return None
