package handler

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

type ChatHandler struct {
	DB *sql.DB
}

type ChatMessage struct {
	ID        int64
	Role      string
	Content   string
}

// ── Request/Response types ───────────────────────────────────────────────────

type SendRequest struct {
	SessionID     *int64 `json:"session_id"`
	Message       string `json:"message"`
	ModelConfigID *int64 `json:"model_config_id"`
}

type TrackMatch struct {
	ID              int64   `json:"id"`
	Title           string  `json:"title"`
	Artist          string  `json:"artist"`
	Album           string  `json:"album,omitempty"`
	InLibrary       bool    `json:"in_library"`
	MatchConfidence float64 `json:"match_confidence"`
}

type SendResponse struct {
	Reply        string        `json:"reply"`
	MusicResults []TrackMatch  `json:"music_results"`
	SessionID    int64         `json:"session_id"`
}

type SessionOut struct {
	ID         int64   `json:"id"`
	Title      string  `json:"title"`
	UpdatedAt  string  `json:"updated_at"`
	LastMsg    *string `json:"last_message,omitempty"`
}

type MessageOut struct {
	ID           int64         `json:"id"`
	Role         string        `json:"role"`
	Content      string        `json:"content"`
	MusicResults []TrackMatch  `json:"music_results"`
	CreatedAt    string        `json:"created_at"`
}

// ── Route handlers ───────────────────────────────────────────────────────────

// GET /api/chat/history
func (h *ChatHandler) ListSessions(c echo.Context) error {
	userID := c.Get("user_id").(int64)

	rows, err := h.DB.Query(`
		SELECT id, title, created_at, updated_at
		FROM chat_sessions
		WHERE user_id = $1
		ORDER BY updated_at DESC
		LIMIT 50`, userID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	defer rows.Close()

	out := []SessionOut{}
	for rows.Next() {
		var id int64
		var title string
		var createdAt, updatedAt time.Time
		if rows.Scan(&id, &title, &createdAt, &updatedAt) != nil {
			continue
		}
		if title == "" {
			title = "新对话"
		}
		var lastMsg sql.NullString
		h.DB.QueryRow(`
			SELECT content FROM chat_messages
			WHERE session_id = $1
			ORDER BY created_at DESC LIMIT 1`, id).Scan(&lastMsg)

		var lastPtr *string
		if lastMsg.Valid {
			s := lastMsg.String
			if len(s) > 50 {
				s = s[:50]
			}
			lastPtr = &s
		}
		out = append(out, SessionOut{
			ID:        id,
			Title:     title,
			UpdatedAt: updatedAt.Format(time.RFC3339),
			LastMsg:   lastPtr,
		})
	}
	return c.JSON(http.StatusOK, out)
}

// GET /api/chat/session/:id
func (h *ChatHandler) GetSession(c echo.Context) error {
	userID := c.Get("user_id").(int64)
	sessionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid session id")
	}

	var owner int64
	if err := h.DB.QueryRow(`SELECT user_id FROM chat_sessions WHERE id = $1`, sessionID).Scan(&owner); err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "session not found")
	}
	if owner != userID {
		return echo.NewHTTPError(http.StatusForbidden, "access denied")
	}

	rows, err := h.DB.Query(`
		SELECT id, role, content, COALESCE(music_results,''), created_at
		FROM chat_messages
		WHERE session_id = $1
		ORDER BY created_at ASC`, sessionID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	defer rows.Close()

	messages := []MessageOut{}
	for rows.Next() {
		var id int64
		var role, content, musicResults string
		var createdAt time.Time
		if rows.Scan(&id, &role, &content, &musicResults, &createdAt) != nil {
			continue
		}
		var results []TrackMatch
		if musicResults != "" {
			json.Unmarshal([]byte(musicResults), &results)
		}
		messages = append(messages, MessageOut{
			ID:           id,
			Role:         role,
			Content:      content,
			MusicResults: results,
			CreatedAt:    createdAt.Format(time.RFC3339),
		})
	}
	return c.JSON(http.StatusOK, messages)
}

// DELETE /api/chat/session/:id
func (h *ChatHandler) DeleteSession(c echo.Context) error {
	userID := c.Get("user_id").(int64)
	sessionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid session id")
	}
	res, err := h.DB.Exec(`DELETE FROM chat_sessions WHERE id = $1 AND user_id = $2`, sessionID, userID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return echo.NewHTTPError(http.StatusNotFound, "session not found")
	}
	return c.NoContent(http.StatusNoContent)
}

