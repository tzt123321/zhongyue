from fastapi import APIRouter, Depends, HTTPException, Form
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession
import secrets

from app.database import get_db
from app.models.user import User
from app.api.auth import get_current_user

router = APIRouter()


@router.post("/login-by-key")
async def login_by_key(api_key: str = Form(...), db: AsyncSession = Depends(get_db)):
    """用 API Key 登录，无需用户名密码"""
    result = await db.execute(select(User).where(User.api_key == api_key))
    user = result.scalar_one_or_none()
    if not user:
        raise HTTPException(status_code=401, detail="无效的 API Key")
    if not user.is_active:
        raise HTTPException(status_code=403, detail="账户尚未激活")
    if user.is_banned:
        raise HTTPException(status_code=403, detail="账户已被封禁")

    from app.utils.jwt import create_access_token
    token = create_access_token({"sub": str(user.id)})
    return {
        "access_token": token,
        "token_type": "bearer",
        "user": {
            "id": user.id,
            "username": user.username,
            "is_admin": user.is_admin,
        },
    }


@router.post("/generate-key")
async def generate_api_key(
    db: AsyncSession = Depends(get_db),
    current_user: User = Depends(get_current_user),
):
    """为当前用户生成或重置 API Key"""
    new_key = secrets.token_urlsafe(32)
    current_user.api_key = new_key
    await db.commit()
    return {"api_key": new_key}


@router.get("/my-key")
async def get_my_key(
    db: AsyncSession = Depends(get_db),
    current_user: User = Depends(get_current_user),
):
    """查看当前用户的 API Key（如果已有）"""
    return {"api_key": current_user.api_key}
