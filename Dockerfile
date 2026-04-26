# 众乐 (ZhongYue) — API 服务镜像（Docker 内编译，无需本地 Go 环境）
#
# 构建：docker build -t zhongyue:latest .
# 启动：docker compose up -d
#

FROM golang:1.22-alpine AS builder

WORKDIR /app

# 安装编译工具
RUN apk add --no-cache git

# Go 代理（加速国内下载）
ENV GOPROXY=https://goproxy.cn,direct
ENV CGO_ENABLED=0

# 下载依赖
COPY go.mod go.sum ./
RUN go mod download

# 编译全部三个二进制
COPY . .
RUN go build -ldflags="-w -s" -o zhongyue-api ./cmd/api && \
    go build -ldflags="-w -s" -o stream     ./cmd/stream && \
    go build -ldflags="-w -s" -o migrate    ./cmd/migrate

# ── 运行阶段 ────────────────────────────────────────────────────────────────
FROM alpine:3.19

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata wget curl

# 从 builder 阶段复制编译好的二进制
COPY --from=builder /app/zhongyue-api /app/
COPY --from=builder /app/stream       /app/
COPY --from=builder /app/migrate     /app/

ENV TZ=Asia/Shanghai

# 启动脚本：先迁移数据库，再启动 API
COPY <<'EOF' /entrypoint.sh
#!/bin/sh
set -e
echo "[zhongyue] Running database migrations..."
/app/migrate
echo "[zhongyue] Starting API..."
exec /app/zhongyue-api "$@"
EOF
RUN chmod +x /entrypoint.sh

EXPOSE 7800 8081

ENTRYPOINT ["/entrypoint.sh"]