#!/usr/bin/env python3
"""扫描音乐文件时长并更新数据库"""
import subprocess
import psycopg2
import os
import re

# 音乐文件在主机上的路径前缀
MUSIC_HOST_PATH = "/root/.openclaw/workspace-taizi/zhongyue_refactored/music"

DB_CONFIG = {
    "host": "localhost",
    "port": 5432,
    "dbname": "zhongyue",
    "user": "zhongyue",
    "password": "zhongyue_secret_pass"
}

def get_duration(filepath):
    """用 ffprobe 获取音频时长（秒）"""
    full_path = os.path.join(MUSIC_HOST_PATH, os.path.basename(filepath))
    if not os.path.exists(full_path):
        # 尝试strip leading /
        full_path = filepath
        if not os.path.exists(full_path):
            return None
    
    result = subprocess.run(
        ["ffprobe", "-v", "quiet", "-show_entries", "format=duration",
         "-of", "default=noprint_wrappers=1:nokey=1", full_path],
        capture_output=True, text=True
    )
    try:
        return int(float(result.stdout.strip()))
    except:
        return None

def main():
    conn = psycopg2.connect(**DB_CONFIG)
    cur = conn.cursor()
    
    # 获取所有tracks
    cur.execute("SELECT id, file_path FROM tracks WHERE duration = 0 OR duration IS NULL;")
    tracks = cur.fetchall()
    
    print(f"找到 {len(tracks)} 首待更新的曲目")
    updated = 0
    
    for track_id, file_path in tracks:
        duration = get_duration(file_path)
        if duration:
            cur.execute("UPDATE tracks SET duration = %s WHERE id = %s", (duration, track_id))
            print(f"✅ id={track_id} duration={duration}s")
            updated += 1
        else:
            print(f"⚠️ id={track_id} 文件不可访问: {file_path}")
    
    conn.commit()
    print(f"\n完成，共更新 {updated}/{len(tracks)} 首")
    cur.close()
    conn.close()

if __name__ == "__main__":
    main()
