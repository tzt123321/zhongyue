package handler

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

// ScrapeHandler handles multi-source metadata scraping
type ScrapeHandler struct {
	DB                   *sql.DB
	DataPath             string
	SpotifyClientID       string
	SpotifyClientSecret   string
	AppleMusicToken      string
}

// coverResult holds a discovered cover URL with its source
type coverResult struct {
	URL    string
	Source string // "apple_music" | "netEase" | "musicbrainz" | "spotify"
	Width  int
	Height int
}

// httpClient returns an http.Client that respects HTTP_PROXY / HTTPS_PROXY env vars
func (h *ScrapeHandler) httpClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
		},
	}
}

// ─── Album Scrape ────────────────────────────────────────────────────────────

// POST /api/scrape/album/:id
func (h *ScrapeHandler) ScrapeAlbum(c echo.Context) error {
	id, err := parseInt64(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid album id")
	}

	var albumName, artistName string
	var artistID sql.NullInt64
	err = h.DB.QueryRow(`
		SELECT a.name, a.artist_id, ar.name
		FROM albums a LEFT JOIN artists ar ON a.artist_id = ar.id
		WHERE a.id = $1`, id).Scan(&albumName, &artistID, &artistName)
	if err == sql.ErrNoRows {
		return echo.NewHTTPError(http.StatusNotFound, "album not found")
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	type res struct{ cover *coverResult; err error }
	ch := make(chan res, 4)

	go func() { r, e := h.scrapeAppleMusicAlbum(albumName, artistName); ch <- res{r, e} }()
	go func() { r, e := h.scrapeNetEaseAlbum(albumName, artistName); ch <- res{r, e} }()
	go func() { r, e := h.scrapeMusicBrainzAlbum(albumName, artistName); ch <- res{r, e} }()
	go func() { r, e := h.scrapeSpotifyAlbum(albumName, artistName); ch <- res{r, e} }()

	var best *coverResult
	var msgs []string
	for i := 0; i < 4; i++ {
		r := <-ch
		if r.err != nil {
			msgs = append(msgs, r.err.Error())
			continue
		}
		if r.cover != nil && r.cover.URL != "" {
			if best == nil || r.cover.Width > best.Width {
				best = r.cover
			}
			msgs = append(msgs, fmt.Sprintf("[%s] %dx%d", r.cover.Source, r.cover.Width, r.cover.Height))
		}
	}

	if best == nil {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"success":    false,
			"cover_path": nil,
			"message":    fmt.Sprintf("所有数据源均未找到专辑 '%s' (%s)。尝试: %s", albumName, artistName, strings.Join(msgs, " | ")),
		})
	}

	coverPath, dlErr := h.saveCover(best.URL, id)
	if dlErr != nil {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"success":    false,
			"cover_path": nil,
			"message":    fmt.Sprintf("下载失败: %v (来源: %s)", dlErr, best.Source),
		})
	}

	h.DB.Exec(`UPDATE albums SET cover_path = $1 WHERE id = $2`, coverPath, id)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success":    true,
		"cover_path": coverPath,
		"source":      best.Source,
		"message":    fmt.Sprintf("从 %s 获取封面成功: %s (%dx%d)", best.Source, albumName, best.Width, best.Height),
	})
}

// POST /api/scrape/artist/:id
func (h *ScrapeHandler) ScrapeArtist(c echo.Context) error {
	id, err := parseInt64(c.Param("id"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid artist id")
	}

	var artistName string
	err = h.DB.QueryRow(`SELECT name FROM artists WHERE id = $1`, id).Scan(&artistName)
	if err == sql.ErrNoRows {
		return echo.NewHTTPError(http.StatusNotFound, "artist not found")
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	type res struct{ cover *coverResult; err error }
	ch := make(chan res, 3)

	go func() { r, e := h.scrapeAppleMusicArtist(artistName); ch <- res{r, e} }()
	go func() { r, e := h.scrapeNetEaseArtist(artistName); ch <- res{r, e} }()
	go func() { r, e := h.scrapeMusicBrainzArtist(artistName); ch <- res{r, e} }()

	var best *coverResult
	var msgs []string
	for i := 0; i < 3; i++ {
		r := <-ch
		if r.err != nil {
			msgs = append(msgs, r.err.Error())
			continue
		}
		if r.cover != nil && r.cover.URL != "" {
			if best == nil || r.cover.Width > best.Width {
				best = r.cover
			}
			msgs = append(msgs, fmt.Sprintf("[%s]", r.cover.Source))
		}
	}

	if best == nil {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"success":    false,
			"cover_path": nil,
			"message":    fmt.Sprintf("所有数据源均未找到艺术家图片 '%s'。详情: %s", artistName, strings.Join(msgs, " | ")),
		})
	}

	coverPath, dlErr := h.saveCover(best.URL, -id)
	if dlErr != nil {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"success":    false,
			"cover_path": nil,
			"message":    fmt.Sprintf("下载失败: %v (来源: %s)", dlErr, best.Source),
		})
	}

	h.DB.Exec(`UPDATE artists SET image_path = $1 WHERE id = $2`, coverPath, id)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success":    true,
		"cover_path": coverPath,
		"source":      best.Source,
		"message":    fmt.Sprintf("从 %s 获取艺术家图片成功: %s", best.Source, artistName),
	})
}