// POST /api/chat/send
func (h *ChatHandler) Send(c echo.Context) error {
	userID := c.Get("user_id").(int64)

	var req SendRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}
	isNewSession := req.SessionID == nil
	isIntroRequest := isNewSession && strings.TrimSpace(req.Message) == ""
	if !isIntroRequest && strings.TrimSpace(req.Message) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "message cannot be empty")
	}

	// ── Resolve model config ─────────────────────────────────────────────────
	var provider, modelName, apiKeyEnc string
	var configID int64

	if req.ModelConfigID != nil {
		err := h.DB.QueryRow(`
			SELECT provider, model_name, api_key_encrypted, user_id
			FROM model_configs WHERE id = $1`, *req.ModelConfigID).
			Scan(&provider, &modelName, &apiKeyEnc, &configID)
		if err != nil || configID != userID {
			return echo.NewHTTPError(http.StatusNotFound, "model config not found")
		}
	} else {
		err := h.DB.QueryRow(`
			SELECT id, provider, model_name, api_key_encrypted
			FROM model_configs
			WHERE user_id = $1 AND is_enabled = true
			ORDER BY is_default DESC LIMIT 1`, userID).
			Scan(&configID, &provider, &modelName, &apiKeyEnc)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest,
				"请先在「设置」→「模型配置」中添加并选择默认大模型后再使用聊天功能")
		}
	}

	apiKey := decodeKey(apiKeyEnc)

	// ── Get or create session ─────────────────────────────────────────────────
	var sessionID int64
	if req.SessionID != nil {
		var owner int64
		if err := h.DB.QueryRow(`SELECT user_id FROM chat_sessions WHERE id = $1`, *req.SessionID).Scan(&owner); err != nil {
			return echo.NewHTTPError(http.StatusNotFound, "session not found")
		}
		if owner != userID {
			return echo.NewHTTPError(http.StatusForbidden, "access denied")
		}
		sessionID = *req.SessionID
	} else {
		title := "新对话"
		if req.Message != "" {
			title = req.Message
			if len(title) > 50 {
				title = title[:50]
			}
		}
		err := h.DB.QueryRow(`
			INSERT INTO chat_sessions (user_id, title) VALUES ($1, $2) RETURNING id`,
			userID, title).Scan(&sessionID)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
	}

	// ── Handle intro request (new session, no user message yet) ─────────────
	if isIntroRequest {
		systemContent := h.buildSystemPrompt(c, userID)
		introPrompt := systemContent + "\n\n【首次对话】用户刚刚打开对话窗口，请你以「小乐」的身份，友好地自我介绍一下（50字以内），并问用户喜欢什么类型的音乐，或者想聊什么话题。不要推荐歌曲，只是友好问候。"
		llmMsgs := []map[string]string{
			{"role": "system", "content": introPrompt},
			{"role": "user", "content": "你好！"},
		}
		reply, err := h.callLLM(c, provider, modelName, apiKey, llmMsgs)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("模型调用失败: %v", err))
		}
		cleanReply, _ := parseLLMResponse(reply)
		if _, err := h.DB.Exec(
			`INSERT INTO chat_messages (session_id, role, content) VALUES ($1, 'assistant', $2)`,
			sessionID, cleanReply); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		h.DB.Exec(`UPDATE chat_sessions SET updated_at = NOW() WHERE id = $1`, sessionID)
		return c.JSON(http.StatusOK, SendResponse{
			Reply:        cleanReply,
			MusicResults: []TrackMatch{},
			SessionID:    sessionID,
		})
	}

	// ── Save user message ────────────────────────────────────────────────────
	if _, err := h.DB.Exec(
		`INSERT INTO chat_messages (session_id, role, content) VALUES ($1, 'user', $2)`,
		sessionID, req.Message); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	h.DB.Exec(`UPDATE chat_sessions SET updated_at = NOW() WHERE id = $1`, sessionID)

	// ── Build LLM messages ──────────────────────────────────────────────────
	systemContent := h.buildSystemPrompt(c, userID)
	history := h.getHistory(c, sessionID)

	llmMsgs := []map[string]string{{"role": "system", "content": systemContent}}
	for _, m := range history {
		llmMsgs = append(llmMsgs, map[string]string{"role": m.Role, "content": m.Content})
	}
	llmMsgs = append(llmMsgs, map[string]string{"role": "user", "content": req.Message})

	// ── Call LLM ─────────────────────────────────────────────────────────────
	reply, err := h.callLLM(c, provider, modelName, apiKey, llmMsgs)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("模型调用失败: %v", err))
	}

	// ── Parse reply for track recommendations ───────────────────────────────
	cleanReply, recTracks := parseLLMResponse(reply)
	matchedTracks := h.matchTracks(c, recTracks)

	// ── Save AI message ─────────────────────────────────────────────────────
	musicJSON, _ := json.Marshal(matchedTracks)
	if _, err := h.DB.Exec(
		`INSERT INTO chat_messages (session_id, role, content, music_results) VALUES ($1, 'assistant', $2, $3)`,
		sessionID, cleanReply, string(musicJSON)); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	h.DB.Exec(`UPDATE chat_sessions SET updated_at = NOW() WHERE id = $1`, sessionID)

	return c.JSON(http.StatusOK, SendResponse{
		Reply:        cleanReply,
		MusicResults: matchedTracks,
		SessionID:    sessionID,
	})
}

