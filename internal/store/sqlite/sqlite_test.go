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
			// 创作控制参数走 config_json 整块 blob（加字段零迁移）。
			Creative: domain.CreativeStyle{
				Preset: "suspense", Narrative: "suspense", Audience: "teen",
				Length: "short", Motion: "strong", Instruction: "多用短句",
			},
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
	if got.Config.Creative != se.Config.Creative {
		t.Fatalf("创作设置往返错误: %+v", got.Config.Creative)
	}

	ep := domain.NewEpisode("guiguzi-e01", "guiguzi", 1, "捭阖之术", "", "/tmp/guiguzi-e01")
	ep.Instruction = "本集只讲一个晚上。"
	ep.CreatedAt = now
	ep.UpdatedAt = now
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

	// 更新版本树并重新读取，验证 nodes 与活跃指针往返。
	ep.Nodes = append(ep.Nodes,
		&domain.VersionNode{ID: "story-abc", Stage: domain.StageStory, ParentID: "", Attempt: 0, Status: domain.NodeDone, Dir: "/tmp/guiguzi-e01/versions/story-abc"},
		&domain.VersionNode{ID: "media-def", Stage: domain.StageMedia, ParentID: "story-abc", Attempt: 0, Status: domain.NodeRunning, Dir: "/tmp/guiguzi-e01/versions/media-def", Error: "画面: x"},
	)
	ep.ActiveNodeID = "media-def"
	if err := store.SaveEpisode(ctx, ep); err != nil {
		t.Fatal(err)
	}
	reloaded, err := store.GetEpisode(ctx, "guiguzi-e01")
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.Nodes) != 2 {
		t.Fatalf("节点数 = %d, 期望 2", len(reloaded.Nodes))
	}
	if reloaded.ActiveNodeID != "media-def" {
		t.Fatalf("活跃指针往返失败: %s", reloaded.ActiveNodeID)
	}
	reloadedMedia := reloaded.NodeByID("media-def")
	if reloadedMedia == nil || reloadedMedia.ParentID != "story-abc" || reloadedMedia.Status != domain.NodeRunning || reloadedMedia.Error != "画面: x" {
		t.Fatalf("media 节点往返失败: %+v", reloadedMedia)
	}
	if reloaded.Instruction != "本集只讲一个晚上。" {
		t.Fatalf("集级附加指令往返失败: %q", reloaded.Instruction)
	}

	// 改指令后写回（SaveEpisode 是唯一入口）。
	reloaded.Instruction = "改成只讲天亮那一刻。"
	if err := store.SaveEpisode(ctx, reloaded); err != nil {
		t.Fatal(err)
	}
	again, err := store.GetEpisode(ctx, "guiguzi-e01")
	if err != nil {
		t.Fatal(err)
	}
	if again.Instruction != "改成只讲天亮那一刻。" {
		t.Fatalf("集级附加指令更新失败: %q", again.Instruction)
	}

	eps, err := store.ListEpisodes(ctx, "guiguzi")
	if err != nil {
		t.Fatal(err)
	}
	if len(eps) != 1 {
		t.Fatalf("集数量 = %d", len(eps))
	}
	if eps[0].Instruction != again.Instruction {
		t.Fatalf("列表往返失败: %q", eps[0].Instruction)
	}

	if _, err := store.GetSeries(ctx, "missing"); err == nil {
		t.Fatal("不存在的系列应返回错误")
	}
}
