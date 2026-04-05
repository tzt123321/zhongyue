#看前提要。本次项目的代码部分由人工智能辅助生成。经由本人测试可以部署落地
#谈点想法，我认为在现在这个人工智能兴起，编程不再拘泥于固定的技术路线，而是变为了idea的博弈。知识广博决定低层建筑，想法决定上层建筑。所以本次项目开源不仅仅开源代码，更是开源看法
#看法：随着openclaw的爆火和破圈，越来越多的人拥有了自己的7*24小时主机，无论是云服务器，nas还是mac mini，轻量应用服务器都说明他们拥有了自己的一台可以提供服务的机器。但是机器往往闲置，不如拿出来做点有意义的事情。我尝试了使用calibre-web做电子书阅读器，还是很不错的。也尝试过使用飞牛影视，整体还行就是太吃内存了。直到用到了navidrome。才觉得这个板块依然需要改进。navidrome是一个家庭音乐流媒体服务。但是原生并不支持客户端，使用其他的客户端则普遍不支持内网穿透（本人使用的樱花内网穿透内核好像是frpc）。所以就尝试着做出自己的音乐流媒体服务。但是因为时间缘故，所以项目并不完善，目前更新迭代的版本是V0.9.6沿用的是linux版本号的命名方法。经过几次小版本的迭代和修正。到了目前的模样。首先不得不提的是依然没有做出专属的客户端，因为这涉及到一些环境问题。因为项目将会参加2026年的计算机设计大赛，所以项目的版本在本次比赛结束前就不会动了。但是并不影响我对项目继续迭代的恒心。of course，本次项目参与的协作者并不止我一个人。后续也会开放共创给大家。
# 众乐 - 自托管音乐服务器

🎵 众乐是一款现代化的自托管音乐服务器，提供美观的 Web 界面和完整的 REST API，支持音乐扫描、流媒体播放、专辑封面获取等功能。

## 功能特性

- 🎧 **Web UI** — 现代深色主题界面，支持播放列表管理、搜索、音乐播放
- 📡 **REST API** — 完整的 API 接口，可用于构建原生客户端
- 🔐 **用户认证** — 基于 JWT 的身份验证，支持多用户管理
- 🎵 **音频流媒体** — 支持 HTTP Range 请求（可拖动播放进度）
- 🔍 **全文搜索** — 基于 SQLite FTS5 的即时搜索
- 📀 **元数据获取** — 集成 MusicBrainz + Last.fm，自动获取专辑封面
- 🐳 **Docker 部署** — 一条命令在任意 Linux 服务器上部署

## 支持的音频格式

MP3、FLAC、WAV、M4A、OGG、Opus、AAC

---

## 快速开始

### 环境要求

- Linux 服务器（Ubuntu 20.04+ / Debian 11+ / CentOS 8+）
- Docker 20.10+
- Docker Compose 2.0+
- 1GB+ 可用内存

### 部署步骤

**第一步：安装 Docker（如果服务器上没有）**

```bash
curl -fsSL https://get.docker.com | sh
sudo systemctl enable docker
sudo systemctl start docker
sudo usermod -aG docker $USER
```

**第二步：创建部署目录**

```bash
mkdir -p ~/zhongyue && cd ~/zhongyue
```

**第三步：下载 docker-compose.yml**

```bash
cat > docker-compose.yml << 'EOF'
services:
  zhongyue:
    image: ghcr.io/zhongyue-music/zhongyue:latest
    container_name: zhongyue
    ports:
      - "7777:7777"
    volumes:
      - ./music:/music:ro
      - zhongyue-data:/app/backend/data
    environment:
      - DATABASE_URL=sqlite+aiosqlite:///./data/zhongyue.db
      - MUSIC_PATHS=/music
      - SECRET_KEY=change-me-in-production-use-strong-random-key
      - DEBUG=false
      - SCAN_ON_STARTUP=false
    restart: unless-stopped

volumes:
  zhongyue-data:
EOF
```

**第四步：创建音乐目录**

```bash
mkdir -p music
# 将音乐文件放入 ./music 目录
```

**第五步：启动服务**

```bash
docker compose up -d
```

**第六步：访问**

```
http://你的服务器IP:7777
```

首次登录使用默认账号：`admin` / `admin123`

> ⚠️ 首次部署后请立即修改默认密码！

