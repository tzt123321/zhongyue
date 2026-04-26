package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
	"zhongyue_refactored/internal/cache"
	"zhongyue_refactored/internal/model"
)

type TrackHandler struct {
	DB        *sql.DB
	MusicPath string
	Cache     *cache.RedisCache
}

type TrackListQuery struct {
	Skip   int    `query:"skip"`
	Limit  int    `query:"limit"`
	Sort   string `query:"sort"`
	Order  string `query:"order"`
	Search string `query:"search"`
}

// GET /api/tracks
func (h *TrackHandler) List(c echo.Context) error {
	var q TrackListQuery
	if err := c.Bind(&q); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid query")
	}
	if q.Limit <= 0 || q.Limit > 200 {
		q.Limit = 50
	}
	if q.Skip < 0 {
		q.Skip = 0
	}
	if q.Sort == "" {
		q.Sort = "id"
	}
	if q.Order == "" {
		q.Order = "desc"
	}
	order := "DESC"
	if q.Order == "asc" {
		order = "ASC"
	}

	// Build query with optional search
	args := []interface{}{}
	argIdx := 1

	baseSelect := `SELECT t.id, t.title, t.artist_id, t.album_id, t.track_number,
		t.disc_number, t.duration, t.bitrate, t.format, t.file_path, t.file_size,
		t.file_mtime, t.play_count, t.last_played, t.created_at, t.updated_at,
		a.id, a.name, al.id, al.name, al.cover_path
		FROM tracks t
		LEFT JOIN artists a ON t.artist_id = a.id
		LEFT JOIN albums al ON t.album_id = al.id`

	countSelect := `SELECT COUNT(t.id) FROM tracks t LEFT JOIN artists a ON t.artist_id = a.id`

	whereClause := ""
	if q.Search != "" {
		whereClause = fmt.Sprintf(` WHERE t.title ILIKE $%d OR a.name ILIKE $%d`, argIdx, argIdx)
		args = append(args, "%"+q.Search+"%")
		argIdx++
	}

	// Count total
	var total int64
	countQuery := countSelect + whereClause
	if err := h.DB.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Fetch items
	limitOffset := fmt.Sprintf(` ORDER BY t.%s %s OFFSET $%d LIMIT $%d`, q.Sort, order, argIdx, argIdx+1)
	args = append(args, q.Skip, q.Limit)
	rows, err := h.DB.Query(baseSelect+whereClause+limitOffset, args...)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	defer rows.Close()

	items := []model.TrackResponse{}
	for rows.Next() {
		var t model.Track
		var artist model.Artist
		var album model.Album
		var artistID, albumID sql.NullInt64
		var artistName sql.NullString
		var albumName, albumCover sql.NullString

		err := rows.Scan(
			&t.ID, &t.Title, &t.ArtistID, &t.AlbumID, &t.TrackNumber,
			&t.DiscNumber, &t.Duration, &t.Bitrate, &t.Format, &t.FilePath, &t.FileSize,
			&t.FileMtime, &t.PlayCount, &t.LastPlayed, &t.CreatedAt, &t.UpdatedAt,
			&artistID, &artistName, &albumID, &albumName, &albumCover,
		)
		if err != nil {
			continue
		}
		if artistID.Valid {
			artist.ID = artistID.Int64
			artist.Name = artistName.String
			t.Artist = &artist
		}
		if albumID.Valid {
			album.ID = albumID.Int64
			album.Name = albumName.String
			album.CoverPath = albumCover
			t.Album = &album
		}
		items = append(items, t.ToResponse())
	}

	return c.JSON(http.StatusOK, model.TrackListResponse{Total: total, Items: items})
}

