from fastapi import APIRouter, Depends
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession
from pydantic import BaseModel

from app.database import get_db
from app.models.user import User
from app.models.playback_state import PlaybackState
from app.api.auth import get_current_user

router = APIRouter()


class PlaybackStateOut(BaseModel):
    track_id: int | None
    position: int
    is_playing: bool
    volume: int


@router.get("/state", response_model=PlaybackStateOut)
async def get_playback_state(db: AsyncSession = Depends(get_db)):
    result = await db.execute(select(PlaybackState).where(PlaybackState.id == 1))
    state = result.scalar_one_or_none()
    if not state:
        return PlaybackStateOut(track_id=None, position=0, is_playing=False, volume=80)
    return PlaybackStateOut(
        track_id=state.track_id,
        position=state.position,
        is_playing=bool(state.is_playing),
        volume=state.volume,
    )


class PlaybackStateIn(BaseModel):
    track_id: int | None
    position: int
    is_playing: bool
    volume: int


@router.put("/state", response_model=PlaybackStateOut)
async def update_playback_state(
    data: PlaybackStateIn,
    db: AsyncSession = Depends(get_db),
    current_user: User = Depends(get_current_user),
):
    result = await db.execute(select(PlaybackState).where(PlaybackState.id == 1))
    state = result.scalar_one_or_none()
    if not state:
        state = PlaybackState(id=1)
        db.add(state)
    state.track_id = data.track_id
    state.position = data.position
    state.is_playing = 1 if data.is_playing else 0
    state.volume = data.volume
    await db.commit()
    return PlaybackStateOut(
        track_id=state.track_id,
        position=state.position,
        is_playing=bool(state.is_playing),
        volume=state.volume,
    )