---

## 从源码构建部署

如果使用预编译镜像拉取困难，可以从源码构建：

```bash
# 克隆项目
git clone https://github.com/zhongyue-music/zhongyue.git
cd zhongyue

# 构建并启动
docker compose up -d --build
```

---

## 目录结构

```
zhongyue/
├── docker-compose.yml    # Docker Compose 配置
├── music/                 # 音乐文件目录（需手动创建）
├── data/                  # 数据库文件（Docker volume 自动管理）
└── README.md
```

---

## 配置说明

### 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `DATABASE_URL` | `sqlite+aiosqlite:///./data/zhongyue.db` | 数据库连接字符串 |
| `MUSIC_PATHS` | `/music` | 音乐文件夹路径 |
| `SECRET_KEY` | `change-me...` | **必须修改为强随机密钥** |
| `DEBUG` | `false` | 调试模式 |
| `SCAN_ON_STARTUP` | `false` | 启动时自动扫描音乐库 |
| `ENABLE_SCRAPING` | `true` | 从 MusicBrainz/Last.fm 获取元数据 |

### 生成强密钥

```bash
openssl rand -hex 32
```

### 修改 SECRE

编辑 `docker-compose.yml`，将 `SECRET_KEY` 值替换为上面命令生成的随机字符串。

### 修改端口

编辑 `docker-compose.yml`：

```yaml
ports:
  - "8080:7777"   # 外部访问 8080
```

### 音乐库扫描

启动后，在 Web UI 中进入「设置」→「扫描」触发音乐库扫描，或设置 `SCAN_ON_STARTUP=true` 自动扫描。

---

## 运维管理

### 常用命令

```bash
# 查看日志
docker compose logs -f

# 重启服务
docker compose restart

# 停止服务
docker compose down

# 更新镜像
docker compose pull
docker compose up -d
```

### 数据持久化

数据库存储在 Docker volume `zhongyue-data` 中，即使删除容器也不会丢失数据。

如需备份数据库：
```bash
docker exec zhongyue sqlite3 ./data/zhongyue.db ".backup /tmp/backup.db"
docker cp zhongyue:/tmp/backup.db ./backup_$(date +%Y%m%d).db
```

### 清理重建

```bash
docker compose down -v   # -v 会删除数据卷，慎用！
docker compose up -d
```

---

## 使用 Nginx 反向代理（可选）

如果需要域名访问和 HTTPS：

```bash
sudo apt install nginx -y
```

创建 `/etc/nginx/sites-available/zhongyue`：

```nginx
server {
    listen 80;
    server_name music.yourdomain.com;

    location / {
        proxy_pass http://127.0.0.1:7777;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location /api/stream/ {
        proxy_pass http://127.0.0.1:7777;
        proxy_buffering off;
        tcp_nodelay on;
    }
}
```

```bash
sudo ln -s /etc/nginx/sites-available/zhongyue /etc/nginx/sites-enabled/
sudo nginx -t
sudo systemctl reload nginx
```

然后使用 certbot 获取 SSL 证书：
```bash
sudo apt install certbot python3-certbot-nginx -y
sudo certbot --nginx -d music.yourdomain.com
```

---

## 健康检查

```bash
curl http://localhost:7777/api/health
```

返回 `{"status":"ok"}` 表示服务正常运行。

---

## API 文档

部署完成后访问：
- Swagger UI: `http://你的服务器:7777/docs`
- ReDoc: `http://你的服务器:7777/redoc`

---

## 常见问题

**Q: 容器启动失败？**
```bash
docker compose logs zhongyue
```

**Q: 音乐文件无法识别？**
- 确保音乐文件格式受支持
- 检查文件权限：`chmod 644 ./music/*`
- 在 Web UI 中手动触发扫描

**Q: 专辑封面不显示？**
- 确认网络畅通（需要访问 MusicBrainz）
- 或手动上传封面

---

## 技术栈

- **后端**：Python 3.10+ / FastAPI / SQLAlchemy 2.0 (async)
- **数据库**：SQLite with FTS5 全文搜索
- **认证**：JWT + bcrypt
- **前端**：Vue 3 / TailwindCSS / Pinia / Howler.js
- **音频**：Mutagen (元数据) / HTTP Range (流媒体)
- **容器化**：Docker / Docker Compose
