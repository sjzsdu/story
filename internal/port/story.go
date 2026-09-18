// Package port 定义流水线依赖的全部接口（端口）。
// engine 只依赖本包，绝不依赖任何具体 provider 实现。
package port

import (
	"context"

	"github.com/sjzsdu/story/internal/domain"
)

// StoryRequest 候选故事生成请求。
type StoryRequest struct {
	SeriesName string // 系列名，如「鬼谷子」
	Dynasty    string // 朝代锚定
	Topic      string // 本集主题/切入点（可空）
	Count      int    // 期望候选数量
}

// StoryGenerator 根据朝代/主题生成候选历史故事。
type StoryGenerator interface {
	GenerateCandidates(ctx context.Context, req StoryRequest) ([]domain.StoryCandidate, error)
}
