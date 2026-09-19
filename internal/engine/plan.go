package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/sjzsdu/story/internal/domain"
	"github.com/sjzsdu/story/internal/port"
)

// MaxPlanDrafts 草案数量的技术上限（防模型失控；集数本身由主题体量决定）。
const MaxPlanDrafts = 100

// GetSeriesPlan 读取系列当前的策划会话；从未策划过时返回空会话（nil 安全）。
func (e *Engine) GetSeriesPlan(ctx context.Context, seriesID string) (*domain.PlanSession, error) {
	if _, err := e.repo.GetSeries(ctx, seriesID); err != nil {
		return nil, err
	}
	ps, err := e.repo.GetPlanSession(ctx, seriesID)
	if err != nil {
		if errors.Is(err, port.ErrNotFound) {
			return &domain.PlanSession{SeriesID: seriesID, Messages: []domain.PlanMessage{}, Drafts: []domain.EpisodeDraft{}}, nil
		}
		return nil, err
	}
	ensurePlanSlices(ps)
	return ps, nil
}

// ChatSeriesPlan 追加一条用户消息，调用 AI 规划/修订分集草案，并持久化整段会话。
func (e *Engine) ChatSeriesPlan(ctx context.Context, seriesID, userText string) (*domain.PlanSession, error) {
	userText = strings.TrimSpace(userText)
	if userText == "" {
		return nil, fmt.Errorf("请先描述你对这一季分集的想法")
	}
	series, err := e.repo.GetSeries(ctx, seriesID)
	if err != nil {
		return nil, err
	}

	ps, err := e.repo.GetPlanSession(ctx, seriesID)
	if err != nil {
		if errors.Is(err, port.ErrNotFound) {
			ps = &domain.PlanSession{
				SeriesID: seriesID,
				Messages: []domain.PlanMessage{},
				Drafts:   []domain.EpisodeDraft{},
			}
		} else {
			return nil, err
		}
	}
	ensurePlanSlices(ps)
	ps.Messages = append(ps.Messages, domain.PlanMessage{Role: "user", Content: userText})

	eps, err := e.repo.ListEpisodes(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	existing := make([]port.ExistingEpisodeBrief, 0, len(eps))
	for _, ep := range eps {
		existing = append(existing, port.ExistingEpisodeBrief{Number: ep.Number, Title: ep.Title, Topic: ep.Topic})
	}

	if e.planner == nil {
		return nil, fmt.Errorf("未配置分集策划能力（SeriesPlanner）")
	}
	result, err := e.planner.PlanEpisodes(ctx, port.SeriesPlanRequest{
		SeriesName:  series.Name,
		Dynasty:     series.Config.Dynasty,
		Description: series.Description,
		Existing:    existing,
		Messages:    ps.Messages,
	})
	if err != nil {
		// 失败时移除本轮用户消息，避免把没得到回复的话钉进历史。
		ps.Messages = ps.Messages[:len(ps.Messages)-1]
		return nil, err
	}
	if err := ValidateDrafts(result.Drafts); err != nil {
		ps.Messages = ps.Messages[:len(ps.Messages)-1]
		return nil, err
	}

	reply := strings.TrimSpace(result.Reply)
	if reply == "" {
		reply = "已根据你的反馈更新分集草案。"
	}
	ps.Messages = append(ps.Messages, domain.PlanMessage{Role: "assistant", Content: reply})
	ps.Drafts = result.Drafts
	if err := e.repo.SavePlanSession(ctx, ps); err != nil {
		return nil, err
	}
	return ps, nil
}

// ResetSeriesPlan 清空系列的策划会话，重新开始讨论。
func (e *Engine) ResetSeriesPlan(ctx context.Context, seriesID string) error {
	if _, err := e.repo.GetSeries(ctx, seriesID); err != nil {
		return err
	}
	return e.repo.DeletePlanSession(ctx, seriesID)
}

// ValidateDrafts 校验分集草案：非空、标题非空、不超技术上限。
func ValidateDrafts(drafts []domain.EpisodeDraft) error {
	if len(drafts) == 0 {
		return fmt.Errorf("分集草案为空")
	}
	if len(drafts) > MaxPlanDrafts {
		return fmt.Errorf("分集草案 %d 集超出上限 %d", len(drafts), MaxPlanDrafts)
	}
	titles := make(map[string]bool, len(drafts))
	for i, d := range drafts {
		if strings.TrimSpace(d.Title) == "" {
			return fmt.Errorf("第 %d 集缺少标题", i+1)
		}
		if titles[d.Title] {
			return fmt.Errorf("分集标题重复：%s", d.Title)
		}
		titles[d.Title] = true
	}
	return nil
}

func ensurePlanSlices(ps *domain.PlanSession) {
	if ps.Messages == nil {
		ps.Messages = []domain.PlanMessage{}
	}
	if ps.Drafts == nil {
		ps.Drafts = []domain.EpisodeDraft{}
	}
}
