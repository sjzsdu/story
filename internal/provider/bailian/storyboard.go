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
	Refs struct {
		Characters []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"characters"`
		Scenes []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"scenes"`
	} `json:"refs"`
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
			req.Brief,
			req.Characters,
			req.EpisodeRefs,
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
	refs := make([]domain.VisualRef, 0, len(resp.Refs.Characters)+len(resp.Refs.Scenes))
	for _, r := range resp.Refs.Characters {
		if r.Name == "" {
			continue
		}
		refs = append(refs, domain.VisualRef{
			Kind:        domain.RefKindCharacter,
			Name:        r.Name,
			Description: r.Description,
		})
	}
	for _, r := range resp.Refs.Scenes {
		if r.Name == "" {
			continue
		}
		refs = append(refs, domain.VisualRef{
			Kind:        domain.RefKindScene,
			Name:        r.Name,
			Description: r.Description,
		})
	}
	return &domain.Storyboard{Refs: refs, Scenes: resp.Scenes}, nil
}

func errEmpty(what string) error {
	return fmt.Errorf("bailian: %s 结果为空", what)
}
