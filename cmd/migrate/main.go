package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	"golang.org/x/crypto/bcrypt"
	_ "github.com/lib/pq"
)

func main() {
	dbHost := getEnv("DB_HOST", "localhost")
	dbPort := getEnv("DB_PORT", "5432")
	dbUser := getEnv("DB_USER", "zhongyue")
	dbPass := getEnv("DB_PASSWORD", "zhongyue_secret_pass")
	dbName := getEnv("DB_NAME", "zhongyue")

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPass, dbName)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatal("DB open failed:", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatal("DB ping failed:", err)
	}
	log.Println("Connected to PostgreSQL, running migrations...")

	// 从环境变量读取 admin 账号配置，默认 admin/admin123
	adminUser := getEnv("ADMIN_USERNAME", "admin")
	adminPass := getEnv("ADMIN_PASSWORD", "admin123")
	adminInviteCode := getEnv("ADMIN_INVITE_CODE", "ADMIN2026")

	if err := runMigrations(db, adminUser, adminPass, adminInviteCode); err != nil {
		log.Fatal("Migration failed:", err)
	}
	log.Println("Migrations complete!")
}

func runMigrations(db *sql.DB, adminUser, adminPass, adminInviteCode string) error {
	// 生成 bcrypt 密码哈希
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(adminPass), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("bcrypt hash failed: %v", err)
	}
	hashedStr := string(hashedPassword)

	log.Printf("[MIGRATE] Admin user will be upserted: username=%s", adminUser)

	// ── 第一批：建表（无参数，直接执行）───────────────────────────────────
	schemaStmts := []string{
		// ── Core tables ──────────────────────────────────────────────────────
		`CREATE TABLE IF NOT EXISTS users (
			id SERIAL PRIMARY KEY,
			username VARCHAR(50) UNIQUE NOT NULL,
			email VARCHAR(255) UNIQUE,
			password_hash VARCHAR(255) NOT NULL,
			initial_password_hash VARCHAR(255),
			initial_password VARCHAR(64),
			is_admin BOOLEAN NOT NULL DEFAULT FALSE,
			is_musician BOOLEAN NOT NULL DEFAULT FALSE,
			is_active BOOLEAN NOT NULL DEFAULT FALSE,
			is_banned BOOLEAN NOT NULL DEFAULT FALSE,
			is_muted BOOLEAN NOT NULL DEFAULT FALSE,
			invite_code VARCHAR(32),
			api_key VARCHAR(64) UNIQUE,
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS artists (
			id SERIAL PRIMARY KEY,
			name VARCHAR(500) NOT NULL,
			musicbrainz_id VARCHAR(36),
			image_path VARCHAR(1000),
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS albums (
			id SERIAL PRIMARY KEY,
			name VARCHAR(500) NOT NULL,
			artist_id INTEGER REFERENCES artists(id) ON DELETE SET NULL,
			cover_path VARCHAR(1000),
			year INTEGER,
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS tracks (
			id SERIAL PRIMARY KEY,
			title VARCHAR(500) NOT NULL,
			artist_id INTEGER REFERENCES artists(id) ON DELETE SET NULL,
			album_id INTEGER REFERENCES albums(id) ON DELETE SET NULL,
			track_number INTEGER,
			disc_number INTEGER,
			duration INTEGER,
			bitrate INTEGER,
			format VARCHAR(10),
			file_path VARCHAR(1000) UNIQUE NOT NULL,
			file_size BIGINT,
			file_mtime TIMESTAMP,
			play_count INTEGER NOT NULL DEFAULT 0,
			last_played TIMESTAMP,
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS track_artists (
			id SERIAL PRIMARY KEY,
			track_id INTEGER NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
			artist_id INTEGER NOT NULL REFERENCES artists(id) ON DELETE CASCADE,
			role VARCHAR(50) NOT NULL DEFAULT 'artist',
			UNIQUE(track_id, artist_id, role)
		)`,
		`CREATE TABLE IF NOT EXISTS playlists (
			id SERIAL PRIMARY KEY,
			name VARCHAR(500) NOT NULL,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			is_public BOOLEAN NOT NULL DEFAULT TRUE,
			is_system BOOLEAN NOT NULL DEFAULT FALSE,
			description TEXT,
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS playlist_tracks (
			id SERIAL PRIMARY KEY,
			playlist_id INTEGER NOT NULL REFERENCES playlists(id) ON DELETE CASCADE,
			track_id INTEGER NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
			position INTEGER NOT NULL,
			added_at TIMESTAMP NOT NULL DEFAULT NOW(),
			UNIQUE(playlist_id, track_id)
		)`,
		`CREATE TABLE IF NOT EXISTS play_history (
			id SERIAL PRIMARY KEY,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			track_id INTEGER NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
			played_at TIMESTAMP NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS invite_codes (
			id SERIAL PRIMARY KEY,
			code VARCHAR(32) UNIQUE NOT NULL,
			max_uses INTEGER NOT NULL DEFAULT 0,
			uses INTEGER NOT NULL DEFAULT 0,
			used BOOLEAN NOT NULL DEFAULT FALSE,
			created_at TIMESTAMP NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS api_keys (
			id SERIAL PRIMARY KEY,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			key VARCHAR(64) UNIQUE NOT NULL,
			name VARCHAR(100),
			created_at TIMESTAMP NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS playback_state (
			id SERIAL PRIMARY KEY,
			user_id INTEGER UNIQUE NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			track_id INTEGER REFERENCES tracks(id) ON DELETE SET NULL,
			position INTEGER NOT NULL DEFAULT 0,
			updated_at TIMESTAMP NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS effect_presets (
			id SERIAL PRIMARY KEY,
			user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
			name VARCHAR(100) NOT NULL,
			config JSONB NOT NULL DEFAULT '{}',
			is_default BOOLEAN NOT NULL DEFAULT FALSE,
			created_at TIMESTAMP NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS radio_state (
			id SERIAL PRIMARY KEY,
			user_id INTEGER UNIQUE NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			current_track_id INTEGER REFERENCES tracks(id) ON DELETE SET NULL,
			history JSONB NOT NULL DEFAULT '[]',
			liked_tracks JSONB NOT NULL DEFAULT '[]',
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS forum_posts (
			id SERIAL PRIMARY KEY,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			title VARCHAR(500) NOT NULL,
			content TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_forum_posts_user ON forum_posts(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_forum_posts_created ON forum_posts(created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS forum_comments (
			id SERIAL PRIMARY KEY,
			track_id INTEGER NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			content TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS model_configs (
			id SERIAL PRIMARY KEY,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			name VARCHAR(100) NOT NULL,
			provider VARCHAR(50) NOT NULL,
			model_name VARCHAR(100) NOT NULL,
			api_key_encrypted TEXT,
			is_default BOOLEAN NOT NULL DEFAULT FALSE,
			is_enabled BOOLEAN NOT NULL DEFAULT TRUE,
			created_at TIMESTAMP NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS chat_sessions (
			id SERIAL PRIMARY KEY,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			title VARCHAR(255) NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMP NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS chat_messages (
			id SERIAL PRIMARY KEY,
			session_id INTEGER NOT NULL REFERENCES chat_sessions(id) ON DELETE CASCADE,
			role VARCHAR(20) NOT NULL,
			content TEXT NOT NULL,
			music_results TEXT,
			created_at TIMESTAMP NOT NULL DEFAULT NOW()
		)`,

		// ── Indexes ─────────────────────────────────────────────────────────
		`CREATE INDEX IF NOT EXISTS idx_artists_name ON artists(name)`,
		`CREATE INDEX IF NOT EXISTS idx_albums_artist ON albums(artist_id)`,
		`CREATE INDEX IF NOT EXISTS idx_tracks_artist ON tracks(artist_id)`,
		`CREATE INDEX IF NOT EXISTS idx_tracks_album ON tracks(album_id)`,
		`CREATE INDEX IF NOT EXISTS idx_tracks_play_count ON tracks(play_count DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_tracks_created ON tracks(created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_playlists_user ON playlists(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_play_history_user ON play_history(user_id, played_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_play_history_track ON play_history(track_id)`,
		`CREATE INDEX IF NOT EXISTS idx_forum_comments_track ON forum_comments(track_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_api_keys_key ON api_keys(key)`,
		`CREATE INDEX IF NOT EXISTS idx_chat_sessions_user ON chat_sessions(user_id, updated_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_chat_messages_session ON chat_messages(session_id, created_at ASC)`,
	}

	for _, stmt := range schemaStmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("schema migration failed [%s]: %w", truncate(stmt, 60), err)
		}
	}
	log.Println("[MIGRATE] Schema created successfully")

	// ── 第二批：种子数据（参数化查询，安全防注入）────────────────────────

	// Seed: upsert admin user
	seedAdminSQL := `INSERT INTO users (username, password_hash, is_admin, is_active, invite_code, initial_password)
		VALUES ($1, $2, TRUE, TRUE, $3, $4)
		ON CONFLICT (username) DO UPDATE SET
			password_hash = EXCLUDED.password_hash,
			is_admin = EXCLUDED.is_admin,
			is_active = EXCLUDED.is_active,
			initial_password = EXCLUDED.initial_password,
			updated_at = NOW()`
	res, err := db.Exec(seedAdminSQL, adminUser, hashedStr, adminInviteCode, adminPass)
	if err != nil {
		return fmt.Errorf("failed to upsert admin user: %w", err)
	}
	rows, _ := res.RowsAffected()
	log.Printf("[MIGRATE] Admin user upsert complete (rows affected: %d, username: %s)", rows, adminUser)

	// Seed: default invite code
	seedInviteSQL := `INSERT INTO invite_codes (code, max_uses, uses, used)
		SELECT 'VIP2026', 100, 0, FALSE
		WHERE NOT EXISTS (SELECT 1 FROM invite_codes WHERE code = 'VIP2026')`
	if _, err := db.Exec(seedInviteSQL); err != nil {
		log.Printf("[MIGRATE] WARN: default invite code insert failed (may already exist): %v", err)
	} else {
		log.Println("[MIGRATE] Default invite code 'VIP2026' ensured")
	}

	// ── 兼容性列迁移 ────────────────────────────────────────────────────
	alterStmts := []string{
		`ALTER TABLE model_configs ADD COLUMN IF NOT EXISTS is_enabled BOOLEAN NOT NULL DEFAULT TRUE`,
	}
	for _, alter := range alterStmts {
		if _, err := db.Exec(alter); err != nil {
			log.Printf("[MIGRATE] WARN: alter column (%s): %v (OK if already exists)", truncate(alter, 50), err)
		}
	}

	return nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
