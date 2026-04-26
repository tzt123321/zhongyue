package handler

import (
	"database/sql"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
	"zhongyue_refactored/internal/model"
	"zhongyue_refactored/internal/search"
)

type SearchHandler struct {
	DB       *sql.DB
	ESClient *search.ESClient
}

type SearchResultGroup struct {
	Total int64               `json:"total"`
	Items []model.TrackResponse `json:"items"`
}

type ArtistResultGroup struct {
	Total int64          `json:"total"`
	Items []model.Artist  `json:"items"`
}

type AlbumResultGroup struct {
	Total int64         `json:"total"`
	Items []model.Album  `json:"items"`
}

// GET /api/search?q=xxx&type=all|tracks|artists|albums
func (h *SearchHandler) Search(c echo.Context) error {
	q := c.QueryParam("q")
	if q == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "q parameter required")
	}
	searchType := c.QueryParam("type")
	if searchType == "" {
		searchType = "all"
	}

	ctx := c.Request().Context()
	tracks := SearchResultGroup{}
	artists := ArtistResultGroup{}
	albums := AlbumResultGroup{}

	likeArg := "%" + q + "%"

	// Search tracks — prefer ES, fallback to PG ILIKE
	if searchType == "all" || searchType == "tracks" {
		var ids []int64
		if h.ESClient != nil {
			ids, _ = h.ESClient.Search(ctx, q, 50)
		}
		if len(ids) > 0 {
			// Use ES-ordered IDs with PG query
			rows, err := h.DB.QueryContext(ctx, `
				SELECT t.id, t.title, t.artist_id, t.album_id, t.track_number,
				 t.disc_number, t.duration, t.bitrate, t.format, t.file_path, t.file_size,
				 t.file_mtime, t.play_count, t.last_played, t.created_at, t.updated_at,
				 ar.id, ar.name, al.id, al.name, al.cover_path
				FROM tracks t
				LEFT JOIN artists ar ON t.artist_id = ar.id
				LEFT JOIN albums al ON t.album_id = al.id
				WHERE t.id = ANY($1)`, pq.Array(ids))
			if err == nil {
				defer rows.Close()
				trackMap := map[int64]model.TrackResponse{}
				for rows.Next() {
					var t model.Track
					var artistID, albumID sql.NullInt64
					var artistName, albumName, albumCover sql.NullString
					if err := rows.Scan(
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
						trackMap[t.ID] = t.ToResponse()
					}
				}
				// Restore ES order
				for _, id := range ids {
					if tr, ok := trackMap[id]; ok {
						tracks.Items = append(tracks.Items, tr)
					}
				}
				tracks.Total = int64(len(tracks.Items))
			}
		} else {
			// PG fallback with ILIKE
			h.DB.QueryRowContext(ctx,
				`SELECT COUNT(t.id) FROM tracks t LEFT JOIN artists a ON t.artist_id = a.id
				 WHERE t.title ILIKE $1 OR a.name ILIKE $1`, likeArg,
			).Scan(&tracks.Total)
			rows, err := h.DB.QueryContext(ctx,
				`SELECT t.id, t.title, t.artist_id, t.album_id, t.track_number,
				 t.disc_number, t.duration, t.bitrate, t.format, t.file_path, t.file_size,
				 t.file_mtime, t.play_count, t.last_played, t.created_at, t.updated_at,
				 ar.id, ar.name, al.id, al.name, al.cover_path
				FROM tracks t
				LEFT JOIN artists ar ON t.artist_id = ar.id
				LEFT JOIN albums al ON t.album_id = al.id
				WHERE t.title ILIKE $1 OR ar.name ILIKE $1
				ORDER BY t.play_count DESC LIMIT 50`, likeArg)
			if err == nil {
				defer rows.Close()
				for rows.Next() {
					var t model.Track
					var artistID, albumID sql.NullInt64
					var artistName, albumName, albumCover sql.NullString
					if err := rows.Scan(
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
						tracks.Items = append(tracks.Items, t.ToResponse())
					}
				}
			}
		}
	}

	// Search artists — always PG
	if searchType == "all" || searchType == "artists" {
		h.DB.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM artists WHERE name ILIKE $1`, likeArg,
		).Scan(&artists.Total)
		rows, err := h.DB.QueryContext(ctx,
			`SELECT id, name, musicbrainz_id, image_path, created_at, updated_at
			 FROM artists WHERE name ILIKE $1 ORDER BY name ASC LIMIT 20`, likeArg)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var a model.Artist
				if err := rows.Scan(&a.ID, &a.Name, &a.MusicbrainzID, &a.ImagePath, &a.CreatedAt, &a.UpdatedAt); err == nil {
					artists.Items = append(artists.Items, a)
				}
			}
		}
	}

	// Search albums — always PG
	if searchType == "all" || searchType == "albums" {
		h.DB.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM albums WHERE name ILIKE $1`, likeArg,
		).Scan(&albums.Total)
		rows, err := h.DB.QueryContext(ctx,
			`SELECT a.id, a.name, a.artist_id, a.cover_path, a.year, a.created_at, a.updated_at,
			 ar.id, ar.name
			 FROM albums a
			 LEFT JOIN artists ar ON a.artist_id = ar.id
			 WHERE a.name ILIKE $1 ORDER BY a.year DESC LIMIT 20`, likeArg)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var a model.Album
				var artistID sql.NullInt64
				var artistName sql.NullString
				if err := rows.Scan(&a.ID, &a.Name, &a.ArtistID, &a.CoverPath, &a.Year, &a.CreatedAt, &a.UpdatedAt,
					&artistID, &artistName); err == nil {
					if artistID.Valid {
						a.Artist = &model.Artist{ID: artistID.Int64, Name: artistName.String}
					}
					albums.Items = append(albums.Items, a)
				}
			}
		}
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"tracks":  tracks,
		"artists": artists,
		"albums":  albums,
	})
}