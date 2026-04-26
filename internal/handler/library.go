package handler

import (
	"database/sql"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dhowden/tag"
	"github.com/labstack/echo/v4"
)

type LibraryHandler struct {
	DB        *sql.DB
	MusicPath string
}

type ScanResult struct {
	Scanned int `json:"scanned"`
	Added   int `json:"added"`
	Updated int `json:"updated"`
	Removed int `json:"removed"`
	Errors  int `json:"errors"`
}

type Stats struct {
	Tracks        int64 `json:"tracks"`
	Artists       int64 `json:"artists"`
	Albums        int64 `json:"albums"`
	TotalDuration int64 `json:"total_duration"`
	TotalSize     int64 `json:"total_size"`
}

// GET /api/stats
func (h *LibraryHandler) Stats(c echo.Context) error {
	var stats Stats
	if err := h.DB.QueryRow(`SELECT COUNT(*) FROM tracks`).Scan(&stats.Tracks); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if err := h.DB.QueryRow(`SELECT COUNT(*) FROM artists`).Scan(&stats.Artists); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if err := h.DB.QueryRow(`SELECT COUNT(*) FROM albums`).Scan(&stats.Albums); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if err := h.DB.QueryRow(`SELECT COALESCE(SUM(duration), 0) FROM tracks`).Scan(&stats.TotalDuration); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if err := h.DB.QueryRow(`SELECT COALESCE(SUM(file_size), 0) FROM tracks`).Scan(&stats.TotalSize); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, stats)
}

// POST /api/library/scan
func (h *LibraryHandler) Scan(c echo.Context) error {
	userID := c.Get("user_id").(int64)
	if userID == 0 {
		return echo.NewHTTPError(http.StatusUnauthorized, "unauthorized")
	}

	var isAdmin bool
	h.DB.QueryRow(`SELECT is_admin FROM users WHERE id = $1`, userID).Scan(&isAdmin)
	if !isAdmin {
		return echo.NewHTTPError(http.StatusForbidden, "admin only")
	}

	result := h.scanLibrary()
	return c.JSON(http.StatusOK, map[string]interface{}{
		"status":  "completed",
		"result":  result,
		"scanned": result.Scanned,
	})
}

