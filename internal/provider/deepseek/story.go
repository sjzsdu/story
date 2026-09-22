package deepseek

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sjzsdu/story/internal/domain"
	"github.com/sjzsdu/story/internal/port"
	"github.com/sjzsdu/story/internal/templates"
)

// storyResponse 模型返回的定稿故事 JSON。
type storyResponse struct {
	Title   string `json:"title"`
	Dynasty string `json:"dynasty"`
	Source  string `json:"source"`
	Summary string `json:"summary"`
	Content string `json:"content"`
}

// GenerateCandidates 实现 port.StoryGenerator。
// 与 bailian 同款：模型只产出一篇定稿，包装为单元素切片返回。
func (c *Client) GenerateCandidates(ctx context.Context, req port.StoryRequest) ([]domain.StoryCandidate, error) {
	systemPrompt := templates.StorySystemPrompt
	userPrompt := templates.StoryUserPrompt(req.SeriesName, req.Dynasty, req.Topic, req.Brief)

	raw, err := c.chat(ctx, systemPrompt, userPrompt, 0.9, 16384)
	if err != nil {
		return nil, err
	}

	content := extractJSON(raw)
	var resp storyResponse
	if err := json.Unmarshal([]byte(content), &resp); err != nil {
		return nil, fmt.Errorf("解析故事 JSON: %w\n原始输出: %s", err, truncate(raw, 300))
	}

	story := domain.StoryCandidate{
		Index:   1,
		Title:   resp.Title,
		Dynasty: resp.Dynasty,
		Source:  resp.Source,
		Summary: resp.Summary,
		Content: resp.Content,
	}
	return []domain.StoryCandidate{story}, nil
}
