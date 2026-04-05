from pydantic import BaseModel
from typing import Optional, List
from datetime import datetime
from .track import TrackResponse


class PlaylistCreate(BaseModel):
    name: str
    description: Optional[str] = None


class PlaylistUpdate(BaseModel):
    name: Optional[str] = None
    description: Optional[str] = None


class PlaylistResponse(BaseModel):
    id: int
    name: str
    description: Optional[str] = None
    user_id: Optional[int] = None
    created_at: datetime
    updated_at: datetime

    class Config:
        from_attributes = True


class PlaylistDetailResponse(BaseModel):
    id: int
    name: str
    description: Optional[str] = None
    user_id: Optional[int] = None
    tracks: List[TrackResponse] = []
    created_at: datetime
    updated_at: datetime

    class Config:
        from_attributes = True


class PlaylistTrackAdd(BaseModel):
    track_ids: List[int]
