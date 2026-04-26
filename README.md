# ZhongYue 音乐平台 — 部署文档

> 项目名：`zhongyue`（部署包会被重命名为此目录）

---

## 目录结构

```
zhongyue/                      ← 项目根目录（改名后）
├── docker-compose.yml         ← 【核心】Docker Compose 编排配置
├── nginx.conf                 ← 【核心】Nginx 反向代理配置（封面/API/前端）
├── .env                       ← 环境变量配置（数据库密码、端口等）
├── .env.example               ← 环境变量模板（新建 .env 时参考）
│
├── Dockerfile                 ← 主程序 Dockerfile（旧/保留）
├── Dockerfile.api             ← API 服务构建文件
├── Dockerfile.stream          ← 流媒体服务构建文件
├── Dockerfile.simple          ← 简化部署 Dockerfile
│
├── zhongyue-api               ← 编译后的 API 可执行文件（用于简单部署）
├── zhongyue-stream            ← 编译后的流媒体可执行文件
│
├── frontend-dist/             ← 【核心】前端编译产物（Nginx 读取）
│   ├── index.html             ← 单页应用入口
│   ├── sw.js                   ← Service Worker（PWA 支持）
│   ├── manifest.json           ← PWA 清单
│   ├── assets/                ← 编译后的 JS/CSS（按组件拆分）
│   │   ├── index-*.js          ← Vue 运行时 + 路由
│   │   ├── Home-*.js           ← 首页组件
│   │   ├── Library-*.js       ← 音乐库组件
│   │   ├── Settings-*.js      ← 设置页组件
│   │   └── ...                ← 其他页面组件
│   ├── music-placeholder*.svg ← 音乐封面占位图
│   └── artist-placeholder*.svg← 艺术家图片占位图
│
├── data/                      ← 【数据持久化】封面/艺术家图片存储
│   ├── covers/                ← 专辑封面（刮削后保存于此）
│   │   ├── 1.jpg              ← 按专辑 ID 命名
│   │   └── *.jpg / *.png
│   └── artists/               ← 艺术家图片（刮削后保存于此）
│
├── music/                     ← 【音乐文件源】扫描后导入数据库
│   └── *.mp3 / *.flac         ← 音乐文件（由 MUSIC_PATH 挂载）
│
├── postgres-entrypoint.sh     ← PostgreSQL 初始化脚本
├── start.sh                   ← 简单部署启动脚本
│
├── go.mod / go.sum            ← Go 模块依赖
├── cmd/                       ← Go 源码（API/Stream/Migrate 等入口）
├── internal/                  ← Go 内部包（handler/model/service 等）
├── migrate/                   ← 数据库迁移工具
└── scripts/                   ← 辅助脚本（标签写入等）
```

---

## 核心文件说明

### docker-compose.yml
整个平台的容器编排文件，定义了 4 个服务：

| 服务 | 镜像 | 端口 | 作用 |
|------|------|------|------|
| `postgres` | postgres:16-alpine | 5432 | 数据库 |
| `api` | zhongyue-api:latest | 7800 | REST API + 业务逻辑 |
| `stream` | zhongyue-stream:latest | 8081 | 音乐流媒体服务 |
| `nginx` | nginx:alpine | 8088 | 反向代理 + 前端静态服务 |

### nginx.conf
Nginx 配置，决定了请求如何路由：

```nginx
location /api/   → proxy_pass → api:7800  (API 请求)
location /data/  → proxy_pass → api:7800  (封面/图片请求)
location /stream/→ proxy_pass → stream:8081 (音乐流)
location /       → root /var/www/dist  (前端静态文件)
```

### .env
环境变量文件，**部署前必须创建**。参考 `.env.example`：

```bash
# 数据库
DB_PASSWORD=change-me          # 修改为强密码
SECRET_KEY=change-me           # JWT 签名密钥

# 端口（可选，默认如下）
HTTP_PORT=8088                 # 前台访问端口
API_PORT=7800                  # API 端口
STREAM_PORT=8081              # 流媒体端口

# 音乐文件路径（可选）
MUSIC_PATH=./music             # 指向音乐文件目录

# 管理员账号
ADMIN_USERNAME=admin
ADMIN_PASSWORD=admin123         # 修改为强密码
ADMIN_INVITE_CODE=ADMIN2026
```

---

## 部署方案

