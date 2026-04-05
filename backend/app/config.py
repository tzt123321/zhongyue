from pydantic_settings import BaseSettings
from typing import List
import os


class Settings(BaseSettings):
    SERVER_HOST: str = "0.0.0.0"
    SERVER_PORT: int = 7777
    DEBUG: bool = False
    DATABASE_URL: str = "sqlite+aiosqlite:///./data/harmonybox.db"
    SECRET_KEY: str = "change-me-in-production"
    ALGORITHM: str = "HS256"
    ACCESS_TOKEN_EXPIRE_MINUTES: int = 1440
    MUSIC_PATHS: str = "/music"
    SCAN_ON_STARTUP: bool = False
    SCAN_INTERVAL: int = 3600
    ENABLE_RANGE_REQUESTS: bool = True
    BUFFER_SIZE: int = 8192
    ENABLE_SCRAPING: bool = True
    SCRAPE_SOURCES: str = "musicbrainz,lastfm"

    class Config:
        env_file = ".env"
        extra = "allow"

    @property
    def music_paths_list(self) -> List[str]:
        return [p.strip() for p in self.MUSIC_PATHS.split(",") if p.strip()]


settings = Settings()
