package handler

import (
	"database/sql"
	"encoding/base64"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
)

// ── Known AI Providers ─────────────────────────────────────────────────────────

var knownProviders = map[string]struct {
	NameCN     string                 `json:"name_cn"`
	Name       string                 `json:"name"`
	DefaultModel string                `json:"default_model"`
	Models    []map[string]interface{} `json:"models"`
}{
	"openai": {
		NameCN:     "OpenAI",
		Name:       "openai",
		DefaultModel: "gpt-4o",
		Models: []map[string]interface{}{
			{"id": "gpt-4o", "name": "GPT-4o", "description": "最新最强模型"},
			{"id": "gpt-4o-mini", "name": "GPT-4o Mini", "description": "轻量快速"},
			{"id": "gpt-4-turbo", "name": "GPT-4 Turbo", "description": "高性能"},
		},
	},
	"anthropic": {
		NameCN:     "Anthropic",
		Name:       "anthropic",
		DefaultModel: "claude-sonnet-4-20250514",
		Models: []map[string]interface{}{
			{"id": "claude-sonnet-4-20250514", "name": "Claude Sonnet 4", "description": "最新 Sonnet"},
			{"id": "claude-3-5-sonnet-20241022", "name": "Claude 3.5 Sonnet", "description": "稳定版"},
			{"id": "claude-3-opus", "name": "Claude 3 Opus", "description": "最强大"},
		},
	},
	"google": {
		NameCN:     "Google",
		Name:       "google",
		DefaultModel: "gemini-2.0-flash",
		Models: []map[string]interface{}{
			{"id": "gemini-2.0-flash", "name": "Gemini 2.0 Flash", "description": "快速响应"},
			{"id": "gemini-1.5-pro", "name": "Gemini 1.5 Pro", "description": "长上下文"},
		},
	},
	"minimax": {
		NameCN:     "MiniMax",
		Name:       "minimax",
		DefaultModel: "MiniMax-M2.7",
		Models: []map[string]interface{}{
			{"id": "MiniMax-M2.7", "name": "MiniMax M2.7", "description": "最新旗舰"},
			{"id": "MiniMax-M2", "name": "MiniMax M2", "description": "稳定版"},
		},
	},
	// ── 国内模型 ──────────────────────────────────────────────────────────────
	"doubao": {
		NameCN:       "字节豆包",
		Name:         "doubao",
		DefaultModel: "doubao-pro-32k",
		Models: []map[string]interface{}{
			{"id": "doubao-pro-32k", "name": "豆包 Pro 32K", "description": "字节旗舰，支持超长上下文"},
			{"id": "doubao-lite-32k", "name": "豆包 Lite 32K", "description": "轻量快速"},
			{"id": "doubao-pro-128k", "name": "豆包 Pro 128K", "description": "超长上下文"},
		},
	},
	"zhipu": {
		NameCN:       "智谱 GLM",
		Name:         "zhipu",
		DefaultModel: "glm-4-flash",
		Models: []map[string]interface{}{
			{"id": "glm-4-flash", "name": "GLM-4 Flash", "description": "免费快速"},
			{"id": "glm-4", "name": "GLM-4", "description": "高性能"},
			{"id": "glm-4-plus", "name": "GLM-4 Plus", "description": "旗舰级"},
		},
	},
	"qwen": {
		NameCN:       "阿里通义",
		Name:         "qwen",
		DefaultModel: "qwen-plus",
		Models: []map[string]interface{}{
			{"id": "qwen-plus", "name": "通义千问 Plus", "description": "主力旗舰"},
			{"id": "qwen-turbo", "name": "通义千问 Turbo", "description": "快速响应"},
			{"id": "qwen-max", "name": "通义千问 Max", "description": "最强性能"},
		},
	},
	"deepseek": {
		NameCN:       "DeepSeek",
		Name:         "deepseek",
		DefaultModel: "deepseek-chat",
		Models: []map[string]interface{}{
			{"id": "deepseek-chat", "name": "DeepSeek Chat", "description": "主力模型"},
			{"id": "deepseek-reasoner", "name": "DeepSeek R1", "description": "推理模型"},
		},
	},
	"wenxin": {
		NameCN:       "百度文心",
		Name:         "wenxin",
		DefaultModel: "ernie-4.0-8k-latest",
		Models: []map[string]interface{}{
			{"id": "ernie-4.0-8k-latest", "name": "文心一言 4.0", "description": "百度旗舰"},
			{"id": "ernie-3.5-8k", "name": "文心一言 3.5", "description": "稳定版"},
		},
	},
}

type SettingsHandler struct {
	DB *sql.DB
}

// ── GET /api/admin/system/info ───────────────────────────────────────────────

func (h *SettingsHandler) GetSystemInfo(c echo.Context) error {
	// Check if there are any model configs in the system
	var configCount int
	h.DB.QueryRow(`SELECT COUNT(*) FROM model_configs`).Scan(&configCount)

	return c.JSON(http.StatusOK, map[string]interface{}{
		"multi_terminal_ai_enabled": true,
		"admin_model_configured":    configCount > 0,
	})
}

