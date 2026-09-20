package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sjzsdu/story/internal/domain"
	"github.com/sjzsdu/story/internal/port"
	"github.com/sjzsdu/story/testutil/mock"
)

type fixture struct {
	repo    *mock.Repo
	stories *mock.StoryGen
	boards  *mock.BoardPlanner
	videos  *mock.VideoGen
	speech  *mock.SpeechGen
	composr *mock.Composer
	planner *mock.SeriesPlanner
	images  *mock.ImageGen
	eng     *Engine
	epID    string
}

func setup(t *testing.T) *fixture {
	t.Helper()
	workRoot := t.TempDir()

	repo := mock.NewRepo()
	now := time.Now()
	series := &domain.Series{
		ID:      "guiguzi",
		Name:    "鬼谷子",
		Dynasty: "战国",
		Config: domain.SeriesConfig{
			Dynasty:        "战国",
			Ratio:          "9:16",
			Resolution:     "1080P",
			VisualMode:     domain.VisualModeVideo, // 既有用例固定 AI 视频模式
			TTSVoice:       "longtian_v3",
			MaxConcurrency: 2,
			MaxRetries:     2,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := repo.CreateSeries(context.Background(), series); err != nil {
		t.Fatal(err)
	}
	workDir := filepath.Join(workRoot, "guiguzi", "guiguzi-e01")
	for _, sub := range []string{"clips", "audio", "tmp", "output"} {
		if err := os.MkdirAll(filepath.Join(workDir, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	ep := &domain.Episode{
		ID:        "guiguzi-e01",
		SeriesID:  "guiguzi",
		Number:    1,
		Title:     "捭阖之术",
		State:     *domain.NewPipelineState(),
		WorkDir:   workDir,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := repo.CreateEpisode(context.Background(), ep); err != nil {
		t.Fatal(err)
	}

	f := &fixture{
		repo:    repo,
		stories: &mock.StoryGen{Candidates: sampleCandidates()},
		boards:  &mock.BoardPlanner{Storyboard: sampleStoryboard()},
		videos:  &mock.VideoGen{},
		speech:  &mock.SpeechGen{},
		composr: &mock.Composer{SceneDuration: 5},
		planner: &mock.SeriesPlanner{Result: port.SeriesPlanResult{
			Reply: "建议规划为 2 集。",
			Drafts: []domain.EpisodeDraft{
				{Title: "捭阖之术", Topic: "总论", Summary: "概述"},
				{Title: "合纵连横", Topic: "展开", Summary: "展开"},
			},
		}},
		images: &mock.ImageGen{},
	}
	f.eng = New(repo, f.stories, f.boards, f.videos, f.speech, f.composr, f.planner, f.images, workRoot, 2, 2, "longtian_v3", "沉稳")
	f.eng.runner.Backoff = time.Millisecond
	f.epID = ep.ID
	return f
}

func TestFullPipeline(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	candidates, err := f.eng.GenerateCandidates(ctx, f.epID)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("故事数 = %d，期望 1（生成即定稿）", len(candidates))
	}
	// 生成后应已自动完成选定，无需再执行 Pick。
	if ep0, _ := f.repo.GetEpisode(ctx, f.epID); ep0.State.Story == nil {
		t.Fatal("generate 后 Story 应已自动定稿")
	} else if ep0.State.Steps[domain.StepPick].Status != domain.StatusDone {
		t.Fatalf("generate 后 pick 应自动完成，实际 %s", ep0.State.Steps[domain.StepPick].Status)
	}
	if _, err := f.eng.PlanStoryboard(ctx, f.epID); err != nil {
		t.Fatalf("storyboard: %v", err)
	}
	if err := f.eng.Produce(ctx, f.epID); err != nil {
		t.Fatalf("produce: %v", err)
	}
	final, err := f.eng.Compose(ctx, f.epID)
	if err != nil {
		t.Fatalf("compose: %v", err)
	}
	if final == "" {
		t.Fatal("成片路径为空")
	}

	ep, _ := f.repo.GetEpisode(ctx, f.epID)
	if !ep.State.IsDone() {
		t.Fatal("流水线应处于完成状态")
	}
	for _, step := range domain.AllSteps() {
		if ep.State.Steps[step].Status != domain.StatusDone {
			t.Fatalf("步骤 %s 状态 = %s", step, ep.State.Steps[step].Status)
		}
	}
}

func TestProduceRetryThenResume(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	// 场景 1 的视频前两次失败，第三次成功。
	f.videos.FailFirst = 2

	if _, err := f.eng.GenerateCandidates(ctx, f.epID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.eng.PlanStoryboard(ctx, f.epID); err != nil {
		t.Fatal(err)
	}
	if err := f.eng.Produce(ctx, f.epID); err != nil {
		t.Fatalf("重试后应成功: %v", err)
	}
	if calls := f.videos.CallsCount(); calls != 6 {
		t.Fatalf("视频调用次数 = %d, 期望 6（4 场景 + 2 次重试）", calls)
	}

	// 再次 Produce：文件已存在，应全部复用而不重新调用。
	if err := f.eng.Produce(ctx, f.epID); err != nil {
		t.Fatalf("续跑: %v", err)
	}
	if calls := f.videos.CallsCount(); calls != 6 {
		t.Fatalf("续跑不应再生成, 调用次数 = %d", calls)
	}
	ep, _ := f.repo.GetEpisode(ctx, f.epID)
	for _, c := range ep.State.Clips {
		if !c.Skipped {
			t.Fatalf("镜头 #%d 应标记为复用", c.SceneID)
		}
	}
}

func TestProduceFailureMarksStepFailed(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	// 重试次数（系列配置 2 次重试 = 共 3 次尝试）用完仍失败。
	f.videos.FailFirst = 100

	if _, err := f.eng.GenerateCandidates(ctx, f.epID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.eng.PlanStoryboard(ctx, f.epID); err != nil {
		t.Fatal(err)
	}
	if err := f.eng.Produce(ctx, f.epID); err == nil {
		t.Fatal("应返回失败")
	}
	ep, _ := f.repo.GetEpisode(ctx, f.epID)
	if ep.State.Steps[domain.StepProduce].Status != domain.StatusFailed {
		t.Fatalf("produce 状态 = %s, 期望 failed", ep.State.Steps[domain.StepProduce].Status)
	}
	if ep.State.Steps[domain.StepProduce].Error == "" {
		t.Fatal("失败步骤应记录错误信息")
	}
}

func sampleCandidates() []domain.StoryCandidate {
	// 新流程每次只生成一篇定稿故事（单元素切片）。
	return []domain.StoryCandidate{
		{Index: 1, Title: "捭阖初试", Dynasty: "战国", Source: "《鬼谷子·捭阖》", Summary: "s1", Content: "故事正文一，画面感足够强。"},
	}
}

func sampleStoryboard() *domain.Storyboard {
	return &domain.Storyboard{Scenes: []domain.Scene{
		{ID: 1, VisualPrompt: "战国书房，竹简与青铜灯", Narration: "第一句旁白", DurationSec: 4, Camera: "远景"},
		{ID: 2, VisualPrompt: "鬼谷子盘膝而坐，深衣", Narration: "第二句旁白", DurationSec: 3, Camera: "近景"},
		{ID: 3, VisualPrompt: "列国宫殿，夯土高台", Narration: "第三句旁白", DurationSec: 5, Camera: "推镜"},
		{ID: 4, VisualPrompt: "夜色下的马车远去", Narration: "第四句旁白", DurationSec: 4, Camera: "远景拉远"},
	}}
}
