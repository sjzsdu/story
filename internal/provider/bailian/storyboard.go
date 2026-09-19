package bailian

import (
	"context"
	"fmt"

	"github.com/sjzsdu/story/internal/domain"
	"github.com/sjzsdu/story/internal/port"
	"github.com/sjzsdu/story/internal/templates"
)

// storyboardResponse 模型返回的分镜 JSON。
type storyboardResponse struct {
	Scenes []domain.Scene `json:"scenes"`
}

// PlanStoryboard 实现 port.StoryboardPlanner。
func (c *Client) PlanStoryboard(ctx context.Context, req port.StoryboardRequest) (*domain.Storyboard, error) {
	args := []string{
		"text", "chat",
		"--system", templates.StoryboardSystemPrompt,
		"--message", templates.StoryboardUserPrompt(
			req.Story.Title,
			req.Dynasty,
			req.Story.Content,
			req.Ratio,
			req.Resolution,
			req.VideoStyle,
			req.Characters,
		),
		"--temperature", "0.7",
		// 6-12 个镜头 ×（visual_prompt+narration），默认 4096 有截断风险。
		"--max-tokens", "16384",
	}
	args = appendModel(args, c.TextModel)

	raw, err := c.runJSON(ctx, args...)
	if err != nil {
		return nil, err
	}
	content, err := parseChatContent(raw)
	if err != nil {
		return nil, err
	}
	resp, err := decodeModelJSON[storyboardResponse](content)
	if err != nil {
		return nil, err
	}
	if len(resp.Scenes) == 0 {
		return nil, errEmpty("分镜")
	}
	return &domain.Storyboard{Scenes: resp.Scenes}, nil
}

func errEmpty(what string) error {
	return fmt.Errorf("bailian: %s 结果为空", what)
}
