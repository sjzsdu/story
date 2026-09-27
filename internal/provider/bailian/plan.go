package bailian

import (
	"context"
	"fmt"
	"strconv"
	"strings"

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

// planMaxTokens 分集策划的输出上限：草案每轮全量重输出（每集 title+topic+summary
// 约 200-400 token），100 集上限时全量草案约需 3 万 token 以上，32k 不够，取 64k。
// 若模型不支持该上限（DashScope 报参数错误），自动降级回旧值 32000 重试一次。
const (
	planMaxTokens    = 65536
	planMaxTokensOld = 32000
)

// planSystem 组装系统提示词：默认模板 + 本轮附加的系统级规划要求（可空）。
func planSystem(systemExtra string) string {
	extra := strings.TrimSpace(systemExtra)
	if extra == "" {
		return templates.SeriesPlanSystemPrompt
	}
	return templates.SeriesPlanSystemPrompt +
		"\n\n栏目编辑本轮附加的规划要求（与内置规则冲突时，在不违反硬性规则的前提下尽量遵循）：\n" + extra
}

// PlanEpisodes 实现 port.SeriesPlanner。
func (c *Client) PlanEpisodes(ctx context.Context, req port.SeriesPlanRequest) (port.SeriesPlanResult, error) {
	if len(req.Messages) == 0 {
		return port.SeriesPlanResult{}, fmt.Errorf("策划对话消息为空")
	}

	// 系列背景 + 长期规划要求 + 已有人物 + 已有集作为首轮用户消息的前缀（只加一次）。
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
	header := templates.SeriesPlanContextPrompt(req.SeriesName, req.Dynasty, req.Description, req.PlanningBrief, existing, charLines)

	args := []string{
		"text", "chat",
		"--system", planSystem(req.SystemExtra),
		"--temperature", "0.85",
		// 大主题可能规划数十集（每轮全量输出草案），放大输出上限。
		"--max-tokens", strconv.Itoa(planMaxTokens),
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
	if err != nil && planMaxTokens != planMaxTokensOld && isMaxTokensUnsupported(err) {
		// 模型不支持 64k 输出上限 → 降级回 32k 再试一次。
		for i, a := range args {
			if a == strconv.Itoa(planMaxTokens) {
				args[i] = strconv.Itoa(planMaxTokensOld)
			}
		}
		raw, err = c.runJSON(ctx, args...)
	}
	if err != nil {
		return port.SeriesPlanResult{}, err
	}
	content, err := parseChatContent(raw)
	if err != nil {
		return port.SeriesPlanResult{}, err
	}
	resp, err := decodeModelJSON[planResponse](content)
	if err != nil {
		return port.SeriesPlanResult{}, fmt.Errorf(
			"%w（草案可能被模型输出上限截断：可在对话里要求「先出前 30 集」分批续接，已有集会自动续编号）", err)
	}
	if len(resp.Drafts) == 0 {
		return port.SeriesPlanResult{}, errEmpty("分集草案")
	}
	return port.SeriesPlanResult{Reply: resp.Reply, Drafts: resp.Drafts, Characters: resp.Characters}, nil
}

// isMaxTokensUnsupported 识别「max_tokens 超出模型上限」类参数错误（降级重试的触发条件）。
func isMaxTokensUnsupported(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "max_tokens") || strings.Contains(msg, "max tokens") ||
		strings.Contains(msg, "invalidparameter") && strings.Contains(msg, "output")
}