// ── System prompt builder ────────────────────────────────────────────────────

func (h *ChatHandler) buildSystemPrompt(ctx interface{}, userID int64) string {
	// Use the request context from the outer scope via a workaround — we need *http.Request
	// We'll build prompt without per-user history for now to keep it simple
	type trackInfo struct {
		Title  string
		Artist string
		Album  string
	}

	// We need c.Request().Context() — caller passes ctx as echo.Context
	httpCtx := ctx.(echo.Context).Request().Context()

	var trackList []string
	rows, err := h.DB.QueryContext(httpCtx, `
		SELECT t.title, COALESCE(a.name,'') as artist, COALESCE(al.name,'') as album
		FROM tracks t
		LEFT JOIN artists a ON t.artist_id = a.id
		LEFT JOIN albums al ON t.album_id = al.id
		JOIN play_history ph ON ph.track_id = t.id
		WHERE ph.user_id = $1
		GROUP BY t.id, a.name, al.name
		ORDER BY COUNT(ph.id) DESC LIMIT 20`, userID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var title, artist, album string
			if rows.Scan(&title, &artist, &album) == nil {
				trackList = append(trackList, fmt.Sprintf("- %s - %s（%s）", title, artist, album))
			}
		}
	}

	var libraryCount int
	h.DB.QueryRowContext(httpCtx, `SELECT COUNT(*) FROM tracks`).Scan(&libraryCount)

	var context strings.Builder
	if len(trackList) > 0 {
		context.WriteString("【用户听歌历史（最近20首，按播放次数排序）】\n")
		context.WriteString(strings.Join(trackList, "\n"))
		context.WriteString("\n\n")
	}
	context.WriteString(fmt.Sprintf("【本地曲库概况】共 %d 首曲目。\n（用户暂无听歌历史，是新用户）\n", libraryCount))

	return `你是一位专业、热情的**音乐推荐顾问**，名叫「小乐」。

【能力范围】
- 根据用户描述的心情、场景、喜好推荐歌曲
- 根据用户的历史听歌口味进行个性化推荐
- 询问用户的音乐偏好（风格、语言、年代、艺人等）来细化推荐
- 回答用户关于音乐的各种问题

【回复要求】
1. **推荐歌曲时**，在回复末尾添加一行 **JSON 行**，格式如下（不要加任何其他内容在这一行之前或之后）：

[TRACKS: [{"title": "歌曲名", "artist": "艺术家名"}, ...]]

2. 如果本轮不推荐歌曲，则不需要添加 [TRACKS: ...] 行。
3. 推荐理由要简洁、有画面感（30字以内）。
4. 推荐曲目时，优先推荐华语流行、欧美热门、日韩潮流等大众熟悉的作品。
5. 每次推荐 1~5 首即可，不要贪多。
6. 用户可以聊任何与音乐有关的话题，也可以要求你帮他找歌、推荐歌、描述音乐风格等。
7. 用**中文**友好地与用户交流，语气温暖但专业。
8. 如果用户说的不是音乐相关，可以礼貌引导回到音乐话题。
` + "\n\n" + context.String()
}

// ── LLM Calling ───────────────────────────────────────────────────────────────

