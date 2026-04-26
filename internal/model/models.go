package model

import (
	"database/sql"
	"time"
)

// ── User ─────────────────────────────────────────────────────────────────────

type User struct {
	ID                   int64          `db:"id" json:"id"`
	Username             string         `db:"username" json:"username"`
	Email                sql.NullString `db:"email" json:"email,omitempty"`
	PasswordHash         string         `db:"password_hash" json:"-"`
	InitialPasswordHash  sql.NullString `db:"initial_password_hash" json:"-"`
	InitialPassword      sql.NullString `db:"initial_password" json:"initial_password,omitempty"`
	IsAdmin              bool           `db:"is_admin" json:"is_admin"`
	IsMusician           bool           `db:"is_musician" json:"is_musician"`
	IsActive             bool           `db:"is_active" json:"is_active"`
	IsBanned             bool           `db:"is_banned" json:"is_banned"`
	IsMuted              bool           `db:"is_muted" json:"is_muted"`
	InviteCode           sql.NullString `db:"invite_code" json:"-"`
	APIKey               sql.NullString `db:"api_key" json:"-"`
	CreatedAt            time.Time      `db:"created_at" json:"created_at"`
	UpdatedAt            time.Time      `db:"updated_at" json:"updated_at"`
}

// UserResponse is the safe JSON view of User (no password fields)
type UserResponse struct {
	ID              int64   `json:"id"`
	Username        string  `json:"username"`
	Email           *string `json:"email,omitempty"`
	IsAdmin         bool    `json:"is_admin"`
	IsMusician      bool    `json:"is_musician"`
	IsActive        bool    `json:"is_active"`
	IsBanned        bool    `json:"is_banned"`
	InitialPassword *string `json:"initial_password,omitempty"` // admin only
}

func (u *User) ToResponse(admin bool) UserResponse {
	resp := UserResponse{
		ID:         u.ID,
		Username:   u.Username,
		IsAdmin:    u.IsAdmin,
		IsMusician: u.IsMusician,
		IsActive:   u.IsActive,
		IsBanned:   u.IsBanned,
	}
	if u.Email.Valid {
		resp.Email = &u.Email.String
	}
	if admin && u.InitialPassword.Valid {
		resp.InitialPassword = &u.InitialPassword.String
	}
	return resp
}

// ── Artist ───────────────────────────────────────────────────────────────────

type Artist struct {
	ID            int64          `db:"id" json:"id"`
	Name          string         `db:"name" json:"name"`
	MusicbrainzID sql.NullString `db:"musicbrainz_id" json:"musicbrainz_id,omitempty"`
	ImagePath     sql.NullString `db:"image_path" json:"image_path,omitempty"`
	CreatedAt     time.Time      `db:"created_at" json:"created_at"`
	UpdatedAt     time.Time      `db:"updated_at" json:"updated_at"`
}

// ── Album ────────────────────────────────────────────────────────────────────

type Album struct {
	ID         int64          `db:"id" json:"id"`
	Name       string         `db:"name" json:"name"`
	ArtistID   sql.NullInt64  `db:"artist_id" json:"artist_id,omitempty"`
	Artist     *Artist        `db:"-" json:"artist,omitempty"`
	CoverPath  sql.NullString `db:"cover_path" json:"cover_path,omitempty"`
	Year       sql.NullInt32  `db:"year" json:"year,omitempty"`
	CreatedAt  time.Time      `db:"created_at" json:"created_at"`
	UpdatedAt  time.Time      `db:"updated_at" json:"updated_at"`
}

// ── Track ────────────────────────────────────────────────────────────────────

