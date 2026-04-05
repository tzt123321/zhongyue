"""
缓存封装 - 支持 Redis 和内存回退
"""
import asyncio
from typing import Optional, Dict
import json


class ScraperCache:
    """刮削结果缓存，支持内存和 Redis"""
    
    def __init__(self, redis_url: Optional[str] = None):
        self.redis = None
        self.redis_url = redis_url
        self.memory_cache: Dict[str, tuple] = {}  # key -> (value, expire_time)
    
    async def init(self):
        """初始化 Redis 连接"""
        if self.redis_url:
            try:
                import redis.asyncio as redis
                self.redis = redis.from_url(self.redis_url)
                await self.redis.ping()
                logger.info("Redis cache connected")
            except Exception as e:
                logger.warning(f"Redis connection failed, using memory cache: {e}")
                self.redis = None
    
    async def get(self, key: str) -> Optional[str]:
        """获取缓存值"""
        # 先查 Redis
        if self.redis:
            try:
                value = await self.redis.get(key)
                if value:
                    return value.decode() if isinstance(value, bytes) else value
            except Exception as e:
                logger.warning(f"Redis get failed: {e}")
        
        # 回退到内存缓存
        if key in self.memory_cache:
            value, expire_time = self.memory_cache[key]
            import time
            if expire_time > time.time():
                return value
            else:
                del self.memory_cache[key]
        return None
    
    async def set(self, key: str, value: str, ttl: int = 86400):
        """设置缓存"""
        import time
        expire_time = time.time() + ttl
        
        # 写入 Redis
        if self.redis:
            try:
                await self.redis.setex(key, ttl, value)
            except Exception as e:
                logger.warning(f"Redis set failed: {e}")
        
        # 写入内存缓存
        self.memory_cache[key] = (value, expire_time)
    
    async def delete(self, key: str):
        """删除缓存"""
        if self.redis:
            try:
                await self.redis.delete(key)
            except Exception:
                pass
        self.memory_cache.pop(key, None)
    
    async def clear_expired(self):
        """清理过期缓存"""
        import time
        now = time.time()
        expired = [k for k, (_, exp) in self.memory_cache.items() if exp <= now]
        for k in expired:
            del self.memory_cache[k]


import logging
logger = logging.getLogger(__name__)
