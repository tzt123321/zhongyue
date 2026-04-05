from fastapi import APIRouter, Depends, HTTPException
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession
from pydantic import BaseModel, field_validator
from typing import List, Optional
import secrets

from app.database import get_db
from app.models.user import User
from app.models.invite import InviteCode
from app.api.auth import get_current_user
from app.utils.password import hash_password

router = APIRouter()


# ── Schemas ──────────────────────────────────────────────────────────────

class UserResponse(BaseModel):
    id: int
    username: str
    email: Optional[str] = None
    is_admin: bool
    is_musician: bool
    is_active: bool
    is_banned: bool
    is_muted: bool
    initial_password: Optional[str] = None  # 仅管理员可看到明文

    class Config:
        from_attributes = True


class UserCreate(BaseModel):
    username: str
    email: Optional[str] = None
    password: str
    invite_code: str


class UserUpdate(BaseModel):
    email: Optional[str] = None
    password: Optional[str] = None
    is_active: Optional[bool] = None
    is_banned: Optional[bool] = None
    is_admin: Optional[bool] = None
    is_musician: Optional[bool] = None


class InviteCodeCreate(BaseModel):
    max_uses: int = 1


class AdminResetPassword(BaseModel):
    password: str

    @field_validator('password')
    @classmethod
    def check_password(cls, v):
        from app.api.auth import validate_password
        return validate_password(v, "新密码")


class InviteCodeResponse(BaseModel):
    id: int
    code: str
    created_by: int
    used: bool
    uses: int
    max_uses: int

    class Config:
        from_attributes = True


# ── Helpers ─────────────────────────────────────────────────────────────

async def get_admin_user(user: User = Depends(get_current_user)) -> User:
    if not user.is_admin:
        raise HTTPException(status_code=403, detail="Admin only")
    return user


# ── User Management ──────────────────────────────────────────────────────

@router.get("/users", response_model=List[UserResponse])
async def list_users(
    db: AsyncSession = Depends(get_db),
    admin: User = Depends(get_admin_user),
):
    result = await db.execute(select(User).order_by(User.id))
    users = result.scalars().all()
    return users


@router.get("/users/{user_id}", response_model=UserResponse)
async def get_user(
    user_id: int,
    db: AsyncSession = Depends(get_db),
    admin: User = Depends(get_admin_user),
):
    result = await db.execute(select(User).where(User.id == user_id))
    user = result.scalar_one_or_none()
    if not user:
        raise HTTPException(status_code=404, detail="User not found")
    return user


@router.patch("/users/{user_id}", response_model=UserResponse)
async def update_user(
    user_id: int,
    data: UserUpdate,
    db: AsyncSession = Depends(get_db),
    admin: User = Depends(get_admin_user),
):
    result = await db.execute(select(User).where(User.id == user_id))
    user = result.scalar_one_or_none()
    if not user:
        raise HTTPException(status_code=404, detail="User not found")

    if data.email is not None:
        user.email = data.email
    if data.password is not None:
        user.password_hash = hash_password(data.password)
    if data.is_active is not None:
        if admin.id != 1:
            raise HTTPException(status_code=403, detail="仅原始管理员可执行此操作")
        user.is_active = data.is_active
    if data.is_banned is not None:
        if admin.id != 1:
            raise HTTPException(status_code=403, detail="仅原始管理员可执行此操作")
        user.is_banned = data.is_banned

    # 乐主权限：admin 可授予/撤销任意用户的乐主身份
    if data.is_musician is not None:
        if admin.id != 1:
            raise HTTPException(status_code=403, detail="仅原始管理员可执行此操作")
        user.is_musician = data.is_musician

    # 管理员权限：只有原始管理员（id=1）可以修改管理员身份
    if data.is_admin is not None and data.is_admin != user.is_admin:
        if admin.id != 1:
            raise HTTPException(status_code=403, detail="只有原始管理员可以修改管理员身份")
        if not data.is_admin:
            # 防止移除最后一个管理员
            admin_count = await db.execute(
                select(User).where(User.is_admin == True)
            )
            if len(admin_count.scalars().all()) <= 1:
                raise HTTPException(status_code=400, detail="Cannot remove last admin")
        user.is_admin = data.is_admin

    await db.commit()
    await db.refresh(user)
    return user


@router.post("/users/{user_id}/reset-password", response_model=dict)
async def admin_reset_password(
    user_id: int,
    data: AdminResetPassword,
    db: AsyncSession = Depends(get_db),
    admin: User = Depends(get_admin_user),
):
    """管理员重置用户密码（不需要旧密码）"""
    result = await db.execute(select(User).where(User.id == user_id))
    user = result.scalar_one_or_none()
    if not user:
        raise HTTPException(status_code=404, detail="User not found")
    user.password_hash = hash_password(data.password)
    await db.commit()
    return {"message": "密码已重置"}


@router.delete("/users/{user_id}", status_code=204)
async def delete_user(
    user_id: int,
    db: AsyncSession = Depends(get_db),
    admin: User = Depends(get_admin_user),
):
    result = await db.execute(select(User).where(User.id == user_id))
    user = result.scalar_one_or_none()
    if not user:
        raise HTTPException(status_code=404, detail="User not found")
    if user.id == admin.id:
        raise HTTPException(status_code=400, detail="Cannot delete yourself")

    await db.delete(user)
    await db.commit()


# ── Mute Management ──────────────────────────────────────────────────────

class MuteUser(BaseModel):
    is_muted: bool

@router.put("/users/{user_id}/mute")
async def mute_user(
    user_id: int,
    data: MuteUser,
    db: AsyncSession = Depends(get_db),
    current_user: User = Depends(get_current_user),
):
    if current_user.id != 1:
        raise HTTPException(status_code=403, detail="仅原始管理员可执行此操作")
    result = await db.execute(select(User).where(User.id == user_id))
    user = result.scalar_one_or_none()
    if not user:
        raise HTTPException(status_code=404, detail="用户不存在")
    user.is_muted = data.is_muted
    await db.commit()
    return {"message": "设置成功"}


# ── Invite Code Management ───────────────────────────────────────────────

@router.post("/invite-codes", response_model=InviteCodeResponse)
async def create_invite_code(
    data: InviteCodeCreate,
    db: AsyncSession = Depends(get_db),
    admin: User = Depends(get_admin_user),
):
    code = secrets.token_urlsafe(8)[:12].upper()
    invite = InviteCode(
        code=code,
        created_by=admin.id,
        max_uses=data.max_uses,
    )
    db.add(invite)
    await db.commit()
    await db.refresh(invite)
    return invite


@router.get("/invite-codes", response_model=List[InviteCodeResponse])
async def list_invite_codes(
    db: AsyncSession = Depends(get_db),
    admin: User = Depends(get_admin_user),
):
    result = await db.execute(select(InviteCode).order_by(InviteCode.id.desc()))
    return result.scalars().all()


@router.delete("/invite-codes/{code_id}", status_code=204)
async def revoke_invite_code(
    code_id: int,
    db: AsyncSession = Depends(get_db),
    admin: User = Depends(get_admin_user),
):
    result = await db.execute(select(InviteCode).where(InviteCode.id == code_id))
    invite = result.scalar_one_or_none()
    if not invite:
        raise HTTPException(status_code=404, detail="Invite code not found")
    await db.delete(invite)
    await db.commit()
