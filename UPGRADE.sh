#!/bin/bash
set -e
echo "========================================="
echo "  ZhongYue 修复包 - $(date +%Y%m%d)"
echo "========================================="

echo ""
echo "[1/6] 停止所有服务..."
docker compose down

echo ""
echo "[2/6] 加载修复后的 API 镜像..."
docker load < zhongyue-api-fixed.tar.gz

echo ""
echo "[3/6] 重启所有服务（让 nginx.conf 生效）..."
docker compose up -d

echo ""
echo "[4/6] 等待服务启动..."
sleep 5

echo ""
echo "[5/6] 验证 API 镜像已替换..."
NEW_IMAGE=$(docker compose images api --format json 2>/dev/null | grep -o '"Repository":"[^"]*"' | cut -d'"' -f4 | head -1)
echo "  API 镜像: $NEW_IMAGE"

echo ""
echo "[6/6] 测试封面访问..."
COVER_STATUS=$(curl -s -o /dev/null -w "%{http_code}" http://127.0.0.1:8088/data/covers/10.jpg)
echo "  封面返回状态码: $COVER_STATUS"

echo ""
echo "========================================="
echo "  修复完成!"
echo "========================================="
echo ""
echo "请访问: http://127.0.0.1:8088/"
echo ""
echo "升级内容:"
echo "  1. 修复首页封面不显示 (nginx /data/ 代理配置)"
echo "  2. 修复一键刮削状态查询 (去掉多余的认证要求)"
echo ""
