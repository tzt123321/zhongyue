package handler

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"zhongyue_refactored/internal/model"
)

type PlaybackHandler struct {
	DB *sql.DB
}

type SaveStateRequest struct {
	TrackID  int64 `json:"track_id"`
	Position int   `json:"position"`
}

// GET /api/playback/history
func (h *PlaybackHandler) History(c echo.Context) error {
	userID := c.Get("user_id").(int64)
	limit := parseInt(c.QueryParam("limit"), 50)
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	rows, err := h.DB.Query(
		`SELECT ph.id, ph.user_id, ph.track_id, ph.played_at,
		 t.id, t.title, t.artist_id, t.album_id, t.track_number,
		 t.disc_number, t.duration, t.bitrate, t.format, t.file_path, t.file_size,
		 t.file_mtime, t.play_count, t.last_played, t.created_at, t.updated_at,
		 ar.id, ar.name, al.id, al.name, al.cover_path
		 FROM play_history ph
		 JOIN tracks t ON ph.track_id = t.id
		 LEFT JOIN artists ar ON t.artist_id = ar.id
		 LEFT JOIN albums al ON t.album_id = al.id
		 WHERE ph.user_id = $1
		 ORDER BY ph.played_at DESC
		 LIMIT $2`, userID, limit)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	defer rows.Close()

	items := []map[string]interface{}{}
	for rows.Next() {
		var ph model.PlayHistory
		var t model.Track
		var artistID, albumID sql.NullInt64
		var artistName, albumName, albumCover sql.NullString

		if err := rows.Scan(
			&ph.ID, &ph.UserID, &ph.TrackID, &ph.PlayedAt,
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

		items = append(items, map[string]interface{}{
			"id":        ph.ID,
			"played_at": ph.PlayedAt,
			"track":     t.ToResponse(),
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"items": items,
	})
}

// POST /api/playback/state
func (h *PlaybackHandler) SaveState(c echo.Context) error {
	userID := c.Get("user_id").(int64)

	var req SaveStateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}
	if req.TrackID <= 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "track_id required")
	}
	if req.Position < 0 {
		req.Position = 0
	}

	// Upsert playback state
	var id int64
	err := h.DB.QueryRow(
		`INSERT INTO playback_state (user_id, track_id, position, updated_at)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (user_id) DO UPDATE SET track_id = $2, position = $3, updated_at = $4
		 RETURNING id`,
		userID, req.TrackID, req.Position, time.Now(),
	).Scan(&id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"id":       id,
		"track_id": req.TrackID,
		"position": req.Position,
	})
}
