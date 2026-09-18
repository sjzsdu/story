package port

import (
	"context"

	"github.com/sjzsdu/story/internal/domain"
)

// StoryboardRequest 分镜拆解请求。
type StoryboardRequest struct {
	Story      domain.StoryCandidate
	Dynasty    string // 朝代（视觉锚定）
	Ratio      string // 目标画面比例
	Resolution string // 目标分辨率
	VideoStyle string // 额外风格指令（可空）
}

// StoryboardPlanner 将完整故事拆为可生产的分镜脚本。
type StoryboardPlanner interface {
	PlanStoryboard(ctx context.Context, req StoryboardRequest) (*domain.Storyboard, error)
}