// GET /api/tracks/recent
func (h *TrackHandler) Recent(c echo.Context) error {
	limit := parseInt(c.QueryParam("limit"), 20)
	if limit <= 0 || limit > 50 {
		limit = 20
	}

	rows, err := h.DB.Query(
		`SELECT t.id, t.title, t.artist_id, t.album_id, t.track_number,
		 t.disc_number, t.duration, t.bitrate, t.format, t.file_path, t.file_size,
		 t.file_mtime, t.play_count, t.last_played, t.created_at, t.updated_at,
		 ar.id, COALESCE(ar.name, ''), al.id, COALESCE(al.name, ''), al.cover_path
		 FROM tracks t
		 LEFT JOIN artists ar ON t.artist_id = ar.id
		 LEFT JOIN albums al ON t.album_id = al.id
		 ORDER BY t.created_at DESC LIMIT $1`, limit)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	defer rows.Close()

	items := []model.TrackResponse{}
	for rows.Next() {
		var t model.Track
		var artistID, albumID sql.NullInt64
		var artistName, albumName, albumCover sql.NullString
		if err := rows.Scan(&t.ID, &t.Title, &t.ArtistID, &t.AlbumID, &t.TrackNumber,
			&t.DiscNumber, &t.Duration, &t.Bitrate, &t.Format, &t.FilePath, &t.FileSize,
			&t.FileMtime, &t.PlayCount, &t.LastPlayed, &t.CreatedAt, &t.UpdatedAt,
			&artistID, &artistName, &albumID, &albumName, &albumCover); err != nil {
			continue
		}
		if artistID.Valid {
			t.Artist = &model.Artist{ID: artistID.Int64, Name: artistName.String}
		}
		if albumID.Valid {
			t.Album = &model.Album{ID: albumID.Int64, Name: albumName.String, CoverPath: albumCover}
		}
		items = append(items, t.ToResponse())
	}
	return c.JSON(http.StatusOK, model.TrackListResponse{Total: int64(len(items)), Items: items})
}

// GET /api/tracks/popular
func (h *TrackHandler) Popular(c echo.Context) error {
	limit := parseInt(c.QueryParam("limit"), 20)
	if limit <= 0 || limit > 50 {
		limit = 20
	}

	rows, err := h.DB.Query(
		`SELECT t.id, t.title, t.artist_id, t.album_id, t.track_number,
		 t.disc_number, t.duration, t.bitrate, t.format, t.file_path, t.file_size,
		 t.file_mtime, t.play_count, t.last_played, t.created_at, t.updated_at,
		 ar.id, COALESCE(ar.name, ''), al.id, COALESCE(al.name, ''), al.cover_path
		 FROM tracks t
		 LEFT JOIN artists ar ON t.artist_id = ar.id
		 LEFT JOIN albums al ON t.album_id = al.id
		 WHERE t.play_count > 0 ORDER BY t.play_count DESC LIMIT $1`, limit)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	defer rows.Close()

	items := []model.TrackResponse{}
	for rows.Next() {
		var t model.Track
		var artistID, albumID sql.NullInt64
		var artistName, albumName, albumCover sql.NullString
		if err := rows.Scan(&t.ID, &t.Title, &t.ArtistID, &t.AlbumID, &t.TrackNumber,
			&t.DiscNumber, &t.Duration, &t.Bitrate, &t.Format, &t.FilePath, &t.FileSize,
			&t.FileMtime, &t.PlayCount, &t.LastPlayed, &t.CreatedAt, &t.UpdatedAt,
			&artistID, &artistName, &albumID, &albumName, &albumCover); err != nil {
			continue
		}
		if artistID.Valid {
			t.Artist = &model.Artist{ID: artistID.Int64, Name: artistName.String}
		}
		if albumID.Valid {
			t.Album = &model.Album{ID: albumID.Int64, Name: albumName.String, CoverPath: albumCover}
		}
		items = append(items, t.ToResponse())
	}
	return c.JSON(http.StatusOK, model.TrackListResponse{Total: int64(len(items)), Items: items})
}

