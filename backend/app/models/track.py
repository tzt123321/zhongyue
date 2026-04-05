from sqlalchemy import Column, Integer, String, ForeignKey, DateTime, BigInteger
from sqlalchemy.sql import func
from sqlalchemy.orm import relationship
from app.database import Base


class Track(Base):
    __tablename__ = "tracks"

    id = Column(Integer, primary_key=True, autoincrement=True)
    title = Column(String(500), nullable=False)
    artist_id = Column(Integer, ForeignKey("artists.id", ondelete="SET NULL"), nullable=True)
    album_id = Column(Integer, ForeignKey("albums.id", ondelete="SET NULL"), nullable=True)
    track_number = Column(Integer, nullable=True)
    disc_number = Column(Integer, nullable=True)
    duration = Column(Integer, nullable=True)  # seconds
    bitrate = Column(Integer, nullable=True)
    format = Column(String(10), nullable=True)
    file_path = Column(String(1000), unique=True, nullable=False)
    file_size = Column(BigInteger, nullable=True)
    file_mtime = Column(DateTime, nullable=True)
    play_count = Column(Integer, default=0)
    last_played = Column(DateTime, nullable=True)
    created_at = Column(DateTime, server_default=func.now())
    updated_at = Column(DateTime, server_default=func.now(), onupdate=func.now())

    artist = relationship("Artist", lazy="select")
    album = relationship("Album", lazy="select")
