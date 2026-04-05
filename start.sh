#!/bin/bash
set -e

cd /app/backend

# Create data subdirectories
mkdir -p ./data/covers
mkdir -p ./data/artists
mkdir -p ./data/lyrics

# If database doesn't exist, run migrations
if [ ! -f "./data/harmonybox.db" ]; then
    echo "Fresh database - running migrations..."
    alembic upgrade head
else
    echo "Database exists - checking migrations..."
    # Check if migration is needed (tables might exist from previous run)
    python3 -c "
import asyncio
import aiosqlite

async def check():
    async with aiosqlite.connect('./data/harmonybox.db') as db:
        cursor = await db.execute(\"SELECT name FROM sqlite_master WHERE type='table' AND name='users'\")
        result = await cursor.fetchone()
        if result is None:
            print('NO_USERS')
        else:
            print('USERS_EXIST')

asyncio.run(check())
" > /tmp/db_state 2>&1 || echo "CHECK_FAILED" > /tmp/db_state

    STATE=$(cat /tmp/db_state)
    if [ "$STATE" = "NO_USERS" ] || [ "$STATE" = "CHECK_FAILED" ]; then
        echo "Tables missing - running migrations..."
        alembic upgrade head
    else
        echo "Tables already present - skipping migrations"
    fi
fi

echo "Starting HarmonyBox..."
exec uvicorn app.main:app --host 0.0.0.0 --port 7777
