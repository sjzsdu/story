package port

import (
	"context"

	"github.com/sjzsdu/story/internal/domain"
)

// ExistingEpisodeBrief 策划时提供给模型的「已存在集」摘要（用于追加规划，避免重复）。
type ExistingEpisodeBrief struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Topic  string `json:"topic"`
}

// SeriesPlanRequest 分集策划请求：携带系列信息、已有集与完整对话历史。
type SeriesPlanRequest struct {
	SeriesName  string
	Dynasty     string
	Description string
	Existing    []ExistingEpisodeBrief
	Messages    []domain.PlanMessage
}

// SeriesPlanResult 模型返回：给用户的本轮文字回应 + 全量最新分集草案。
type SeriesPlanResult struct {
	Reply  string
	Drafts []domain.EpisodeDraft
}

// SeriesPlanner 系列分集策划端口（由 bailian 等 AI provider 实现）。
type SeriesPlanner interface {
	PlanEpisodes(ctx context.Context, req SeriesPlanRequest) (SeriesPlanResult, error)
}