type Track struct {
	ID          int64          `db:"id" json:"id"`
	Title       string         `db:"title" json:"title"`
	ArtistID    sql.NullInt64  `db:"artist_id" json:"artist_id,omitempty"`
	AlbumID     sql.NullInt64  `db:"album_id" json:"album_id,omitempty"`
	TrackNumber sql.NullInt32  `db:"track_number" json:"track_number,omitempty"`
	DiscNumber  sql.NullInt32  `db:"disc_number" json:"disc_number,omitempty"`
	Duration    sql.NullInt32  `db:"duration" json:"duration,omitempty"` // seconds
	Bitrate     sql.NullInt32  `db:"bitrate" json:"bitrate,omitempty"`
	Format      sql.NullString `db:"format" json:"format,omitempty"`
	FilePath    string         `db:"file_path" json:"file_path"`
	FileSize    sql.NullInt64  `db:"file_size" json:"file_size,omitempty"`
	FileMtime   sql.NullTime   `db:"file_mtime" json:"file_mtime,omitempty"`
	PlayCount   int            `db:"play_count" json:"play_count"`
	LastPlayed  sql.NullTime   `db:"last_played" json:"last_played,omitempty"`
	CreatedAt   time.Time      `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time      `db:"updated_at" json:"updated_at"`

	// Joined fields (not in DB)
	Artist *Artist `db:"-" json:"artist,omitempty"`
	Album  *Album  `db:"-" json:"album,omitempty"`
}

// TrackResponse is what the API returns
type TrackResponse struct {
	ID          int64           `json:"id"`
	Title       string          `json:"title"`
	ArtistID    *int64          `json:"artist_id,omitempty"`
	AlbumID     *int64          `json:"album_id,omitempty"`
	TrackNumber *int            `json:"track_number,omitempty"`
	DiscNumber  *int            `json:"disc_number,omitempty"`
	Duration    *int            `json:"duration,omitempty"`
	Format      string          `json:"format,omitempty"`
	FilePath    string          `json:"file_path"`
	PlayCount   int             `json:"play_count"`
	CreatedAt   time.Time       `json:"created_at"`
	Artist      *ArtistResponse `json:"artist,omitempty"`
	Album       *AlbumResponse   `json:"album,omitempty"`
}

func (t *Track) ToResponse() TrackResponse {
	var artistResp *ArtistResponse
	if t.Artist != nil {
		r := t.Artist.ToResponse()
		artistResp = &r
	}
	var albumResp *AlbumResponse
	if t.Album != nil {
		r := t.Album.ToResponse()
		albumResp = &r
	}
	resp := TrackResponse{
		ID:        t.ID,
		Title:     t.Title,
		FilePath:  t.FilePath,
		PlayCount: t.PlayCount,
		CreatedAt: t.CreatedAt,
		Artist:    artistResp,
		Album:     albumResp,
	}
	if t.ArtistID.Valid {
		resp.ArtistID = &t.ArtistID.Int64
	}
	if t.AlbumID.Valid {
		resp.AlbumID = &t.AlbumID.Int64
	}
	if t.TrackNumber.Valid {
		v := int(t.TrackNumber.Int32)
		resp.TrackNumber = &v
	}
	if t.DiscNumber.Valid {
		v := int(t.DiscNumber.Int32)
		resp.DiscNumber = &v
	}
	if t.Duration.Valid {
		v := int(t.Duration.Int32)
		resp.Duration = &v
	}
	if t.Format.Valid {
		resp.Format = t.Format.String
	}
	return resp
}

// ── TrackListResponse ─────────────────────────────────────────────────────────

type TrackListResponse struct {
	Total int64            `json:"total"`
	Items []TrackResponse   `json:"items"`
}

// ── Playlist ──────────────────────────────────────────────────────────────────

type Playlist struct {
	ID        int64     `db:"id" json:"id"`
	Name      string    `db:"name" json:"name"`
	UserID    int64     `db:"user_id" json:"user_id"`
	IsPublic  bool      `db:"is_public" json:"is_public"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

type PlaylistTrack struct {
	ID          int64     `db:"id" json:"id"`
	PlaylistID  int64     `db:"playlist_id" json:"playlist_id"`
	TrackID     int64     `db:"track_id" json:"track_id"`
	Position    int       `db:"position" json:"position"`
	AddedAt     time.Time `db:"added_at" json:"added_at"`
	Track       *Track    `db:"-" json:"track,omitempty"`
}

// ── PlayHistory ───────────────────────────────────────────────────────────────

type PlayHistory struct {
	ID       int64     `db:"id" json:"id"`
	UserID   int64     `db:"user_id" json:"user_id"`
	TrackID  int64     `db:"track_id" json:"track_id"`
	PlayedAt time.Time `db:"played_at" json:"played_at"`
}

// ── InviteCode ────────────────────────────────────────────────────────────────

type InviteCode struct {
	ID        int64          `db:"id" json:"id"`
	Code      string         `db:"code" json:"code"`
	MaxUses   int            `db:"max_uses" json:"max_uses"`
	Uses      int            `db:"uses" json:"uses"`
	Used      bool           `db:"used" json:"used"`
	CreatedAt time.Time      `db:"created_at" json:"created_at"`
}

// ── APIKey ────────────────────────────────────────────────────────────────────

type APIKey struct {
	ID        int64     `db:"id" json:"id"`
	UserID    int64     `db:"user_id" json:"user_id"`
	Key       string    `db:"key" json:"key"`
	Name      string    `db:"name" json:"name"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

// ── EffectPreset ─────────────────────────────────────────────────────────────

type EffectPreset struct {
	ID        int64           `db:"id" json:"id"`
	UserID    sql.NullInt64   `db:"user_id" json:"user_id,omitempty"`
	Name      string          `db:"name" json:"name"`
	Config    string          `db:"config" json:"config"` // JSONB as string
	IsDefault bool            `db:"is_default" json:"is_default"`
	CreatedAt time.Time       `db:"created_at" json:"created_at"`
}

// ── RadioState ────────────────────────────────────────────────────────────────

type RadioState struct {
	ID             int64          `db:"id" json:"id"`
	UserID         int64          `db:"user_id" json:"user_id"`
	CurrentTrackID sql.NullInt64  `db:"current_track_id" json:"current_track_id,omitempty"`
	History        string         `db:"history" json:"history"`   // JSON array
	LikedTracks    string         `db:"liked_tracks" json:"liked_tracks"` // JSON array
	CreatedAt      time.Time      `db:"created_at" json:"created_at"`
	UpdatedAt      time.Time      `db:"updated_at" json:"updated_at"`
}

// ── ForumPost ───────────────────────────────────────────────────────────────

type ForumPost struct {
	ID        int64     `db:"id" json:"id"`
	UserID    int64     `db:"user_id" json:"user_id"`
	Title     string    `db:"title" json:"title"`
	Content   string    `db:"content" json:"content"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
	Username  string    `db:"-" json:"username,omitempty"`
}

// ── ForumComment ──────────────────────────────────────────────────────────────

type ForumComment struct {
	ID        int64     `db:"id" json:"id"`
	TrackID   int64     `db:"track_id" json:"track_id"`
	UserID    int64     `db:"user_id" json:"user_id"`
	Content   string    `db:"content" json:"content"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
	Username  string    `db:"-" json:"username,omitempty"`
}

// ── Notification ─────────────────────────────────────────────────────────────

type Notification struct {
	ID        int64     `db:"id" json:"id"`
	UserID    int64     `db:"user_id" json:"user_id"`
	Type      string    `db:"type" json:"type"`
	Title     string    `db:"title" json:"title"`
	Content   string    `db:"content" json:"content,omitempty"`
	IsRead    bool      `db:"is_read" json:"is_read"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

// ── PlaybackState ─────────────────────────────────────────────────────────────

type PlaybackState struct {
	ID        int64          `db:"id" json:"id"`
	UserID    int64          `db:"user_id" json:"user_id"`
	TrackID   sql.NullInt64  `db:"track_id" json:"track_id,omitempty"`
	Position  int            `db:"position" json:"position"`
	UpdatedAt time.Time      `db:"updated_at" json:"updated_at"`
}

// ── ModelConfig ───────────────────────────────────────────────────────────────

type ModelConfig struct {
	ID              int64          `db:"id" json:"id"`
	UserID          int64          `db:"user_id" json:"user_id"`
	Name            string         `db:"name" json:"name"`
	Provider        string         `db:"provider" json:"provider"`       // "openai" / "anthropic" / "azure"
	ModelName       string         `db:"model_name" json:"model_name"`
	APIKeyEncrypted sql.NullString `db:"api_key_encrypted" json:"-"`
	IsDefault       bool           `db:"is_default" json:"is_default"`
}

// ── AlbumResponse ─────────────────────────────────────────────────────────────

type AlbumResponse struct {
	ID         int64         `json:"id"`
	Name       string        `json:"name"`
	ArtistID   *int64        `json:"artist_id"`
	CoverPath  *string       `json:"cover_path"`
	Year       *int          `json:"year"`
	CreatedAt  time.Time     `json:"created_at"`
	UpdatedAt  time.Time     `json:"updated_at"`
	Artist     *ArtistResponse `json:"artist,omitempty"`
}

func (a *Album) ToResponse() AlbumResponse {
	resp := AlbumResponse{
		ID:        a.ID,
		Name:      a.Name,
		CreatedAt: a.CreatedAt,
		UpdatedAt: a.UpdatedAt,
	}
	if a.ArtistID.Valid {
		resp.ArtistID = &a.ArtistID.Int64
	}
	if a.CoverPath.Valid && a.CoverPath.String != "" {
		resp.CoverPath = &a.CoverPath.String
	}
	if a.Year.Valid {
		v := int(a.Year.Int32)
		resp.Year = &v
	}
	if a.Artist != nil {
		r := a.Artist.ToResponse()
		resp.Artist = &r
	}
	return resp
}

// ── ArtistResponse ─────────────────────────────────────────────────────────────

type ArtistResponse struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	MusicbrainzID *string   `json:"musicbrainz_id,omitempty"`
	ImagePath     *string   `json:"cover_path,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (a *Artist) ToResponse() ArtistResponse {
	resp := ArtistResponse{
		ID:        a.ID,
		Name:      a.Name,
		CreatedAt: a.CreatedAt,
		UpdatedAt: a.UpdatedAt,
	}
	if a.MusicbrainzID.Valid && a.MusicbrainzID.String != "" {
		resp.MusicbrainzID = &a.MusicbrainzID.String
	}
	if a.ImagePath.Valid && a.ImagePath.String != "" {
		resp.ImagePath = &a.ImagePath.String
	}
	return resp
}
