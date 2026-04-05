from sqlalchemy import Column, Integer, String, Boolean, DateTime, ForeignKey
from sqlalchemy.orm import relationship
from sqlalchemy.sql import func
from app.database import Base


class User(Base):
    __tablename__ = "users"

    id = Column(Integer, primary_key=True, autoincrement=True)
    username = Column(String(50), unique=True, nullable=False)
    email = Column(String(255), unique=True, nullable=True)
    password_hash = Column(String(255), nullable=False)
    initial_password_hash = Column(String(255), nullable=True)  # 保留初始密码
    initial_password = Column(String(64), nullable=True)  # 明文初始密码（仅管理员可见）
    is_admin = Column(Boolean, default=False, nullable=False)
    is_musician = Column(Boolean, default=False, nullable=False)  # 乐主：可上传音乐
    is_active = Column(Boolean, default=False, nullable=False)  # needs admin to activate
    is_banned = Column(Boolean, default=False, nullable=False)
    is_muted = Column(Boolean, default=False, nullable=False)
    invite_code = Column(String(32), nullable=True)
    api_key = Column(String(64), nullable=True, unique=True)
    created_at = Column(DateTime, server_default=func.now())
    updated_at = Column(DateTime, server_default=func.now(), onupdate=func.now())
