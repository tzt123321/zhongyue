package handler

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"
)

type AdminHandler struct {
	DB *sql.DB
}

// ── GET /admin/users ─────────────────────────────────────────────────────────

func (h *AdminHandler) ListUsers(c echo.Context) error {
	rows, err := h.DB.Query(`
		SELECT id, username, email, is_admin, is_musician,
		       is_active, is_banned, is_muted, created_at
		FROM users ORDER BY id ASC`)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	defer rows.Close()

	users := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var username string
		var email sql.NullString
		var isAdmin, isMusician, isActive, isBanned, isMuted bool
		var createdAt time.Time
		if err := rows.Scan(&id, &username, &email, &isAdmin, &isMusician,
			&isActive, &isBanned, &isMuted, &createdAt); err != nil {
			continue
		}
		u := map[string]interface{}{
			"id":         id,
			"username":   username,
			"is_admin":   isAdmin,
			"is_musician": isMusician,
			"is_active":  isActive,
			"is_banned":  isBanned,
			"is_muted":   isMuted,
			"created_at": createdAt,
		}
		if email.Valid {
			u["email"] = email.String
		} else {
			u["email"] = nil
		}
		users = append(users, u)
	}
	return c.JSON(http.StatusOK, users)
}

// ── PATCH /admin/users/:id ──────────────────────────────────────────────────

func (h *AdminHandler) UpdateUser(c echo.Context) error {
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid user id")
	}

	body := make(map[string]interface{})
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid body")
	}

	// Build update query dynamically
	allowed := []string{"is_admin", "is_musician", "is_active", "is_banned", "is_muted"}
	setParts := []string{}
	args := []interface{}{}
	argIdx := 1

	for _, field := range allowed {
		if val, ok := body[field]; ok {
			setParts = append(setParts, field+" = $"+strconv.Itoa(argIdx))
			args = append(args, val)
			argIdx++
		}
	}

	if len(setParts) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "no valid fields to update")
	}

	args = append(args, userID)
	query := "UPDATE users SET " + joinStrings(setParts, ", ") +
		", updated_at = NOW() WHERE id = $" + strconv.Itoa(argIdx)

	res, err := h.DB.Exec(query, args...)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return echo.NewHTTPError(http.StatusNotFound, "用户不存在")
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "更新成功"})
}

// ── DELETE /admin/users/:id ─────────────────────────────────────────────────

func (h *AdminHandler) DeleteUser(c echo.Context) error {
	currentUserID := c.Get("user_id").(int64)
	targetID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid user id")
	}
	if targetID == currentUserID {
		return echo.NewHTTPError(http.StatusForbidden, "不能删除自己")
	}

	res, err := h.DB.Exec(`DELETE FROM users WHERE id = $1`, targetID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return echo.NewHTTPError(http.StatusNotFound, "用户不存在")
	}
	return c.JSON(http.StatusOK, map[string]string{"message": "删除成功"})
}

// ── PUT /admin/users/:id/mute ───────────────────────────────────────────────

func (h *AdminHandler) ToggleMute(c echo.Context) error {
	targetID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid user id")
	}
	var isMuted bool
	err = h.DB.QueryRow(`SELECT is_muted FROM users WHERE id = $1`, targetID).Scan(&isMuted)
	if err == sql.ErrNoRows {
		return echo.NewHTTPError(http.StatusNotFound, "用户不存在")
	}

	h.DB.Exec(`UPDATE users SET is_muted = $1, updated_at = NOW() WHERE id = $2`, !isMuted, targetID)
	return c.JSON(http.StatusOK, map[string]interface{}{"is_muted": !isMuted})
}

// ── POST /admin/users/:id/reset-password ─────────────────────────────────────

func (h *AdminHandler) ResetPassword(c echo.Context) error {
	targetID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid user id")
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := c.Bind(&req); err != nil || req.Password == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "password required")
	}
	if len(req.Password) < 6 {
		return echo.NewHTTPError(http.StatusBadRequest, "密码至少6位")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "密码加密失败")
	}

	res, err := h.DB.Exec(`UPDATE users SET password_hash = $1, updated_at = NOW() WHERE id = $2`,
		string(hash), targetID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return echo.NewHTTPError(http.StatusNotFound, "用户不存在")
	}
	return c.JSON(http.StatusOK, map[string]string{"message": "密码已重置"})
}

// ── GET /admin/invite-codes ─────────────────────────────────────────────────

func (h *AdminHandler) ListInviteCodes(c echo.Context) error {
	rows, err := h.DB.Query(`
		SELECT id, code, max_uses, uses, used, created_at
		FROM invite_codes ORDER BY id DESC`)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	defer rows.Close()

	codes := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var code string
		var maxUses, uses int
		var used bool
		var createdAt time.Time
		if err := rows.Scan(&id, &code, &maxUses, &uses, &used, &createdAt); err != nil {
			continue
		}
		codes = append(codes, map[string]interface{}{
			"id":         id,
			"code":       code,
			"max_uses":   maxUses,
			"uses":       uses,
			"used":       used,
			"created_at": createdAt,
		})
	}
	return c.JSON(http.StatusOK, codes)
}

// ── POST /admin/invite-codes ────────────────────────────────────────────────

func (h *AdminHandler) CreateInviteCode(c echo.Context) error {
	var req struct {
		Code    string `json:"code"`
		MaxUses int    `json:"max_uses"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}
	if req.MaxUses < 1 {
		req.MaxUses = 1
	}
	code := req.Code
	if code == "" {
		// Auto-generate a random code
		b := make([]byte, 4)
		if _, err := rand.Read(b); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to generate code")
		}
		code = fmt.Sprintf("%X%X%X%X", b[0], b[1], b[2], b[3])
	}

	var id int64
	err := h.DB.QueryRow(`
		INSERT INTO invite_codes (code, max_uses, uses, used)
		VALUES ($1, $2, 0, FALSE)
		RETURNING id`, code, req.MaxUses,
	).Scan(&id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "邀请码可能已存在: "+err.Error())
	}
	return c.JSON(http.StatusCreated, map[string]interface{}{
		"id": id, "code": code, "max_uses": req.MaxUses, "uses": 0, "used": false,
	})
}

// ── DELETE /admin/invite-codes/:id ──────────────────────────────────────────

func (h *AdminHandler) DeleteInviteCode(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}
	res, err := h.DB.Exec(`DELETE FROM invite_codes WHERE id = $1`, id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return echo.NewHTTPError(http.StatusNotFound, "邀请码不存在")
	}
	return c.JSON(http.StatusOK, map[string]string{"message": "删除成功"})
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func joinStrings(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result += sep + parts[i]
	}
	return result
}
