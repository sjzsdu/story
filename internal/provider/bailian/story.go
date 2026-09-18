package bailian

import (
	"context"

	"github.com/sjzsdu/story/internal/domain"
	"github.com/sjzsdu/story/internal/port"
	"github.com/sjzsdu/story/internal/templates"
)

// candidatesResponse 模型返回的候选故事 JSON。
type candidatesResponse struct {
	Candidates []domain.StoryCandidate `json:"candidates"`
}

// GenerateCandidates 实现 port.StoryGenerator。
func (c *Client) GenerateCandidates(ctx context.Context, req port.StoryRequest) ([]domain.StoryCandidate, error) {
	count := req.Count
	if count <= 0 {
		count = 5
	}
	args := []string{
		"text", "chat",
		"--system", templates.StorySystemPrompt,
		"--message", templates.StoryUserPrompt(req.SeriesName, req.Dynasty, req.Topic, count),
		"--temperature", "0.9",
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
	resp, err := decodeModelJSON[candidatesResponse](content)
	if err != nil {
		return nil, err
	}
	if len(resp.Candidates) == 0 {
		return nil, errEmpty("候选故事")
	}
	return resp.Candidates, nil
}
