FROM python:3.10-slim

LABEL maintainer="众乐"
WORKDIR /app

# Install system deps for audio metadata (mutagen)
RUN apt-get update && apt-get install -y --no-install-recommends \
    libasound2-dev \
    && rm -rf /var/lib/apt/lists/*

# Install Python dependencies
COPY backend/requirements.txt ./
RUN pip install --no-cache-dir -r requirements.txt

# Copy backend source
COPY backend/ ./backend/
COPY frontend/dist ./frontend/dist/
COPY start.sh ./

# Create data directory for SQLite
RUN mkdir -p /app/data

# Environment defaults
ENV DATABASE_URL=sqlite+aiosqlite:///./data/zhongyue.db
ENV MUSIC_PATHS=/music
ENV SECRET_KEY=change-me-in-production
ENV DEBUG=false
ENV SCAN_ON_STARTUP=false
ENV SERVER_HOST=0.0.0.0
ENV SERVER_PORT=7777

EXPOSE 7777

ENTRYPOINT ["sh", "/app/start.sh"]
