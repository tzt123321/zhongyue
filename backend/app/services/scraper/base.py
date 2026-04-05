"""
刮削器基类 - 定义统一接口
"""
from abc import ABC, abstractmethod
from typing import Optional, Dict, Any, List
import logging
import asyncio

logger = logging.getLogger(__name__)


class BaseScraper(ABC):
    """所有刮削器的基类，定义统一接口"""
    
    def __init__(self, cache_ttl: int = 86400):
        self.cache_ttl = cache_ttl  # 缓存时间（秒），默认1天
        self._cache: Dict[str, Any] = {}
        self._timeout = 10  # 默认超时（秒）
    
    @property
    @abstractmethod
    def name(self) -> str:
        """刮削器名称"""
        pass
    
    @property
    def order(self) -> int:
        """优先级，数字越小越优先"""
        return 100
    
    async def search_album(self, artist: str, album: str) -> Optional[Dict[str, Any]]:
        """
        搜索专辑元数据
        
        Returns: {
            'musicbrainz_id': str,  # 可选
            'title': str,
            'year': int,
            'cover_url': str
        }
        """
        return None
    
    async def get_artist_info(self, artist: str) -> Optional[Dict[str, Any]]:
        """
        获取艺术家信息
        
        Returns: {
            'biography': str,
            'tags': List[str],
            'similar_artists': List[str],
            'cover_url': str
        }
        """
        return None
    
    async def get_cover(self, musicbrainz_id: str) -> Optional[str]:
        """
        根据 MusicBrainz ID 获取专辑封面 URL
        """
        return None
    
    async def get_lyrics(self, artist: str, title: str) -> Optional[str]:
        """获取歌词（LRC 格式或纯文本）"""
        return None
    
    def _get_cache_key(self, prefix: str, *args) -> str:
        """生成缓存键"""
        return f"{self.name}:{prefix}:{':'.join(str(arg) for arg in args)}"
    
    async def _get_from_cache(self, key: str) -> Optional[Any]:
        """从缓存获取"""
        return self._cache.get(key)
    
    async def _set_cache(self, key: str, value: Any):
        """设置缓存"""
        self._cache[key] = value
    
    async def _fetch_with_timeout(self, coro):
        """带超时的异步请求"""
        try:
            return await asyncio.wait_for(coro, timeout=self._timeout)
        except asyncio.TimeoutError:
            logger.warning(f"{self.name}: Request timeout")
            return None
        except Exception as e:
            logger.warning(f"{self.name}: {e}")
            return None
