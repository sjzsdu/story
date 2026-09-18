package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/sjzsdu/story/internal/domain"
)

func TestSeriesEpisodeCRUD(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "story.db")
	store, err := Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	now := time.Now()
	se := &domain.Series{
		ID: "guiguzi", Name: "鬼谷子", Dynasty: "战国",
		Config: domain.SeriesConfig{
			Ratio: "9:16", Resolution: "1080P", MaxConcurrency: 3, MaxRetries: 3,
		},
		CreatedAt: now, UpdatedAt: now,
	}
	if err := store.CreateSeries(ctx, se); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetSeries(ctx, "guiguzi")
	if err != nil {
		t.Fatal(err)
	}
	if got.Config.Ratio != "9:16" || got.Config.MaxConcurrency != 3 {
		t.Fatalf("系列配置往返错误: %+v", got.Config)
	}

	ep := &domain.Episode{
		ID: "guiguzi-e01", SeriesID: "guiguzi", Number: 1, Title: "捭阖之术",
		State:     *domain.NewPipelineState(),
		WorkDir:   "/tmp/guiguzi-e01",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := store.CreateEpisode(ctx, ep); err != nil {
		t.Fatal(err)
	}

	n, err := store.NextEpisodeNumber(ctx, "guiguzi")
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("下一集序号 = %d, 期望 2", n)
	}

	// 更新状态并重新读取，验证 JSON 状态往返。
	ep.State.Current = domain.StepProduce
	ep.State.Mark(domain.StepGenerate, domain.StatusDone, "")
	if err := store.SaveEpisode(ctx, ep); err != nil {
		t.Fatal(err)
	}
	reloaded, err := store.GetEpisode(ctx, "guiguzi-e01")
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.State.Current != domain.StepProduce {
		t.Fatalf("状态往返失败: current=%s", reloaded.State.Current)
	}
	if reloaded.State.Steps[domain.StepGenerate].Status != domain.StatusDone {
		t.Fatal("步骤状态往返失败")
	}

	eps, err := store.ListEpisodes(ctx, "guiguzi")
	if err != nil {
		t.Fatal(err)
	}
	if len(eps) != 1 {
		t.Fatalf("集数量 = %d", len(eps))
	}

	if _, err := store.GetSeries(ctx, "missing"); err == nil {
		t.Fatal("不存在的系列应返回错误")
	}
}
