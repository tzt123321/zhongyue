from fastapi import APIRouter, Depends, HTTPException
from fastapi.security import OAuth2PasswordBearer, OAuth2PasswordRequestForm
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession
from pydantic import BaseModel, field_validator
from typing import Optional
import re

from app.database import get_db
from app.models.user import User
from app.models.invite import InviteCode
from app.utils.password import hash_password, verify_password
from app.utils.jwt import create_access_token, verify_token

router = APIRouter()
oauth2_scheme = OAuth2PasswordBearer(tokenUrl="/api/auth/login")

# ── Password validation ─────────────────────────────────────────────────

PASSWORD_RE = re.compile(r'^[\w\u4e00-\u9fff]{6,}$')

def validate_password(password: str, field_name: str = "密码") -> str:
    if not PASSWORD_RE.match(password):
        raise HTTPException(
            status_code=400,
            detail=f"{field_name}至少6位，仅支持中英文、数字和下划线"
        )
    return password

# ── Schemas ──────────────────────────────────────────────────────────────

class UserResponse(BaseModel):
    id: int
    username: str
    email: Optional[str] = None
    is_admin: bool
    is_musician: bool
    is_active: bool
    is_banned: bool
    initial_password: Optional[str] = None  # 仅管理员可看到

    class Config:
        from_attributes = True


class UserCreate(BaseModel):
    username: str
    email: Optional[str] = None
    password: str
    invite_code: str

    @field_validator('password')
    @classmethod
    def check_password(cls, v):
        validate_password(v, "密码")
        return v


class ChangePassword(BaseModel):
    old_password: str
    new_password: str

    @field_validator('new_password')
    @classmethod
    def check_new_password(cls, v):
        validate_password(v, "新密码")
        return v


class AdminResetPassword(BaseModel):
    password: str

    @field_validator('password')
    @classmethod
    def check_password(cls, v):
        validate_password(v, "新密码")
        return v


async def get_current_user(
    token: str = Depends(oauth2_scheme),
    db: AsyncSession = Depends(get_db),
) -> User:
    token_data = verify_token(token)
    if token_data is None:
        raise HTTPException(status_code=401, detail="Invalid token")
    # token sub can be username (legacy) or user ID string (new login-by-key flow)
    sub = token_data.username
    if sub is None:
        raise HTTPException(status_code=401, detail="Invalid token")
    # Check if sub is a numeric user ID
    if sub.isdigit():
        result = await db.execute(select(User).where(User.id == int(sub)))
    else:
        result = await db.execute(select(User).where(User.username == sub))
    user = result.scalar_one_or_none()
    if user is None:
        raise HTTPException(status_code=401, detail="Invalid token")
    return user


@router.post("/login", response_model=dict)
async def login(
    form_data: OAuth2PasswordRequestForm = Depends(),
    db: AsyncSession = Depends(get_db),
):
    result = await db.execute(select(User).where(User.username == form_data.username))
    user = result.scalar_one_or_none()

    if not user or not verify_password(form_data.password, user.password_hash):
        raise HTTPException(status_code=401, detail="Incorrect username or password")

    if user.is_banned:
        raise HTTPException(status_code=403, detail="账户已被封禁，请联系管理员")

    if not user.is_active:
        raise HTTPException(status_code=403, detail="账户尚未激活，请等待管理员激活")

    access_token = create_access_token(data={"sub": user.username})
    return {
        "access_token": access_token,
        "token_type": "bearer",
        "user": UserResponse.model_validate(user),
    }


@router.post("/register", response_model=UserResponse, status_code=201)
async def register(user_data: UserCreate, db: AsyncSession = Depends(get_db)):
    # Validate invite code
    result = await db.execute(
        select(InviteCode).where(InviteCode.code == user_data.invite_code)
    )
    invite = result.scalar_one_or_none()
    if not invite:
        raise HTTPException(status_code=400, detail="邀请码无效")
    if invite.max_uses > 0 and invite.uses >= invite.max_uses:
        raise HTTPException(status_code=400, detail="邀请码已用尽")

    # Check username/email uniqueness
    result = await db.execute(select(User).where(User.username == user_data.username))
    if result.scalar_one_or_none():
        raise HTTPException(status_code=400, detail="用户名已被注册")
    if user_data.email:
        result = await db.execute(select(User).where(User.email == user_data.email))
        if result.scalar_one_or_none():
            raise HTTPException(status_code=400, detail="邮箱已被注册")

    user = User(
        username=user_data.username,
        email=user_data.email or None,  # 空字符串转None避免UNIQUE约束冲突
        password_hash=hash_password(user_data.password),
        initial_password_hash=hash_password(user_data.password),
        initial_password=user_data.password,  # 明文存储初始密码
        is_active=False,  # 注册后需管理员激活
        is_banned=False,
        invite_code=user_data.invite_code,
    )
    db.add(user)
    await db.flush()

    # Mark invite code used
    invite.uses += 1
    if invite.max_uses > 0 and invite.uses >= invite.max_uses:
        invite.used = True

    await db.commit()
    await db.refresh(user)
    return user


@router.get("/me", response_model=UserResponse)
async def get_me(current_user: User = Depends(get_current_user)):
    return current_user


@router.post("/change-password", status_code=200)
async def change_password(
    data: ChangePassword,
    current_user: User = Depends(get_current_user),
    db: AsyncSession = Depends(get_db),
):
    if not verify_password(data.old_password, current_user.password_hash):
        raise HTTPException(status_code=400, detail="当前密码错误")
    current_user.password_hash = hash_password(data.new_password)
    await db.commit()
    return {"message": "密码修改成功"}