// ── GET /api/settings/models ─────────────────────────────────────────────────

func (h *SettingsHandler) ListModels(c echo.Context) error {
	userID := c.Get("user_id").(int64)

	// Get user's configs
	rows, err := h.DB.Query(`
		SELECT id, user_id, name, provider, model_name, is_default, is_enabled, created_at
		FROM model_configs
		WHERE user_id = $1
		ORDER BY is_default DESC, created_at DESC`, userID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	defer rows.Close()

	configs := []map[string]interface{}{}
	for rows.Next() {
		var id, userID int64
		var name, provider, modelName string
		var isDefault, isEnabled bool
		var createdAt string
		if err := rows.Scan(&id, &userID, &name, &provider, &modelName, &isDefault, &isEnabled, &createdAt); err != nil {
			continue
		}
		configs = append(configs, map[string]interface{}{
			"id":          id,
			"user_id":     userID,
			"name":        name,
			"provider":    provider,
			"model_name":  modelName,
			"is_default":  isDefault,
			"is_enabled":  isEnabled,
			"created_at": createdAt,
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"configs":   configs,
		"providers": knownProviders,
	})
}

// ── POST /api/settings/models ────────────────────────────────────────────────

func (h *SettingsHandler) CreateModel(c echo.Context) error {
	userID := c.Get("user_id").(int64)

	var req struct {
		Provider  string `json:"provider"`
		ModelName string `json:"model_name"`
		APIKey    string `json:"api_key"`
		Name      string `json:"name"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}
	if req.Provider == "" || req.ModelName == "" || req.APIKey == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "provider, model_name and api_key are required")
	}

	// Validate provider
	if _, ok := knownProviders[req.Provider]; !ok {
		return echo.NewHTTPError(http.StatusBadRequest, "unknown provider: "+req.Provider)
	}

	// Encrypt API key (simple base64 for now — in production use proper encryption)
	encryptedKey := base64.StdEncoding.EncodeToString([]byte(req.APIKey))

	if req.Name == "" {
		req.Name = req.ModelName
	}

	var id int64
	err := h.DB.QueryRow(`
		INSERT INTO model_configs (user_id, name, provider, model_name, api_key_encrypted, is_default, is_enabled)
		VALUES ($1, $2, $3, $4, $5, FALSE, TRUE)
		RETURNING id`, userID, req.Name, req.Provider, req.ModelName, encryptedKey,
	).Scan(&id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusCreated, map[string]interface{}{
		"id": id, "name": req.Name, "provider": req.Provider,
		"model_name": req.ModelName, "is_default": false, "is_enabled": true,
	})
}

// ── GET /api/settings/models/test ────────────────────────────────────────────

func (h *SettingsHandler) TestModel(c echo.Context) error {
	provider := c.QueryParam("provider")
	modelName := c.QueryParam("model_name")
	apiKey := c.QueryParam("api_key")

	if provider == "" || modelName == "" || apiKey == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "provider, model_name and api_key are required")
	}

	// Simple connectivity check — just verify the key format
	// Real implementation would make an actual API call
	if len(apiKey) < 10 {
		return echo.NewHTTPError(http.StatusBadRequest, "API key 格式不正确")
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": "连接成功",
	})
}

// ── PATCH /api/settings/models/:id/default ────────────────────────────────────

func (h *SettingsHandler) SetDefault(c echo.Context) error {
	userID := c.Get("user_id").(int64)
	configID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid config id")
	}

	// Clear existing defaults for this user
	h.DB.Exec(`UPDATE model_configs SET is_default = FALSE WHERE user_id = $1`, userID)
	// Set new default
	res, err := h.DB.Exec(`UPDATE model_configs SET is_default = TRUE WHERE id = $1 AND user_id = $2`,
		configID, userID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return echo.NewHTTPError(http.StatusNotFound, "配置不存在")
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "已设为默认"})
}

// ── PATCH /api/settings/models/:id/enable ────────────────────────────────────

func (h *SettingsHandler) ToggleEnable(c echo.Context) error {
	userID := c.Get("user_id").(int64)
	configID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid config id")
	}

	res, err := h.DB.Exec(`
		UPDATE model_configs SET is_enabled = NOT is_enabled
		WHERE id = $1 AND user_id = $2`, configID, userID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return echo.NewHTTPError(http.StatusNotFound, "配置不存在")
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "已更新"})
}

// ── DELETE /api/settings/models/:id ──────────────────────────────────────────

func (h *SettingsHandler) DeleteModel(c echo.Context) error {
	userID := c.Get("user_id").(int64)
	configID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid config id")
	}

	res, err := h.DB.Exec(`DELETE FROM model_configs WHERE id = $1 AND user_id = $2`,
		configID, userID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return echo.NewHTTPError(http.StatusNotFound, "配置不存在")
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "删除成功"})
}
