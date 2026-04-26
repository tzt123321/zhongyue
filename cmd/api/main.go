package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"
	echomw "github.com/labstack/echo/v4/middleware"
	_ "github.com/lib/pq"

	"zhongyue_refactored/internal/cache"
	"zhongyue_refactored/internal/config"
	"zhongyue_refactored/internal/handler"
	"zhongyue_refactored/internal/middleware"
)

func main() {
	cfg := config.Load()

	// Connect to PostgreSQL
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPassword, cfg.DBName)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("failed to open DB: %v", err)
	}
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		log.Fatalf("failed to ping DB: %v", err)
	}
	log.Println("Connected to PostgreSQL")

	// Initialize Redis
	redisCache := cache.NewRedisCache(cfg.RedisHost + ":" + cfg.RedisPort)
	defer redisCache.Close()
	log.Println("Redis client initialized")

	// Initialize JWT middleware
	jwtMw := middleware.NewJWTMiddleware(cfg.SecretKey, cfg.JWTExpire)

	// Echo instance
	e := echo.New()
	e.HideBanner = true

	// Global middleware
	e.Use(echomw.Logger())
	e.Use(echomw.Recover())
	e.Use(echomw.CORS())
	e.Use(echomw.RequestID())

	// Health check (unauthenticated)
	e.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})

	// Audio streaming proxy → stream service on :8081
	streamClient := &http.Client{Timeout: 0} // no timeout for streaming
	streamProxy := func(c echo.Context) error {
		id := c.Param("id")
		streamURL := "http://localhost:8081/stream/" + id
		req, err := http.NewRequest(c.Request().Method, streamURL, nil)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadGateway, "failed to create request")
		}
		// Forward relevant headers (Range, Authorization, etc.)
		for k, vals := range c.Request().Header {
			for _, v := range vals {
				if k == "Range" || k == "Authorization" || k == "Origin" {
					req.Header.Add(k, v)
				}
			}
		}
		resp, err := streamClient.Do(req)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadGateway, "stream service unavailable")
		}
		defer resp.Body.Close()
		for k, vals := range resp.Header {
			for _, v := range vals {
				c.Response().Header().Add(k, v)
			}
		}
		// Explicitly allow GET and HEAD
		c.Response().Header().Set("Allow", "GET, HEAD, OPTIONS")
		c.Response().WriteHeader(resp.StatusCode)
		if c.Request().Method != "HEAD" {
			_, err = io.Copy(c.Response(), resp.Body)
		}
		return nil
	}
	e.GET("/stream/:id", streamProxy)
	e.HEAD("/stream/:id", streamProxy)

	// System features (public)
	e.GET("/api/system/features", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"version":   "0.9.0-go",
			"music_path": cfg.MusicPath,
			"features": []string{"search", "radio", "effects", "forum", "chat", "ai"},
		})
	})

	// Initialize handlers
	authHandler := &handler.AuthHandler{DB: db, JWTExp: 1440, JWTMw: jwtMw}
	trackHandler := &handler.TrackHandler{DB: db, MusicPath: cfg.MusicPath, Cache: redisCache}
	albumHandler := &handler.AlbumHandler{DB: db}
	artistHandler := &handler.ArtistHandler{DB: db}
	playlistHandler := &handler.PlaylistHandler{DB: db}
	libraryHandler := &handler.LibraryHandler{DB: db, MusicPath: cfg.MusicPath}
	playbackHandler := &handler.PlaybackHandler{DB: db}
	searchHandler := &handler.SearchHandler{DB: db}
	forumHandler := &handler.ForumHandler{DB: db}
	adminHandler := &handler.AdminHandler{DB: db}
	settingsHandler := &handler.SettingsHandler{DB: db}
	effectsHandler := &handler.EffectsHandler{DB: db}
	chatHandler := &handler.ChatHandler{DB: db}
	evaHandler := handler.NewEvaHandler()
	scrapeHandler := &handler.ScrapeHandler{
		DB:                    db,
		DataPath:              cfg.DataPath,
		SpotifyClientID:       cfg.SpotifyClientID,
		SpotifyClientSecret:   cfg.SpotifyClientSecret,
		AppleMusicToken:       cfg.AppleMusicToken,
	}

	// Auth routes
	auth := e.Group("/api/auth")
	auth.POST("/login", authHandler.Login)
	auth.POST("/register", authHandler.Register)
	auth.GET("/me", authHandler.Me, authRequired(jwtMw, db))
	auth.POST("/change-password", authHandler.ChangePassword, authRequired(jwtMw, db))
	auth.POST("/generate-key", authHandler.GenerateAPIKey, authRequired(jwtMw, db))

	// Track routes
	tracks := e.Group("/api/tracks")
	tracks.GET("", trackHandler.List)
	tracks.GET("/recent", trackHandler.Recent)
	tracks.GET("/popular", trackHandler.Popular)
	tracks.GET("/recommend", trackHandler.DailyRecommend)
	tracks.GET("/random-recommend", trackHandler.RandomRecommend)
	tracks.GET("/ai-recommend", trackHandler.AIRecommend, authRequired(jwtMw, db))
	tracks.GET("/personalized", trackHandler.Personalized, authRequired(jwtMw, db))
	tracks.GET("/:id", trackHandler.Get)
	tracks.DELETE("/:id", trackHandler.Delete, authRequired(jwtMw, db))

	// Album routes
	e.GET("/api/albums", albumHandler.List)
	e.GET("/api/albums/:id", albumHandler.Get)
	e.GET("/api/albums/:id/tracks", albumHandler.Tracks)

	// Artist routes
	e.GET("/api/artists", artistHandler.List)
	e.GET("/api/artists/:id", artistHandler.Get)
	e.GET("/api/artists/:id/albums", artistHandler.Albums)
	e.GET("/api/artists/:id/tracks", artistHandler.Tracks)

	// Playlist routes (auth required)
	playlists := e.Group("/api/playlists", authRequired(jwtMw, db))
	playlists.GET("", playlistHandler.List)
	playlists.POST("", playlistHandler.Create)
	playlists.GET("/:id", playlistHandler.Get)
	playlists.PUT("/:id", playlistHandler.Update)
	playlists.DELETE("/:id", playlistHandler.Delete)
	playlists.POST("/:id/tracks", playlistHandler.AddTracks)
	playlists.DELETE("/:id/tracks/:track_id", playlistHandler.RemoveTrack)
	playlists.GET("/history/tracks", playlistHandler.HistoryTracks)

	// Library / Stats routes
	e.GET("/api/stats", libraryHandler.Stats)
	e.POST("/api/library/scan", libraryHandler.Scan, authRequired(jwtMw, db))
	e.POST("/api/library/scrape-all", scrapeHandler.ScrapeAll, authRequired(jwtMw, db))
	e.GET("/api/library/scrape-status", scrapeHandler.ScrapeStatus, authRequired(jwtMw, db))

	// Playback routes (auth required)
	playback := e.Group("/api/playback", authRequired(jwtMw, db))
	playback.GET("/history", playbackHandler.History)
	playback.POST("/state", playbackHandler.SaveState)

	// Search route
	e.GET("/api/search", searchHandler.Search)

	// ── Forum routes ────────────────────────────────────────────────────────
	// Public: list posts
	e.GET("/api/forum/posts", forumHandler.ListPosts)
	// Auth required: create post
	e.POST("/api/forum/posts", forumHandler.CreatePost, authRequired(jwtMw, db))
	e.DELETE("/api/forum/posts/:id", forumHandler.DeletePost, authRequired(jwtMw, db))
	e.GET("/api/forum/posts/:id/comments", forumHandler.ListComments)
	e.POST("/api/forum/posts/:id/comments", forumHandler.CreateComment, authRequired(jwtMw, db))
	e.DELETE("/api/forum/comments/:id", forumHandler.DeleteComment, authRequired(jwtMw, db))
	e.GET("/api/forum/notifications", forumHandler.GetNotifications, authRequired(jwtMw, db))
	e.PUT("/api/forum/notifications/:id/read", forumHandler.MarkRead, authRequired(jwtMw, db))
	e.PUT("/api/forum/notifications/read-all", forumHandler.MarkAllRead, authRequired(jwtMw, db))
	e.POST("/api/forum/admin/announcements", forumHandler.CreateAnnouncement, authRequired(jwtMw, db))

	// ── Admin routes ───────────────────────────────────────────────────────
	admin := e.Group("/api/admin", authRequired(jwtMw, db))
	admin.GET("/users", adminHandler.ListUsers)
	admin.PATCH("/users/:id", adminHandler.UpdateUser)
	admin.DELETE("/users/:id", adminHandler.DeleteUser)
	admin.PUT("/users/:id/mute", adminHandler.ToggleMute)
	admin.POST("/users/:id/reset-password", adminHandler.ResetPassword)
	admin.GET("/invite-codes", adminHandler.ListInviteCodes)
	admin.POST("/invite-codes", adminHandler.CreateInviteCode)
	admin.DELETE("/invite-codes/:id", adminHandler.DeleteInviteCode)
	admin.GET("/system/info", settingsHandler.GetSystemInfo)

	// ── Settings / Model config routes ────────────────────────────────────
	settings := e.Group("/api/settings", authRequired(jwtMw, db))
	settings.GET("/models", settingsHandler.ListModels)
	settings.POST("/models", settingsHandler.CreateModel)
	settings.GET("/models/test", settingsHandler.TestModel)
	settings.PATCH("/models/:id/default", settingsHandler.SetDefault)
	settings.PATCH("/models/:id/enable", settingsHandler.ToggleEnable)
	settings.DELETE("/models/:id", settingsHandler.DeleteModel)

	// ── Eva profile routes ─────────────────────────────────────────────────
	e.GET("/api/eva/profile", evaHandler.GetProfile, authRequired(jwtMw, db))

	// ── Chat routes ────────────────────────────────────────────────────────
	chat := e.Group("/api/chat", authRequired(jwtMw, db))
	chat.GET("/history", chatHandler.ListSessions)
	chat.POST("/send", chatHandler.Send)
	chat.GET("/session/:id", chatHandler.GetSession)
	chat.DELETE("/session/:id", chatHandler.DeleteSession)

	// Effects preset routes
	effects := e.Group("/api/effects")
	effects.GET("/presets", effectsHandler.ListPresets, authRequired(jwtMw, db))
	effects.POST("/presets", effectsHandler.CreatePreset, authRequired(jwtMw, db))
	effects.PUT("/presets/:id", effectsHandler.UpdatePreset, authRequired(jwtMw, db))
	effects.DELETE("/presets/:id", effectsHandler.DeletePreset, authRequired(jwtMw, db))

	// Scrape routes (metadata scraping)
	scrape := e.Group("/api/scrape")
	scrape.POST("/album/:id", scrapeHandler.ScrapeAlbum, authRequired(jwtMw, db))
	scrape.POST("/artist/:id", scrapeHandler.ScrapeArtist, authRequired(jwtMw, db))

	// ── Radio routes (stub — returns empty stations for now) ──────────────
	radio := e.Group("/api/radio", authRequired(jwtMw, db))
	radio.GET("/list", func(c echo.Context) error {
		return c.JSON(http.StatusOK, []interface{}{})
	})
	radio.POST("/stations", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]interface{}{"id": 1, "title": "stub"})
	})
	radio.POST("/join/:id", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]interface{}{})
	})
	radio.POST("/leave/:id", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]interface{}{})
	})

	// ── Frontend static files (Vue SPA) ────────────────────────────────────
	frontendDist := "./frontend-dist"
	// Serve assets (JS/CSS/images) from /assets/
	e.GET("/assets/*", echo.WrapHandler(http.StripPrefix("/assets/", http.FileServer(http.Dir(frontendDist+"/assets")))))
	// Serve static files (placeholders, etc.)
	e.GET("/music-placeholder*", echo.WrapHandler(http.FileServer(http.Dir(frontendDist))))
	e.GET("/artist-placeholder*", echo.WrapHandler(http.FileServer(http.Dir(frontendDist))))
	e.GET("/favicon.svg", echo.WrapHandler(http.FileServer(http.Dir(frontendDist))))
	e.GET("/manifest.json", echo.WrapHandler(http.FileServer(http.Dir(frontendDist))))
	// Serve scraped data (covers, artist images) from ./data/
	e.GET("/data/*", echo.WrapHandler(http.StripPrefix("/data/", http.FileServer(http.Dir(cfg.DataPath)))))
	// SPA fallback: all non-API routes → index.html
	e.GET("/*", func(c echo.Context) error {
		p := filepath.Join(frontendDist, c.Param("_"))
		return c.File(p)
	})

	// Start server
	srv := &http.Server{
		Addr:         ":" + cfg.ServerPort,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown
	go func() {
		if err := e.StartServer(srv); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	log.Printf("API server listening on :%s", cfg.ServerPort)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := e.Shutdown(ctx); err != nil {
		log.Fatal(err)
	}
	log.Println("Server shutdown")
}

// authRequired returns an auth middleware that validates JWT and sets user_id in context
func authRequired(jwtMw *middleware.JWTMiddleware, db *sql.DB) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			auth := c.Request().Header.Get("Authorization")
			if len(auth) < 7 || auth[:7] != "Bearer " {
				return echo.NewHTTPError(http.StatusUnauthorized, "missing or invalid authorization header")
			}

			token := auth[7:]
			claims, err := jwtMw.ValidateToken(token)
			if err != nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid token")
			}

			// Parse user ID from sub (numeric string = ID, else username)
			userID := parseUserID(claims.Sub)
			if userID == 0 && claims.Sub != "" {
				// DB lookup by username
				db.QueryRow(`SELECT id FROM users WHERE username = $1`, claims.Sub).Scan(&userID)
			}
			if userID == 0 {
				return echo.NewHTTPError(http.StatusUnauthorized, "user not found")
			}
			c.Set("user_id", userID)
			return next(c)
		}
	}
}

func parseUserID(sub string) int64 {
	id, err := strconv.ParseInt(sub, 10, 64)
	if err == nil {
		return id
	}
	return 0 // username format
}