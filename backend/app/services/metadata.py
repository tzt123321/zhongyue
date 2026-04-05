import os
import re
from datetime import datetime
from typing import Optional, Dict, Any, List
from mutagen import File as MutagenFile
from mutagen.mp3 import MP3
from mutagen.flac import FLAC
from mutagen.wave import WAVE
from mutagen.m4a import M4A


SUPPORTED_EXTENSIONS = {".mp3", ".flac", ".wav", ".m4a", ".ogg", ".opus", ".aac"}

# Separators used to split collaborative artist names
ARTIST_SEPARATORS = re.compile(r'[&/,；,\uff0c+]')


def split_artists(artist_str: str) -> List[str]:
    """Split a collaborative artist string like 'A & B' or 'A/B' into individual names."""
    if not artist_str or artist_str.strip() in ('Unknown Artist',):
        return ['Unknown Artist']
    parts = ARTIST_SEPARATORS.split(artist_str)
    names = [p.strip() for p in parts if p.strip()]
    return names if names else ['Unknown Artist']


def get_audio_format(path: str) -> str:
    ext = os.path.splitext(path.lower())[1]
    format_map = {
        ".mp3": "MP3",
        ".flac": "FLAC",
        ".wav": "WAV",
        ".m4a": "AAC",
        ".ogg": "OGG",
        ".opus": "OPUS",
        ".aac": "AAC",
    }
    return format_map.get(ext, "UNKNOWN")


def extract_metadata(file_path: str) -> Optional[Dict[str, Any]]:
    """Extract metadata from an audio file using mutagen."""
    try:
        if not os.path.exists(file_path):
            return None
        ext = os.path.splitext(file_path.lower())[1]
        if ext not in SUPPORTED_EXTENSIONS:
            return None

        audio = MutagenFile(file_path, easy=True)
        if audio is None:
            return None

        # Basic info
        info = {}
        if hasattr(audio, "info"):
            info = {
                "duration": int(getattr(audio.info, "length", 0)),
                "bitrate": int(getattr(audio.info, "bitrate", 0)) // 1000 if hasattr(audio.info, "bitrate") else None,
            }

        # Get tags
        tags = audio.tags if audio.tags else {}

        def get_tag(name: str, default=None):
            vals = tags.get(name, [])
            if not vals:
                vals = tags.get(name.lower(), [])
            if not vals:
                return default
            if isinstance(vals, list):
                return str(vals[0]) if vals else default
            return str(vals) if vals else default

        # Extract common fields
        title = get_tag("title") or os.path.splitext(os.path.basename(file_path))[0]
        artist = get_tag("artist") or get_tag("albumartist") or "Unknown Artist"
        artist_names = split_artists(artist)
        album = get_tag("album") or "Unknown Album"
        year = get_tag("date") or get_tag("year")
        if year:
            try:
                year = int(year[:4])
            except (ValueError, IndexError):
                year = None
        track_number = get_tag("tracknumber")
        if track_number:
            try:
                track_number = int(track_number.split("/")[0])
            except (ValueError, IndexError):
                track_number = None
        disc_number = get_tag("discnumber")
        if disc_number:
            try:
                disc_number = int(disc_number.split("/")[0])
            except (ValueError, IndexError):
                disc_number = None

        return {
            "title": title.strip(),
            "artist": artist.strip(),
            "artist_names": artist_names,
            "album": album.strip(),
            "year": year,
            "track_number": track_number,
            "disc_number": disc_number,
            "duration": info.get("duration"),
            "bitrate": info.get("bitrate"),
            "format": get_audio_format(file_path),
            "file_size": os.path.getsize(file_path),
            "file_mtime": datetime.fromtimestamp(os.path.getmtime(file_path)),
        }
    except Exception as e:
        return None
