#!/bin/bash
set -e
MUSIC_DIR="/root/.openclaw/workspace-taizi/zhongyue_refactored/music"
echo "扫描 $MUSIC_DIR 下的音乐文件..."

COUNT=0
for f in "$MUSIC_DIR"/*.mp3; do
    [ -f "$f" ] || continue
    BASENAME=$(basename "$f")
    # 从文件名找DB中的track
    DURATION=$(ffprobe -v quiet -show_entries format=duration -of default=noprint_wrappers=1:nokey=1 "$f" 2>/dev/null | cut -d. -f1)
    if [ -z "$DURATION" ] || [ "$DURATION" = "0" ]; then
        echo "⚠️ $BASENAME 时长获取失败"
        continue
    fi
    # 更新数据库
    docker exec zhongyue_postgres psql -U zhongyue -d zhongyue -c "UPDATE tracks SET duration = $DURATION WHERE file_path = '/music/$BASENAME';" 2>/dev/null
    echo "✅ $BASENAME -> ${DURATION}s"
    COUNT=$((COUNT+1))
done
echo "完成，共更新 $COUNT 首"
