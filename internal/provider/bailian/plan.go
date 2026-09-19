package bailian

import (
	"context"
	"fmt"

	"github.com/sjzsdu/story/internal/domain"
	"github.com/sjzsdu/story/internal/port"
	"github.com/sjzsdu/story/internal/templates"
)

// planResponse 模型返回的分集策划 JSON。
type planResponse struct {
	Reply      string                    `json:"reply"`
	Drafts     []domain.EpisodeDraft     `json:"drafts"`
	Characters []domain.CharacterSetting `json:"characters"`
}

// PlanEpisodes 实现 port.SeriesPlanner。
func (c *Client) PlanEpisodes(ctx context.Context, req port.SeriesPlanRequest) (port.SeriesPlanResult, error) {
	if len(req.Messages) == 0 {
		return port.SeriesPlanResult{}, fmt.Errorf("策划对话消息为空")
	}

	// 系列背景 + 已有人物 + 已有集作为首轮用户消息的前缀（只加一次）。
	existing := make([]string, 0, len(req.Existing))
	for _, e := range req.Existing {
		line := fmt.Sprintf("第 %d 集《%s》", e.Number, e.Title)
		if e.Topic != "" {
			line += "：" + e.Topic
		}
		existing = append(existing, line)
	}
	charLines := make([]string, 0, len(req.Characters))
	for _, ch := range req.Characters {
		line := ch.Name + "（" + ch.Identity + "）：" + ch.Appearance
		if ch.Temperament != "" {
			line += "；气质：" + ch.Temperament
		}
		charLines = append(charLines, line)
	}
	header := templates.SeriesPlanContextPrompt(req.SeriesName, req.Dynasty, req.Description, existing, charLines)

	args := []string{
		"text", "chat",
		"--system", templates.SeriesPlanSystemPrompt,
		"--temperature", "0.85",
		// 大主题可能规划数十集（每集梗概 200 字），放大输出上限。
		"--max-tokens", "32000",
	}
	args = appendModel(args, c.TextModel)

	for i, m := range req.Messages {
		text := m.Content
		if i == 0 {
			text = header + "\n编辑的要求：" + text
		}
		args = append(args, "--message", m.Role+":"+text)
	}

	raw, err := c.runJSON(ctx, args...)
	if err != nil {
		return port.SeriesPlanResult{}, err
	}
	content, err := parseChatContent(raw)
	if err != nil {
		return port.SeriesPlanResult{}, err
	}
	resp, err := decodeModelJSON[planResponse](content)
	if err != nil {
		return port.SeriesPlanResult{}, err
	}
	if len(resp.Drafts) == 0 {
		return port.SeriesPlanResult{}, errEmpty("分集草案")
	}
	return port.SeriesPlanResult{Reply: resp.Reply, Drafts: resp.Drafts, Characters: resp.Characters}, nil
}