// GET /api/tracks/recommend
func (h *TrackHandler) DailyRecommend(c echo.Context) error {
	ctx := c.Request().Context()
	today := time.Now().UTC().Format("2006-01-02")
	cacheKey := "zhongyue:daily:recommend:" + today

	// Try Redis cache first
	if cached := h.getFromCache(ctx, cacheKey); cached != "" {
		var track model.Track
		if err := json.Unmarshal([]byte(cached), &track); err == nil {
			return c.JSON(http.StatusOK, track.ToResponse())
		}
	}

	// Compute today's recommended track (hash-based deterministic random)
	todayBytes := []byte(today)
	seed := int64(0)
	for _, b := range todayBytes {
		seed = seed*31 + int64(b)
	}

	// Prefer tracks with album covers; fall back to all tracks
	var coveredTotal, total int64
	h.DB.QueryRowContext(ctx,
		`SELECT COUNT(t.id) FROM tracks t JOIN albums al ON t.album_id = al.id WHERE COALESCE(al.cover_path, '') != ''`,
	).Scan(&coveredTotal)
	h.DB.QueryRowContext(ctx, `SELECT COUNT(id) FROM tracks`).Scan(&total)
	if total == 0 {
		return echo.NewHTTPError(http.StatusNotFound, "no tracks available")
	}

	poolTotal := coveredTotal
	if poolTotal == 0 {
		poolTotal = total
	}
	offsetIdx := int(seed % poolTotal)

	var t model.Track
	var artistID, albumID sql.NullInt64
	var artistName, albumName, albumCover sql.NullString
	var err error

	if coveredTotal > 0 {
		// Prefer tracks with covers
		err = h.DB.QueryRowContext(ctx,
			`SELECT t.id, t.title, t.artist_id, t.album_id, t.track_number,
			 t.disc_number, t.duration, t.bitrate, t.format, t.file_path, t.file_size,
			 t.file_mtime, t.play_count, t.last_played, t.created_at, t.updated_at,
			 ar.id, COALESCE(ar.name, ''), al.id, COALESCE(al.name, ''), al.cover_path
			 FROM tracks t
			 JOIN albums al ON t.album_id = al.id
			 LEFT JOIN artists ar ON t.artist_id = ar.id
			 WHERE COALESCE(al.cover_path, '') != ''
			 ORDER BY t.id OFFSET $1 LIMIT 1`, offsetIdx,
		).Scan(&t.ID, &t.Title, &t.ArtistID, &t.AlbumID, &t.TrackNumber,
			&t.DiscNumber, &t.Duration, &t.Bitrate, &t.Format, &t.FilePath, &t.FileSize,
			&t.FileMtime, &t.PlayCount, &t.LastPlayed, &t.CreatedAt, &t.UpdatedAt,
			&artistID, &artistName, &albumID, &albumName, &albumCover)
	} else {
		// Fall back to all tracks
		err = h.DB.QueryRowContext(ctx,
			`SELECT t.id, t.title, t.artist_id, t.album_id, t.track_number,
			 t.disc_number, t.duration, t.bitrate, t.format, t.file_path, t.file_size,
			 t.file_mtime, t.play_count, t.last_played, t.created_at, t.updated_at,
			 ar.id, COALESCE(ar.name, ''), al.id, COALESCE(al.name, ''), al.cover_path
			 FROM tracks t
			 LEFT JOIN artists ar ON t.artist_id = ar.id
			 LEFT JOIN albums al ON t.album_id = al.id
			 ORDER BY t.id OFFSET $1 LIMIT 1`, offsetIdx,
		).Scan(&t.ID, &t.Title, &t.ArtistID, &t.AlbumID, &t.TrackNumber,
			&t.DiscNumber, &t.Duration, &t.Bitrate, &t.Format, &t.FilePath, &t.FileSize,
			&t.FileMtime, &t.PlayCount, &t.LastPlayed, &t.CreatedAt, &t.UpdatedAt,
			&artistID, &artistName, &albumID, &albumName, &albumCover)
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "no tracks available")
	}
	if artistID.Valid {
		t.Artist = &model.Artist{ID: artistID.Int64, Name: artistName.String}
	}
	if albumID.Valid {
		t.Album = &model.Album{ID: albumID.Int64, Name: albumName.String, CoverPath: albumCover}
	}

	// Cache for 24h
	if data, err := json.Marshal(t); err == nil {
		h.setInCache(ctx, cacheKey, string(data), 86400)
	}

	return c.JSON(http.StatusOK, t.ToResponse())
}

