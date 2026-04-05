"""
刮削管理器 - 协调多个刮削源，处理缓存和错误
"""
import asyncio
import logging
from typing import Optional, Dict, Any, List
from pathlib import Path
from .base import BaseScraper
from .cache import ScraperCache
from .musicbrainz import MusicBrainzScraper
from .coverart import CoverArtScraper
from .lastfm import LastFMScraper
from .netease import NetEaseScraper
from .applemusic import AppleMusicScraper
from .qqmusic import QQMusicScraper
from .wikipedia import WikipediaScraper
from .lyrics import LyricsScraper

logger = logging.getLogger(__name__)

# 数据目录
DATA_DIR = Path(__file__).parent.parent.parent.parent / "data"
COVER_DIR = DATA_DIR / "covers"
ARTIST_DIR = DATA_DIR / "artists"
LYRICS_DIR = DATA_DIR / "lyrics"


class ScraperManager:
    """刮削管理器，协调多个数据源"""
    
    def __init__(self, config: Optional[Dict] = None):
        self.config = config or {}
        self.cache = ScraperCache(redis_url=self.config.get("redis_url"))
        
        # 初始化刮削器（按优先级排序）
        self.scrapers: List[BaseScraper] = []
        self._setup_scrapers()
        
        # 确保目录存在
        for d in [COVER_DIR, ARTIST_DIR, LYRICS_DIR]:
            d.mkdir(parents=True, exist_ok=True)
    
    def _setup_scrapers(self):
        """设置刮削器"""
        # ===== 专辑封面刮削器（按优先级） =====
        # 1. 网易云音乐 - 国内优先
        self.scrapers.append(NetEaseScraper())
        
        # 2. Apple Music - 国际通用（周杰伦等独占内容）
        self.scrapers.append(AppleMusicScraper())
        
        # 3. QQ音乐 - 国内备选
        self.scrapers.append(QQMusicScraper())
        
        # 4. MusicBrainz - 国际数据源
        self.scrapers.append(MusicBrainzScraper())
        
        # 5. Cover Art Archive - 封面（可能被屏蔽）
        self.scrapers.append(CoverArtScraper())
        
        # ===== 艺术家图片刮削器（按优先级） =====
        # 1. 网易云音乐 - 艺术家图片
        # (已添加，见上方)
        
        # 2. Apple Music - 艺术家图片
        # (已添加，见上方)
        
        # 3. QQ音乐 - 歌手图片（已添加，见上方）
        
        # 4. Wikipedia - 百科图片（网络不可用时跳过）
        # self.scrapers.append(WikipediaScraper())  # 暂禁用，需要国际网络
        
        # 5. Last.fm - 艺术家信息（可选，需 API Key）
        lastfm_key = self.config.get("lastfm_api_key", "")
        if lastfm_key:
            self.scrapers.append(LastFMScraper(api_key=lastfm_key))
        
        # ===== 歌词刮削器 =====
        self.scrapers.append(LyricsScraper())
        
        # 按优先级排序
        self.scrapers.sort(key=lambda s: s.order)
        logger.info(f"ScraperManager initialized with scrapers: {[s.name for s in self.scrapers]}")
    
    def _hash_key(self, text: str) -> str:
        """生成短哈希"""
        import hashlib
        return hashlib.md5(text.encode()).hexdigest()[:12]
    
    async def fetch_album_cover(self, artist: str, album: str) -> Optional[str]:
        """
        获取专辑封面
        优先级: 网易云(国内) > MusicBrainz + CoverArt > Last.fm
        """
        cache_key = f"album_cover:{artist}:{album}"
        
        # 检查缓存
        cached = await self.cache.get(cache_key)
        if cached:
            logger.info(f"Cover cache hit for {album}")
            return cached
        
        logger.info(f"Searching cover for: {artist} - {album}")
        
        # 方法1: 网易云音乐（国内可访问）
        ne_scraper = next((s for s in self.scrapers if s.name == "netease"), None)
        if ne_scraper:
            try:
                result = await ne_scraper.search_album(artist, album)
                if result and result.get("cover_url"):
                    logger.info(f"NetEase found cover for {album}")
                    local_path = await self._download_cover(result["cover_url"], artist, album)
                    if local_path:
                        await self.cache.set(cache_key, str(local_path), ttl=86400 * 30)
                        return str(local_path)
            except Exception as e:
                logger.warning(f"NetEase failed: {e}")
        
        # 方法2: Apple Music（周杰伦等独占内容）
        apple_scraper = next((s for s in self.scrapers if s.name == "applemusic"), None)
        if apple_scraper:
            try:
                result = await apple_scraper.search_album(artist, album)
                if result and result.get("cover_url"):
                    logger.info(f"Apple Music found cover for {album}")
                    local_path = await self._download_cover(result["cover_url"], artist, album)
                    if local_path:
                        await self.cache.set(cache_key, str(local_path), ttl=86400 * 30)
                        return str(local_path)
            except Exception as e:
                logger.warning(f"Apple Music failed: {e}")
        
        # 方法3: QQ音乐（国内备选）
        qq_scraper = next((s for s in self.scrapers if s.name == "qqmusic"), None)
        if qq_scraper:
            try:
                result = await qq_scraper.search_album(artist, album)
                if result and result.get("cover_url"):
                    logger.info(f"QQMusic found cover for {album}")
                    local_path = await self._download_cover(result["cover_url"], artist, album)
                    if local_path:
                        await self.cache.set(cache_key, str(local_path), ttl=86400 * 30)
                        return str(local_path)
            except Exception as e:
                logger.warning(f"QQMusic failed: {e}")
        
        # 方法2: MusicBrainz + CoverArt
        mb_scraper = next((s for s in self.scrapers if s.name == "musicbrainz"), None)
        ca_scraper = next((s for s in self.scrapers if s.name == "coverartarchive"), None)
        
        if mb_scraper and ca_scraper:
            try:
                releases = await mb_scraper.search_release(artist, album, limit=3)
                if not releases:
                    releases = await mb_scraper.search_album_by_name(album, limit=5)
                
                for release in releases:
                    release_id = release.get("id")
                    if not release_id:
                        continue
                    
                    cover_url = await ca_scraper.get_cover(release_id)
                    if cover_url:
                        local_path = await self._download_cover(cover_url, artist, album)
                        if local_path:
                            await self.cache.set(cache_key, str(local_path), ttl=86400 * 30)
                            return str(local_path)
            except Exception as e:
                logger.warning(f"MusicBrainz/CoverArt failed: {e}")
        
        logger.warning(f"No cover found for {artist} - {album}")
        return None
    
    async def _download_cover(self, url: str, artist: str, album: str) -> Optional[str]:
        """下载封面到本地，返回 URL 路径（如 /data/covers/...）"""
        import aiohttp
        import hashlib
        
        filename = f"album_{self._hash_key(f'{artist}{album}')}.jpg"
        dest = COVER_DIR / filename
        url_path = f"/data/covers/{filename}"
        
        if dest.exists():
            return url_path
        
        try:
            async def _do_download():
                async with aiohttp.ClientSession() as session:
                    async with session.get(
                        url, 
                        timeout=aiohttp.ClientTimeout(total=15)
                    ) as resp:
                        if resp.status == 200:
                            content = await resp.read()
                            # 验证是图片
                            if len(content) > 1000 and (content[:3] == b'\xff\xd8' or b'JFIF' in content[:20]):
                                with open(dest, "wb") as f:
                                    f.write(content)
                                return url_path
                        return None
            
            result = await asyncio.wait_for(_do_download(), timeout=20)
            return result
        except Exception as e:
            logger.error(f"Download cover failed: {e}")
            if dest.exists():
                dest.unlink()
            return None
    
    async def fetch_artist_image(self, artist: str) -> Optional[str]:
        """获取艺术家图片 - 多源轮询"""
        cache_key = f"artist_image:{artist}"
        
        cached = await self.cache.get(cache_key)
        if cached:
            return cached
        
        logger.info(f"Searching artist image for: {artist}")
        
        # 方法1: 网易云音乐（国内优先）
        ne_scraper = next((s for s in self.scrapers if s.name == "netease"), None)
        if ne_scraper:
            try:
                image_url = await ne_scraper.get_artist_image(artist)
                if image_url:
                    logger.info(f"NetEase found image for {artist}")
                    local_path = await self._download_artist_image(image_url, artist)
                    if local_path:
                        await self.cache.set(cache_key, str(local_path), ttl=86400 * 30)
                        return str(local_path)
            except Exception as e:
                logger.warning(f"NetEase artist image failed: {e}")
        
        # 方法2: Apple Music（艺术家图片）
        apple_scraper = next((s for s in self.scrapers if s.name == "applemusic"), None)
        if apple_scraper:
            try:
                image_url = await apple_scraper.get_artist_image(artist)
                if image_url:
                    logger.info(f"Apple Music found image for {artist}")
                    local_path = await self._download_artist_image(image_url, artist)
                    if local_path:
                        await self.cache.set(cache_key, str(local_path), ttl=86400 * 30)
                        return str(local_path)
            except Exception as e:
                logger.warning(f"Apple Music artist image failed: {e}")
        
        # 方法2: Wikipedia（多语言）
        wiki_scraper = next((s for s in self.scrapers if s.name == "wikipedia"), None)
        if wiki_scraper:
            try:
                image_url = await wiki_scraper.get_artist_image(artist)
                if image_url:
                    logger.info(f"Wikipedia found image for {artist}")
                    local_path = await self._download_artist_image(image_url, artist)
                    if local_path:
                        await self.cache.set(cache_key, str(local_path), ttl=86400 * 30)
                        return str(local_path)
            except Exception as e:
                logger.warning(f"Wikipedia artist image failed: {e}")
        
        # 方法3: Last.fm
        lastfm = next((s for s in self.scrapers if s.name == "lastfm"), None)
        if lastfm:
            try:
                cover_url = await lastfm.get_artist_cover(artist)
                if cover_url:
                    local_path = await self._download_artist_image(cover_url, artist)
                    if local_path:
                        await self.cache.set(cache_key, str(local_path), ttl=86400 * 30)
                        return str(local_path)
            except Exception as e:
                logger.warning(f"Last.fm artist image failed: {e}")
        
        logger.warning(f"No artist image found for {artist}")
        return None
    
    async def _download_artist_image(self, url: str, artist: str) -> Optional[str]:
        """下载艺术家图片到本地，返回 URL 路径（如 /data/artists/...）"""
        import aiohttp
        
        filename = f"artist_{self._hash_key(artist)}.jpg"
        dest = ARTIST_DIR / filename
        url_path = f"/data/artists/{filename}"
        
        if dest.exists():
            return url_path
        
        try:
            async def _do_download():
                async with aiohttp.ClientSession() as session:
                    async with session.get(
                        url,
                        timeout=aiohttp.ClientTimeout(total=15)
                    ) as resp:
                        if resp.status == 200:
                            content = await resp.read()
                            if len(content) > 1000:
                                with open(dest, "wb") as f:
                                    f.write(content)
                                return url_path
                        return None
            
            result = await asyncio.wait_for(_do_download(), timeout=20)
            return result
        except Exception as e:
            logger.error(f"Download artist image failed: {e}")
            if dest.exists():
                dest.unlink()
            return None
    
    async def fetch_lyrics(self, artist: str, title: str, track_id: int) -> Optional[str]:
        """获取歌词"""
        cache_key = f"lyrics:{artist}:{title}"
        
        cached = await self.cache.get(cache_key)
        if cached:
            return cached
        
        lyrics_scraper = next((s for s in self.scrapers if s.name == "lrclib"), None)
        if not lyrics_scraper:
            return None
        
        lyrics = await lyrics_scraper.get_lyrics(artist, title)
        if lyrics:
            # 保存到文件
            local_path = await self._save_lyrics(lyrics, track_id)
            if local_path:
                await self.cache.set(cache_key, str(local_path), ttl=86400 * 365)
                return str(local_path)
        
        return None
    
    async def _save_lyrics(self, lyrics: str, track_id: int) -> Optional[Path]:
        """保存歌词到文件"""
        dest = LYRICS_DIR / f"lyrics_{track_id}.lrc"
        
        try:
            with open(dest, "w", encoding="utf-8") as f:
                f.write(lyrics)
            return dest
        except Exception as e:
            logger.error(f"Save lyrics failed: {e}")
            return None
