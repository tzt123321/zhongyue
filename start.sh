#!/bin/bash
# ─────────────────────────────────────────────────────────────────────────────
# 众乐 — 启动脚本（Docker 编译模式，无需本地 Go 环境）
#
# 用法：
#   bash start.sh up        启动全部服务
#   bash start.sh down      停止服务
#   bash start.sh ps        查看状态
#   bash start.sh logs      查看日志
#   bash start.sh rebuild   重新构建镜像
#   bash start.sh clean     删除所有容器和数据
# ─────────────────────────────────────────────────────────────────────────────
set -e
cd "$(dirname "$0")"

GREEN='\033[0;32m'; YELLOW='\033[1;33m'; RED='\033[0;31m'; NC='\033[0m'
log()  { echo -e "${GREEN}[START]${NC} $1"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
err()  { echo -e "${RED}[ERROR]${NC} $1"; }

CMD="${1:-up}"

# 检查 .env
if [ ! -f .env ]; then
    warn ".env not found, copying from .env.example"
    cp .env.example .env
fi

case "$CMD" in
  up)
    log "Building Docker image (if needed)..."
    docker build -t zhongyue:latest . || { err "Docker build failed"; exit 1; }

    log "Starting all services..."
    docker compose up -d

    log "Waiting for services to be healthy..."
    for i in $(seq 1 60); do
        HEALTHY=$(docker compose ps --format json 2>/dev/null | grep -c '"Health":"healthy"' || echo "0")
        RUNNING=$(docker compose ps -q | wc -l)
        echo "  [$i/60] running=$RUNNING healthy=$HEALTHY"
        if [ "$HEALTHY" -ge 3 ] && [ "$RUNNING" -ge 5 ]; then
            log "All services ready!"
            break
        fi
        sleep 3
    done

    echo ""
    log "众乐重构版已启动！"
    echo ""
    echo "访问地址："
    echo "  前端   http://localhost"
    echo "  API    http://localhost:7800"
    echo "  媒体流 http://localhost:8081"
    echo "  ES     http://localhost:9200"
    echo ""
    echo "管理命令："
    echo "  bash start.sh logs     查看日志"
    echo "  bash start.sh ps       查看状态"
    echo "  bash start.sh rebuild   重新构建镜像"
    echo "  bash start.sh clean    删除全部容器和数据"
    ;;

  down)
    log "Stopping all services..."
    docker compose down
    log "All services stopped."
    ;;

  ps)
    docker compose ps
    ;;

  logs)
    docker compose logs -f --tail=50
    ;;

  rebuild)
    log "Rebuilding image..."
    docker build -t zhongyue:latest .
    ;;

  clean)
    echo "This will DELETE all containers and data volumes."
    read -p "Continue? [y/N] " confirm
    [ "$confirm" = "y" ] || [ "$confirm" = "Y" ] || exit 0
    docker compose down -v --remove-orphans
    log "All containers and volumes removed."
    ;;

  *)
    echo "Usage: bash start.sh [up|down|ps|logs|rebuild|clean]"
    ;;
esac