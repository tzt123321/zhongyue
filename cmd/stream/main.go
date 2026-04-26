package main

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/golang-jwt/jwt/v5"
	_ "github.com/lib/pq"
)

var (
	db        *sql.DB
	musicPath  string
	port       = "8081"
	jwtSecret  []byte
)

// JWTClaims mirrors the main API's claim structure
type JWTClaims struct {
	Sub string `json:"sub"`
	jwt.RegisteredClaims
}

// validateToken checks the Authorization: Bearer <token> header.
// Returns the user ID from the 'sub' claim, or 0 if invalid/missing.
func validateToken(r *http.Request) int64 {
	// Try Authorization header first (standard approach)
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		tokenStr := strings.TrimPrefix(auth, "Bearer ")
		if userID := parseToken(tokenStr); userID > 0 {
			return userID
		}
	}
	// Fallback: ?token= query parameter (for HTML5 audio/audio requests)
	tokenStr := r.URL.Query().Get("token")
	if tokenStr != "" {
		if userID := parseToken(tokenStr); userID > 0 {
			return userID
		}
	}
	return 0
}

func parseToken(tokenStr string) int64 {
	claims := &JWTClaims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		return jwtSecret, nil
	})
	if err != nil || !token.Valid {
		return 0
	}
	userID, err := strconv.ParseInt(claims.Sub, 10, 64)
	if err != nil {
		return 0
	}
	return userID
}

func main() {
	musicPath = os.Getenv("MUSIC_PATH")
	if musicPath == "" {
		musicPath = "/music"
	}

	secretKey := os.Getenv("SECRET_KEY")
	if secretKey == "" {
		log.Fatal("SECRET_KEY environment variable is required")
	}
	jwtSecret = []byte(secretKey)

	dbHost := getEnv("DB_HOST", "localhost")
	dbPort := getEnv("DB_PORT", "5432")
	dbUser := getEnv("DB_USER", "zhongyue")
	dbPass := getEnv("DB_PASSWORD", "zhongyue_secret_pass")
	dbName := getEnv("DB_NAME", "zhongyue")

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPass, dbName)
	var err error
	db, err = sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("failed to open DB: %v", err)
	}
	db.SetMaxOpenConns(50)
	db.SetMaxIdleConns(10)
	if err = db.Ping(); err != nil {
		log.Fatalf("failed to ping DB: %v", err)
	}
	log.Println("Stream service: DB connected")

	mux := http.NewServeMux()
	mux.HandleFunc("GET /stream/{id}", handleStream)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"ok"}`))
	})

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // streaming, no timeout
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("stream server error: %v", err)
		}
	}()

	log.Printf("Stream service listening on :%s", port)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
}

func handleStream(w http.ResponseWriter, r *http.Request) {
	// ── JWT Authentication ──────────────────────────────────────────────────
	userID := validateToken(r)
	if userID == 0 {
		ip := r.RemoteAddr
		path := r.URL.Path
		log.Printf("[AUTH] JWT validation failed: ip=%s path=%s", ip, path)
		http.Error(w, `{"error":"unauthorized","message":"valid token required"}`, http.StatusUnauthorized)
		return
	}

	// Get track ID from path
	idStr := r.PathValue("id")
	trackID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid track id", http.StatusBadRequest)
		return
	}

	// Fetch track file path from DB
	var filePath string
	err = db.QueryRow(`SELECT file_path FROM tracks WHERE id = $1`, trackID).Scan(&filePath)
	if err == sql.ErrNoRows {
		http.Error(w, "track not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}

	// Resolve absolute path
	absPath := filePath
	if !filepath.IsAbs(filePath) {
		absPath = filepath.Join(musicPath, filePath)
	}

	// Check file exists
	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "file not found", http.StatusNotFound)
			return
		}
		http.Error(w, "file error", http.StatusInternalServerError)
		return
	}
	fileSize := info.Size()

	// Log play history (fire-and-forget goroutine)
	go logPlayHistory(trackID, r)

	// ── Range request handling ─────────────────────────────────────────────────
	rangeHeader := r.Header.Get("Range")
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Type", "audio/mpeg")

	if rangeHeader == "" {
		// Full file
		w.Header().Set("Content-Length", strconv.FormatInt(fileSize, 10))
		w.WriteHeader(http.StatusOK)
		serveFile(w, r, absPath, 0, fileSize-1)
		return
	}

	// Parse Range header: "bytes=start-end"
	ranges, err := parseRange(rangeHeader, fileSize)
	if err != nil || len(ranges) == 0 {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", fileSize))
		http.Error(w, "invalid range", http.StatusRequestedRangeNotSatisfiable)
		return
	}

	// Use first range only (simplified; multi-range not supported)
	rg := ranges[0]
	contentLength := rg.End - rg.Start + 1

	w.Header().Set("Content-Length", strconv.FormatInt(contentLength, 10))
	w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", rg.Start, rg.End, fileSize))
	w.WriteHeader(http.StatusPartialContent)

	serveFile(w, r, absPath, rg.Start, rg.End)
}

func serveFile(w http.ResponseWriter, r *http.Request, path string, start, end int64) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	f.Seek(start, io.SeekStart)
	remaining := end - start + 1
	buf := make([]byte, 32*1024)
	for remaining > 0 {
		n := int64(len(buf))
		if n > remaining {
			n = remaining
		}
		nn, err := f.Read(buf[:n])
		if nn <= 0 || err != nil {
			break
		}
		if _, werr := w.Write(buf[:nn]); werr != nil {
			break
		}
		remaining -= int64(nn)
	}
}

type byteRange struct {
	Start int64
	End   int64
}

func parseRange(header string, fileSize int64) ([]byteRange, error) {
	// Format: "bytes=start-end" or "bytes=start-" (end omitted means to EOF)
	header = strings.TrimPrefix(header, "bytes=")
	parts := strings.Split(header, "-")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid range")
	}
	start, err1 := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	if err1 != nil {
		return nil, fmt.Errorf("invalid range")
	}
	// If end is empty (e.g. "bytes=0-"), default to fileSize-1
	var end int64
	if strings.TrimSpace(parts[1]) == "" {
		end = fileSize - 1
	} else {
		var err2 error
		end, err2 = strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
		if err2 != nil {
			return nil, fmt.Errorf("invalid range")
		}
	}
	if start > end || start >= fileSize {
		return nil, fmt.Errorf("invalid range")
	}
	if end >= fileSize {
		end = fileSize - 1
	}
	return []byteRange{{Start: start, End: end}}, nil
}

func logPlayHistory(trackID int64, r *http.Request) {
	// Extract user ID from Authorization header if present
	auth := r.Header.Get("Authorization")
	var userID int64
	if strings.HasPrefix(auth, "Bearer ") {
		// In production: validate JWT and extract user ID
		// For now, skip
	}

	if userID > 0 {
		db.Exec(`INSERT INTO play_history (user_id, track_id, played_at) VALUES ($1, $2, NOW())`,
			userID, trackID)
		db.Exec(`UPDATE tracks SET play_count = play_count + 1 WHERE id = $1`, trackID)
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