// GET /api/tracks/random-recommend
func (h *TrackHandler) RandomRecommend(c echo.Context) error {
	excludeID := parseInt(c.QueryParam("exclude_id"), 0)

	var total int64
	if excludeID > 0 {
		h.DB.QueryRow(`SELECT COUNT(id) FROM tracks WHERE id != $1`, excludeID).Scan(&total)
	} else {
		h.DB.QueryRow(`SELECT COUNT(id) FROM tracks`).Scan(&total)
	}
	if total == 0 {
		return echo.NewHTTPError(http.StatusNotFound, "no tracks available")
	}

	offset := rand.Int63n(total)
	var t model.Track
	var artistID, albumID sql.NullInt64
	var artistName, albumName, albumCover sql.NullString

	baseQuery := `SELECT t.id, t.title, t.artist_id, t.album_id, t.track_number,
		 t.disc_number, t.duration, t.bitrate, t.format, t.file_path, t.file_size,
		 t.file_mtime, t.play_count, t.last_played, t.created_at, t.updated_at,
		 ar.id, COALESCE(ar.name, ''), al.id, COALESCE(al.name, ''), al.cover_path
		 FROM tracks t
		 LEFT JOIN artists ar ON t.artist_id = ar.id
		 LEFT JOIN albums al ON t.album_id = al.id`

	var err error
	if excludeID > 0 {
		err = h.DB.QueryRow(baseQuery+` WHERE t.id != $1 ORDER BY t.id OFFSET $2 LIMIT 1`, excludeID, offset).Scan(
			&t.ID, &t.Title, &t.ArtistID, &t.AlbumID, &t.TrackNumber,
			&t.DiscNumber, &t.Duration, &t.Bitrate, &t.Format, &t.FilePath, &t.FileSize,
			&t.FileMtime, &t.PlayCount, &t.LastPlayed, &t.CreatedAt, &t.UpdatedAt,
			&artistID, &artistName, &albumID, &albumName, &albumCover)
	} else {
		err = h.DB.QueryRow(baseQuery+` ORDER BY t.id OFFSET $1 LIMIT 1`, offset).Scan(
			&t.ID, &t.Title, &t.ArtistID, &t.AlbumID, &t.TrackNumber,
			&t.DiscNumber, &t.Duration, &t.Bitrate, &t.Format, &t.FilePath, &t.FileSize,
			&t.FileMtime, &t.PlayCount, &t.LastPlayed, &t.CreatedAt, &t.UpdatedAt,
			&artistID, &artistName, &albumID, &albumName, &albumCover)
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "no tracks available")
	}
	if artistID.Valid {
		t.Artist = &model.Artist{ID: artistID.Int64, Name: artistName.String}
	}
	if albumID.Valid {
		t.Album = &model.Album{ID: albumID.Int64, Name: albumName.String, CoverPath: albumCover}
	}

	return c.JSON(http.StatusOK, t.ToResponse())
}

