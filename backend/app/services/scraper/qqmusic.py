"""
QQ音乐刮削器 - 基于逆向工程的 API
参考: music-tag-web 项目
"""
import json
import uuid
import aiohttp
from typing import Optional, Dict, List
from .base import BaseScraper


class QQMusicScraper(BaseScraper):
    """QQ音乐刮削器，支持搜索和获取专辑/艺术家信息"""
    
    BASE_URL = "https://u.y.qq.com/cgi-bin/musicu.fcg"
    # 专辑封面URL格式
    ALBUM_COVER_URL = "https://y.gtimg.cn/music/photo_new/T002R300x300M000{id}.jpg"
    # 艺术家封面URL格式
    ARTIST_COVER_URL = "https://y.gtimg.cn/music/photo_new/T001R300x300M000{id}.jpg"
    
    def __init__(self, cookies: str = "", cache_ttl: int = 86400 * 30):
        super().__init__(cache_ttl)
        self.cookies = cookies or self._get_default_cookies()
        self._timeout = 12
    
    def _get_default_cookies(self) -> str:
        """获取默认Cookie（公开接口可能需要的最小Cookie）"""
        return ""
    
    @property
    def name(self) -> str:
        return "qqmusic"
    
    @property
    def order(self) -> int:
        return 2  # 高优先级
    
    def _get_headers(self) -> Dict:
        """获取请求头"""
        return {
            "Referer": "https://y.qq.com/portal/profile.html",
            "Content-Type": "application/json;charset=utf-8",
            "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
            "Accept": "application/json",
            "Accept-Language": "zh-CN,zh;q=0.9,en;q=0.8",
        }
    
    async def _post_json(self, url: str, data: Dict) -> Optional[Dict]:
        """发送 POST 请求获取 JSON 数据"""
        try:
            async with aiohttp.ClientSession() as session:
                async with session.post(
                    url,
                    json=data,
                    headers=self._get_headers(),
                    timeout=aiohttp.ClientTimeout(total=self._timeout)
                ) as resp:
                    if resp.status == 200:
                        text = await resp.text()
                        # QQ Music 可能返回 text/plain，手动解析 JSON
                        content_type = resp.headers.get("Content-Type", "")
                        if "json" not in content_type and "javascript" not in content_type:
                            # 手动解析 JSON（即使 Content-Type 不是 application/json）
                            import json
                            return json.loads(text)
                        return await resp.json()
        except Exception as e:
            print(f"QQMusic request failed: {e}")
        return None
    
    async def search_album(self, artist: str, album: str) -> Optional[Dict]:
        """搜索专辑"""
        cache_key = self._get_cache_key("album", artist, album)
        cached = await self._get_from_cache(cache_key)
        if cached:
            return cached
        
        # 使用 QQ 音乐搜索 API
        data = {
            "comm": {
                "ct": 6, "cv": 80600,
                "gray": "0",
                "nettype": "2",
                "patch": "2",
                "psrf_qqopenid": "",
                "psrf_qqunionid": "",
                "psrf_access_token_expiresAt": "",
                "psrf_qqaccess_token": "",
                "tmeLoginType": "2",
                "uid": "",
                "gray": "0",
                "OpenUDID": uuid.uuid4().__str__(),
                "sid": "",
                "gzip": "0",
                "qq": "",
                "tmeAppID": "qqmusic",
                "wid": "",
                "authst": "",
            },
            "music.search.SearchCgiService.DoSearchForQQMusicDesktop": {
                "module": "music.search.SearchCgiService",
                "method": "DoSearchForQQMusicDesktop",
                "param": {
                    "num_per_page": 15,
                    "page_num": 1,
                    "remoteplace": "txt.mac.search",
                    "search_type": 0,  # 0=单曲, 2=专辑
                    "query": f"{artist} {album}",
                    "grp": 1,
                    "searchid": uuid.uuid4().__str__(),
                    "nqc_flag": 0
                }
            }
        }
        
        result = await self._post_json(self.BASE_URL, data)
        if not result:
            return None
        
        try:
            search_data = result.get("music.search.SearchCgiService.DoSearchForQQMusicDesktop", {}).get("data", {})
            songs = search_data.get("body", {}).get("song", {}).get("list", [])
            
            if not songs:
                return None
            
            # 精确匹配专辑
            for song in songs:
                album_info = song.get("album", {})
                album_title = album_info.get("title", "")
                if album.lower() in album_title.lower() or album_title.lower() in album.lower():
                    album_mid = album_info.get("mid", "")
                    if album_mid:
                        result_data = {
                            "album_name": album_title,
                            "artist_name": self._format_singer(song.get("singer", [])),
                            "album_mid": album_mid,
                            "cover_url": self.ALBUM_COVER_URL.format(id=album_mid),
                        }
                        await self._set_cache(cache_key, result_data)
                        return result_data
            
            # 如果没精确匹配，返回第一个
            if songs:
                song = songs[0]
                album_info = song.get("album", {})
                album_mid = album_info.get("mid", "")
                result_data = {
                    "album_name": album_info.get("title", ""),
                    "artist_name": self._format_singer(song.get("singer", [])),
                    "album_mid": album_mid,
                    "cover_url": self.ALBUM_COVER_URL.format(id=album_mid) if album_mid else None,
                }
                await self._set_cache(cache_key, result_data)
                return result_data
        except Exception as e:
            print(f"QQMusic parse error: {e}")
        
        return None
    
    async def search_artist(self, artist: str) -> Optional[Dict]:
        """搜索艺术家 - 从歌曲搜索结果中提取"""
        cache_key = self._get_cache_key("artist", artist)
        cached = await self._get_from_cache(cache_key)
        if cached:
            return cached
        
        # 搜索歌曲，从中提取歌手信息
        data = {
            "comm": {
                "ct": 6, "cv": 80600, "gray": "0", "nettype": "2", "patch": "2",
                "OpenUDID": uuid.uuid4().__str__(), "gzip": "0", "uid": "", "qq": "",
                "tmeAppID": "qqmusic", "tmeLoginType": "2", "psrf_qqopenid": "",
                "psrf_qqunionid": "", "psrf_access_token_expiresAt": "",
                "psrf_qqaccess_token": "", "wid": "", "authst": "", "sid": ""
            },
            "music.search.SearchCgiService.DoSearchForQQMusicDesktop": {
                "module": "music.search.SearchCgiService",
                "method": "DoSearchForQQMusicDesktop",
                "param": {
                    "num_per_page": 5,
                    "page_num": 1,
                    "remoteplace": "txt.mac.search",
                    "search_type": 0,  # 0=单曲
                    "query": artist,
                    "grp": 1,
                    "searchid": uuid.uuid4().__str__(),
                }
            }
        }
        
        result = await self._post_json(self.BASE_URL, data)
        if not result:
            return None
        
        try:
            search_data = result.get("music.search.SearchCgiService.DoSearchForQQMusicDesktop", {}).get("data", {})
            songs = search_data.get("body", {}).get("song", {}).get("list", [])
            
            # 从歌曲中提取歌手信息
            for song in songs:
                singers = song.get("singer", [])
                for singer in singers:
                    singer_name = singer.get("name", "")
                    if artist.lower() in singer_name.lower() or singer_name.lower() in artist.lower():
                        singer_mid = singer.get("mid", "")
                        result_data = {
                            "name": singer_name,
                            "singer_mid": singer_mid,
                            "cover_url": self.ARTIST_COVER_URL.format(id=singer_mid) if singer_mid else None,
                        }
                        await self._set_cache(cache_key, result_data)
                        return result_data
            
            # 返回第一个歌手
            if songs:
                singers = songs[0].get("singer", [])
                if singers:
                    singer = singers[0]
                    singer_mid = singer.get("mid", "")
                    result_data = {
                        "name": singer.get("name", ""),
                        "singer_mid": singer_mid,
                        "cover_url": self.ARTIST_COVER_URL.format(id=singer_mid) if singer_mid else None,
                    }
                    await self._set_cache(cache_key, result_data)
                    return result_data
        except Exception as e:
            print(f"QQMusic artist parse error: {e}")
        
        return None
    
    async def get_artist_image(self, artist: str) -> Optional[str]:
        """获取艺术家图片"""
        result = await self.search_artist(artist)
        if result and result.get("cover_url"):
            return result["cover_url"]
        return None
    
    async def get_album_detail(self, album_mid: str) -> Optional[Dict]:
        """通过专辑 MID 获取专辑详情"""
        data = {
            "get_song_detail": {
                "module": "music.pf_song_detail_svr",
                "method": "get_song_detail",
                "param": {
                    "song_mid": album_mid,
                    "song_type": 0
                }
            },
            "comm": {
                "g_tk": 0, "uin": "", "format": "json", "ct": 6, "cv": 80600,
                "platform": "wk_v17", "uid": "", "guid": uuid.uuid4().__str__()
            }
        }
        
        result = await self._post_json(self.BASE_URL, data)
        if not result:
            return None
        
        try:
            track_info = result.get("get_song_detail", {}).get("data", {}).get("track_info", {})
            return {
                "name": track_info.get("title", ""),
                "album_mid": album_mid,
            }
        except Exception as e:
            print(f"QQMusic album detail error: {e}")
        
        return None
    
    def _format_singer(self, singers: List[Dict]) -> str:
        """格式化歌手名列表"""
        if not singers:
            return "未知艺术家"
        return ",".join([s.get("name", "") for s in singers])
