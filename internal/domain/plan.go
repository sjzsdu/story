package domain

import "time"

// EpisodeDraft 是 AI 分集策划产出的单集草案（采纳后批量创建为 Episode）。
type EpisodeDraft struct {
	// Title 本集标题（4-10 字为宜）。
	Title string `json:"title"`
	// Topic 本集主题 / 切入点（一句话）。
	Topic string `json:"topic"`
	// Summary 本集梗概（100-200 字，保证系列叙事弧线连贯）。
	Summary string `json:"summary"`
}

// PlanMessage 分集策划会话中的一条消息。
type PlanMessage struct {
	Role    string `json:"role"` // user / assistant
	Content string `json:"content"`
}

// PlanSession 某个系列的分集策划会话（一个系列当前仅保留一个会话，1:1）。
type PlanSession struct {
	SeriesID   string             `json:"series_id"`
	Messages   []PlanMessage      `json:"messages"`
	Drafts     []EpisodeDraft     `json:"drafts"`
	Characters []CharacterSetting `json:"characters"`
	CreatedAt  time.Time          `json:"created_at"`
	UpdatedAt  time.Time          `json:"updated_at"`
}
