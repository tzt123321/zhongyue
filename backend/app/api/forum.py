from fastapi import APIRouter, Depends, HTTPException, Query
from sqlalchemy import select, desc
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy.orm import selectinload
from app.database import get_db
from app.models.forum import ForumPost, ForumComment, Notification
from app.models.user import User
from app.api.auth import get_current_user
from pydantic import BaseModel
from datetime import datetime

router = APIRouter()

class PostIn(BaseModel):
    title: str
    content: str

class PostOut(BaseModel):
    id: int
    user_id: int
    username: str
    title: str
    content: str
    created_at: datetime

    class Config:
        from_attributes = True

@router.get("/posts")
async def list_posts(
    page: int = Query(1, ge=1),
    limit: int = Query(20, ge=1, le=100),
    db: AsyncSession = Depends(get_db),
):
    offset = (page - 1) * limit
    result = await db.execute(
        select(ForumPost)
        .options(selectinload(ForumPost.user))
        .order_by(desc(ForumPost.created_at))
        .offset(offset)
        .limit(limit)
    )
    posts = result.scalars().all()
    count_result = await db.execute(select(ForumPost))
    total = len(count_result.scalars().all())
    # Transform posts to include username
    items = []
    for p in posts:
        items.append({
            "id": p.id,
            "user_id": p.user_id,
            "username": p.user.username,
            "title": p.title,
            "content": p.content,
            "created_at": p.created_at,
        })
    return {
        "items": items,
        "total": total,
        "page": page,
        "limit": limit
    }

@router.post("/posts", response_model=PostOut)
async def create_post(
    data: PostIn,
    db: AsyncSession = Depends(get_db),
    current_user: User = Depends(get_current_user),
):
    if current_user.is_muted:
        raise HTTPException(status_code=403, detail="您已被禁言，无法发帖")
    post = ForumPost(
        user_id=current_user.id,
        title=data.title,
        content=data.content,
    )
    db.add(post)
    await db.commit()
    await db.refresh(post)
    return {
        "id": post.id,
        "user_id": post.user_id,
        "username": current_user.username,
        "title": post.title,
        "content": post.content,
        "created_at": post.created_at,
    }

@router.delete("/posts/{post_id}")
async def delete_post(
    post_id: int,
    db: AsyncSession = Depends(get_db),
    current_user: User = Depends(get_current_user),
):
    result = await db.execute(select(ForumPost).where(ForumPost.id == post_id))
    post = result.scalar_one_or_none()
    if not post:
        raise HTTPException(status_code=404, detail="帖子不存在")
    if post.user_id != current_user.id and not current_user.is_admin:
        raise HTTPException(status_code=403, detail="无权删除此帖子")
    await db.delete(post)
    await db.commit()
    return {"message": "删除成功"}

class CommentIn(BaseModel):
    content: str

class CommentOut(BaseModel):
    id: int
    post_id: int
    user_id: int
    username: str
    content: str
    created_at: datetime

@router.get("/posts/{post_id}/comments")
async def list_comments(post_id: int, db: AsyncSession = Depends(get_db)):
    result = await db.execute(
        select(ForumComment)
        .options(selectinload(ForumComment.user))
        .where(ForumComment.post_id == post_id)
        .order_by(ForumComment.created_at)
    )
    comments = result.scalars().all()
    return [{
        "id": c.id,
        "post_id": c.post_id,
        "user_id": c.user_id,
        "username": c.user.username,
        "content": c.content,
        "created_at": c.created_at,
    } for c in comments]

