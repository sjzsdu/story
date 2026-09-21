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
	// §17：集工作目录下只有版本目录（各阶段产物落在 versions/<节点键>/）。
	if err := os.MkdirAll(filepath.Join(workDir, VersionsDirName), 0o755); err != nil {
		t.Fatal(err)
	}
	ep := domain.NewEpisode("guiguzi-e01", "guiguzi", 1, "捭阖之术", "", workDir)
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

// episodeOf 读取最新的集状态。
func episodeOf(t *testing.T, f *fixture) *domain.Episode {
	t.Helper()
	ep, err := f.repo.GetEpisode(context.Background(), f.epID)
	if err != nil {
		t.Fatal(err)
	}
	return ep
}

// activeStageNode 取活跃路径上指定阶段的节点，缺失即失败。
func activeStageNode(t *testing.T, f *fixture, stage domain.Stage) *domain.VersionNode {
	t.Helper()
	n := episodeOf(t, f).ActiveNodeOfStage(stage)
	if n == nil {
		t.Fatalf("活跃路径上没有「%s」节点", domain.StageLabel(stage))
	}
	return n
}

func TestFullPipeline(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	story, err := f.eng.GenerateStory(ctx, f.epID, DeriveOptions{})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	// 生成即定稿：story 节点直接带内容且已完成（旧流程的 pick 环节已移除）。
	if story.Stage != domain.StageStory || story.ParentID != "" {
		t.Fatalf("story 应为根节点: %+v", story)
	}
	if story.Story == nil || !story.Done() {
		t.Fatalf("generate 后 story 节点应已定稿: %+v", story)
	}

	board, err := f.eng.PlanStoryboard(ctx, f.epID, DeriveOptions{})
	if err != nil {
		t.Fatalf("storyboard: %v", err)
	}
	if board.ParentID != story.ID || !board.Done() {
		t.Fatalf("storyboard 节点异常: %+v", board)
	}

	media, err := f.eng.Produce(ctx, f.epID, DeriveOptions{})
	if err != nil {
		t.Fatalf("produce: %v", err)
	}
	if media.ParentID != board.ID || !media.Done() {
		t.Fatalf("media 节点异常: %+v", media)
	}

	final, err := f.eng.Compose(ctx, f.epID, DeriveOptions{})
	if err != nil {
		t.Fatalf("compose: %v", err)
	}
	if final == "" {
		t.Fatal("成片路径为空")
	}

	ep := episodeOf(t, f)
	if len(ep.Nodes) != 4 {
		t.Fatalf("版本树节点数 = %d，期望 4", len(ep.Nodes))
	}
	path := ep.ActivePath()
	if len(path) != 4 {
		t.Fatalf("活跃路径长度 = %d，期望 4", len(path))
	}
	for i, st := range domain.AllStages() {
		if path[i].Stage != st {
			t.Fatalf("活跃路径第 %d 段 = %s，期望 %s", i, path[i].Stage, st)
		}
		if !path[i].Done() {
			t.Fatalf("阶段 %s 未完成: %s", st, path[i].Status)
		}
	}
	if finalNode := ep.ActiveNodeOfStage(domain.StageFinal); finalNode == nil || len(finalNode.Outputs) == 0 {
		t.Fatal("final 节点应登记成片产物")
	}
	// 各阶段产物落在自己的版本目录里。
	for _, n := range ep.Nodes {
		if !strings.Contains(n.Dir, filepath.Join(VersionsDirName, n.ID)) {
			t.Fatalf("节点目录不符合约定: %s", n.Dir)
		}
	}
}

