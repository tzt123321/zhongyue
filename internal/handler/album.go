package handler

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"zhongyue_refactored/internal/model"
)

type AlbumHandler struct {
	DB *sql.DB
}

// GET /api/albums
func (h *AlbumHandler) List(c echo.Context) error {
	skip := parseInt(c.QueryParam("skip"), 0)
	limit := parseInt(c.QueryParam("limit"), 50)
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if skip < 0 {
		skip = 0
	}

	var total int64
	if err := h.DB.QueryRow(`SELECT COUNT(*) FROM albums`).Scan(&total); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	rows, err := h.DB.Query(
		`SELECT a.id, a.name, a.artist_id, a.cover_path, a.year, a.created_at, a.updated_at,
		 ar.id, ar.name
		 FROM albums a
		 LEFT JOIN artists ar ON a.artist_id = ar.id
		 ORDER BY a.id DESC
		 OFFSET $1 LIMIT $2`, skip, limit)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	defer rows.Close()

	items := []model.AlbumResponse{}
	for rows.Next() {
		var a model.Album
		var artistID sql.NullInt64
		var artistName sql.NullString

		if err := rows.Scan(&a.ID, &a.Name, &a.ArtistID, &a.CoverPath, &a.Year, &a.CreatedAt, &a.UpdatedAt,
			&artistID, &artistName); err != nil {
			continue
		}
		if artistID.Valid {
			a.Artist = &model.Artist{ID: artistID.Int64, Name: artistName.String}
		}
		items = append(items, a.ToResponse())
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"total": total,
		"items": items,
	})
}

// GET /api/albums/:id
func (h *AlbumHandler) Get(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid album id")
	}

	var a model.Album
	var artistID sql.NullInt64
	var artistName sql.NullString

	err = h.DB.QueryRow(
		`SELECT a.id, a.name, a.artist_id, a.cover_path, a.year, a.created_at, a.updated_at,
		 ar.id, ar.name
		 FROM albums a
		 LEFT JOIN artists ar ON a.artist_id = ar.id
		 WHERE a.id = $1`, id,
	).Scan(&a.ID, &a.Name, &a.ArtistID, &a.CoverPath, &a.Year, &a.CreatedAt, &a.UpdatedAt,
		&artistID, &artistName)
	if err == sql.ErrNoRows {
		return echo.NewHTTPError(http.StatusNotFound, "album not found")
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	if artistID.Valid {
		a.Artist = &model.Artist{ID: artistID.Int64, Name: artistName.String}
	}

	return c.JSON(http.StatusOK, a.ToResponse())
}

// GET /api/albums/:id/tracks
func (h *AlbumHandler) Tracks(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid album id")
	}

	skip := parseInt(c.QueryParam("skip"), 0)
	limit := parseInt(c.QueryParam("limit"), 50)
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	var total int64
	if err := h.DB.QueryRow(`SELECT COUNT(*) FROM tracks WHERE album_id = $1`, id).Scan(&total); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	rows, err := h.DB.Query(
		`SELECT t.id, t.title, t.artist_id, t.album_id, t.track_number,
		 t.disc_number, t.duration, t.bitrate, t.format, t.file_path, t.file_size,
		 t.file_mtime, t.play_count, t.last_played, t.created_at, t.updated_at,
		 ar.id, ar.name, al.id, al.name, al.cover_path
		 FROM tracks t
		 LEFT JOIN artists ar ON t.artist_id = ar.id
		 LEFT JOIN albums al ON t.album_id = al.id
		 WHERE t.album_id = $1
		 ORDER BY t.disc_number ASC, t.track_number ASC
		 OFFSET $2 LIMIT $3`, id, skip, limit)
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
		"total": total,
		"items": items,
	})
}
