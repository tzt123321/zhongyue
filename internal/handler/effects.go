package handler

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
)

type EffectsHandler struct {
	DB *sql.DB
}

// System presets matching the Vue player store
var systemPresets = []map[string]interface{}{
	{"id": "flat", "name": "平坦", "eq_low": 0, "eq_mid": 0, "eq_high": 0, "balance": 0, "dry_wet": 0.0, "room_size": 0.5, "damping": 0.5, "stereo_width": 1.0, "is_system": true},
	{"id": "bass", "name": "低音增强", "eq_low": 8, "eq_mid": 0, "eq_high": 2, "balance": 0, "dry_wet": 0.2, "room_size": 0.3, "damping": 0.6, "stereo_width": 0.8, "is_system": true},
	{"id": "vocal", "name": "人声增强", "eq_low": 2, "eq_mid": 6, "eq_high": 3, "balance": 0, "dry_wet": 0.4, "room_size": 0.5, "damping": 0.5, "stereo_width": 1.2, "is_system": true},
	{"id": "rock", "name": "摇滚", "eq_low": 5, "eq_mid": -1, "eq_high": 6, "balance": 0, "dry_wet": 0.3, "room_size": 0.6, "damping": 0.4, "stereo_width": 1.3, "is_system": true},
	{"id": "electronic", "name": "电子乐", "eq_low": 7, "eq_mid": 2, "eq_high": 5, "balance": 0, "dry_wet": 0.5, "room_size": 0.7, "damping": 0.3, "stereo_width": 1.5, "is_system": true},
}

// GET /api/effects/presets
func (h *EffectsHandler) ListPresets(c echo.Context) error {
	userID := getUserID(c)

	rows, err := h.DB.Query(`
		SELECT id, name, eq_low, eq_mid, eq_high, balance, dry_wet, room_size, damping, stereo_width
		FROM effect_presets WHERE user_id = $1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	defer rows.Close()

	userItems := []map[string]interface{}{}
	for rows.Next() {
		var id int64
		var name string
		var eqLow, eqMid, eqHigh, balance float64
		var dryWet, roomSize, damping, stereoWidth float64
		if err := rows.Scan(&id, &name, &eqLow, &eqMid, &eqHigh, &balance, &dryWet, &roomSize, &damping, &stereoWidth); err != nil {
			continue
		}
		userItems = append(userItems, map[string]interface{}{
			"id": id, "name": name,
			"eq_low": eqLow, "eq_mid": eqMid, "eq_high": eqHigh, "balance": balance,
			"dry_wet": dryWet, "room_size": roomSize, "damping": damping, "stereo_width": stereoWidth,
			"is_system": false,
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"system": systemPresets,
		"user":   userItems,
	})
}

// POST /api/effects/presets
func (h *EffectsHandler) CreatePreset(c echo.Context) error {
	userID := getUserID(c)

	var req struct {
		Name       string  `json:"name"`
		EqLow      float64 `json:"eq_low"`
		EqMid      float64 `json:"eq_mid"`
		EqHigh     float64 `json:"eq_high"`
		Balance    float64 `json:"balance"`
		DryWet     float64 `json:"dry_wet"`
		RoomSize   float64 `json:"room_size"`
		Damping    float64 `json:"damping"`
		StereoWidth float64 `json:"stereo_width"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name required")
	}

	var id int64
	err := h.DB.QueryRow(`
		INSERT INTO effect_presets (user_id, name, eq_low, eq_mid, eq_high, balance, dry_wet, room_size, damping, stereo_width)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id`,
		userID, req.Name, req.EqLow, req.EqMid, req.EqHigh, req.Balance,
		req.DryWet, req.RoomSize, req.Damping, req.StereoWidth,
	).Scan(&id)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusCreated, map[string]interface{}{
		"id": id, "name": req.Name,
		"eq_low": req.EqLow, "eq_mid": req.EqMid, "eq_high": req.EqHigh, "balance": req.Balance,
		"dry_wet": req.DryWet, "room_size": req.RoomSize, "damping": req.Damping, "stereo_width": req.StereoWidth,
		"is_system": false,
	})
}

// PUT /api/effects/presets/:id
func (h *EffectsHandler) UpdatePreset(c echo.Context) error {
	userID := getUserID(c)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	var req struct {
		Name       string  `json:"name"`
		EqLow      float64 `json:"eq_low"`
		EqMid      float64 `json:"eq_mid"`
		EqHigh     float64 `json:"eq_high"`
		Balance    float64 `json:"balance"`
		DryWet     float64 `json:"dry_wet"`
		RoomSize   float64 `json:"room_size"`
		Damping    float64 `json:"damping"`
		StereoWidth float64 `json:"stereo_width"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	res, err := h.DB.Exec(`
		UPDATE effect_presets SET name=$1, eq_low=$2, eq_mid=$3, eq_high=$4, balance=$5,
		dry_wet=$6, room_size=$7, damping=$8, stereo_width=$9
		WHERE id=$10 AND user_id=$11`,
		req.Name, req.EqLow, req.EqMid, req.EqHigh, req.Balance,
		req.DryWet, req.RoomSize, req.Damping, req.StereoWidth,
		id, userID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return echo.NewHTTPError(http.StatusNotFound, "preset not found")
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"id": id, "name": req.Name,
		"eq_low": req.EqLow, "eq_mid": req.EqMid, "eq_high": req.EqHigh, "balance": req.Balance,
		"dry_wet": req.DryWet, "room_size": req.RoomSize, "damping": req.Damping, "stereo_width": req.StereoWidth,
		"is_system": false,
	})
}

// DELETE /api/effects/presets/:id
func (h *EffectsHandler) DeletePreset(c echo.Context) error {
	userID := getUserID(c)
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid id")
	}

	res, err := h.DB.Exec(`DELETE FROM effect_presets WHERE id=$1 AND user_id=$2`, id, userID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return echo.NewHTTPError(http.StatusNotFound, "preset not found")
	}

	return c.NoContent(http.StatusNoContent)
}

// getUserID extracts user_id from echo.Context (set by auth middleware)
func getUserID(c echo.Context) int64 {
	if id, ok := c.Get("user_id").(int64); ok {
		return id
	}
	return 0
}
