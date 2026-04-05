from fastapi import APIRouter, Depends, HTTPException
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession
from pydantic import BaseModel
from app.database import get_db
from app.models.effect_preset import EffectPreset
from app.models.user import User
from app.api.auth import get_current_user

router = APIRouter()


class EffectPresetIn(BaseModel):
    name: str
    eq_low: float = 0.0
    eq_mid: float = 0.0
    eq_high: float = 0.0
    balance: float = 0.0
    dry_wet: float = 0.0
    room_size: float = 0.5
    damping: float = 0.5
    stereo_width: float = 1.0


class EffectPresetOut(BaseModel):
    id: int
    name: str
    is_system: bool
    eq_low: float
    eq_mid: float
    eq_high: float
    balance: float
    dry_wet: float
    room_size: float
    damping: float
    stereo_width: float

    class Config:
        from_attributes = True


SYSTEM_PRESETS = [
    {"name": "原声", "eq_low": 0, "eq_mid": 0, "eq_high": 0, "balance": 0, "dry_wet": 0.0, "room_size": 0.5, "damping": 0.5, "stereo_width": 1.0},
    {"name": "低音", "eq_low": 8, "eq_mid": 0, "eq_high": 2, "balance": 0, "dry_wet": 0.2, "room_size": 0.3, "damping": 0.6, "stereo_width": 0.8},
    {"name": "人声", "eq_low": 2, "eq_mid": 6, "eq_high": 3, "balance": 0, "dry_wet": 0.4, "room_size": 0.5, "damping": 0.5, "stereo_width": 1.2},
    {"name": "摇滚", "eq_low": 5, "eq_mid": -1, "eq_high": 6, "balance": 0, "dry_wet": 0.3, "room_size": 0.6, "damping": 0.4, "stereo_width": 1.3},
    {"name": "电子", "eq_low": 7, "eq_mid": 2, "eq_high": 5, "balance": 0, "dry_wet": 0.5, "room_size": 0.7, "damping": 0.3, "stereo_width": 1.5},
]


@router.get("/presets")
async def get_presets(db: AsyncSession = Depends(get_db), current_user: User = Depends(get_current_user)):
    result = await db.execute(
        select(EffectPreset).where(
            EffectPreset.user_id == current_user.id,
            EffectPreset.is_system == False
        )
    )
    user_presets = result.scalars().all()
    return {
        "system": SYSTEM_PRESETS,
        "user": [EffectPresetOut.model_validate(p) for p in user_presets]
    }


@router.post("/presets", response_model=EffectPresetOut)
async def create_preset(
    data: EffectPresetIn,
    db: AsyncSession = Depends(get_db),
    current_user: User = Depends(get_current_user),
):
    preset = EffectPreset(
        user_id=current_user.id,
        name=data.name,
        eq_low=data.eq_low,
        eq_mid=data.eq_mid,
        eq_high=data.eq_high,
        balance=data.balance,
        dry_wet=data.dry_wet,
        room_size=data.room_size,
        damping=data.damping,
        stereo_width=data.stereo_width,
    )
    db.add(preset)
    await db.commit()
    await db.refresh(preset)
    return preset


@router.put("/presets/{preset_id}", response_model=EffectPresetOut)
async def update_preset(
    preset_id: int,
    data: EffectPresetIn,
    db: AsyncSession = Depends(get_db),
    current_user: User = Depends(get_current_user),
):
    result = await db.execute(
        select(EffectPreset).where(
            EffectPreset.id == preset_id,
            EffectPreset.user_id == current_user.id,
            EffectPreset.is_system == False,
        )
    )
    preset = result.scalar_one_or_none()
    if not preset:
        raise HTTPException(status_code=404, detail="Preset not found")
    preset.name = data.name
    preset.eq_low = data.eq_low
    preset.eq_mid = data.eq_mid
    preset.eq_high = data.eq_high
    preset.balance = data.balance
    preset.dry_wet = data.dry_wet
    preset.room_size = data.room_size
    preset.damping = data.damping
    preset.stereo_width = data.stereo_width
    await db.commit()
    await db.refresh(preset)
    return preset


@router.delete("/presets/{preset_id}")
async def delete_preset(
    preset_id: int,
    db: AsyncSession = Depends(get_db),
    current_user: User = Depends(get_current_user),
):
    result = await db.execute(
        select(EffectPreset).where(
            EffectPreset.id == preset_id,
            EffectPreset.user_id == current_user.id,
            EffectPreset.is_system == False,
        )
    )
    preset = result.scalar_one_or_none()
    if not preset:
        raise HTTPException(status_code=404, detail="Preset not found")
    await db.delete(preset)
    await db.commit()
    return {"message": "deleted"}
