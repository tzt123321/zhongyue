#!/bin/bash
echo "[HOTFIX] 只替换 API 二进制（不停止服务）..."
docker compose stop api
docker load < zhongyue-api-fixed.tar.gz
docker compose up -d api
sleep 3
echo "完成! 测试刮削状态:"
curl -s http://127.0.0.1:7800/api/library/scrape-status | python3 -m json.tool
