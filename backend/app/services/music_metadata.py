"""
统一导出接口 - 兼容旧代码
"""
from .scraper.manager import ScraperManager, ScraperCache
from .scraper.musicbrainz import MusicBrainzScraper
from .scraper.coverart import CoverArtScraper
from .scraper.lyrics import LyricsScraper

# 全局管理器实例（延迟初始化）
_manager = None


def get_scraper_manager() -> ScraperManager:
    """获取刮削管理器单例"""
    global _manager
    if _manager is None:
        _manager = ScraperManager()
    return _manager


async def scrape_all_for_track(track_title, artist_name, album_name, track_id, album_id=None, artist_id=None):
    """为单个曲目刮削所有元数据"""
    manager = get_scraper_manager()
    results = {
        "cover_path": None,
        "artist_cover_path": None,
        "lyrics_path": None,
    }
    
    # 并行执行
    tasks = []
    
    if album_name and album_name != "Unknown Album":
        tasks.append(("cover", manager.fetch_album_cover(artist_name, album_name)))
    
    tasks.append(("artist", manager.fetch_artist_image(artist_name)))
    
    if track_title and track_title != "Unknown Track":
        tasks.append(("lyrics", manager.fetch_lyrics(artist_name, track_title, track_id)))
    
    for key, coro in tasks:
        try:
            result = await coro
            if key == "cover" and result:
                results["cover_path"] = result
            elif key == "artist" and result:
                results["artist_cover_path"] = result
            elif key == "lyrics" and result:
                results["lyrics_path"] = result
        except Exception as e:
            print(f"Scraping {key} failed: {e}")
    
    return results


async def scrape_album_cover(album_name: str, artist_name: str) -> str:
    """刮削专辑封面"""
    manager = get_scraper_manager()
    return await manager.fetch_album_cover(artist_name, album_name)


async def scrape_artist_image(artist_name: str) -> str:
    """刮削艺术家图片"""
    manager = get_scraper_manager()
    return await manager.fetch_artist_image(artist_name)


# 兼容性别名
search_and_download_album_cover = scrape_album_cover
search_and_download_artist_image = scrape_artist_image
search_and_save_lyrics = lambda a, t, tid: get_scraper_manager().fetch_lyrics(a, t, tid)
