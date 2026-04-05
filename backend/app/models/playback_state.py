from sqlalchemy import Column, Integer, DateTime
from app.database import Base
from datetime import datetime


class PlaybackState(Base):
    __tablename__ = "playback_state"

    id = Column(Integer, primary_key=True, default=1)  # 只有一个全局状态
    track_id = Column(Integer, nullable=True)
    position = Column(Integer, default=0)  # 播放位置（秒）
    is_playing = Column(Integer, default=0)  # 0=暂停 1=播放
    volume = Column(Integer, default=80)  # 音量 0-100
    updated_at = Column(DateTime, default=datetime.utcnow, onupdate=datetime.utcnow)
