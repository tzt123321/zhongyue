package handler

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"zhongyue_refactored/internal/model"
)

type PlaylistHandler struct {
	DB *sql.DB
}

type CreatePlaylistRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	IsPublic    bool   `json:"is_public"`
}

type UpdatePlaylistRequest struct {
	Name     string `json:"name"`
	IsPublic *bool  `json:"is_public,omitempty"`
}

type AddTracksRequest struct {
	TrackIDs []int64 `json:"track_ids"`
}

// GET /api/playlists
func (h *PlaylistHandler) List(c echo.Context) error {
	userID := c.Get("user_id").(int64)

	rows, err := h.DB.Query(
		`SELECT id, name, user_id, is_public, created_at, updated_at
		 FROM playlists
		 WHERE user_id = $1
		 ORDER BY updated_at DESC`, userID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	defer rows.Close()

	items := []model.Playlist{}
	for rows.Next() {
		var p model.Playlist
		if err := rows.Scan(&p.ID, &p.Name, &p.UserID, &p.IsPublic, &p.CreatedAt, &p.UpdatedAt); err != nil {
			continue
		}
		items = append(items, p)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"items": items,
	})
}

// POST /api/playlists
func (h *PlaylistHandler) Create(c echo.Context) error {
	userID := c.Get("user_id").(int64)

	var req CreatePlaylistRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}
	if req.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name is required")
	}

	var id int64
	var createdAt time.Time
	err := h.DB.QueryRow(
		`INSERT INTO playlists (name, description, user_id, is_public, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $5)
		 RETURNING id, created_at`,
		req.Name, req.Description, userID, req.IsPublic, time.Now(),
	).Scan(&id, &createdAt)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusCreated, map[string]interface{}{
		"id":          id,
		"name":        req.Name,
		"description": req.Description,
		"user_id":     userID,
		"is_public":   req.IsPublic,
		"created_at":  createdAt,
	})
}

// GET /api/playlists/:id
func (h *PlaylistHandler) Get(c echo.Context) error {
	userID := c.Get("user_id").(int64)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid playlist id")
	}

	var p model.Playlist
	err = h.DB.QueryRow(
		`SELECT id, name, user_id, is_public, created_at, updated_at
		 FROM playlists WHERE id = $1`, id,
	).Scan(&p.ID, &p.Name, &p.UserID, &p.IsPublic, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return echo.NewHTTPError(http.StatusNotFound, "playlist not found")
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Check access
	if !p.IsPublic && p.UserID != userID {
		return echo.NewHTTPError(http.StatusForbidden, "access denied")
	}

	// Fetch tracks
	rows, err := h.DB.Query(
		`SELECT pt.id, pt.playlist_id, pt.track_id, pt.position, pt.added_at,
		 t.id, t.title, t.artist_id, t.album_id, t.track_number,
		 t.disc_number, t.duration, t.bitrate, t.format, t.file_path, t.file_size,
		 t.file_mtime, t.play_count, t.last_played, t.created_at, t.updated_at,
		 ar.id, ar.name, al.id, al.name, al.cover_path
		 FROM playlist_tracks pt
		 JOIN tracks t ON pt.track_id = t.id
		 LEFT JOIN artists ar ON t.artist_id = ar.id
		 LEFT JOIN albums al ON t.album_id = al.id
		 WHERE pt.playlist_id = $1
		 ORDER BY pt.position ASC`, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	defer rows.Close()

	tracks := []model.PlaylistTrack{}
	for rows.Next() {
		var pt model.PlaylistTrack
		var t model.Track
		var artistID, albumID sql.NullInt64
		var artistName, albumName, albumCover sql.NullString

		if err := rows.Scan(
			&pt.ID, &pt.PlaylistID, &pt.TrackID, &pt.Position, &pt.AddedAt,
			&t.ID, &t.Title, &t.ArtistID, &t.AlbumID, &t.TrackNumber,
			&t.DiscNumber, &t.Duration, &t.Bitrate, &t.Format, &t.FilePath, &t.FileSize,
			&t.FileMtime, &t.PlayCount, &t.LastPlayed, &t.CreatedAt, &t.UpdatedAt,
			&artistID, &artistName, &albumID, &albumName, &albumCover,
		); err != nil {
			continue
		}
		if artistID.Valid {
			t.Artist = &model.Artist{ID: artistID.Int64, Name: artistName.String}
		}
		if albumID.Valid {
			t.Album = &model.Album{ID: albumID.Int64, Name: albumName.String, CoverPath: albumCover}
		}
		pt.Track = &t
		tracks = append(tracks, pt)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"playlist": p,
		"tracks":   tracks,
	})
}

