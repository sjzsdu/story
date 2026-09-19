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
	// Characters 系列人物设定集（每项为「姓名：外貌」一行）。非空时分镜 prompt 注入，
	// 要求同一人物跨镜头逐字复用外貌描述。
	Characters []string
}

// StoryboardPlanner 将完整故事拆为可生产的分镜脚本。
type StoryboardPlanner interface {
	PlanStoryboard(ctx context.Context, req StoryboardRequest) (*domain.Storyboard, error)
}
