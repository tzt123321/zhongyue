"""
手动封面管理 API
当自动刮削失败时，允许手动上传封面
"""
from fastapi import APIRouter, UploadFile, File, HTTPException
from pathlib import Path
import shutil
import uuid

router = APIRouter(prefix="/api/covers", tags=["covers"])

# 封面存储目录
COVER_DIR = Path(__file__).parent.parent.parent.parent / "data" / "covers"
COVER_DIR.mkdir(parents=True, exist_ok=True)


@router.post("/upload/album/{album_id}")
async def upload_album_cover(album_id: int, file: UploadFile = File(...)):
    """为专辑上传封面"""
    # 验证文件类型
    allowed_types = ["image/jpeg", "image/png", "image/webp"]
    if file.content_type not in allowed_types:
        raise HTTPException(status_code=400, detail="Only JPEG, PNG, WebP allowed")
    
    # 生成文件名
    ext = file.content_type.split("/")[-1]
    filename = f"album_{album_id}_{uuid.uuid4().hex[:8]}.{ext}"
    filepath = COVER_DIR / filename
    
    # 保存文件
    try:
        with open(filepath, "wb") as f:
            shutil.copyfileobj(file.file, f)
        
        # 更新数据库（需要通过 scraper API 的路径更新）
        return {
            "success": True,
            "cover_path": str(filepath),
            "message": "Cover uploaded successfully"
        }
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))


@router.post("/upload/artist/{artist_id}")
async def upload_artist_cover(artist_id: int, file: UploadFile = File(...)):
    """为艺术家上传封面"""
    allowed_types = ["image/jpeg", "image/png", "image/webp"]
    if file.content_type not in allowed_types:
        raise HTTPException(status_code=400, detail="Only JPEG, PNG, WebP allowed")
    
    ext = file.content_type.split("/")[-1]
    filename = f"artist_{artist_id}_{uuid.uuid4().hex[:8]}.{ext}"
    filepath = COVER_DIR / filename
    
    try:
        with open(filepath, "wb") as f:
            shutil.copyfileobj(file.file, f)
        
        return {
            "success": True,
            "cover_path": str(filepath),
            "message": "Artist image uploaded successfully"
        }
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))


@router.get("/serve/{filename}")
async def serve_cover(filename: str):
    """提供封面的直接访问（可选）"""
    filepath = COVER_DIR / filename
    if not filepath.exists():
        raise HTTPException(status_code=404, detail="Cover not found")
    
    from fastapi.responses import FileResponse
    return FileResponse(filepath)
