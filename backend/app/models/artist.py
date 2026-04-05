from sqlalchemy import Column, Integer, String, Text, DateTime
from sqlalchemy.sql import func
from app.database import Base


class Artist(Base):
    __tablename__ = "artists"

    id = Column(Integer, primary_key=True, autoincrement=True)
    name = Column(String(255), nullable=False)
    sort_name = Column(String(255), nullable=True)
    musicbrainz_id = Column(String(36), nullable=True)
    biography = Column(Text, nullable=True)
    cover_path = Column(String(500), nullable=True)
    created_at = Column(DateTime, server_default=func.now())