func (h *ChatHandler) callLLM(ctx interface{}, provider, modelName, apiKey string, messages []map[string]string) (string, error) {
	switch provider {
	case "openai", "minimax", "doubao", "zhipu", "qwen", "deepseek":
		return h.callOpenAICompat(ctx, provider, modelName, apiKey, messages)
	case "anthropic":
		return h.callAnthropic(ctx, modelName, apiKey, messages)
	case "google":
		return h.callGoogle(ctx, modelName, apiKey, messages)
	case "wenxin":
		return h.callWenxin(ctx, modelName, apiKey, messages)
	default:
		return h.callOpenAICompat(ctx, "openai", modelName, apiKey, messages)
	}
}

func (h *ChatHandler) callOpenAICompat(ctx interface{}, provider, modelName, apiKey string, messages []map[string]string) (string, error) {
	httpCtx := ctx.(echo.Context).Request().Context()

	body, _ := json.Marshal(map[string]interface{}{
		"model":      modelName,
		"messages":   messages,
		"max_tokens": 1024,
	})

	baseURL := "https://api.openai.com/v1"
	switch provider {
	case "minimax":
		baseURL = "https://api.minimax.chat/v1"
	case "doubao":
		baseURL = "https://ark.cn-beijing.volces.com/api/v3"
	case "zhipu":
		baseURL = "https://open.bigmodel.cn/api/paas/v4"
	case "qwen":
		baseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	case "deepseek":
		baseURL = "https://api.deepseek.com/v1"
	}

	url := baseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(httpCtx, "POST", url, strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %v", err)
	}
	defer resp.Body.Close()

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %v", err)
	}
	if result.Error != nil {
		return "", fmt.Errorf("API error: %s", result.Error.Message)
	}
	if len(result.Choices) > 0 {
		return result.Choices[0].Message.Content, nil
	}
	return "", nil
}

func (h *ChatHandler) callAnthropic(ctx interface{}, modelName, apiKey string, messages []map[string]string) (string, error) {
	httpCtx := ctx.(echo.Context).Request().Context()

	var systemPrompt string
	anthropicMsgs := []map[string]string{}
	for _, m := range messages {
		if m["role"] == "system" {
			systemPrompt = m["content"]
		} else {
			anthropicMsgs = append(anthropicMsgs, m)
		}
	}

	reqBody := map[string]interface{}{
		"model":      modelName,
		"max_tokens": 1024,
		"messages":   anthropicMsgs,
	}
	if systemPrompt != "" {
		reqBody["system"] = systemPrompt
	}

	body, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(httpCtx, "POST",
		"https://api.anthropic.com/v1/messages", strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %v", err)
	}
	defer resp.Body.Close()

	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %v", err)
	}
	if result.Error != nil {
		return "", fmt.Errorf("API error: %s", result.Error.Message)
	}
	if len(result.Content) > 0 {
		return result.Content[0].Text, nil
	}
	return "", nil
}

func (h *ChatHandler) callGoogle(ctx interface{}, modelName, apiKey string, messages []map[string]string) (string, error) {
	httpCtx := ctx.(echo.Context).Request().Context()

	contents := []map[string]interface{}{}
	for _, m := range messages {
		if m["role"] == "system" {
			continue
		}
		role := "user"
		if m["role"] == "assistant" {
			role = "model"
		}
		contents = append(contents, map[string]interface{}{
			"role": role,
			"parts": []map[string]string{{"text": m["content"]}},
		})
	}

	body, _ := json.Marshal(map[string]interface{}{
		"contents": contents,
		"generationConfig": map[string]int{
			"maxOutputTokens": 1024,
		},
	})

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s",
		modelName, apiKey)
	req, err := http.NewRequestWithContext(httpCtx, "POST", url, strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %v", err)
	}
	defer resp.Body.Close()

	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %v", err)
	}
	if len(result.Candidates) > 0 && len(result.Candidates[0].Content.Parts) > 0 {
		return result.Candidates[0].Content.Parts[0].Text, nil
	}
	return "", nil
}

