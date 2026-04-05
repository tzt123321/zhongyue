from pydantic import BaseModel
from typing import Optional, List
from datetime import datetime


class ArtistBrief(BaseModel):
    id: int
    name: str

    class Config:
        from_attributes = True


class AlbumBrief(BaseModel):
    id: int
    name: str
    year: Optional[int] = None
    cover_path: Optional[str] = None

    class Config:
        from_attributes = True


class TrackResponse(BaseModel):
    id: int
    title: str
    artist_id: Optional[int] = None
    album_id: Optional[int] = None
    track_number: Optional[int] = None
    disc_number: Optional[int] = None
    duration: Optional[int] = None
    bitrate: Optional[int] = None
    format: Optional[str] = None
    file_path: str
    file_size: Optional[int] = None
    play_count: int = 0
    created_at: datetime
    artist: Optional[ArtistBrief] = None
    album: Optional[AlbumBrief] = None

    class Config:
        from_attributes = True


class TrackListResponse(BaseModel):
    total: int
    items: List[TrackResponse]
