from pydantic import BaseModel
from typing import Optional, List
from datetime import datetime


class ArtistBrief(BaseModel):
    id: int
    name: str

    class Config:
        from_attributes = True


class AlbumResponse(BaseModel):
    id: int
    name: str
    artist_id: Optional[int] = None
    year: Optional[int] = None
    cover_path: Optional[str] = None
    total_tracks: int = 0
    musicbrainz_id: Optional[str] = None
    created_at: datetime
    artist: Optional[ArtistBrief] = None

    class Config:
        from_attributes = True


class AlbumListResponse(BaseModel):
    total: int
    items: List[AlbumResponse]