### 方案一：Docker Compose（推荐）

**步骤：**

```powershell
# 1. 将部署包解压到 F:\project\zhongyue\

# 2. 创建 .env 文件
cp .env.example .env
# 编辑 .env，修改 DB_PASSWORD 和 ADMIN_PASSWORD

# 3. 启动所有服务
docker compose up -d

# 4. 查看服务状态
docker compose ps

# 5. 访问前台
# http://127.0.0.1:8088/
```

**首次启动会自动：**
- 创建数据库 schema
- 创建管理员账号
- 初始化 PostgreSQL

**升级（替换 API 镜像）：**
```powershell
docker compose build api
docker compose up -d --force-recreate api
```

**重启 nginx（修改 nginx.conf 后）：**
```powershell
docker compose restart nginx
```

---

### 方案二：简单部署（无 Docker Build）

适用于不想每次都构建镜像的场景：

```powershell
# 1. 安装 PostgreSQL（本地或 Docker）
# 2. 创建数据库和用户
# 3. 修改 .env 中的 DB_HOST 等
# 4. 执行 ./start.sh
```

---

## 目录职责速查

| 操作 | 目标位置 |
|------|---------|
| 修改网站标题/Logo | `frontend-dist/`（需重新编译） |
| 封面图片存放 | `data/covers/` |
| 艺术家图片存放 | `data/artists/` |
| 音乐文件目录 | `music/`（挂载点） |
| 修改 Nginx 路由规则 | `nginx.conf` |
| 修改环境变量/端口 | `.env` |
| 查看日志 | `docker compose logs api` |
| 数据库连接 | postgres:5432 / zhongyue |

---

## 端口说明

| 端口 | 服务 | 外部访问地址 |
|------|------|-------------|
| 8088 | Nginx（前台） | http://127.0.0.1:8088 |
| 7800 | API | http://127.0.0.1:7800（内部） |
| 8081 | Stream | http://127.0.0.1:8081（内部） |
| 5432 | PostgreSQL | localhost:5432（仅本地） |

---

## 构建（Docker Build）

> 如果修改了 Go 源码或需要重新编译 API/Stream，需要执行以下步骤

**构建 API 镜像：**
```powershell
docker compose build api
```

**构建 Stream 镜像：**
```powershell
docker compose build stream
```

**同时构建两者：**
```powershell
docker compose build
```

**构建并强制重建：**
```powershell
docker compose build --no-cache api
docker compose up -d --force-recreate api
```

构建完成后镜像会自动打标签为 `zhongyue-api:latest` 和 `zhongyue-stream:latest`。

---

## 常见问题

**Q: 封面图片显示 404**
A: 检查 `nginx.conf` 是否为 `proxy_pass http://zhongyue_api:7800`（不是 `alias /var/www/data/`）。执行 `docker compose restart nginx`

**Q: 数据库连接失败**
A: 确认 `.env` 里的 `DB_PASSWORD` 与 `docker-compose.yml` 中的 `POSTGRES_PASSWORD` 一致

**Q: 一键刮削只刮一个**
A: `scrape-status` 接口需要 auth token，确认前端发送请求时带有 `Authorization: Bearer <token>` 头

**Q: 首页空白/音乐不显示**
A: 清浏览器缓存；检查 `docker compose logs api` 是否有报错；确认访问的是 `http://127.0.0.1:8088/` 而非 `7800` 端口

---

## 音乐库管理

**手动刮削封面：**
- 在设置页面对单个专辑/艺术家点击刮削按钮

**一键刮削（全库）：**
- 在设置页面点击"🎯 一键刮削全库"
- 会遍历所有封面为空的专辑和艺术家，自动从 Apple Music / 网易云 / QQ音乐 获取

**添加新音乐：**
1. 将 `.mp3` / `.flac` 文件放入 `music/` 目录
2. 在设置页面点击"🔍 扫描音乐"
3. 系统自动解析 ID3 标签并导入数据库

---

## 备份与恢复

**备份数据：**
```powershell
# 备份数据库
docker compose exec postgres pg_dump -U zhongyue zhongyue > backup.sql

# 备份封面图片
Copy-Item -Recurse data/covers backup-covers/
```

**恢复数据：**
```powershell
# 恢复数据库
Get-Content backup.sql | docker compose exec -T postgres psql -U zhongyue zhongyue
```