package bailian

import (
	"context"

	"github.com/sjzsdu/story/internal/domain"
	"github.com/sjzsdu/story/internal/port"
	"github.com/sjzsdu/story/internal/templates"
)

// storyResponse 模型返回的定稿故事 JSON（2026-09-19 起不再生成多个候选）。
type storyResponse struct {
	Title   string `json:"title"`
	Dynasty string `json:"dynasty"`
	Source  string `json:"source"`
	Summary string `json:"summary"`
	Content string `json:"content"`
}

// GenerateCandidates 实现 port.StoryGenerator。
// 产品上已取消「多候选人工选择」：模型只产出一篇定稿，这里包装为
// 单元素切片返回，保持 port 接口与状态结构不变。
func (c *Client) GenerateCandidates(ctx context.Context, req port.StoryRequest) ([]domain.StoryCandidate, error) {
	args := []string{
		"text", "chat",
		"--system", templates.StorySystemPrompt,
		"--message", templates.StoryUserPrompt(req.SeriesName, req.Dynasty, req.Topic),
		"--temperature", "0.9",
		// 定稿口播稿数百字，默认 4096 有截断风险。
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
	resp, err := decodeModelJSON[storyResponse](content)
	if err != nil {
		return nil, err
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
