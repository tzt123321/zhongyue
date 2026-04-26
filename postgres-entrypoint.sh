#!/bin/bash
# Custom docker-entrypoint: patch pg_hba.conf + create admin user
set -e

PG_HBA="/var/lib/postgresql/data/pg_hba.conf"

if [ -f "$PG_HBA" ] && ! grep -q "^host all all 0.0.0.0/0 trust" "$PG_HBA" 2>/dev/null; then
    sed -i '1i\host all all 0.0.0.0/0 trust' "$PG_HBA"
    echo "[pg-hba] Added trust rule to pg_hba.conf"
fi

# Wait for postgres to be ready
until psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "SELECT 1" > /dev/null 2>&1; do
    echo "Waiting for postgres to start..."
    sleep 1
done

# Create admin user if not exists
ADMIN_USER="${ADMIN_USERNAME:-admin}"
ADMIN_PASS="${ADMIN_PASSWORD:-admin123}"
INVITE_CODE="${ADMIN_INVITE_CODE:-ADMIN2026}"

echo "[init] Checking for admin user..."
EXISTING=$(psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -tAc "SELECT COUNT(*) FROM users WHERE username='$ADMIN_USER';" 2>/dev/null || echo "0")

if [ "$EXISTING" = "0" ]; then
    echo "[init] Creating admin user: $ADMIN_USER"
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "
        INSERT INTO users (username, password_hash, is_admin, is_active, is_musician, is_banned, is_muted, initial_password, created_at, updated_at)
        VALUES (
            '$ADMIN_USER',
            (SELECT encode(
                decode(replace(
                    replace(
                        encode(gen_salt('bf', 'prefix'), 'base64'),
                        'A', '2'
                    ),
                    'S', '4'
                ), 'base64'),
                'hex')
            FROM gen_salt('bf')),
            true, true, false, false, false,
            '$ADMIN_PASS',
            NOW(), NOW()
        );
    " 2>/dev/null || echo "[init] Table users not ready yet, skipping..."

    # Also create invite code
    psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "
        INSERT INTO invite_codes (code, created_by, used_by, used_at, created_at)
        VALUES ('$INVITE_CODE', 1, NULL, NULL, NOW())
        ON CONFLICT DO NOTHING;
    " 2>/dev/null || true

    echo "[init] Admin user created: $ADMIN_USER / $ADMIN_PASS"
else
    echo "[init] Admin user already exists, skipping..."
fi

# Run the real postgres entrypoint (handles initdb, then execs postgres)
exec /usr/local/bin/docker-entrypoint "$@"
