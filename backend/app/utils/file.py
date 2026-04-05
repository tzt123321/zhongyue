import os
from typing import List


SUPPORTED_FORMATS = {".mp3", ".flac", ".wav", ".m4a", ".ogg", ".opus", ".aac"}


def is_audio_file(path: str) -> bool:
    ext = os.path.splitext(path.lower())[1]
    return ext in SUPPORTED_FORMATS


def find_audio_files(paths: List[str]) -> List[str]:
    audio_files = []
    for base_path in paths:
        if not os.path.exists(base_path):
            continue
        for root, _, files in os.walk(base_path):
            for f in files:
                full_path = os.path.join(root, f)
                if is_audio_file(full_path):
                    audio_files.append(full_path)
    return audio_files
