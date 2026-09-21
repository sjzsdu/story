// Package port 定义流水线依赖的全部接口（端口）。
// engine 只依赖本包，绝不依赖任何具体 provider 实现。
package port

import (
	"context"

	"github.com/sjzsdu/story/internal/domain"
)

// StoryRequest 故事生成请求。
type StoryRequest struct {
	SeriesName string // 系列名，如「鬼谷子」
	Dynasty    string // 朝代锚定
	Topic      string // 本集主题/切入点（可空）
	// Brief 创作要求（templates.StoryBrief 组装：叙事风格/受众/篇幅/用户自定义指令）。
	// 空串＝全部默认，provider 不得因此改变 prompt（默认产出与历史行为逐字一致）。
	Brief string

	// Count 已废弃：2026-09-19 起取消多候选人工选择，每次只生成一篇定稿。
	// 字段保留仅为兼容旧调用，provider 忽略。
	Count int
}

// StoryGenerator 根据朝代/主题生成历史故事（现为单篇定稿，返回单元素切片）。
type StoryGenerator interface {
	GenerateCandidates(ctx context.Context, req StoryRequest) ([]domain.StoryCandidate, error)
}
