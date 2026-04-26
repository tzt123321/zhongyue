package handler

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"net/http"
	"regexp"

	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"
	"zhongyue_refactored/internal/middleware"
	"zhongyue_refactored/internal/model"
)

type AuthHandler struct {
	DB      *sql.DB
	JWTMw   *middleware.JWTMiddleware
	JWTExp  int // minutes
}

type LoginRequest struct {
	Username string `form:"username"`
	Password string `form:"password"`
}

type RegisterRequest struct {
	Username  string `json:"username"`
	Email     string `json:"email,omitempty"`
	Password  string `json:"password"`
	InviteCode string `json:"invite_code"`
}

type ChangePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

type LoginResponse struct {
	AccessToken string              `json:"access_token"`
	TokenType   string              `json:"token_type"`
	User        model.UserResponse  `json:"user"`
}

// POST /api/auth/login
func (h *AuthHandler) Login(c echo.Context) error {
	var req LoginRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}

	var user model.User
	err := h.DB.QueryRow(
		`SELECT id, username, email, password_hash, is_admin, is_musician, is_active, is_banned, initial_password
		 FROM users WHERE username = $1`,
		req.Username,
	).Scan(&user.ID, &user.Username, &user.Email, &user.PasswordHash,
		&user.IsAdmin, &user.IsMusician, &user.IsActive, &user.IsBanned, &user.InitialPassword)

	if err == sql.ErrNoRows {
		return echo.NewHTTPError(http.StatusUnauthorized, "incorrect username or password")
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Verify password (bcrypt)
	if !verifyPassword(req.Password, user.PasswordHash) {
		return echo.NewHTTPError(http.StatusUnauthorized, "incorrect username or password")
	}

	if user.IsBanned {
		return echo.NewHTTPError(http.StatusForbidden, "账户已被封禁，请联系管理员")
	}
	if !user.IsActive {
		return echo.NewHTTPError(http.StatusForbidden, "账户尚未激活，请等待管理员激活")
	}

	token, err := h.JWTMw.GenerateTokenByID(user.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to generate token")
	}

	return c.JSON(http.StatusOK, LoginResponse{
		AccessToken: token,
		TokenType:   "bearer",
		User:        user.ToResponse(true),
	})
}

// POST /api/auth/register
func (h *AuthHandler) Register(c echo.Context) error {
	var req RegisterRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}

	// Validate password
	if err := validatePassword(req.Password, "密码"); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	// Validate invite code
	var invite model.InviteCode
	err := h.DB.QueryRow(
		`SELECT id, max_uses, uses FROM invite_codes WHERE code = $1`,
		req.InviteCode,
	).Scan(&invite.ID, &invite.MaxUses, &invite.Uses)
	if err == sql.ErrNoRows {
		return echo.NewHTTPError(http.StatusBadRequest, "邀请码无效")
	}
	if invite.MaxUses > 0 && invite.Uses >= invite.MaxUses {
		return echo.NewHTTPError(http.StatusBadRequest, "邀请码已用尽")
	}

	// Check username uniqueness
	var existingID int64
	err = h.DB.QueryRow(`SELECT id FROM users WHERE username = $1`, req.Username).Scan(&existingID)
	if err != sql.ErrNoRows {
		return echo.NewHTTPError(http.StatusBadRequest, "用户名已被注册")
	}

	// Hash password
	hash := hashPassword(req.Password)

	// Insert user
	var user model.User
	err = h.DB.QueryRow(
		`INSERT INTO users (username, email, password_hash, initial_password_hash, initial_password,
		 is_active, is_banned, is_muted, invite_code)
		 VALUES ($1, NULLIF($2,''), $3, $3, $4, false, false, false, $5)
		 RETURNING id, username, email, is_admin, is_musician, is_active, is_banned, initial_password`,
		req.Username, req.Email, hash, req.Password, req.InviteCode,
	).Scan(&user.ID, &user.Username, &user.Email, &user.IsAdmin, &user.IsMusician,
		&user.IsActive, &user.IsBanned, &user.InitialPassword)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Mark invite code used
	if invite.MaxUses > 0 {
		h.DB.Exec(`UPDATE invite_codes SET uses = uses + 1, used = CASE WHEN uses + 1 >= max_uses THEN true ELSE used END WHERE id = $1`, invite.ID)
	}

	return c.JSON(http.StatusCreated, user.ToResponse(true))
}

// GET /api/auth/me
func (h *AuthHandler) Me(c echo.Context) error {
	user := getUserFromContext(c, h.DB)
	if user == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid token")
	}
	return c.JSON(http.StatusOK, user.ToResponse(true))
}

// POST /api/auth/change-password
func (h *AuthHandler) ChangePassword(c echo.Context) error {
	user := getUserFromContext(c, h.DB)
	if user == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid token")
	}

	var req ChangePasswordRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}
	if err := validatePassword(req.NewPassword, "新密码"); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	// Verify old password
	if !verifyPassword(req.OldPassword, user.PasswordHash) {
		return echo.NewHTTPError(http.StatusBadRequest, "当前密码错误")
	}

	newHash := hashPassword(req.NewPassword)
	_, err := h.DB.Exec(`UPDATE users SET password_hash = $1, updated_at = NOW() WHERE id = $2`, newHash, user.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "密码修改成功"})
}

// ── POST /api/auth/generate-key ──────────────────────────────────────────────

func (h *AuthHandler) GenerateAPIKey(c echo.Context) error {
	user := getUserFromContext(c, h.DB)
	if user == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid token")
	}

	// Generate a random API key
	key := generateRandomKey(32)

	_, err := h.DB.Exec(
		`INSERT INTO api_keys (user_id, key, name, created_at) VALUES ($1, $2, $3, NOW())`,
		user.ID, key, "API Key",
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to create api key: "+err.Error())
	}

	return c.JSON(http.StatusCreated, map[string]interface{}{
		"api_key": key,
		"message": "API Key 生成成功，请妥善保管",
	})
}

func generateRandomKey(length int) string {
	b := make([]byte, (length+1)/2)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)[:length]
}

// ── Password helpers ──────────────────────────────────────────────────────────

// PASSWORD_RE matches Chinese/English chars + digits + underscore, min 6 chars
var passwordRE = regexp.MustCompile(`^[\S]{6,}$`)

func validatePassword(password, fieldName string) error {
	if !passwordRE.MatchString(password) {
		return echo.NewHTTPError(http.StatusBadRequest, fieldName+"至少6位，仅支持中英文、数字和下划线")
	}
	return nil
}

func hashPassword(password string) string {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return ""
	}
	return string(hash)
}

func verifyPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// getUserFromContext retrieves the authenticated user from echo context
func getUserFromContext(c echo.Context, db *sql.DB) *model.User {
	// The JWT middleware stores the user ID in context key "user_id"
	if id, ok := c.Get("user_id").(int64); ok {
		user := &model.User{}
		err := db.QueryRow(
			`SELECT id, username, email, password_hash, is_admin, is_musician, is_active, is_banned, is_muted, initial_password
			 FROM users WHERE id = $1`, id,
		).Scan(&user.ID, &user.Username, &user.Email, &user.PasswordHash,
			&user.IsAdmin, &user.IsMusician, &user.IsActive, &user.IsBanned, &user.IsMuted, &user.InitialPassword)
		if err == nil && user.IsActive {
			return user
		}
	}
	return nil
}
