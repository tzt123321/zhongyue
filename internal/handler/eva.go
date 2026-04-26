package handler

import (
	"net/http"
	"github.com/labstack/echo/v4"
)

type EvaHandler struct{}

func NewEvaHandler() *EvaHandler {
	return &EvaHandler{}
}

type EvaProfileResponse struct {
	Total       *TotalDuration  `json:"total"`
	SingerRank  []SingerItem   `json:"singerRank"`
	StyleList   []StyleItem    `json:"styleList"`
	Personality PersonalityInfo `json:"personality"`
	FriendRank  []FriendItem    `json:"friendRank"`
}

type TotalDuration struct {
	TotalDuration int64 `json:"totalDuration"`
}

type SingerItem struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type StyleItem struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

type PersonalityInfo struct {
	Name  string `json:"name"`
	Desc  string `json:"desc"`
	Score int    `json:"score"`
}

type FriendItem struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func (h *EvaHandler) GetProfile(c echo.Context) error {
	userID := c.Get("user_id")
	_ = userID // 未登录也可以查看公开画像

	// 返回模拟数据（根据用户听歌历史生成真实数据）
	profile := EvaProfileResponse{
		Total: &TotalDuration{
			TotalDuration: 36000, // 默认10小时
		},
		SingerRank: []SingerItem{
			{Name: " Rafael Krux", Count: 28},
			{Name: "Kevin MacLeod", Count: 15},
			{Name: "Ghostley", Count: 10},
		},
		StyleList: []StyleItem{
			{Name: "古典", Value: 30},
			{Name: "电子", Value: 25},
			{Name: "环境", Value: 20},
			{Name: "流行", Value: 15},
			{Name: "爵士", Value: 10},
		},
		Personality: PersonalityInfo{
			Name:  "多元随性听众",
			Desc:  "你听歌风格丰富多变，不被单一风格限制，随心情切换喜好，性格包容百变",
			Score: 85,
		},
		FriendRank: []FriendItem{},
	}

	return c.JSON(http.StatusOK, profile)
}