// TestSameDerivationReusesNode 验证内容寻址派生：同一组输入重复执行命中同一节点、
// 不再调用模型（零费用）；只有 --reroll 才开新版本。
func TestSameDerivationReusesNode(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	s1, err := f.eng.GenerateStory(ctx, f.epID, DeriveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	s2, err := f.eng.GenerateStory(ctx, f.epID, DeriveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if s2.ID != s1.ID {
		t.Fatalf("同派生输入应命中同一节点: %s → %s", s1.ID, s2.ID)
	}
	if f.stories.Calls != 1 {
		t.Fatalf("不应重复调用故事模型: %d", f.stories.Calls)
	}

	b1, err := f.eng.PlanStoryboard(ctx, f.epID, DeriveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	b2, err := f.eng.PlanStoryboard(ctx, f.epID, DeriveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if b2.ID != b1.ID || f.boards.Calls != 1 {
		t.Fatalf("分镜应复用: %s → %s, calls=%d", b1.ID, b2.ID, f.boards.Calls)
	}

	if _, err := f.eng.Produce(ctx, f.epID, DeriveOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.eng.Produce(ctx, f.epID, DeriveOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := f.videos.CallsCount(); got != 4 {
		t.Fatalf("续跑不应重复出画面, 视频调用 = %d", got)
	}
	if f.speech.Calls != 4 {
		t.Fatalf("续跑不应重复合成旁白, 调用 = %d", f.speech.Calls)
	}

	if _, err := f.eng.Compose(ctx, f.epID, DeriveOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.eng.Compose(ctx, f.epID, DeriveOptions{}); err != nil {
		t.Fatal(err)
	}
	if f.composr.ComposeCalls != 1 {
		t.Fatalf("成片不应重复合成, 调用 = %d", f.composr.ComposeCalls)
	}

	ep := episodeOf(t, f)
	if len(ep.Nodes) != 4 {
		t.Fatalf("重复执行不应新增节点，实际 %d 个", len(ep.Nodes))
	}

	// 换一版：同输入下 Attempt+1，得到新节点并重新调用模型。
	s3, err := f.eng.GenerateStory(ctx, f.epID, DeriveOptions{Reroll: true})
	if err != nil {
		t.Fatal(err)
	}
	if s3.ID == s1.ID || s3.Attempt != 1 {
		t.Fatalf("reroll 应开新版本: %+v", s3)
	}
	if f.stories.Calls != 2 {
		t.Fatalf("reroll 应重新调用故事模型: %d", f.stories.Calls)
	}
}

// TestRerollStoryboardIsolatesOldClips 是版本树的核心回归：
// 分镜换一版后，新分镜派生出新的 media 节点并重新生产，旧片段绝不被复用；
// 同时旧版本节点与磁盘产物原样保留。
func TestRerollStoryboardIsolatesOldClips(t *testing.T) {
	f := setup(t)
	prepareStoryboard(t, f)
	ctx := context.Background()

	if _, err := f.eng.Produce(ctx, f.epID, DeriveOptions{}); err != nil {
		t.Fatal(err)
	}
	ep := episodeOf(t, f)
	board1 := ep.ActiveNodeOfStage(domain.StageStoryboard)
	media1 := ep.ActiveNodeOfStage(domain.StageMedia)
	oldClip := filepath.Join(media1.Dir, "clips", "scene-01.mp4")
	if _, err := os.Stat(oldClip); err != nil {
		t.Fatalf("旧片段应已落盘: %v", err)
	}
	if got := f.videos.CallsCount(); got != 4 {
		t.Fatalf("首轮视频调用 = %d，期望 4", got)
	}

	board2, err := f.eng.PlanStoryboard(ctx, f.epID, DeriveOptions{Reroll: true})
	if err != nil {
		t.Fatal(err)
	}
	if board2.ID == board1.ID || board2.Attempt != 1 {
		t.Fatalf("分镜换一版应得到新节点: %+v", board2)
	}

	if _, err := f.eng.Produce(ctx, f.epID, DeriveOptions{}); err != nil {
		t.Fatal(err)
	}
	ep = episodeOf(t, f)
	media2 := ep.ActiveNodeOfStage(domain.StageMedia)
	if media2.ID == media1.ID {
		t.Fatal("新分镜必须派生出新的 media 节点（否则会复用旧分镜的片段）")
	}
	if media2.ParentID != board2.ID {
		t.Fatalf("新 media 节点的父应为新分镜: %s", media2.ParentID)
	}
	if got := f.videos.CallsCount(); got != 8 {
		t.Fatalf("新分镜必须重新生产 4 镜，视频调用 = %d，期望 8", got)
	}
	for _, c := range media2.Clips {
		if c.Skipped {
			t.Fatalf("新版本的镜头不应被标记为复用: %+v", c)
		}
	}

	// 旧分支完整保留：节点、目录、片段都在。
	if ep.NodeByID(media1.ID) == nil || ep.NodeByID(board1.ID) == nil {
		t.Fatal("旧分镜/旧画面节点应保留在树上")
	}
	if ep.NodeByID(media1.ID).ParentID != board1.ID {
		t.Fatal("旧 media 节点应仍挂在旧分镜下")
	}
	if _, err := os.Stat(oldClip); err != nil {
		t.Fatalf("旧版本片段应保留: %v", err)
	}
}

// TestProduceResumeAfterReroll 验证换一版后仍能断点续跑：
// 新版本内只生产指定镜头，之后再跑一次只补剩余镜头。
func TestProduceResumeAfterReroll(t *testing.T) {
	f := setup(t)
	prepareStoryboard(t, f)
	ctx := context.Background()

	if _, err := f.eng.Produce(ctx, f.epID, DeriveOptions{Scenes: []int{1, 2}}); err != nil {
		t.Fatal(err)
	}
	if got := f.videos.CallsCount(); got != 2 {
		t.Fatalf("视频调用 = %d，期望 2", got)
	}

	if _, err := f.eng.PlanStoryboard(ctx, f.epID, DeriveOptions{Reroll: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.eng.Produce(ctx, f.epID, DeriveOptions{Scenes: []int{1}}); err != nil {
		t.Fatal(err)
	}
	if got := f.videos.CallsCount(); got != 3 {
		t.Fatalf("新版本只应生产 1 镜，累计调用 = %d，期望 3", got)
	}

	// 续跑补齐新版本剩余 3 镜。
	if _, err := f.eng.Produce(ctx, f.epID, DeriveOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := f.videos.CallsCount(); got != 6 {
		t.Fatalf("续跑后累计调用 = %d，期望 6", got)
	}
	media := activeStageNode(t, f, domain.StageMedia)
	if !media.Done() {
		t.Fatalf("续跑后 media 节点应完成: %s / %s", media.Status, media.Error)
	}
}

func TestProduceRetryThenResume(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	// 场景 1 的视频前两次失败，第三次成功。
	f.videos.FailFirst = 2

	if _, err := f.eng.GenerateStory(ctx, f.epID, DeriveOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.eng.PlanStoryboard(ctx, f.epID, DeriveOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.eng.Produce(ctx, f.epID, DeriveOptions{}); err != nil {
		t.Fatalf("重试后应成功: %v", err)
	}
	if calls := f.videos.CallsCount(); calls != 6 {
		t.Fatalf("视频调用次数 = %d, 期望 6（4 场景 + 2 次重试）", calls)
	}

	// 再次 Produce：文件已存在，应全部复用而不重新调用。
	if _, err := f.eng.Produce(ctx, f.epID, DeriveOptions{}); err != nil {
		t.Fatalf("续跑: %v", err)
	}
	if calls := f.videos.CallsCount(); calls != 6 {
		t.Fatalf("续跑不应再生成, 调用次数 = %d", calls)
	}
	for _, c := range activeStageNode(t, f, domain.StageMedia).Clips {
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

	if _, err := f.eng.GenerateStory(ctx, f.epID, DeriveOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.eng.PlanStoryboard(ctx, f.epID, DeriveOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.eng.Produce(ctx, f.epID, DeriveOptions{}); err == nil {
		t.Fatal("应返回失败")
	}
	media := activeStageNode(t, f, domain.StageMedia)
	if media.Status != domain.NodeFailed {
		t.Fatalf("media 节点状态 = %s, 期望 failed", media.Status)
	}
	if media.Error == "" {
		t.Fatal("失败节点应记录错误信息")
	}
}

// prepareStoryboard 生成定稿故事与 4 镜分镜，作为 produce 系列用例的前置。
func prepareStoryboard(t *testing.T, f *fixture) {
	t.Helper()
	ctx := context.Background()
	if _, err := f.eng.GenerateStory(ctx, f.epID, DeriveOptions{}); err != nil {
		t.Fatalf("generate: %v", err)
	}
	if _, err := f.eng.PlanStoryboard(ctx, f.epID, DeriveOptions{}); err != nil {
		t.Fatalf("storyboard: %v", err)
	}
}

// TestProduceSkipsCompletedScenes 验证「绝不重复执行已成功的镜头」：
// 只缺 1 镜时重跑 produce，只补这 1 镜，其余直接复用且不产生旁白调用。
func TestProduceSkipsCompletedScenes(t *testing.T) {
	f := setup(t)
	prepareStoryboard(t, f)
	ctx := context.Background()

	if _, err := f.eng.Produce(ctx, f.epID, DeriveOptions{}); err != nil {
		t.Fatalf("produce: %v", err)
	}
	if got := f.videos.CallsCount(); got != 4 {
		t.Fatalf("首轮视频调用 = %d，期望 4", got)
	}
	speechBefore := f.speech.Calls

	// 模拟镜头 2 画面失败的现场：它的片段在版本目录里缺失。
	media := activeStageNode(t, f, domain.StageMedia)
	if err := os.Remove(filepath.Join(media.Dir, "clips", "scene-02.mp4")); err != nil {
		t.Fatal(err)
	}

	if _, err := f.eng.Produce(ctx, f.epID, DeriveOptions{}); err != nil {
		t.Fatalf("重试: %v", err)
	}
	if got := f.videos.CallsCount(); got != 5 {
		t.Fatalf("重试只应补 1 镜，视频调用 = %d，期望 5", got)
	}
	if f.speech.Calls != speechBefore {
		t.Fatalf("旁白不应重跑：%d → %d", speechBefore, f.speech.Calls)
	}
	reused := 0
	for _, c := range activeStageNode(t, f, domain.StageMedia).Clips {
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

	if _, err := f.eng.Produce(ctx, f.epID, DeriveOptions{}); err == nil {
		t.Fatal("画面全失败时应返回错误")
	}
	if f.speech.Calls != 4 {
		t.Fatalf("旁白合成次数 = %d，期望 4（画面失败不影响旁白，且重试时复用）", f.speech.Calls)
	}
	media := activeStageNode(t, f, domain.StageMedia)
	for _, c := range media.Clips {
		if c.Path != "" || !strings.Contains(c.Err, "画面") {
			t.Fatalf("镜头 #%d 画面结果异常: %+v", c.SceneID, c)
		}
	}
	for _, a := range media.Audios {
		if a.Path == "" {
			t.Fatalf("镜头 #%d 旁白应有产物: %+v", a.SceneID, a)
		}
	}
	if media.Status != domain.NodeFailed {
		t.Fatalf("media 节点状态 = %s，期望 failed", media.Status)
	}
}

// TestProduceScenesSubset 验证只生产指定镜头：未指定的镜头一概不碰，
// 且整集未齐时节点标注剩余镜头供续跑。
func TestProduceScenesSubset(t *testing.T) {
	f := setup(t)
	prepareStoryboard(t, f)
	ctx := context.Background()

	if _, err := f.eng.Produce(ctx, f.epID, DeriveOptions{Scenes: []int{2, 3}}); err != nil {
		t.Fatalf("本次请求的镜头都成功，不应返回错误: %v", err)
	}
	if got := f.videos.CallsCount(); got != 2 {
		t.Fatalf("视频调用 = %d，期望 2（只跑指定镜头）", got)
	}
	media := activeStageNode(t, f, domain.StageMedia)
	produced := map[int]bool{}
	for _, c := range media.Clips {
		if c.Path != "" {
			produced[c.SceneID] = true
		}
	}
	if !produced[2] || !produced[3] || produced[1] || produced[4] {
		t.Fatalf("生产范围不符: %v", produced)
	}
	if !strings.Contains(media.Error, "#1、#4") {
		t.Fatalf("错误应列出剩余镜头，实际: %s", media.Error)
	}

	// 指定不存在的镜头直接报错，不做任何生成。
	if _, err := f.eng.Produce(ctx, f.epID, DeriveOptions{Scenes: []int{99}}); err == nil || !strings.Contains(err.Error(), "分镜中没有镜头 #99") {
		t.Fatalf("越界镜头应报错，实际: %v", err)
	}
	if got := f.videos.CallsCount(); got != 2 {
		t.Fatalf("越界请求不应触发生成，视频调用 = %d", got)
	}

	// 续跑补齐剩余镜头。
	if _, err := f.eng.Produce(ctx, f.epID, DeriveOptions{}); err != nil {
		t.Fatalf("续跑: %v", err)
	}
	if got := f.videos.CallsCount(); got != 4 {
		t.Fatalf("续跑后视频调用 = %d，期望 4", got)
	}
}

// TestProduceCanceledKeepsProgress 验证手动停止：返回 context.Canceled、
// 节点标注「已手动停止」，且本次已完成的产物仍然落盘（可续跑）。
func TestProduceCanceledKeepsProgress(t *testing.T) {
	f := setup(t)
	prepareStoryboard(t, f)
	ctx := context.Background()

	// 先完成 2 镜，模拟停止前的既有进度。
	if _, err := f.eng.Produce(ctx, f.epID, DeriveOptions{Scenes: []int{1, 2}}); err != nil {
		t.Fatalf("子集生产: %v", err)
	}

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := f.eng.Produce(canceled, f.epID, DeriveOptions{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("取消后应返回 context.Canceled，实际 %v", err)
	}

	media := activeStageNode(t, f, domain.StageMedia)
	if media.Status != domain.NodeFailed || !strings.Contains(media.Error, "已手动停止") {
		t.Fatalf("节点状态 = %s / %q，期望 failed 且标注已手动停止", media.Status, media.Error)
	}
	if len(media.Clips) != 4 || len(media.Audios) != 4 {
		t.Fatalf("取消后应落盘完整结果：clips %d / audios %d", len(media.Clips), len(media.Audios))
	}
	for _, c := range media.Clips {
		if (c.SceneID == 1 || c.SceneID == 2) && c.Path == "" {
			t.Fatalf("已完成镜头 #%d 的产物应保留：%+v", c.SceneID, c)
		}
	}
}

// TestActivateNode 验证活跃指针切换：只改指针，不触碰产物。
func TestActivateNode(t *testing.T) {
	f := setup(t)
	prepareStoryboard(t, f)
	ctx := context.Background()
	if _, err := f.eng.Produce(ctx, f.epID, DeriveOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.eng.Compose(ctx, f.epID, DeriveOptions{}); err != nil {
		t.Fatal(err)
	}

	ep := episodeOf(t, f)
	story := ep.ActiveNodeOfStage(domain.StageStory)
	media := ep.ActiveNodeOfStage(domain.StageMedia)

	// 回到 story 节点：活跃路径只剩根。
	if err := f.eng.ActivateNode(ctx, f.epID, story.ID); err != nil {
		t.Fatal(err)
	}
	ep = episodeOf(t, f)
	if ep.ActiveNodeID != story.ID {
		t.Fatalf("活跃节点 = %s，期望 %s", ep.ActiveNodeID, story.ID)
	}
	if p := ep.ActivePath(); len(p) != 1 || p[0].ID != story.ID {
		t.Fatalf("活跃路径异常: %d 段", len(p))
	}

	// 切回 media：路径为 story → storyboard → media。
	if err := f.eng.ActivateNode(ctx, f.epID, media.ID); err != nil {
		t.Fatal(err)
	}
	ep = episodeOf(t, f)
	if p := ep.ActivePath(); len(p) != 3 || p[2].ID != media.ID {
		t.Fatalf("活跃路径异常: %d 段", len(p))
	}
	if len(ep.Nodes) != 4 {
		t.Fatalf("切换活跃节点不应改变版本树: %d 个节点", len(ep.Nodes))
	}

	if err := f.eng.ActivateNode(ctx, f.epID, "node-not-exist"); err == nil {
		t.Fatal("不存在的节点应报错")
	}
}

// TestDeleteNodeRemovesFiles 验证删除语义：级联删后代 + 删磁盘目录，
// 活跃节点落在被删子树内时指针回退到父节点。
func TestDeleteNodeRemovesFiles(t *testing.T) {
	f := setup(t)
	prepareStoryboard(t, f)
	ctx := context.Background()
	if _, err := f.eng.Produce(ctx, f.epID, DeriveOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.eng.Compose(ctx, f.epID, DeriveOptions{}); err != nil {
		t.Fatal(err)
	}

	ep := episodeOf(t, f)
	board := ep.ActiveNodeOfStage(domain.StageStoryboard)
	media := ep.ActiveNodeOfStage(domain.StageMedia)
	final := ep.ActiveNodeOfStage(domain.StageFinal)
	mediaDir, finalDir := media.Dir, final.Dir

	if err := f.eng.DeleteNode(ctx, f.epID, media.ID); err != nil {
		t.Fatal(err)
	}
	ep = episodeOf(t, f)
	if ep.NodeByID(media.ID) != nil {
		t.Fatal("media 节点应被删除")
	}
	if ep.NodeByID(final.ID) != nil {
		t.Fatal("final 应被级联删除")
	}
	if _, err := os.Stat(mediaDir); !os.IsNotExist(err) {
		t.Fatalf("media 目录应被删除: %v", err)
	}
	if _, err := os.Stat(finalDir); !os.IsNotExist(err) {
		t.Fatalf("final 目录应被删除: %v", err)
	}
	if ep.ActiveNodeID != board.ID {
		t.Fatalf("活跃指针应回退到父节点 %s，实际 %s", board.ID, ep.ActiveNodeID)
	}
	if ep.NodeByID(board.ID) == nil {
		t.Fatal("分镜节点应保留")
	}

	if err := f.eng.DeleteNode(ctx, f.epID, "node-not-exist"); err == nil {
		t.Fatal("不存在的节点应报错")
	}
}

// TestLegacyChainReusesMigratedNodes 验证旧集迁移的派生键一致性：
// 迁移出的节点必须与「现在重新执行该步骤」算出的键完全相同，
// 否则旧集续跑会把已付费的分镜/画面全部重做。
func TestLegacyChainReusesMigratedNodes(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	ep := episodeOf(t, f)
	series, err := f.repo.GetSeries(ctx, "guiguzi")
	if err != nil {
		t.Fatal(err)
	}

	// 旧布局：产物直接落在集工作目录根下。
	for _, sub := range []string{"clips", "audio", "tmp", "output"} {
		if err := os.MkdirAll(filepath.Join(ep.WorkDir, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	clip := filepath.Join(ep.WorkDir, "clips", "scene-01.mp4")
	if err := os.WriteFile(clip, []byte("old-mp4"), 0o644); err != nil {
		t.Fatal(err)
	}

	story := sampleCandidates()[0]
	nodes, active := f.eng.DeriveLegacyChain(ctx, ep, series, LegacyChainInput{
		Story:      &story,
		Storyboard: sampleStoryboard(),
		Clips:      []domain.MediaResult{{SceneID: 1, Path: clip, DurationSec: 5}},
		Audios:     []domain.MediaResult{{SceneID: 1, Path: filepath.Join(ep.WorkDir, "audio", "scene-01.mp3"), DurationSec: 5}},
	})
	if len(nodes) != 3 {
		t.Fatalf("节点数 = %d，期望 3（story/storyboard/media）", len(nodes))
	}
	ep.ActiveNodeID = active
	if err := f.repo.SaveEpisode(ctx, ep); err != nil {
		t.Fatal(err)
	}
	migratedMedia := activeStageNode(t, f, domain.StageMedia)
	migratedBoard := activeStageNode(t, f, domain.StageStoryboard)

	// 迁移后继续执行：必须命中迁移出来的同一节点（不新建、不重新拆分分镜）。
	if _, err := f.eng.Produce(ctx, f.epID, DeriveOptions{}); err != nil {
		t.Fatal(err)
	}
	ep = episodeOf(t, f)
	if len(ep.Nodes) != 3 {
		t.Fatalf("续跑不应新建节点: %d 个", len(ep.Nodes))
	}
	if f.boards.Calls != 0 {
		t.Fatalf("不应重新拆分分镜: %d", f.boards.Calls)
	}
	if got := activeStageNode(t, f, domain.StageMedia).ID; got != migratedMedia.ID {
		t.Fatalf("media 派生键与迁移结果不一致: %s ≠ %s", got, migratedMedia.ID)
	}
	if got := activeStageNode(t, f, domain.StageStoryboard).ID; got != migratedBoard.ID {
		t.Fatalf("storyboard 派生键与迁移结果不一致: %s ≠ %s", got, migratedBoard.ID)
	}
	if f.stories.Calls != 0 {
		t.Fatalf("不应重新生成故事: %d", f.stories.Calls)
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
