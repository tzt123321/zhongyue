from sqlalchemy import Column, Integer, ForeignKey, Boolean
from app.database import Base


class TrackArtist(Base):
    __tablename__ = "track_artists"

    id = Column(Integer, primary_key=True, autoincrement=True)
    track_id = Column(Integer, ForeignKey("tracks.id", ondelete="CASCADE"), nullable=False)
    artist_id = Column(Integer, ForeignKey("artists.id", ondelete="CASCADE"), nullable=False)
    is_primary = Column(Boolean, default=False, nullable=False)
