from sqlalchemy import Column, Integer, String, ForeignKey, DateTime
from sqlalchemy.sql import func
from sqlalchemy.orm import relationship
from app.database import Base


class Album(Base):
    __tablename__ = "albums"

    id = Column(Integer, primary_key=True, autoincrement=True)
    name = Column(String(255), nullable=False)
    artist_id = Column(Integer, ForeignKey("artists.id", ondelete="SET NULL"), nullable=True)
    year = Column(Integer, nullable=True)
    cover_path = Column(String(500), nullable=True)
    total_tracks = Column(Integer, default=0)
    musicbrainz_id = Column(String(36), nullable=True)
    created_at = Column(DateTime, server_default=func.now())

    artist = relationship("Artist", lazy="select")