// GET /api/tracks/:id
func (h *TrackHandler) Get(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid track id")
	}

	var t model.Track
	var artistID, albumID sql.NullInt64
	var artistName, albumName, albumCover sql.NullString

	err = h.DB.QueryRow(
		`SELECT t.id, t.title, t.artist_id, t.album_id, t.track_number,
		 t.disc_number, t.duration, t.bitrate, t.format, t.file_path, t.file_size,
		 t.file_mtime, t.play_count, t.last_played, t.created_at, t.updated_at,
		 t.artist_id, a.name, al.id, al.name, al.cover_path
		 FROM tracks t
		 LEFT JOIN artists a ON t.artist_id = a.id
		 LEFT JOIN albums al ON t.album_id = al.id
		 WHERE t.id = $1`, id,
	).Scan(
		&t.ID, &t.Title, &t.ArtistID, &t.AlbumID, &t.TrackNumber,
		&t.DiscNumber, &t.Duration, &t.Bitrate, &t.Format, &t.FilePath, &t.FileSize,
		&t.FileMtime, &t.PlayCount, &t.LastPlayed, &t.CreatedAt, &t.UpdatedAt,
		&artistID, &artistName, &albumID, &albumName, &albumCover,
	)
	if err == sql.ErrNoRows {
		return echo.NewHTTPError(http.StatusNotFound, "track not found")
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	if artistID.Valid {
		t.Artist = &model.Artist{ID: artistID.Int64, Name: artistName.String}
	}
	if albumID.Valid {
		t.Album = &model.Album{ID: albumID.Int64, Name: albumName.String, CoverPath: albumCover}
	}

	return c.JSON(http.StatusOK, t.ToResponse())
}

// DELETE /api/tracks/:id
func (h *TrackHandler) Delete(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid track id")
	}

	result, err := h.DB.Exec(`DELETE FROM tracks WHERE id = $1`, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return echo.NewHTTPError(http.StatusNotFound, "track not found")
	}

	return c.NoContent(http.StatusNoContent)
}

// ── Cache helpers (Redis) ─────────────────────────────────────────────────────

func (h *TrackHandler) getFromCache(ctx context.Context, key string) string {
	if h.Cache == nil {
		return ""
	}
	type result struct {
		data string
		err  error
	}
	done := make(chan result, 1)
	go func() {
		data, err := h.Cache.Get(ctx, key)
		done <- result{data, err}
	}()
	select {
	case r := <-done:
		if r.err != nil || r.data == "" {
			return ""
		}
		return r.data
	case <-time.After(100 * time.Millisecond):
		return ""
	}
}

