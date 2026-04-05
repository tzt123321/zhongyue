from pydantic import BaseModel
from typing import Optional, List
from datetime import datetime


class ArtistResponse(BaseModel):
    id: int
    name: str
    sort_name: Optional[str] = None
    musicbrainz_id: Optional[str] = None
    biography: Optional[str] = None
    cover_path: Optional[str] = None
    created_at: datetime

    class Config:
        from_attributes = True


class ArtistListResponse(BaseModel):
    total: int
    items: List[ArtistResponse]
