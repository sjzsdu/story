package domain

import "time"

// Episode 一集视频 = 一次完整流水线。
type Episode struct {
	ID       string `json:"id"`
	SeriesID string `json:"series_id"`
	Number   int    `json:"number"`
	Title    string `json:"title"`
	// Topic 本集主题/切入点（可空，空则由 AI 在系列范围内自由命题）。
	Topic string        `json:"topic"`
	State PipelineState `json:"state"`
	// Refs 本集视觉参考（人物 + 跨镜重复场景），分镜阶段产出、可人工编辑；
	// 参考图按需手动生成。与系列级人物设定合并后注入分镜/produce（同名集级优先）。
	Refs []VisualRef `json:"refs,omitempty"`
	// WorkDir 该集媒体文件的工作目录。
	WorkDir   string    `json:"workdir"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
