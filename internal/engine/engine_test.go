package engine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
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

// prepareStoryboard 生成定稿故事与 4 镜分镜，作为 produce 系列用例的前置。
func prepareStoryboard(t *testing.T, f *fixture) {
	t.Helper()
	ctx := context.Background()
	if _, err := f.eng.GenerateCandidates(ctx, f.epID); err != nil {
		t.Fatalf("generate: %v", err)
	}
	if _, err := f.eng.PlanStoryboard(ctx, f.epID); err != nil {
		t.Fatalf("storyboard: %v", err)
	}
}

// TestProduceSkipsCompletedScenes 验证「绝不重复执行已成功的镜头」：
// 只缺 1 镜时重跑 produce，只补这 1 镜，其余直接复用且不产生旁白调用。
func TestProduceSkipsCompletedScenes(t *testing.T) {
	f := setup(t)
	prepareStoryboard(t, f)
	ctx := context.Background()

	if err := f.eng.Produce(ctx, f.epID); err != nil {
		t.Fatalf("produce: %v", err)
	}
	if got := f.videos.CallsCount(); got != 4 {
		t.Fatalf("首轮视频调用 = %d，期望 4", got)
	}
	speechBefore := f.speech.Calls

	// 模拟镜头 2 画面失败的现场：它的片段在磁盘上缺失。
	ep, _ := f.repo.GetEpisode(ctx, f.epID)
	if err := os.Remove(filepath.Join(ep.WorkDir, "clips", "scene-02.mp4")); err != nil {
		t.Fatal(err)
	}

	if err := f.eng.Produce(ctx, f.epID); err != nil {
		t.Fatalf("重试: %v", err)
	}
	if got := f.videos.CallsCount(); got != 5 {
		t.Fatalf("重试只应补 1 镜，视频调用 = %d，期望 5", got)
	}
	if f.speech.Calls != speechBefore {
		t.Fatalf("旁白不应重跑：%d → %d", speechBefore, f.speech.Calls)
	}
	ep, _ = f.repo.GetEpisode(ctx, f.epID)
	reused := 0
	for _, c := range ep.State.Clips {
		if c.Skipped {
			reused++
		}
	}
	if reused != 3 {
		t.Fatalf("复用镜头数 = %d，期望 3", reused)
	}
}

// TestProduceClipFailureStillSynthesizesAudio 验证画面与旁白互相独立：
// 画面全失败时旁白照常合成（只合成一次），避免下次重试把已付费的旁白再跑一遍。
func TestProduceClipFailureStillSynthesizesAudio(t *testing.T) {
	f := setup(t)
	prepareStoryboard(t, f)
	ctx := context.Background()
	f.videos.FailFirst = 100 // 画面全部失败（重试后仍失败）

	if err := f.eng.Produce(ctx, f.epID); err == nil {
		t.Fatal("画面全失败时应返回错误")
	}
	if f.speech.Calls != 4 {
		t.Fatalf("旁白合成次数 = %d，期望 4（画面失败不影响旁白，且重试时复用）", f.speech.Calls)
	}
	ep, _ := f.repo.GetEpisode(ctx, f.epID)
	for _, c := range ep.State.Clips {
		if c.Path != "" || !strings.Contains(c.Err, "画面") {
			t.Fatalf("镜头 #%d 画面结果异常: %+v", c.SceneID, c)
		}
	}
	for _, a := range ep.State.Audios {
		if a.Path == "" {
			t.Fatalf("镜头 #%d 旁白应有产物: %+v", a.SceneID, a)
		}
	}
	if st := ep.State.Steps[domain.StepProduce]; st.Status != domain.StatusFailed {
		t.Fatalf("produce 状态 = %s，期望 failed", st.Status)
	}
}

// TestProduceScenesSubset 验证只生产指定镜头：未指定的镜头一概不碰，
// 且整集未齐时步骤标注剩余镜头供续跑。
func TestProduceScenesSubset(t *testing.T) {
	f := setup(t)
	prepareStoryboard(t, f)
	ctx := context.Background()

	if err := f.eng.ProduceScenes(ctx, f.epID, []int{2, 3}); err != nil {
		t.Fatalf("本次请求的镜头都成功，不应返回错误: %v", err)
	}
	if got := f.videos.CallsCount(); got != 2 {
		t.Fatalf("视频调用 = %d，期望 2（只跑指定镜头）", got)
	}
	ep, _ := f.repo.GetEpisode(ctx, f.epID)
	produced := map[int]bool{}
	for _, c := range ep.State.Clips {
		if c.Path != "" {
			produced[c.SceneID] = true
		}
	}
	if !produced[2] || !produced[3] || produced[1] || produced[4] {
		t.Fatalf("生产范围不符: %v", produced)
	}
	if msg := ep.State.Steps[domain.StepProduce].Error; !strings.Contains(msg, "#1、#4") {
		t.Fatalf("错误应列出剩余镜头，实际: %s", msg)
	}

	// 指定不存在的镜头直接报错，不做任何生成。
	if err := f.eng.ProduceScenes(ctx, f.epID, []int{99}); err == nil || !strings.Contains(err.Error(), "分镜中没有镜头 #99") {
		t.Fatalf("越界镜头应报错，实际: %v", err)
	}
	if got := f.videos.CallsCount(); got != 2 {
		t.Fatalf("越界请求不应触发生成，视频调用 = %d", got)
	}

	// 续跑补齐剩余镜头。
	if err := f.eng.Produce(ctx, f.epID); err != nil {
		t.Fatalf("续跑: %v", err)
	}
	if got := f.videos.CallsCount(); got != 4 {
		t.Fatalf("续跑后视频调用 = %d，期望 4", got)
	}
}

// TestProduceCanceledKeepsProgress 验证手动停止：返回 context.Canceled、
// 步骤标注「已手动停止」，且本次已完成的产物仍然落盘（可续跑）。
func TestProduceCanceledKeepsProgress(t *testing.T) {
	f := setup(t)
	prepareStoryboard(t, f)
	ctx := context.Background()

	// 先完成 2 镜，模拟停止前的既有进度。
	if err := f.eng.ProduceScenes(ctx, f.epID, []int{1, 2}); err != nil {
		t.Fatalf("子集生产: %v", err)
	}

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := f.eng.Produce(canceled, f.epID); !errors.Is(err, context.Canceled) {
		t.Fatalf("取消后应返回 context.Canceled，实际 %v", err)
	}

	ep, _ := f.repo.GetEpisode(ctx, f.epID)
	st := ep.State.Steps[domain.StepProduce]
	if st.Status != domain.StatusFailed || !strings.Contains(st.Error, "已手动停止") {
		t.Fatalf("步骤状态 = %s / %q，期望 failed 且标注已手动停止", st.Status, st.Error)
	}
	if len(ep.State.Clips) != 4 || len(ep.State.Audios) != 4 {
		t.Fatalf("取消后应落盘完整结果：clips %d / audios %d", len(ep.State.Clips), len(ep.State.Audios))
	}
	for _, c := range ep.State.Clips {
		if (c.SceneID == 1 || c.SceneID == 2) && c.Path == "" {
			t.Fatalf("已完成镜头 #%d 的产物应保留：%+v", c.SceneID, c)
		}
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