@router.post("/posts/{post_id}/comments", response_model=CommentOut)
async def create_comment(
    post_id: int,
    data: CommentIn,
    db: AsyncSession = Depends(get_db),
    current_user: User = Depends(get_current_user),
):
    if current_user.is_muted:
        raise HTTPException(status_code=403, detail="您已被禁言，无法评论")
    # 确认帖子存在
    post_result = await db.execute(select(ForumPost).where(ForumPost.id == post_id))
    post = post_result.scalar_one_or_none()
    if not post:
        raise HTTPException(status_code=404, detail="帖子不存在")
    comment = ForumComment(
        post_id=post_id,
        user_id=current_user.id,
        content=data.content,
    )
    db.add(comment)
    await db.commit()
    await db.refresh(comment)
    # 通知帖子作者（如果评论者不是作者本人）
    if post.user_id != current_user.id:
        notif = Notification(
            user_id=post.user_id,
            type="comment_reply",
            title="有人回复了你的帖子",
            content=f"{current_user.username} 回复了「{post.title}」",
        )
        db.add(notif)
        await db.commit()
    return {
        "id": comment.id,
        "post_id": comment.post_id,
        "user_id": comment.user_id,
        "username": current_user.username,
        "content": comment.content,
        "created_at": comment.created_at,
    }

@router.delete("/comments/{comment_id}")
async def delete_comment(
    comment_id: int,
    db: AsyncSession = Depends(get_db),
    current_user: User = Depends(get_current_user),
):
    result = await db.execute(select(ForumComment).where(ForumComment.id == comment_id))
    comment = result.scalar_one_or_none()
    if not comment:
        raise HTTPException(status_code=404, detail="评论不存在")
    if comment.user_id != current_user.id and not current_user.is_admin:
        raise HTTPException(status_code=403, detail="无权删除此评论")
    await db.delete(comment)
    await db.commit()
    return {"message": "删除成功"}

class NotificationIn(BaseModel):
    title: str
    content: str

class NotificationOut(BaseModel):
    id: int
    user_id: int
    type: str
    title: str
    content: str
    is_read: bool
    created_at: datetime

@router.get("/notifications")
async def get_notifications(db: AsyncSession = Depends(get_db), current_user: User = Depends(get_current_user)):
    result = await db.execute(
        select(Notification)
        .where(Notification.user_id == current_user.id)
        .order_by(desc(Notification.created_at))
        .limit(50)
    )
    notifications = result.scalars().all()
    return [{"id": n.id, "user_id": n.user_id, "type": n.type, "title": n.title, "content": n.content, "is_read": n.is_read, "created_at": n.created_at} for n in notifications]

@router.put("/notifications/{notif_id}/read")
async def mark_read(notif_id: int, db: AsyncSession = Depends(get_db), current_user: User = Depends(get_current_user)):
    result = await db.execute(select(Notification).where(Notification.id == notif_id, Notification.user_id == current_user.id))
    n = result.scalar_one_or_none()
    if not n:
        raise HTTPException(status_code=404, detail="通知不存在")
    n.is_read = True
    await db.commit()
    return {"message": "已读"}

@router.put("/notifications/read-all")
async def mark_all_read(db: AsyncSession = Depends(get_db), current_user: User = Depends(get_current_user)):
    result = await db.execute(
        select(Notification).where(Notification.user_id == current_user.id, Notification.is_read == False)
    )
    unread = result.scalars().all()
    for n in unread:
        n.is_read = True
    await db.commit()
    return {"message": f"已标记 {len(unread)} 条为已读"}

# 管理员发布公告
@router.post("/admin/announcements", response_model=NotificationOut)
async def create_announcement(
    data: NotificationIn,
    db: AsyncSession = Depends(get_db),
    current_user: User = Depends(get_current_user),
):
    if not current_user.is_admin:
        raise HTTPException(status_code=403, detail="需要管理员权限")
    result = await db.execute(select(User.id))
    all_user_ids = [r[0] for r in result.fetchall()]
    notifications = []
    for uid in all_user_ids:
        n = Notification(
            user_id=uid,
            type="announcement",
            title=data.title,
            content=data.content,
        )
        db.add(n)
        notifications.append(n)
    await db.commit()
    for n in notifications:
        await db.refresh(n)
    return {
        "id": notifications[0].id,
        "user_id": notifications[0].user_id,
        "type": notifications[0].type,
        "title": notifications[0].title,
        "content": notifications[0].content,
        "is_read": notifications[0].is_read,
        "created_at": notifications[0].created_at,
    }