func (h *TrackHandler) setInCache(ctx context.Context, key, value string, ttlSeconds int) {
	if h.Cache == nil {
		return
	}
	h.Cache.Set(ctx, key, value, time.Duration(ttlSeconds)*time.Second)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// GET /api/tracks/ai-recommend — AI-based track recommendation from play history
func (h *TrackHandler) AIRecommend(c echo.Context) error {
	userID, ok := c.Get("user_id").(int64)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	ctx := c.Request().Context()

	rows, err := h.DB.QueryContext(ctx, `
		SELECT t.id, t.title, COALESCE(a.name,'') as artist_name, COALESCE(al.name,'') as album_name
		FROM tracks t
		LEFT JOIN artists a ON t.artist_id = a.id
		LEFT JOIN albums al ON t.album_id = al.id
		JOIN play_history ph ON ph.track_id = t.id
		WHERE ph.user_id = $1
		GROUP BY t.id, a.name, al.name
		ORDER BY COUNT(ph.id) DESC
		LIMIT 20`, userID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	defer rows.Close()

	var trackList []string
	for rows.Next() {
		var id int64
		var title, artist, album string
		if err := rows.Scan(&id, &title, &artist, &album); err == nil {
			trackList = append(trackList, fmt.Sprintf("%s - %s (%s)", title, artist, album))
		}
	}

	if len(trackList) == 0 {
		return h.RandomRecommend(c)
	}

	var provider, modelName, apiKey string
	err = h.DB.QueryRowContext(ctx,
		`SELECT provider, model_name, api_key_encrypted FROM model_configs
		 WHERE user_id = $1 AND is_default = true LIMIT 1`, userID).Scan(&provider, &modelName, &apiKey)
	if err != nil {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"message": "no model config found, returning top tracks",
			"tracks":  trackList[:len(trackList)/2],
		})
	}

	prompt := "Based on these songs the user enjoys: " + strings.Join(trackList, "; ") +
		". Recommend 5 similar songs I might like. Respond with ONLY a JSON array of song names, nothing else."

	respText, err := h.callLLM(ctx, provider, modelName, apiKey, prompt)
	if err != nil || respText == "" {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"message": "LLM call failed, returning top tracks",
			"tracks":  trackList[:len(trackList)/2],
		})
	}

	var recommendedNames []string
	start := strings.Index(respText, "[")
	end := strings.LastIndex(respText, "]")
	if start != -1 && end != -1 {
		jsonStr := respText[start : end+1]
		if err := json.Unmarshal([]byte(jsonStr), &recommendedNames); err != nil {
			recommendedNames = []string{respText}
		}
	} else {
		recommendedNames = []string{respText}
	}

	var trackIDs []int64
	for _, name := range recommendedNames {
		var id int64
		clean := strings.Split(name, " (")[0]
		h.DB.QueryRowContext(ctx,
			`SELECT id FROM tracks WHERE LOWER(title) LIKE LOWER($1) LIMIT 1`, "%"+clean+"%").Scan(&id)
		if id > 0 {
			trackIDs = append(trackIDs, id)
		}
	}

	if len(trackIDs) == 0 {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"message":      "no matches found",
			"raw_response": respText,
		})
	}

	rows2, err := h.DB.QueryContext(ctx,
		`SELECT t.id, t.title, t.artist_id, t.album_id, t.track_number, t.disc_number,
		 t.duration, t.bitrate, t.format, t.file_path, t.file_size, t.file_mtime,
		 t.play_count, t.last_played, t.created_at, t.updated_at,
		 ar.id, ar.name, al.id, al.name, al.cover_path
		 FROM tracks t
		 LEFT JOIN artists ar ON t.artist_id = ar.id
		 LEFT JOIN albums al ON t.album_id = al.id
		 WHERE t.id = ANY($1)`, pq.Array(trackIDs))
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	defer rows2.Close()

	items := []model.TrackResponse{}
	for rows2.Next() {
		var t model.Track
		var artistID, albumID sql.NullInt64
		var artistName, albumName, albumCover sql.NullString
		if err := rows2.Scan(
			&t.ID, &t.Title, &t.ArtistID, &t.AlbumID, &t.TrackNumber,
			&t.DiscNumber, &t.Duration, &t.Bitrate, &t.Format, &t.FilePath, &t.FileSize,
			&t.FileMtime, &t.PlayCount, &t.LastPlayed, &t.CreatedAt, &t.UpdatedAt,
			&artistID, &artistName, &albumID, &albumName, &albumCover,
		); err == nil {
			if artistID.Valid {
				t.Artist = &model.Artist{ID: artistID.Int64, Name: artistName.String}
			}
			if albumID.Valid {
				t.Album = &model.Album{ID: albumID.Int64, Name: albumName.String, CoverPath: albumCover}
			}
			items = append(items, t.ToResponse())
		}
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"items": items})
}

