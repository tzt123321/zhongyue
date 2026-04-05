"""
QQ音乐爬虫 - 使用浏览器自动化获取数据
当 API 不可用时，使用浏览器直接抓取网页
"""
import asyncio
import aiohttp
import subprocess
import json
import re
from typing import Optional, Dict, List
from pathlib import Path


class QQMusicCrawler:
    """QQ音乐爬虫，支持 API 和浏览器两种模式"""
    
    BASE_URL = "https://y.qq.com"
    SEARCH_URL = "https://y.qq.com/portal/search.html"
    
    def __init__(self, cookies: Optional[str] = None):
        self.cookies = cookies or ""
        self._browser_session = None
    
    async def search_album_api(self, artist: str, album: str) -> Optional[Dict]:
        """尝试使用 API 搜索专辑"""
        # 尝试多个可能的 API 端点
        apis = [
            ("PC Search", "https://c.y.qq.com/soso/fcgi-bin/client_search_cp", {
                "w": f"{artist} {album}",
                "format": "json",
                "p": 1,
                "n": 10,
                "cr": 1,
            }),
            ("Mobile Search", "https://c.y.qq.com/v8/fcg-bin/fcg_search_cp.fcg", {
                "w": f"{artist} {album}",
                "format": "json",
                "p": 1,
                "n": 10,
            }),
        ]
        
        headers = {
            "Referer": "https://y.qq.com/",
            "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
            "Accept": "application/json, text/javascript, */*; q=0.01",
            "Accept-Language": "zh-CN,zh;q=0.9,en;q=0.8",
            "Cookie": self.cookies or "pgv_pvid=random12345",
        }
        
        for name, url, params in apis:
            try:
                async with aiohttp.ClientSession() as session:
                    async with session.get(url, params=params, headers=headers, timeout=aiohttp.ClientTimeout(total=10)) as resp:
                        if resp.status == 200:
                            content_type = resp.headers.get("Content-Type", "")
                            if "json" in content_type or "javascript" in content_type:
                                text = await resp.text()
                                data = json.loads(text)
                                # 解析响应
                                result = self._parse_search_response(data)
                                if result:
                                    return result
                        elif resp.status == 500:
                            print(f"QQ Music {name} returned 500")
            except Exception as e:
                print(f"QQ Music {name} failed: {e}")
        
        return None
    
    def _parse_search_response(self, data) -> Optional[Dict]:
        """解析搜索响应"""
        try:
            # 尝试多种响应格式
            if "song" in data:
                songs = data.get("song", {}).get("list", [])
                if songs:
                    song = songs[0]
                    album_name = song.get("albumname", "")
                    album_mid = song.get("albummid", "")
                    if album_mid:
                        return {
                            "album_name": album_name,
                            "cover_url": f"https://y.gtimg.cn/music/photo_new/T002R300x300M000{album_mid}.jpg",
                            "album_mid": album_mid,
                        }
            if "data" in data:
                if isinstance(data["data"], dict):
                    songs = data["data"].get("song", {}).get("list", [])
                    if songs:
                        song = songs[0]
                        album_name = song.get("albumname", "")
                        album_mid = song.get("albummid", "")
                        if album_mid:
                            return {
                                "album_name": album_name,
                                "cover_url": f"https://y.gtimg.cn/music/photo_new/T002R300x300M000{album_mid}.jpg",
                                "album_mid": album_mid,
                            }
        except Exception as e:
            print(f"Parse error: {e}")
        return None
    
    async def search_album_browser(self, artist: str, album: str) -> Optional[Dict]:
        """
        使用浏览器搜索专辑（当 API 不可用时）
        需要系统安装 Chrome 和 browser CLI
        """
        search_url = f"https://y.qq.com/portal/search.html#search=jq&type=album&search_key={artist}+{album}"
        
        try:
            # 使用 browser CLI 导航并提取数据
            result = await self._browser_crawl(search_url, artist, album)
            return result
        except Exception as e:
            print(f"Browser crawl failed: {e}")
            return None
    
    async def _browser_crawl(self, url: str, artist: str, album: str) -> Optional[Dict]:
        """使用浏览器爬取"""
        # 构建搜索 URL
        search_query = f"{artist} {album}"
        encoded_query = asyncio.get_event_loop().run_until_complete(
            self._url_encode(search_query)
        )
        
        # 导航到搜索页
        nav_cmd = f'browser navigate "https://y.qq.com/portal/search.html?search=jq&type=album&search_key={encoded_query}"'
        result = subprocess.run(nav_cmd, shell=True, capture_output=True, text=True, timeout=30)
        
        if result.returncode != 0:
            print(f"Browser navigation failed: {result.stderr}")
            return None
        
        # 等待页面加载
        await asyncio.sleep(3)
        
        # 截图确认
        subprocess.run("browser screenshot", shell=True, capture_output=True, timeout=10)
        
        # 提取专辑信息
        extract_cmd = f'browser extract "find the first album cover image URL"'
        result = subprocess.run(extract_cmd, shell=True, capture_output=True, text=True, timeout=30)
        
        # 关闭浏览器
        subprocess.run("browser close", shell=True, capture_output=True, timeout=10)
        
        return None  # 返回提取结果
    
    async def _url_encode(self, text: str) -> str:
        """URL编码"""
        import urllib.parse
        return urllib.parse.quote(text)
    
    async def get_album_detail(self, album_mid: str) -> Optional[Dict]:
        """通过专辑MID获取详情"""
        url = f"https://y.qq.com/n/ryqq/album/detail/{album_mid}"
        
        headers = {
            "Referer": "https://y.qq.com/",
            "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
        }
        
        try:
            async with aiohttp.ClientSession() as session:
                async with session.get(url, headers=headers, timeout=aiohttp.ClientTimeout(total=15)) as resp:
                    if resp.status == 200:
                        text = await resp.text()
                        # 尝试从 HTML 中提取专辑信息
                        return self._parse_album_html(text)
        except Exception as e:
            print(f"Get album detail failed: {e}")
        
        return None
    
    def _parse_album_html(self, html: str) -> Optional[Dict]:
        """从 HTML 页面解析专辑信息"""
        try:
            # 尝试提取 JSON 数据
            patterns = [
                r'albumDetail\s*=\s*(\{[^;]+\});',
                r'"album_mid"\s*:\s*"([^"]+)"',
                r'cover_url\s*:\s*"([^"]+)"',
            ]
            
            for pattern in patterns:
                match = re.search(pattern, html)
                if match:
                    # 找到了数据，尝试进一步解析
                    pass
            
            # 尝试提取og:image
            img_match = re.search(r'<meta[^>]+property="og:image"[^>]+content="([^"]+)"', html)
            if img_match:
                return {
                    "cover_url": img_match.group(1)
                }
        except Exception as e:
            print(f"HTML parse error: {e}")
        
        return None
    
    async def search_album(self, artist: str, album: str) -> Optional[Dict]:
        """
        搜索专辑 - 自动选择可用方式
        优先级: API > 浏览器
        """
        # 先尝试 API
        result = await self.search_album_api(artist, album)
        if result:
            return result
        
        # API 失败，尝试浏览器（如果有）
        # result = await self.search_album_browser(artist, album)
        # if result:
        #     return result
        
        return None
    
    async def get_artist_image(self, artist: str) -> Optional[str]:
        """获取艺术家图片"""
        # 尝试通过专辑封面获取艺术家图片
        result = await self.search_album(artist, "")
        if result:
            return result.get("cover_url")
        return None