func (h *LibraryHandler) scanLibrary() ScanResult {
	result := ScanResult{}
	musicDir := h.MusicPath
	if musicDir == "" {
		musicDir = "/tmp/music"
	}

	audioExts := map[string]bool{
		".mp3": true, ".flac": true, ".wav": true,
		".m4a": true, ".ogg": true, ".opus": true, ".aac": true,
	}

	var audioFiles []string
	filepath.Walk(musicDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if audioExts[ext] {
			audioFiles = append(audioFiles, path)
		}
		return nil
	})

	// Get existing file_paths from DB
	rows, _ := h.DB.Query(`SELECT file_path FROM tracks`)
	existingPaths := map[string]bool{}
	if rows != nil {
		for rows.Next() {
			var fp string
			rows.Scan(&fp)
			existingPaths[fp] = true
		}
		rows.Close()
	}

	// Artist cache: name -> id
	artistCache := map[string]int64{}
	artistRows, _ := h.DB.Query(`SELECT id, name FROM artists`)
	if artistRows != nil {
		for artistRows.Next() {
			var id int64
			var name string
			artistRows.Scan(&id, &name)
			artistCache[name] = id
		}
		artistRows.Close()
	}

	// Album cache: "name|artistID" -> id
	albumCache := map[string]int64{}
	albumRows, _ := h.DB.Query(`SELECT id, name, artist_id FROM albums`)
	if albumRows != nil {
		for albumRows.Next() {
			var id, artistID int64
			var name string
			albumRows.Scan(&id, &name, &artistID)
			key := name + "|" + strconv.FormatInt(artistID, 10)
			albumCache[key] = id
		}
		albumRows.Close()
	}

	for _, filePath := range audioFiles {
		meta := extractFileMetadata(filePath)
		if meta == nil {
			result.Errors++
			continue
		}

		result.Scanned++

		title := meta.title
		if title == "" {
			title = strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
		}
		artistName := meta.artist
		if artistName == "" {
			artistName = "Unknown Artist"
		}
		albumName := meta.album
		if albumName == "" {
			albumName = "Unknown Album"
		}

		// Get or create artist
		artistID := artistCache[artistName]
		if artistID == 0 {
			h.DB.QueryRow(`
				INSERT INTO artists (name, created_at, updated_at)
				VALUES ($1, NOW(), NOW()) RETURNING id`,
				artistName).Scan(&artistID)
			artistCache[artistName] = artistID
			result.Added++
		}

		// Get or create album
		albumKey := albumName + "|" + strconv.FormatInt(artistID, 10)
		albumID := albumCache[albumKey]
		if albumID == 0 {
			h.DB.QueryRow(`
				INSERT INTO albums (name, artist_id, year, created_at, updated_at)
				VALUES ($1, $2, $3, NOW(), NOW()) RETURNING id`,
				albumName, artistID, meta.year).Scan(&albumID)
			albumCache[albumKey] = albumID
			result.Added++
		}

		if existingPaths[filePath] {
			var trackID int64
			h.DB.QueryRow(`SELECT id FROM tracks WHERE file_path = $1`, filePath).Scan(&trackID)
			h.DB.Exec(`
				UPDATE tracks SET
					title=$1, artist_id=$2, album_id=$3,
					track_number=$4, disc_number=$5, duration=$6,
					file_size=$7, updated_at=NOW()
				WHERE id=$8`,
				title, artistID, albumID,
				meta.trackNumber, meta.discNumber,
				meta.duration, meta.fileSize, trackID)
			result.Updated++
		} else {
			h.DB.Exec(`
				INSERT INTO tracks (title, artist_id, album_id, track_number, disc_number,
					duration, format, file_path, file_size, file_mtime, created_at, updated_at)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NOW(),NOW())`,
				title, artistID, albumID,
				meta.trackNumber, meta.discNumber,
				meta.duration, meta.format,
				filePath, meta.fileSize, meta.fileMtime)
			result.Added++
			existingPaths[filePath] = true
		}
	}

	// Update album track counts
	h.DB.Exec(`
		UPDATE albums SET total_tracks = sub.c
		FROM (
			SELECT album_id, COUNT(*) as c
			FROM tracks WHERE album_id IS NOT NULL
			GROUP BY album_id
		) AS sub
		WHERE albums.id = sub.album_id`)

	return result
}

type fileMetadata struct {
	title       string
	artist      string
	album       string
	year        interface{}
	trackNumber interface{}
	discNumber  interface{}
	duration    int
	format      string
	fileSize    int64
	fileMtime   time.Time
}

func extractFileMetadata(filePath string) *fileMetadata {
	meta := &fileMetadata{
		format:    strings.ToUpper(strings.TrimPrefix(filepath.Ext(filePath), ".")),
		fileMtime: time.Now(),
	}

	file, err := os.Open(filePath)
	if err != nil {
		return nil
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return nil
	}
	meta.fileSize = stat.Size()
	meta.fileMtime = stat.ModTime()

	tags, err := tag.ReadFrom(file)
	if err != nil {
		return meta
	}

	if t := tags.Title(); t != "" {
		meta.title = t
	}
	if t := tags.Artist(); t != "" {
		meta.artist = t
	}
	if t := tags.Album(); t != "" {
		meta.album = t
	}
	if y := tags.Year(); y > 0 {
		meta.year = y
	}
	tp, _ := tags.Track()
	if tp > 0 {
		meta.trackNumber = tp
	}
	dp, _ := tags.Disc()
	if dp > 0 {
		meta.discNumber = dp
	}

	return meta
}