// GET /api/tracks/personalized — collaborative filtering recommendation
func (h *TrackHandler) Personalized(c echo.Context) error {
	userID, ok := c.Get("user_id").(int64)
	if !ok {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}
	ctx := c.Request().Context()
	limit := parseInt(c.QueryParam("limit"), 20)

	rows, err := h.DB.QueryContext(ctx, `
		SELECT ph2.user_id, COUNT(*) as overlap
		FROM play_history ph1
		JOIN play_history ph2 ON ph1.track_id = ph2.track_id AND ph1.user_id != ph2.user_id
		WHERE ph1.user_id = $1
		GROUP BY ph2.user_id
		ORDER BY overlap DESC
		LIMIT 10`, userID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	defer rows.Close()

	var similarUserIDs []int64
	for rows.Next() {
		var uid int64
		var overlap int
		if err := rows.Scan(&uid, &overlap); err == nil {
			similarUserIDs = append(similarUserIDs, uid)
		}
	}

	if len(similarUserIDs) == 0 {
		return h.RandomRecommend(c)
	}

	trackRows, err := h.DB.QueryContext(ctx, `
		SELECT t.id, t.title, t.artist_id, t.album_id, t.track_number, t.disc_number,
		 t.duration, t.bitrate, t.format, t.file_path, t.file_size, t.file_mtime,
		 t.play_count, t.last_played, t.created_at, t.updated_at,
		 ar.id, ar.name, al.id, al.name, al.cover_path,
		 COUNT(ph.id) as freq
		FROM tracks t
		LEFT JOIN artists ar ON t.artist_id = ar.id
		LEFT JOIN albums al ON t.album_id = al.id
		JOIN play_history ph ON ph.track_id = t.id
		WHERE ph.user_id = ANY($1)
		AND t.id NOT IN (SELECT track_id FROM play_history WHERE user_id = $2)
		GROUP BY t.id
		ORDER BY freq DESC
		LIMIT $3`, pq.Array(similarUserIDs), userID, limit)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	defer trackRows.Close()

	items := []model.TrackResponse{}
	for trackRows.Next() {
		var t model.Track
		var artistID, albumID sql.NullInt64
		var artistName, albumName, albumCover sql.NullString
		if err := trackRows.Scan(
			&t.ID, &t.Title, &t.ArtistID, &t.AlbumID, &t.TrackNumber,
			&t.DiscNumber, &t.Duration, &t.Bitrate, &t.Format, &t.FilePath, &t.FileSize,
			&t.FileMtime, &t.PlayCount, &t.LastPlayed, &t.CreatedAt, &t.UpdatedAt,
			&artistID, &artistName, &albumID, &albumName, &albumCover,
		); err == nil {
			if artistID.Valid {
				t.Artist = &model.Artist{ID: artistID.Int64, Name: artistName.String}
			}
			if albumID.Valid {
				t.Album = &model.Album{ID: albumID.Int64, Name: albumName.String, CoverPath: albumCover}
			}
			items = append(items, t.ToResponse())
		}
	}

	return c.JSON(http.StatusOK, model.TrackListResponse{Total: int64(len(items)), Items: items})
}

func (h *TrackHandler) callLLM(ctx context.Context, provider, modelName, apiKey, prompt string) (string, error) {
	switch provider {
	case "openai":
		return h.callOpenAI(ctx, modelName, apiKey, prompt)
	case "anthropic":
		return h.callAnthropic(ctx, modelName, apiKey, prompt)
	default:
		return "", fmt.Errorf("unsupported provider: %s", provider)
	}
}

func (h *TrackHandler) callOpenAI(ctx context.Context, modelName, apiKey, prompt string) (string, error) {
	type openAIMessage struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type openAIReqBody struct {
		Model     string          `json:"model"`
		Messages  []openAIMessage `json:"messages"`
		MaxTokens int             `json:"max_tokens"`
	}
	type openAIResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	body, err := json.Marshal(openAIReqBody{
		Model:     modelName,
		Messages:  []openAIMessage{{Role: "user", Content: prompt}},
		MaxTokens: 256,
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var result openAIResp
	json.NewDecoder(resp.Body).Decode(&result)
	if len(result.Choices) > 0 {
		return result.Choices[0].Message.Content, nil
	}
	return "", nil
}

func (h *TrackHandler) callAnthropic(ctx context.Context, modelName, apiKey, prompt string) (string, error) {
	type anthropicMessage struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type anthropicReqBody struct {
		Model     string             `json:"model"`
		MaxTokens int                `json:"max_tokens"`
		Messages  []anthropicMessage `json:"messages"`
	}

	body, err := json.Marshal(anthropicReqBody{
		Model:     modelName,
		MaxTokens: 256,
		Messages:  []anthropicMessage{{Role: "user", Content: prompt}},
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.anthropic.com/v1/messages", bytes.NewBuffer(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	if content, ok := result["content"].([]interface{}); ok && len(content) > 0 {
		if block, ok := content[0].(map[string]interface{}); ok {
			if text, ok := block["text"].(string); ok {
				return text, nil
			}
		}
	}
	return "", nil
}

// parseInt parses a string to int, returning fallback on error
func parseInt(s string, fallback int) int {
	if v, err := strconv.Atoi(s); err == nil {
		return v
	}
	return fallback
}