// ── Wenxin (百度文心) via Qianfan API ────────────────────────────────────────
// Wenxin uses Baidu Qianfan endpoint with access_token (not Bearer key directly)
func (h *ChatHandler) callWenxin(ctx interface{}, modelName, apiKey string, messages []map[string]string) (string, error) {
	httpCtx := ctx.(echo.Context).Request().Context()

	// Map friendly model names to Qianfan model IDs
	modelMap := map[string]string{
		"ernie-4.0-8k-latest": "ernie-4.0-8k",
		"ernie-3.5-8k":       "ernie-3.5-8k",
	}
	qianfanModel := modelMap[modelName]
	if qianfanModel == "" {
		qianfanModel = modelName
	}

	reqBody := map[string]interface{}{
		"model":      qianfanModel,
		"messages":   messages,
		"max_tokens": 1024,
	}
	body, _ := json.Marshal(reqBody)

	url := "https://qianfan.baidubce.com/v2/chat/completions"
	req, err := http.NewRequestWithContext(httpCtx, "POST", url, strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %v", err)
	}
	defer resp.Body.Close()

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %v", err)
	}
	if result.Error != nil {
		return "", fmt.Errorf("API error: %s", result.Error.Message)
	}
	if len(result.Choices) > 0 {
		return result.Choices[0].Message.Content, nil
	}
	return "", nil
}

// ── Parse LLM Response ────────────────────────────────────────────────────────

func parseLLMResponse(text string) (string, []map[string]string) {
	re := regexp.MustCompile(`(?i)\[TRACKS:\s*(\[\{.*?\}\])\]`)
	matches := re.FindStringSubmatch(text)
	if len(matches) < 2 {
		return strings.TrimSpace(text), nil
	}

	var tracks []map[string]string
	if err := json.Unmarshal([]byte(matches[1]), &tracks); err != nil {
		return strings.TrimSpace(text), nil
	}
	idx := strings.Index(text, "[TRACKS:")
	return strings.TrimSpace(text[:idx]), tracks
}

// ── Match Tracks in Library ───────────────────────────────────────────────────

func (h *ChatHandler) matchTracks(ctx interface{}, recs []map[string]string) []TrackMatch {
	if len(recs) == 0 {
		return nil
	}
	httpCtx := ctx.(echo.Context).Request().Context()
	results := []TrackMatch{}

	for _, rec := range recs {
		title := rec["title"]
		artist := rec["artist"]
		if title == "" {
			continue
		}

		query := `
			SELECT t.id, t.title, COALESCE(a.name,''), COALESCE(al.name,'') FROM tracks t
			LEFT JOIN artists a ON t.artist_id = a.id
			LEFT JOIN albums al ON t.album_id = al.id
			WHERE LOWER(t.title) LIKE LOWER($1)`
		arg1 := "%" + title + "%"
		var id int64
		var dbTitle, dbArtist, dbAlbum sql.NullString

		if artist != "" {
			err := h.DB.QueryRowContext(httpCtx, query+" AND LOWER(COALESCE(a.name,'')) LIKE LOWER($2)", arg1, "%"+artist+"%").
				Scan(&id, &dbTitle, &dbArtist, &dbAlbum)
			if err != nil {
				// Try just title
				h.DB.QueryRowContext(httpCtx, query, arg1).Scan(&id, &dbTitle, &dbArtist, &dbAlbum)
			}
		} else {
			h.DB.QueryRowContext(httpCtx, query, arg1).Scan(&id, &dbTitle, &dbArtist, &dbAlbum)
		}

		if id > 0 {
			results = append(results, TrackMatch{
				ID:              id,
				Title:           dbTitle.String,
				Artist:          dbArtist.String,
				Album:           dbAlbum.String,
				InLibrary:       true,
				MatchConfidence: 1.0,
			})
		} else {
			results = append(results, TrackMatch{
				Title:     title,
				Artist:    artist,
				InLibrary: false,
			})
		}
	}
	return results
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func decodeKey(enc string) string {
	if enc == "" {
		return ""
	}
	data, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return enc
	}
	return string(data)
}

func (h *ChatHandler) getHistory(ctx interface{}, sessionID int64) []ChatMessage {
	httpCtx := ctx.(echo.Context).Request().Context()
	rows, _ := h.DB.QueryContext(httpCtx, `
		SELECT id, role, content FROM chat_messages
		WHERE session_id = $1
		ORDER BY created_at ASC LIMIT 20`, sessionID)
	defer rows.Close()

	var msgs []ChatMessage
	for rows.Next() {
		var m ChatMessage
		if rows.Scan(&m.ID, &m.Role, &m.Content) == nil {
			msgs = append(msgs, m)
		}
	}
	return msgs
}
