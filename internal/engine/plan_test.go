package engine

import (
	"context"
	"testing"

	"github.com/sjzsdu/story/internal/domain"
)

func TestSeriesPlanChat(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	// 从未策划过时返回空会话。
	empty, err := f.eng.GetSeriesPlan(ctx, "guiguzi")
	if err != nil {
		t.Fatalf("get empty plan: %v", err)
	}
	if len(empty.Messages) != 0 || len(empty.Drafts) != 0 {
		t.Fatalf("空会话应无消息与草案，得到 %+v", empty)
	}

	// 首轮对话：消息落库、草案更新。
	ps, err := f.eng.ChatSeriesPlan(ctx, "guiguzi", "先规划两集")
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if len(ps.Messages) != 2 {
		t.Fatalf("应有 user+assistant 两条消息，得到 %d", len(ps.Messages))
	}
	if len(ps.Drafts) != 2 {
		t.Fatalf("应有 2 条草案，得到 %d", len(ps.Drafts))
	}
	if f.planner.Calls != 1 {
		t.Fatalf("planner 应被调用 1 次，得到 %d", f.planner.Calls)
	}

	// 再读一次：会话持久化。
	again, err := f.eng.GetSeriesPlan(ctx, "guiguzi")
	if err != nil {
		t.Fatalf("get plan: %v", err)
	}
	if len(again.Messages) != 2 {
		t.Fatalf("持久化消息数不符: %d", len(again.Messages))
	}

	// provider 失败：本轮用户消息不应留在历史里。
	f.planner.Err = context.DeadlineExceeded
	if _, err := f.eng.ChatSeriesPlan(ctx, "guiguzi", "再来一轮"); err == nil {
		t.Fatal("应返回错误")
	}
	after, _ := f.eng.GetSeriesPlan(ctx, "guiguzi")
	if len(after.Messages) != 2 {
		t.Fatalf("失败后消息应回滚为 2 条，得到 %d", len(after.Messages))
	}
	f.planner.Err = nil

	// 重置：回到空会话。
	if err := f.eng.ResetSeriesPlan(ctx, "guiguzi"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	reset, _ := f.eng.GetSeriesPlan(ctx, "guiguzi")
	if len(reset.Messages) != 0 || len(reset.Drafts) != 0 {
		t.Fatalf("重置后应为空会话，得到 %+v", reset)
	}
}

func TestValidateDrafts(t *testing.T) {
	good := []domain.EpisodeDraft{{Title: "甲"}, {Title: "乙"}}
	if err := ValidateDrafts(good); err != nil {
		t.Fatalf("合法草案被拒: %v", err)
	}
	if err := ValidateDrafts(nil); err == nil {
		t.Fatal("空草案应报错")
	}
	if err := ValidateDrafts([]domain.EpisodeDraft{{Title: "甲"}, {Title: "甲"}}); err == nil {
		t.Fatal("重复标题应报错")
	}
	if err := ValidateDrafts([]domain.EpisodeDraft{{Title: ""}}); err == nil {
		t.Fatal("空标题应报错")
	}
	big := make([]domain.EpisodeDraft, 101)
	for i := range big {
		big[i] = domain.EpisodeDraft{Title: string(rune('a'+i%26)) + string(rune(i))}
	}
	if err := ValidateDrafts(big); err == nil {
		t.Fatal("超过技术上限应报错")
	}
}
