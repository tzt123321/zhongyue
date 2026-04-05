"""
HarmonyBox Metadata Scraper Module
从互联网刮削音乐元数据：专辑封面、艺术家信息、歌词等
"""
from .base import BaseScraper
from .manager import ScraperManager
from .cache import ScraperCache
from .musicbrainz import MusicBrainzScraper
from .lastfm import LastFMScraper
from .coverart import CoverArtScraper
from .netease import NetEaseScraper
from .applemusic import AppleMusicScraper
from .qqmusic import QQMusicScraper
from .wikipedia import WikipediaScraper
from .lyrics import LyricsScraper

__all__ = [
    "BaseScraper",
    "ScraperManager", 
    "ScraperCache",
    "MusicBrainzScraper",
    "LastFMScraper",
    "CoverArtScraper",
    "NetEaseScraper",
    "AppleMusicScraper",
    "QQMusicScraper",
    "WikipediaScraper",
    "LyricsScraper",
]
