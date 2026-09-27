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
	// EpisodeRefs 本集已有视觉参考（人物/场景，每项格式化一行）。重跑分镜时回灌，
	// 要求模型在 refs 输出中沿用原名原描述，人工编辑不被覆盖。
	EpisodeRefs []string
	// Brief 创作要求（templates.BoardBrief 组装：镜头数/时长档位 / 本集附加指令 / 本版附加要求）。
	// 空串＝全部默认，provider 不得因此改变 prompt（默认产出与历史行为逐字一致）。
	Brief string
	// Model 模型覆盖（系列级 text_model，分镜与故事共用文本模型）：空＝系统默认。
	Model string
}

// StoryboardPlanner 将完整故事拆为可生产的分镜脚本。
type StoryboardPlanner interface {
	PlanStoryboard(ctx context.Context, req StoryboardRequest) (*domain.Storyboard, error)
}
