package handler

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
)

type ForumHandler struct {
	DB *sql.DB
}

// ── Schemas ──────────────────────────────────────────────────────────────────

type PostIn struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

type PostOut struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	Username  string    `json:"username"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type CommentIn struct {
	Content string `json:"content"`
}

type CommentOut struct {
	ID        int64     `json:"id"`
	PostID    int64     `json:"post_id"`
	UserID    int64     `json:"user_id"`
	Username  string    `json:"username"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type PostListOut struct {
	Items []PostOut `json:"items"`
	Total int       `json:"total"`
	Page  int       `json:"page"`
	Limit int       `json:"limit"`
}

type NotifOut struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	Type      string    `json:"type"`
	Title     string    `json:"title"`
	Content   string    `json:"content,omitempty"`
	IsRead    bool      `json:"is_read"`
	CreatedAt time.Time `json:"created_at"`
}

// ── GET /api/forum/posts ─────────────────────────────────────────────────────

func (h *ForumHandler) ListPosts(c echo.Context) error {
	page, _ := strconv.Atoi(c.QueryParam("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	var total int
	h.DB.QueryRow(`SELECT COUNT(*) FROM forum_posts`).Scan(&total)

	rows, err := h.DB.Query(`
		SELECT p.id, p.user_id, p.title, p.content, p.created_at, u.username
		FROM forum_posts p
		LEFT JOIN users u ON p.user_id = u.id
		ORDER BY p.created_at DESC
		LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	defer rows.Close()

	items := []PostOut{}
	for rows.Next() {
		var p PostOut
		var createdAt time.Time
		if err := rows.Scan(&p.ID, &p.UserID, &p.Title, &p.Content, &createdAt, &p.Username); err != nil {
			continue
		}
		p.CreatedAt = createdAt
		items = append(items, p)
	}

	return c.JSON(http.StatusOK, PostListOut{Items: items, Total: total, Page: page, Limit: limit})
}

// ── POST /api/forum/posts ───────────────────────────────────────────────────

func (h *ForumHandler) CreatePost(c echo.Context) error {
	userID := c.Get("user_id").(int64)

	var isMuted bool
	h.DB.QueryRow(`SELECT is_muted FROM users WHERE id = $1`, userID).Scan(&isMuted)
	if isMuted {
		return echo.NewHTTPError(http.StatusForbidden, "您已被禁言，无法发帖")
	}

	var req PostIn
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}
	if req.Title == "" || req.Content == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "title and content are required")
	}

	var p PostOut
	var createdAt time.Time
	err := h.DB.QueryRow(`
		INSERT INTO forum_posts (user_id, title, content)
		VALUES ($1, $2, $3)
		RETURNING id, user_id, title, content, created_at`,
		userID, req.Title, req.Content,
	).Scan(&p.ID, &p.UserID, &p.Title, &p.Content, &createdAt)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	p.CreatedAt = createdAt

	var username string
	h.DB.QueryRow(`SELECT username FROM users WHERE id = $1`, userID).Scan(&username)
	p.Username = username

	return c.JSON(http.StatusCreated, p)
}

// ── DELETE /api/forum/posts/:id ────────────────────────────────────────────

func (h *ForumHandler) DeletePost(c echo.Context) error {
	userID := c.Get("user_id").(int64)
	postID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid post id")
	}

	var isAdmin bool
	h.DB.QueryRow(`SELECT is_admin FROM users WHERE id = $1`, userID).Scan(&isAdmin)

	var ownerID int64
	h.DB.QueryRow(`SELECT user_id FROM forum_posts WHERE id = $1`, postID).Scan(&ownerID)
	if ownerID == 0 {
		return echo.NewHTTPError(http.StatusNotFound, "帖子不存在")
	}
	if ownerID != userID && !isAdmin {
		return echo.NewHTTPError(http.StatusForbidden, "无权删除此帖子")
	}

	h.DB.Exec(`DELETE FROM forum_posts WHERE id = $1`, postID)
	return c.JSON(http.StatusOK, map[string]string{"message": "删除成功"})
}

// ── GET /api/forum/posts/:id/comments ─────────────────────────────────────

func (h *ForumHandler) ListComments(c echo.Context) error {
	postID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid post id")
	}

	rows, err := h.DB.Query(`
		SELECT c.id, c.post_id, c.user_id, c.content, c.created_at, u.username
		FROM forum_comments c
		LEFT JOIN users u ON c.user_id = u.id
		WHERE c.post_id = $1
		ORDER BY c.created_at ASC`, postID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	defer rows.Close()

	items := []CommentOut{}
	for rows.Next() {
		var c CommentOut
		var createdAt time.Time
		if err := rows.Scan(&c.ID, &c.PostID, &c.UserID, &c.Content, &createdAt, &c.Username); err != nil {
			continue
		}
		c.CreatedAt = createdAt
		items = append(items, c)
	}
	return c.JSON(http.StatusOK, items)
}

// ── POST /api/forum/posts/:id/comments ─────────────────────────────────────

func (h *ForumHandler) CreateComment(c echo.Context) error {
	userID := c.Get("user_id").(int64)
	postID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid post id")
	}

	var isMuted bool
	h.DB.QueryRow(`SELECT is_muted FROM users WHERE id = $1`, userID).Scan(&isMuted)
	if isMuted {
		return echo.NewHTTPError(http.StatusForbidden, "您已被禁言，无法评论")
	}

	var postOwnerID int64
	var postTitle string
	err = h.DB.QueryRow(`SELECT user_id, title FROM forum_posts WHERE id = $1`, postID).Scan(&postOwnerID, &postTitle)
	if err == sql.ErrNoRows {
		return echo.NewHTTPError(http.StatusNotFound, "帖子不存在")
	}

	var req CommentIn
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}
	if req.Content == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "content is required")
	}

	var commentID, commentUserID int64
	var commentContent string
	var commentCreatedAt time.Time
	err = h.DB.QueryRow(`
		INSERT INTO forum_comments (post_id, user_id, content)
		VALUES ($1, $2, $3)
		RETURNING id, user_id, content, created_at`,
		postID, userID, req.Content,
	).Scan(&commentID, &commentUserID, &commentContent, &commentCreatedAt)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	var username string
	h.DB.QueryRow(`SELECT username FROM users WHERE id = $1`, userID).Scan(&username)

	// Notify post author
	if postOwnerID != userID {
		h.DB.Exec(`
			INSERT INTO notifications (user_id, type, title, content)
			VALUES ($1, 'comment_reply', '有人回复了你的帖子', $2)`,
			postOwnerID, username+" 回复了「"+postTitle+"」")
	}

	return c.JSON(http.StatusCreated, CommentOut{
		ID:        commentID,
		PostID:    postID,
		UserID:    commentUserID,
		Username:  username,
		Content:   commentContent,
		CreatedAt: commentCreatedAt,
	})
}

// ── DELETE /api/forum/comments/:id ─────────────────────────────────────────

func (h *ForumHandler) DeleteComment(c echo.Context) error {
	userID := c.Get("user_id").(int64)
	commentID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid comment id")
	}

	var isAdmin bool
	h.DB.QueryRow(`SELECT is_admin FROM users WHERE id = $1`, userID).Scan(&isAdmin)

	var commentOwnerID int64
	h.DB.QueryRow(`SELECT user_id FROM forum_comments WHERE id = $1`, commentID).Scan(&commentOwnerID)
	if commentOwnerID == 0 {
		return echo.NewHTTPError(http.StatusNotFound, "评论不存在")
	}
	if commentOwnerID != userID && !isAdmin {
		return echo.NewHTTPError(http.StatusForbidden, "无权删除此评论")
	}

	h.DB.Exec(`DELETE FROM forum_comments WHERE id = $1`, commentID)
	return c.JSON(http.StatusOK, map[string]string{"message": "删除成功"})
}

// ── GET /api/forum/notifications ───────────────────────────────────────────

func (h *ForumHandler) GetNotifications(c echo.Context) error {
	userID := c.Get("user_id").(int64)

	rows, err := h.DB.Query(`
		SELECT id, user_id, type, title, content, is_read, created_at
		FROM notifications
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT 50`, userID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	defer rows.Close()

	items := []NotifOut{}
	for rows.Next() {
		var n NotifOut
		var content sql.NullString
		var createdAt time.Time
		if err := rows.Scan(&n.ID, &n.UserID, &n.Type, &n.Title, &content, &n.IsRead, &createdAt); err != nil {
			continue
		}
		if content.Valid {
			n.Content = content.String
		}
		n.CreatedAt = createdAt
		items = append(items, n)
	}
	return c.JSON(http.StatusOK, items)
}

// ── PUT /api/forum/notifications/:id/read ──────────────────────────────────

func (h *ForumHandler) MarkRead(c echo.Context) error {
	userID := c.Get("user_id").(int64)
	notifID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid notification id")
	}

	res, err := h.DB.Exec(`
		UPDATE notifications SET is_read = TRUE
		WHERE id = $1 AND user_id = $2`, notifID, userID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return echo.NewHTTPError(http.StatusNotFound, "通知不存在")
	}
	return c.JSON(http.StatusOK, map[string]string{"message": "已读"})
}

// ── PUT /api/forum/notifications/read-all ─────────────────────────────────

func (h *ForumHandler) MarkAllRead(c echo.Context) error {
	userID := c.Get("user_id").(int64)

	res, err := h.DB.Exec(`
		UPDATE notifications SET is_read = TRUE
		WHERE user_id = $1 AND is_read = FALSE`, userID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	rows, _ := res.RowsAffected()
	return c.JSON(http.StatusOK, map[string]string{
		"message": "已标记 " + strconv.FormatInt(rows, 10) + " 条为已读",
	})
}

// ── POST /api/forum/admin/announcements ────────────────────────────────────

func (h *ForumHandler) CreateAnnouncement(c echo.Context) error {
	userID := c.Get("user_id").(int64)

	var isAdmin bool
	h.DB.QueryRow(`SELECT is_admin FROM users WHERE id = $1`, userID).Scan(&isAdmin)
	if !isAdmin {
		return echo.NewHTTPError(http.StatusForbidden, "需要管理员权限")
	}

	var req struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}
	if req.Title == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "title is required")
	}

	rows, err := h.DB.Query(`SELECT id FROM users`)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	var firstID int64
	for rows.Next() {
		var uid int64
		rows.Scan(&uid)
		if _, err := h.DB.Exec(`
			INSERT INTO notifications (user_id, type, title, content)
			VALUES ($1, 'announcement', $2, $3)`, uid, req.Title, req.Content); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to create notification: "+err.Error())
		}
		if firstID == 0 {
			firstID = uid
		}
	}
	rows.Close()

	return c.JSON(http.StatusOK, map[string]interface{}{
		"id":       0,
		"user_id":  firstID,
		"type":     "announcement",
		"title":    req.Title,
		"content":  req.Content,
		"is_read": false,
	})
}
