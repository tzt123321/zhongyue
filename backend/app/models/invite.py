from sqlalchemy import Column, Integer, String, Boolean, DateTime, ForeignKey
from sqlalchemy.sql import func
from app.database import Base


class InviteCode(Base):
    __tablename__ = "invite_codes"

    id = Column(Integer, primary_key=True, autoincrement=True)
    code = Column(String(32), unique=True, nullable=False, index=True)
    created_by = Column(Integer, ForeignKey("users.id", ondelete="CASCADE"), nullable=False)
    used_by = Column(Integer, ForeignKey("users.id", ondelete="SET NULL"), nullable=True)
    used = Column(Boolean, default=False, nullable=False)
    max_uses = Column(Integer, default=1, nullable=False)  # 0 = unlimited
    uses = Column(Integer, default=0, nullable=False)
    created_at = Column(DateTime, server_default=func.now())