// ─── Apple Music (iTunes Search API) ──────────────────────────────────────────

func (h *ScrapeHandler) scrapeAppleMusicAlbum(album, artist string) (*coverResult, error) {
	term := url.QueryEscape(album + " " + artist)
	apiURL := fmt.Sprintf(
		"https://itunes.apple.com/search?term=%s&media=music&entity=album&limit=5&explicit=no",
		term)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("Apple Music 请求失败: %v", err)
	}
	req.Header.Set("User-Agent", "ZhongYue-Music/1.0")

	client := h.httpClient(10 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Apple Music 连接失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Apple Music HTTP %d", resp.StatusCode)
	}

	var out struct {
		Results []struct {
			CollectionName  string `json:"collectionName"`
			ArtistName      string `json:"artistName"`
			ArtworkUrl100   string `json:"artworkUrl100"`
			ArtworkUrl600   string `json:"artworkUrl600"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("Apple Music JSON 解析失败: %v", err)
	}

	var best *coverResult
	for _, r := range out.Results {
		artwork := r.ArtworkUrl600
		if artwork == "" {
			artwork = r.ArtworkUrl100
			if artwork != "" {
				artwork = strings.Replace(artwork, "/100x100bb.jpg", "/600x600bb.jpg", 1)
			}
		}
		if artwork == "" {
			continue
		}
		cr := &coverResult{URL: artwork, Source: "apple_music", Width: 600, Height: 600}
		if best == nil {
			best = cr
		}
	}
	if best != nil {
		return best, nil
	}
	return nil, fmt.Errorf("Apple Music: 未找到专辑 '%s'", album)
}

func (h *ScrapeHandler) scrapeAppleMusicArtist(artist string) (*coverResult, error) {
	term := url.QueryEscape(artist)
	apiURL := fmt.Sprintf(
		"https://itunes.apple.com/search?term=%s&media=music&entity=musicArtist&limit=3&explicit=no",
		term)

	req, _ := http.NewRequest("GET", apiURL, nil)
	req.Header.Set("User-Agent", "ZhongYue-Music/1.0")

	client := h.httpClient(10 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Apple Music 连接失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Apple Music HTTP %d", resp.StatusCode)
	}

	var out struct {
		Results []struct {
			ArtistName     string `json:"artistName"`
			ArtworkUrl100  string `json:"artworkUrl100"`
			ArtworkUrl30  string `json:"artworkUrl30"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("Apple Music JSON 解析失败: %v", err)
	}

	if len(out.Results) == 0 {
		return nil, fmt.Errorf("Apple Music: 未找到艺术家 '%s'", artist)
	}

	r := out.Results[0]
	artwork := r.ArtworkUrl100
	if artwork == "" {
		artwork = r.ArtworkUrl30
	}
	if artwork == "" {
		return nil, fmt.Errorf("Apple Music: 艺术家 '%s' 无图片", artist)
	}
	artwork = strings.Replace(artwork, "/100x100bb.jpg", "/300x300bb.jpg", 1)

	return &coverResult{URL: artwork, Source: "apple_music", Width: 300, Height: 300}, nil
}

// ─── NetEase Music (网易云音乐) ───────────────────────────────────────────────

