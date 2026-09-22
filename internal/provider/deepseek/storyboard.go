package deepseek

import (
	"context"
	"encoding/json"
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
	systemPrompt := templates.StoryboardSystemPrompt
	userPrompt := templates.StoryboardUserPrompt(
		req.Story.Title,
		req.Dynasty,
		req.Story.Content,
		req.Ratio,
		req.Resolution,
		req.VideoStyle,
		req.Brief,
		req.Characters,
		req.EpisodeRefs,
	)

	raw, err := c.chat(ctx, systemPrompt, userPrompt, 0.7, 16384)
	if err != nil {
		return nil, err
	}

	content := extractJSON(raw)
	var resp storyboardResponse
	if err := json.Unmarshal([]byte(content), &resp); err != nil {
		return nil, fmt.Errorf("解析分镜 JSON: %w\n原始输出: %s", err, truncate(raw, 300))
	}
	if len(resp.Scenes) == 0 {
		return nil, fmt.Errorf("deepseek: 分镜结果为空")
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
