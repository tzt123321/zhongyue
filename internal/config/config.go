package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	// Server
	ServerPort string

	// Database
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string

	// Redis
	RedisHost string
	RedisPort string

	// Elasticsearch
	ESHost string

	// Security
	SecretKey string

	// Music
	MusicPath string

	// Data (covers, artists, etc.)
	DataPath string

	// Spotify OAuth
	SpotifyClientID     string
	SpotifyClientSecret string

	// Apple Music MusicKit token
	AppleMusicToken string

	// JWT
	JWTExpire time.Duration

	// TZ
	TZ string
}

func Load() *Config {
	return &Config{
		ServerPort: getEnv("SERVER_PORT", "7800"),

		DBHost:     getEnv("DB_HOST", "localhost"),
		DBPort:     getEnv("DB_PORT", "5432"),
		DBUser:     getEnv("DB_USER", "zhongyue"),
		DBPassword: getEnv("DB_PASSWORD", "zhongyue_secret_pass"),
		DBName:     getEnv("DB_NAME", "zhongyue"),

		RedisHost: getEnv("REDIS_HOST", "localhost"),
		RedisPort: getEnv("REDIS_PORT", "6379"),

		ESHost: getEnv("ES_HOST", "http://localhost:9200"),

		SecretKey: getEnv("SECRET_KEY", "change-me-use-strong-random-key"),

		MusicPath: getEnv("MUSIC_PATH", "/music"),

		DataPath:           getEnv("DATA_PATH", "./data"),

		SpotifyClientID:     getEnv("SPOTIFY_CLIENT_ID", ""),
		SpotifyClientSecret: getEnv("SPOTIFY_CLIENT_SECRET", ""),
		AppleMusicToken:     getEnv("APPLE_MUSIC_TOKEN", ""),

		JWTExpire: time.Duration(duration(getEnv("JWT_EXPIRE_MINUTES", "1440"))) * time.Minute,

		TZ: getEnv("TZ", "Asia/Shanghai"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func duration(minutes string) int64 {
	v, _ := strconv.ParseInt(minutes, 10, 64)
	if v <= 0 {
		return 1440
	}
	return v
}
