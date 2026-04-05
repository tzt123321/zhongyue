from fastapi import APIRouter
from .auth import router as auth_router
from .tracks import router as tracks_router
from .tracks_upload import router as tracks_upload_router
from .albums import router as albums_router
from .artists import router as artists_router
from .playlists import router as playlists_router
from .search import router as search_router
from .stats import router as stats_router
from .library import router as library_router
from .system import router as system_router
from .scraper import router as scraper_router
from .covers import router as covers_router
from .admin_users import router as admin_users_router
from .effects import router as effects_router
from .forum import router as forum_router
from .api_keys import router as api_keys_router
from .playback import router as playback_router

api_router = APIRouter()

api_router.include_router(auth_router, prefix="/auth", tags=["auth"])
api_router.include_router(tracks_router, prefix="/tracks", tags=["tracks"])
api_router.include_router(tracks_upload_router, prefix="/tracks", tags=["tracks-upload"])
api_router.include_router(albums_router, prefix="/albums", tags=["albums"])
api_router.include_router(artists_router, prefix="/artists", tags=["artists"])
api_router.include_router(playlists_router, prefix="/playlists", tags=["playlists"])
api_router.include_router(search_router, prefix="/search", tags=["search"])
api_router.include_router(stats_router, prefix="/stats", tags=["stats"])
api_router.include_router(library_router, prefix="/library", tags=["library"])
api_router.include_router(system_router, tags=["system"])
api_router.include_router(scraper_router, tags=["scraper"])
api_router.include_router(covers_router, tags=["covers"])
api_router.include_router(admin_users_router, prefix="/admin", tags=["admin"])
api_router.include_router(effects_router, prefix="/effects", tags=["effects"])
api_router.include_router(forum_router, prefix="/forum", tags=["forum"])
api_router.include_router(api_keys_router, prefix="/auth", tags=["api-keys"])
api_router.include_router(playback_router, prefix="/playback", tags=["playback"])