// PUT /api/playlists/:id
func (h *PlaylistHandler) Update(c echo.Context) error {
	userID := c.Get("user_id").(int64)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid playlist id")
	}

	var req UpdatePlaylistRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}

	// Check ownership
	var ownerID int64
	var isSystem bool
	if err := h.DB.QueryRow(`SELECT user_id, FALSE FROM playlists WHERE id = $1`, id).Scan(&ownerID, &isSystem); err == sql.ErrNoRows {
		return echo.NewHTTPError(http.StatusNotFound, "playlist not found")
	} else if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if ownerID != userID {
		return echo.NewHTTPError(http.StatusForbidden, "access denied")
	}

	if req.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name is required")
	}

	_, err = h.DB.Exec(
		`UPDATE playlists SET name = $1, is_public = COALESCE($2, is_public), updated_at = $3 WHERE id = $4`,
		req.Name, req.IsPublic, time.Now(), id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.NoContent(http.StatusNoContent)
}

// DELETE /api/playlists/:id
func (h *PlaylistHandler) Delete(c echo.Context) error {
	userID := c.Get("user_id").(int64)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid playlist id")
	}

	result, err := h.DB.Exec(
		`DELETE FROM playlists WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return echo.NewHTTPError(http.StatusNotFound, "playlist not found or access denied")
	}

	return c.NoContent(http.StatusNoContent)
}

// POST /api/playlists/:id/tracks
func (h *PlaylistHandler) AddTracks(c echo.Context) error {
	userID := c.Get("user_id").(int64)
	playlistID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid playlist id")
	}

	var req AddTracksRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}
	if len(req.TrackIDs) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "track_ids required")
	}

	// Check ownership
	var ownerID int64
	if err := h.DB.QueryRow(`SELECT user_id FROM playlists WHERE id = $1`, playlistID).Scan(&ownerID); err == sql.ErrNoRows {
		return echo.NewHTTPError(http.StatusNotFound, "playlist not found")
	} else if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if ownerID != userID {
		return echo.NewHTTPError(http.StatusForbidden, "access denied")
	}

	// Get max position
	var maxPos int
	h.DB.QueryRow(`SELECT COALESCE(MAX(position), 0) FROM playlist_tracks WHERE playlist_id = $1`, playlistID).Scan(&maxPos)

	tx, err := h.DB.Begin()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	for i, trackID := range req.TrackIDs {
		_, err := tx.Exec(
			`INSERT INTO playlist_tracks (playlist_id, track_id, position, added_at)
			 VALUES ($1, $2, $3, $4)
			 ON CONFLICT (playlist_id, track_id) DO NOTHING`,
			playlistID, trackID, maxPos+i+1, time.Now())
		if err != nil {
			tx.Rollback()
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
	}

	if err := tx.Commit(); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.NoContent(http.StatusNoContent)
}

// DELETE /api/playlists/:id/tracks/:track_id
func (h *PlaylistHandler) RemoveTrack(c echo.Context) error {
	userID := c.Get("user_id").(int64)
	playlistID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid playlist id")
	}
	trackID, err := strconv.ParseInt(c.Param("track_id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid track id")
	}

	// Check ownership
	var ownerID int64
	if err := h.DB.QueryRow(`SELECT user_id FROM playlists WHERE id = $1`, playlistID).Scan(&ownerID); err == sql.ErrNoRows {
		return echo.NewHTTPError(http.StatusNotFound, "playlist not found")
	} else if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if ownerID != userID {
		return echo.NewHTTPError(http.StatusForbidden, "access denied")
	}

	result, err := h.DB.Exec(
		`DELETE FROM playlist_tracks WHERE playlist_id = $1 AND track_id = $2`, playlistID, trackID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return echo.NewHTTPError(http.StatusNotFound, "track not found in playlist")
	}

	return c.NoContent(http.StatusNoContent)
}

// GET /api/playlists/history/tracks — system history playlist
func (h *PlaylistHandler) HistoryTracks(c echo.Context) error {
	userID := c.Get("user_id").(int64)

	rows, err := h.DB.Query(
		`SELECT t.id, t.title, t.artist_id, t.album_id, t.track_number,
		 t.disc_number, t.duration, t.bitrate, t.format, t.file_path, t.file_size,
		 t.file_mtime, t.play_count, t.last_played, t.created_at, t.updated_at,
		 ar.id, ar.name, al.id, al.name, al.cover_path
		 FROM tracks t
		 LEFT JOIN artists ar ON t.artist_id = ar.id
		 LEFT JOIN albums al ON t.album_id = al.id
		 WHERE t.play_count > 0
		 ORDER BY t.play_count DESC, t.title ASC
		 LIMIT 100`)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	defer rows.Close()

	items := []model.TrackResponse{}
	for rows.Next() {
		var t model.Track
		var artistID, albumID sql.NullInt64
		var artistName, albumName, albumCover sql.NullString

		if err := rows.Scan(
			&t.ID, &t.Title, &t.ArtistID, &t.AlbumID, &t.TrackNumber,
			&t.DiscNumber, &t.Duration, &t.Bitrate, &t.Format, &t.FilePath, &t.FileSize,
			&t.FileMtime, &t.PlayCount, &t.LastPlayed, &t.CreatedAt, &t.UpdatedAt,
			&artistID, &artistName, &albumID, &albumName, &albumCover,
		); err != nil {
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

	return c.JSON(http.StatusOK, map[string]interface{}{
		"user_id": userID,
		"items":   items,
	})
}