func (h *ScrapeHandler) scrapeNetEaseAlbum(album, artist string) (*coverResult, error) {
	time.Sleep(500 * time.Millisecond)

	form := fmt.Sprintf("csrf_token=&offset=0&total=true&limit=10&type=10&s=%s",
		url.QueryEscape(album+" "+artist))

	req, err := http.NewRequest("POST", "https://music.163.com/api/search/get", strings.NewReader(form))
	if err != nil {
		return nil, fmt.Errorf("NetEase 请求创建失败: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://music.163.com")
	req.Header.Set("Origin", "https://music.163.com")

	client := h.httpClient(10 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("NetEase 连接失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("NetEase HTTP %d", resp.StatusCode)
	}

	// NetEase album search returns result.albums[] — NOT result.songs[].album
	var raw struct {
		Result struct {
			Albums []struct {
				Name   string `json:"name"`
				PicURL string `json:"picUrl"` // full direct URL
				Artist struct {
					Name string `json:"name"`
				} `json:"artist"`
			} `json:"albums"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("NetEase JSON 解析失败: %v", err)
	}

	albums := raw.Result.Albums
	if len(albums) == 0 {
		return nil, fmt.Errorf("NetEase: 未找到专辑 '%s'", album)
	}

	// Use first result
	first := albums[0]
	coverURL := first.PicURL
	if coverURL == "" {
		return nil, fmt.Errorf("NetEase: 专辑 '%s' 无封面 URL", album)
	}

	return &coverResult{
		URL:    coverURL,
		Source: "netEase",
		Width:  1200,
		Height: 1200,
	}, nil
}

func (h *ScrapeHandler) scrapeNetEaseArtist(artist string) (*coverResult, error) {
	time.Sleep(500 * time.Millisecond)

	form := fmt.Sprintf("csrf_token=&offset=0&total=true&limit=5&type=100&s=%s", url.QueryEscape(artist))

	req, err := http.NewRequest("POST", "https://music.163.com/api/search/get", strings.NewReader(form))
	if err != nil {
		return nil, fmt.Errorf("NetEase 请求创建失败: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Referer", "https://music.163.com")

	client := h.httpClient(10 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("NetEase 连接失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("NetEase HTTP %d", resp.StatusCode)
	}

	var raw struct {
		Result struct {
			Artists []struct {
				Name      string `json:"name"`
				ID        int64  `json:"id"`
				Img1v1URL string `json:"img1v1Url"` // high-res
			} `json:"artists"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("NetEase JSON 解析失败: %v", err)
	}

	if len(raw.Result.Artists) == 0 {
		return nil, fmt.Errorf("NetEase: 未找到艺术家 '%s'", artist)
	}

	a := raw.Result.Artists[0]
	imgURL := a.Img1v1URL
	if imgURL == "" {
		return nil, fmt.Errorf("NetEase: 艺术家 '%s' 无图片", artist)
	}

	return &coverResult{URL: imgURL, Source: "netEase", Width: 1000, Height: 1000}, nil
}

// ─── Spotify ─────────────────────────────────────────────────────────────────

	type spotifyToken struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn  int    `json:"expires_in"`
	}

// ─── QQMusic (QQ音乐) ────────────────────────────────────────────────────────

func (h *ScrapeHandler) scrapeQQMusicAlbum(album, artist string) (*coverResult, error) {
	payload := map[string]interface{}{
		"comm": map[string]interface{}{
			"ct": 6, "cv": 80600, "gray": "0", "nettype": "2", "patch": "2",
			"OpenUDDI": fmt.Sprintf("%x", time.Now().UnixNano()),
			"gzip": 0, "uid": "", "qq": "", "tmeAppID": "qqmusic",
			"tmeLoginType": 2, "psrf_qqopenid": "", "psrf_qqunionid": "",
			"psrf_access_token_expiresAt": "", "psrf_qqaccess_token": "",
			"wid": "", "authst": "", "sid": "",
		},
		"music.search.SearchCgiService.DoSearchForQQMusicDesktop": map[string]interface{}{
			"module": "music.search.SearchCgiService",
			"method": "DoSearchForQQMusicDesktop",
			"param": map[string]interface{}{
				"num_per_page":   10,
				"page_num":       1,
				"remoteplace":    "txt.mac.search",
				"search_type":     2, // 2=album
				"query":           album + " " + artist,
				"grp":             1,
				"searchid":        fmt.Sprintf("%x", time.Now().UnixNano()),
				"nqc_flag":        0,
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", "https://u.y.qq.com/cgi-bin/musicu.fcg", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json;charset=utf-8")
	req.Header.Set("Referer", "https://y.qq.com")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	client := h.httpClient(8 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("QQMusic 连接失败: %v", err)
	}
	defer resp.Body.Close()

	var out map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("QQMusic JSON 解析失败: %v", err)
	}

	music, _ := out["music"].(map[string]interface{})
	svc, _ := music["music.search.SearchCgiService.DoSearchForQQMusicDesktop"].(map[string]interface{})
	data, _ := svc["data"].(map[string]interface{})
	bodyMap, _ := data["body"].(map[string]interface{})
	albumData, _ := bodyMap["album"].(map[string]interface{})
	albumsRaw, _ := albumData["list"].([]interface{})

	if len(albumsRaw) == 0 {
		return nil, fmt.Errorf("QQMusic: 未找到专辑 '%s'", album)
	}

	// Pick best match
	var bestMID, bestPic string
	for _, item := range albumsRaw {
		a, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		title, _ := a["title"].(string)
		mid, _ := a["mid"].(string)
		pic, _ := a["picurl"].(string)

		if strings.Contains(strings.ToLower(title), strings.ToLower(album)) ||
			strings.Contains(strings.ToLower(album), strings.ToLower(title)) {
			bestMID, bestPic = mid, pic
			break
		}
	}
	if bestMID == "" {
		// Fallback to first result
		if a, ok := albumsRaw[0].(map[string]interface{}); ok {
			bestMID, _ = a["mid"].(string)
			bestPic, _ = a["picurl"].(string)
		}
	}

	if bestMID == "" || bestPic == "" {
		return nil, fmt.Errorf("QQMusic: 专辑 '%s' 无封面", album)
	}

	return &coverResult{
		URL:    bestPic,
		Source: "qqmusic",
		Width:  600,
		Height: 600,
	}, nil
}

func (h *ScrapeHandler) scrapeQQMusicArtist(artist string) (*coverResult, error) {
	// QQMusic artist search via song search → extract singer
	payload := map[string]interface{}{
		"comm": map[string]interface{}{
			"ct": 6, "cv": 80600, "gray": "0", "nettype": "2", "patch": "2",
			"OpenUDDI": fmt.Sprintf("%x", time.Now().UnixNano()),
			"gzip": 0, "uid": "", "qq": "", "tmeAppID": "qqmusic",
			"tmeLoginType": 2, "psrf_qqopenid": "", "psrf_qqunionid": "",
			"psrf_access_token_expiresAt": "", "psrf_qqaccess_token": "",
			"wid": "", "authst": "", "sid": "",
		},
		"music.search.SearchCgiService.DoSearchForQQMusicDesktop": map[string]interface{}{
			"module": "music.search.SearchCgiService",
			"method": "DoSearchForQQMusicDesktop",
			"param": map[string]interface{}{
				"num_per_page":   5,
				"page_num":       1,
				"remoteplace":    "txt.mac.search",
				"search_type":    0, // 0=song
				"query":           artist,
				"grp":             1,
				"searchid":        fmt.Sprintf("%x", time.Now().UnixNano()),
				"nqc_flag":        0,
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", "https://u.y.qq.com/cgi-bin/musicu.fcg", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json;charset=utf-8")
	req.Header.Set("Referer", "https://y.qq.com")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	client := h.httpClient(8 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("QQMusic 连接失败: %v", err)
	}
	defer resp.Body.Close()

	var out map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("QQMusic JSON 解析失败: %v", err)
	}

	music, _ := out["music"].(map[string]interface{})
	svc, _ := music["music.search.SearchCgiService.DoSearchForQQMusicDesktop"].(map[string]interface{})
	data, _ := svc["data"].(map[string]interface{})
	songBody, _ := data["body"].(map[string]interface{})
	songData, _ := songBody["song"].(map[string]interface{})
	songsRaw, _ := songData["list"].([]interface{})

	if len(songsRaw) == 0 {
		return nil, fmt.Errorf("QQMusic: 未找到艺术家 '%s'", artist)
	}

	// Find matching singer
	var singerMID string
	for _, item := range songsRaw {
		song, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		singersRaw, _ := song["singer"].([]interface{})
		for _, si := range singersRaw {
			s, ok := si.(map[string]interface{})
			if !ok {
				continue
			}
			name, _ := s["name"].(string)
			mid, _ := s["mid"].(string)
			if name != "" && (strings.Contains(strings.ToLower(name), strings.ToLower(artist)) ||
				strings.Contains(strings.ToLower(artist), strings.ToLower(name))) {
				singerMID = mid
				break
			}
		}
		if singerMID != "" {
			break
		}
	}
	if singerMID == "" && len(songsRaw) > 0 {
		if song, ok := songsRaw[0].(map[string]interface{}); ok {
			if singersRaw, ok := song["singer"].([]interface{}); ok && len(singersRaw) > 0 {
				if s, ok := singersRaw[0].(map[string]interface{}); ok {
					singerMID, _ = s["mid"].(string)
				}
			}
		}
	}

	if singerMID == "" {
		return nil, fmt.Errorf("QQMusic: 艺术家 '%s' 无 MID", artist)
	}

	// Artist image URL from QQMusic CDN
	coverURL := fmt.Sprintf("https://y.gtimg.cn/music/photo_new/T001R300x300M000%s.jpg", singerMID)

	return &coverResult{
		URL:    coverURL,
		Source: "qqmusic",
		Width:  300,
		Height: 300,
	}, nil
}

func (h *ScrapeHandler) getSpotifyToken() (string, error) {
	if h.SpotifyClientID == "" || h.SpotifyClientSecret == "" {
		return "", fmt.Errorf("Spotify 未配置 (SPOTIFY_CLIENT_ID/SPOTIFY_CLIENT_SECRET)")
	}

	req, err := http.NewRequest("POST", "https://accounts.spotify.com/api/token",
		strings.NewReader("grant_type=client_credentials"))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(h.SpotifyClientID, h.SpotifyClientSecret)

	client := h.httpClient(10 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("Spotify token HTTP %d", resp.StatusCode)
	}

	var tok spotifyToken
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return "", err
	}
	return tok.AccessToken, nil
}

func (h *ScrapeHandler) scrapeSpotifyAlbum(album, artist string) (*coverResult, error) {
	if h.SpotifyClientID == "" || h.SpotifyClientSecret == "" {
		return nil, fmt.Errorf("Spotify 未配置凭证")
	}

	token, err := h.getSpotifyToken()
	if err != nil {
		return nil, err
	}

	apiURL := "https://api.spotify.com/v1/search?q=" +
		url.QueryEscape(album+" "+artist) + "&type=album&limit=5"

	req, _ := http.NewRequest("GET", apiURL, nil)
	req.Header.Set("Authorization", "Bearer "+token)

	client := h.httpClient(10 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Spotify 请求失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 {
		return nil, fmt.Errorf("Spotify Token 失效")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Spotify HTTP %d", resp.StatusCode)
	}

	var out struct {
		Albums struct {
			Items []struct {
				Name    string `json:"name"`
				Artists []struct{ Name string `json:"name"` } `json:"artists"`
				Images  []struct {
					URL    string `json:"url"`
					Width  int    `json:"width"`
					Height int    `json:"height"`
				} `json:"images"`
			} `json:"items"`
		} `json:"albums"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("Spotify JSON 解析失败: %v", err)
	}

	if len(out.Albums.Items) == 0 {
		return nil, fmt.Errorf("Spotify: 未找到专辑 '%s'", album)
	}

	item := out.Albums.Items[0]
	var imgURL string
	var imgW, imgH int
	for _, im := range item.Images {
		if im.Width > imgW {
			imgURL, imgW, imgH = im.URL, im.Width, im.Height
		}
	}
	if imgURL == "" {
		return nil, fmt.Errorf("Spotify: 专辑 '%s' 无封面", album)
	}

	return &coverResult{URL: imgURL, Source: "spotify", Width: imgW, Height: imgH}, nil
}

// ─── MusicBrainz ─────────────────────────────────────────────────────────────

func (h *ScrapeHandler) scrapeMusicBrainzAlbum(album, artist string) (*coverResult, error) {
	time.Sleep(1100 * time.Millisecond)

	query := url.QueryEscape(fmt.Sprintf(`release:"%s" AND artist:"%s"`, album, artist))
	searchURL := "https://musicbrainz.org/ws/2/release/?query=" + query + "&limit=5&fmt=json"

	req, _ := http.NewRequest("GET", searchURL, nil)
	req.Header.Set("User-Agent", "ZhongYue-Music/1.0 (music-app; mailto:admin@zhongyue)")

	client := h.httpClient(15 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("MusicBrainz 连接失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("MusicBrainz HTTP %d", resp.StatusCode)
	}

	var result struct {
		Releases []struct {
			ID           string `json:"id"`
			Title        string `json:"title"`
		} `json:"releases"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if len(result.Releases) == 0 {
		// Try looser search
		query2 := url.QueryEscape(fmt.Sprintf(`release:"%s"`, album))
		req2, _ := http.NewRequest("GET",
			"https://musicbrainz.org/ws/2/release/?query="+query2+"&limit=5&fmt=json", nil)
		req2.Header.Set("User-Agent", "ZhongYue-Music/1.0")
		resp2, err2 := client.Do(req2)
		if err2 == nil {
			defer resp2.Body.Close()
			if resp2.StatusCode == 200 {
				json.NewDecoder(resp2.Body).Decode(&result)
			}
		}
	}

	if len(result.Releases) == 0 {
		return nil, fmt.Errorf("MusicBrainz: 未找到专辑 '%s'", album)
	}

	mbzID := result.Releases[0].ID
	coverURL, err := h.getCoverArtArchive(mbzID)
	if err != nil || coverURL == "" {
		return nil, fmt.Errorf("CoverArtArchive: '%s' 无封面 (MBID: %s)", album, mbzID)
	}

	return &coverResult{
		URL:    coverURL,
		Source: "musicbrainz",
		Width:  1200,
		Height: 1200,
	}, nil
}

func (h *ScrapeHandler) scrapeMusicBrainzArtist(artist string) (*coverResult, error) {
	time.Sleep(1100 * time.Millisecond)

	query := url.QueryEscape(fmt.Sprintf(`artist:"%s"`, artist))
	apiURL := "https://musicbrainz.org/ws/2/artist/?query=" + query + "&limit=3&fmt=json"

	req, _ := http.NewRequest("GET", apiURL, nil)
	req.Header.Set("User-Agent", "ZhongYue-Music/1.0")

	client := h.httpClient(15 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("MusicBrainz HTTP %d", resp.StatusCode)
	}

	var result struct {
		Artists []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"artists"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if len(result.Artists) == 0 {
		return nil, fmt.Errorf("MusicBrainz: 未找到艺术家 '%s'", artist)
	}

	// MusicBrainz doesn't store artist photos directly — only via Wikipedia links.
	// Skip photo scraping for now and return nil to fall back to other sources.
	return nil, fmt.Errorf("MusicBrainz: 艺术家 '%s' 无直接图片 (需 Wikipedia 关联)", artist)
}

func (h *ScrapeHandler) getCoverArtArchive(mbzID string) (string, error) {
	apiURL := fmt.Sprintf("https://coverartarchive.org/release/%s", mbzID)
	req, _ := http.NewRequest("GET", apiURL, nil)
	req.Header.Set("User-Agent", "ZhongYue-Music/1.0")

	client := h.httpClient(10 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return "", nil
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("CoverArtArchive HTTP %d", resp.StatusCode)
	}

	var out struct {
		Images []struct {
			Image      string `json:"image"`
			Front      bool   `json:"front"`
			Thumbnails *struct {
				Large string `json:"large"`
			} `json:"thumbnails"`
		} `json:"images"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}

	for _, img := range out.Images {
		if img.Front {
			if img.Thumbnails != nil && img.Thumbnails.Large != "" {
				return img.Thumbnails.Large, nil
			}
			return img.Image, nil
		}
	}
	if len(out.Images) > 0 {
		img := out.Images[0]
		if img.Thumbnails != nil && img.Thumbnails.Large != "" {
			return img.Thumbnails.Large, nil
		}
		return img.Image, nil
	}
	return "", nil
}

// ─── Download & Save ───────────────────────────────────────────────────────────

func (h *ScrapeHandler) saveCover(imageURL string, id int64) (string, error) {
	dataPath := h.DataPath
	if dataPath == "" {
		dataPath = "./data"
	}

	ext := "jpg"
	if strings.Contains(imageURL, ".png") || strings.Contains(imageURL, "image/png") {
		ext = "png"
	} else if strings.Contains(imageURL, ".webp") {
		ext = "webp"
	}

	var subdir, filename string
	if id > 0 {
		subdir, filename = "covers", fmt.Sprintf("%d.%s", id, ext)
	} else {
		subdir, filename = "artists", fmt.Sprintf("%d.%s", -id, ext)
	}
	saveDir := filepath.Join(dataPath, subdir)
	os.MkdirAll(saveDir, 0755)
	savePath := filepath.Join(saveDir, filename)
	apiPath := filepath.Join("/data", subdir, filename)

	client := h.httpClient(30 * time.Second)
	req, _ := http.NewRequest("GET", imageURL, nil)
	req.Header.Set("User-Agent", "ZhongYue-Music/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return apiPath, fmt.Errorf("下载请求失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return apiPath, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, resp.Body); err != nil {
		return apiPath, fmt.Errorf("读取响应失败: %v", err)
	}

	// Detect format from magic bytes
	mime := resp.Header.Get("Content-Type")
	b := buf.Bytes()
	if strings.Contains(mime, "png") || (len(b) >= 4 && b[0] == 0x89 && b[1] == 0x50) {
		ext = "png"
	} else if strings.Contains(mime, "webp") || (len(b) >= 4 && b[0] == 0x52 && b[1] == 0x49) {
		ext = "webp"
	}

	// Update extension if needed
	if ext != filepath.Ext(savePath)[1:] {
		savePath = strings.TrimSuffix(savePath, filepath.Ext(savePath)) + "." + ext
		idVal := id
		if idVal < 0 {
			idVal = -idVal
		}
		apiPath = filepath.Join("/data", subdir, fmt.Sprintf("%d.%s", idVal, ext))
	}

	if err := os.WriteFile(savePath, buf.Bytes(), 0644); err != nil {
		return apiPath, fmt.Errorf("写入文件失败: %v", err)
	}

	return apiPath, nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func parseInt64(s string) (int64, error) {
	var v int64
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("invalid")
		}
		v = v*10 + int64(c-'0')
	}
	return v, nil
}

// ─── Library Scrape All ───────────────────────────────────────────────────────

type scrapeAllStatus struct {
	State     string // "idle" | "running" | "done" | "error"
	AlbumOK    int
	AlbumFail  int
	ArtistOK   int
	ArtistFail int
	Message    string
	StartedAt  time.Time
}

var scrapeAllState = &scrapeAllStatus{State: "idle"}

// POST /api/library/scrape-all — start background scrape of all albums + artists
func (h *ScrapeHandler) ScrapeAll(c echo.Context) error {
	if scrapeAllState.State == "running" {
		return c.JSON(http.StatusConflict, map[string]interface{}{
			"state":  "running",
			"message": "刮削任务正在进行中，请稍候...",
		})
	}

	// Reset state and launch goroutine
	*scrapeAllState = scrapeAllStatus{State: "running", StartedAt: time.Now()}

	go h.runScrapeAll()

	return c.JSON(http.StatusAccepted, map[string]interface{}{
		"state":  "running",
		"message": "刮削任务已启动，正在后台运行...",
	})
}

// GET /api/library/scrape-status — poll scrape progress
func (h *ScrapeHandler) ScrapeStatus(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]interface{}{
		"state":       scrapeAllState.State,
		"album_ok":    scrapeAllState.AlbumOK,
		"album_fail":  scrapeAllState.AlbumFail,
		"artist_ok":   scrapeAllState.ArtistOK,
		"artist_fail": scrapeAllState.ArtistFail,
		"message":    scrapeAllState.Message,
		"started_at":  scrapeAllState.StartedAt,
	})
}

func (h *ScrapeHandler) runScrapeAll() {
	// Get all albums with missing covers
	albumRows, err := h.DB.Query(`SELECT id, name, COALESCE(cover_path, '') FROM albums`)
	if err != nil {
		scrapeAllState.State = "error"
		scrapeAllState.Message = "查询专辑失败: " + err.Error()
		return
	}
	defer albumRows.Close()

	var albums []struct{ ID int64; Name string }
	for albumRows.Next() {
		var id int64
		var name, cover string
		if albumRows.Scan(&id, &name, &cover) == nil && cover == "" {
			albums = append(albums, struct{ ID int64; Name string }{id, name})
		}
	}

	// Get all artists with missing images
	artistRows, err := h.DB.Query(`SELECT id, name, COALESCE(image_path, '') FROM artists`)
	if err != nil {
		scrapeAllState.State = "error"
		scrapeAllState.Message = "查询艺术家失败: " + err.Error()
		return
	}
	defer artistRows.Close()

	var artists []struct{ ID int64; Name string }
	for artistRows.Next() {
		var id int64
		var name, imgPath string
		if artistRows.Scan(&id, &name, &imgPath) == nil && imgPath == "" {
			artists = append(artists, struct{ ID int64; Name string }{id, name})
		}
	}

	scrapeAllState.Message = fmt.Sprintf("开始刮削 %d 个专辑 + %d 个艺术家...", len(albums), len(artists))

	// ── Album scraping: sequential fallback through all sources ──
	for i, alb := range albums {
		scrapeAllState.Message = fmt.Sprintf("刮削专辑 %d/%d: %s", i+1, len(albums), alb.Name)

		var artistName string
		h.DB.QueryRow(`
			SELECT COALESCE(ar.name, '') FROM albums a
			LEFT JOIN artists ar ON a.artist_id = ar.id
			WHERE a.id = $1`, alb.ID).Scan(&artistName)

		var best *coverResult
		sources := []struct {
			name   string
			tryFn  func(album, artist string) (*coverResult, error)
		}{
			{"Apple Music", h.scrapeAppleMusicAlbum},
			{"网易云音乐", h.scrapeNetEaseAlbum},
			{"QQ音乐", h.scrapeQQMusicAlbum},
			{"MusicBrainz", h.scrapeMusicBrainzAlbum},
			{"Spotify", h.scrapeSpotifyAlbum},
		}

		for _, src := range sources {
			// Sequential with 7s timeout per source
			ch := make(chan *coverResult, 1)
			go func(album, artist string, c chan<- *coverResult) {
				r, _ := src.tryFn(album, artist)
				select {
				case c <- r:
				default:
				}
			}(alb.Name, artistName, ch)

			select {
			case r := <-ch:
				if r != nil && r.URL != "" && (best == nil || r.Width > best.Width) {
					best = r
				}
			case <-time.After(7 * time.Second):
				// Timeout, try next source
			}

			if best != nil && best.Width >= 600 {
				break // Got good enough cover
			}
		}

		if best != nil {
			path, err := h.saveCover(best.URL, alb.ID)
			if err == nil {
				h.DB.Exec(`UPDATE albums SET cover_path = $1 WHERE id = $2`, path, alb.ID)
				scrapeAllState.AlbumOK++
			} else {
				scrapeAllState.AlbumFail++
			}
		} else {
			scrapeAllState.AlbumFail++
		}

		scrapeAllState.Message = fmt.Sprintf("专辑 %d/%d 完成: %s", i+1, len(albums), alb.Name)
		time.Sleep(300 * time.Millisecond)
	}

	// ── Artist scraping: sequential fallback ──
	for i, art := range artists {
		scrapeAllState.Message = fmt.Sprintf("刮削艺术家 %d/%d: %s", i+1, len(artists), art.Name)

		var best *coverResult
		sources := []struct {
			name   string
			tryFn  func(artist string) (*coverResult, error)
		}{
			{"Apple Music", h.scrapeAppleMusicArtist},
			{"网易云音乐", h.scrapeNetEaseArtist},
			{"QQ音乐", h.scrapeQQMusicArtist},
			{"MusicBrainz", h.scrapeMusicBrainzArtist},
		}

		for _, src := range sources {
			ch := make(chan *coverResult, 1)
			go func(a string, c chan<- *coverResult) {
				r, _ := src.tryFn(a)
				select {
				case c <- r:
				default:
				}
			}(art.Name, ch)

			select {
			case r := <-ch:
				if r != nil && r.URL != "" && (best == nil || r.Width > best.Width) {
					best = r
				}
			case <-time.After(7 * time.Second):
			}

			if best != nil && best.Width >= 300 {
				break
			}
		}

		if best != nil {
			path, err := h.saveCover(best.URL, -art.ID)
			if err == nil {
				h.DB.Exec(`UPDATE artists SET image_path = $1 WHERE id = $2`, path, art.ID)
				scrapeAllState.ArtistOK++
			} else {
				scrapeAllState.ArtistFail++
			}
		} else {
			scrapeAllState.ArtistFail++
		}

		scrapeAllState.Message = fmt.Sprintf("艺术家 %d/%d 完成: %s", i+1, len(artists), art.Name)
		time.Sleep(300 * time.Millisecond)
	}

	scrapeAllState.State = "done"
	scrapeAllState.Message = fmt.Sprintf(
		"刮削完成！专辑: %d 成功 / %d 失败，艺术家: %d 成功 / %d 失败",
		scrapeAllState.AlbumOK, scrapeAllState.AlbumFail,
		scrapeAllState.ArtistOK, scrapeAllState.ArtistFail)
}
