from sqlalchemy import Column, Integer, String, Float, Boolean, ForeignKey
from app.database import Base
from pydantic import BaseModel


class EffectPreset(Base):
    __tablename__ = "effect_presets"

    id = Column(Integer, primary_key=True, autoincrement=True)
    user_id = Column(Integer, ForeignKey("users.id", ondelete="CASCADE"), nullable=False)
    name = Column(String(100), nullable=False)
    is_system = Column(Boolean, default=False, nullable=False)
    eq_low = Column(Float, default=0.0)
    eq_mid = Column(Float, default=0.0)
    eq_high = Column(Float, default=0.0)
    balance = Column(Float, default=0.0)
    dry_wet = Column(Float, default=0.0)       # 干湿比 0~1
    room_size = Column(Float, default=0.5)     # 房间尺寸 0~1
    damping = Column(Float, default=0.5)        # 阻尼 0~1
    stereo_width = Column(Float, default=1.0)  # 立体声宽度 0~2


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
    id: int | None = None
    name: str
    is_system: bool = False
    eq_low: float = 0.0
    eq_mid: float = 0.0
    eq_high: float = 0.0
    balance: float = 0.0
    dry_wet: float = 0.0
    room_size: float = 0.5
    damping: float = 0.5
    stereo_width: float = 1.0
