// Package engine 是流水线调度核心：只依赖 port 接口，不依赖任何具体 provider。
package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sjzsdu/story/internal/domain"
	"github.com/sjzsdu/story/internal/port"
)

// Engine 流水线引擎，编排各步骤并维护状态机。
type Engine struct {
	repo     port.Repository
	stories  port.StoryGenerator
	boards   port.StoryboardPlanner
	videos   port.VideoGenerator
	speech   port.SpeechSynthesizer
	composer port.VideoComposer

	runner      *Runner
	voice       string
	instruction string
}

// New 创建引擎。
func New(
	repo port.Repository,
	stories port.StoryGenerator,
	boards port.StoryboardPlanner,
	videos port.VideoGenerator,
	speech port.SpeechSynthesizer,
	composer port.VideoComposer,
	concurrency, retries int,
	voice, instruction string,
) *Engine {
	return &Engine{
		repo:        repo,
		stories:     stories,
		boards:      boards,
		videos:      videos,
		speech:      speech,
		composer:    composer,
		runner:      NewRunner(concurrency, retries),
		voice:       voice,
		instruction: instruction,
	}
}

// GenerateCandidates 步骤 1：生成候选故事。
func (e *Engine) GenerateCandidates(ctx context.Context, episodeID string) ([]domain.StoryCandidate, error) {
	ep, series, err := e.load(ctx, episodeID)
	if err != nil {
		return nil, err
	}

	ep.State.BeginAttempt(domain.StepGenerate)
	if err := e.repo.SaveEpisode(ctx, ep); err != nil {
		return nil, err
	}

	candidates, err := e.stories.GenerateCandidates(ctx, port.StoryRequest{
		SeriesName: series.Name,
		Dynasty:    series.Config.Dynasty,
		Topic:      ep.Topic,
		Count:      5,
	})
	if err != nil {
		return nil, e.fail(ctx, ep, domain.StepGenerate, err)
	}
	if err := ValidateCandidates(candidates); err != nil {
		return nil, e.fail(ctx, ep, domain.StepGenerate, err)
	}
	for i := range candidates {
		candidates[i].Index = i + 1
	}

	ep.State.Candidates = candidates
	ep.State.Current = domain.StepPick
	ep.State.Mark(domain.StepGenerate, domain.StatusDone, "")
	if err := e.repo.SaveEpisode(ctx, ep); err != nil {
		return nil, err
	}
	return candidates, nil
}

// Pick 步骤 2：选定候选故事（index 从 1 开始）。
func (e *Engine) Pick(ctx context.Context, episodeID string, index int) error {
	ep, _, err := e.load(ctx, episodeID)
	if err != nil {
		return err
	}
	if err := ValidateSelection(ep.State.Candidates, index); err != nil {
		return e.fail(ctx, ep, domain.StepPick, err)
	}

	chosen := ep.State.Candidates[index-1]
	chosen.Index = index
	idx := index
	ep.State.Selected = &idx
	ep.State.Story = &chosen
	ep.State.Current = domain.StepStoryboard
	ep.State.Mark(domain.StepPick, domain.StatusDone, "")
	if err := e.writeReviewCopy(ep); err != nil {
		return err
	}
	return e.repo.SaveEpisode(ctx, ep)
}

// PlanStoryboard 步骤 3：拆分分镜。
func (e *Engine) PlanStoryboard(ctx context.Context, episodeID string) (*domain.Storyboard, error) {
	ep, series, err := e.load(ctx, episodeID)
	if err != nil {
		return nil, err
	}
	if ep.State.Story == nil {
		return nil, e.fail(ctx, ep, domain.StepStoryboard, fmt.Errorf("尚未选定故事，请先执行 pick"))
	}

	ep.State.BeginAttempt(domain.StepStoryboard)
	if err := e.repo.SaveEpisode(ctx, ep); err != nil {
		return nil, err
	}

	sb, err := e.boards.PlanStoryboard(ctx, port.StoryboardRequest{
		Story:      *ep.State.Story,
		Dynasty:    firstNonEmpty(series.Config.Dynasty, ep.State.Story.Dynasty),
		Ratio:      series.Config.Ratio,
		Resolution: series.Config.Resolution,
		VideoStyle: series.Config.VideoStyle,
	})
	if err != nil {
		return nil, e.fail(ctx, ep, domain.StepStoryboard, err)
	}
	if err := ValidateStoryboard(sb); err != nil {
		return nil, e.fail(ctx, ep, domain.StepStoryboard, err)
	}
	for i := range sb.Scenes {
		sb.Scenes[i].ID = i + 1
	}

	ep.State.Storyboard = sb
	ep.State.Current = domain.StepProduce
	ep.State.Mark(domain.StepStoryboard, domain.StatusDone, "")
	if err := e.writeReviewCopy(ep); err != nil {
		return nil, err
	}
	if err := e.repo.SaveEpisode(ctx, ep); err != nil {
		return nil, err
	}
	return sb, nil
}

// Produce 步骤 4：并发生成每个镜头的视频片段与旁白音频（支持断点续跑）。
func (e *Engine) Produce(ctx context.Context, episodeID string) error {
	ep, series, err := e.load(ctx, episodeID)
	if err != nil {
		return err
	}
	if ep.State.Storyboard == nil {
		return fmt.Errorf("尚未生成分镜，请先执行 storyboard")
	}

	clipsDir := filepath.Join(ep.WorkDir, "clips")
	audioDir := filepath.Join(ep.WorkDir, "audio")
	if err := os.MkdirAll(clipsDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(audioDir, 0o755); err != nil {
		return err
	}

	ep.State.BeginAttempt(domain.StepProduce)
	if err := e.repo.SaveEpisode(ctx, ep); err != nil {
		return err
	}

	scenes := ep.State.Storyboard.Scenes
	tasks := make([]Task, len(scenes))
	clipResults := make([]domain.MediaResult, len(scenes))
	audioResults := make([]domain.MediaResult, len(scenes))

	for i, sc := range scenes {
		i, sc := i, sc
		clipPath := filepath.Join(clipsDir, fmt.Sprintf("scene-%02d.mp4", sc.ID))
		audioPath := filepath.Join(audioDir, fmt.Sprintf("scene-%02d.mp3", sc.ID))

		tasks[i] = Task{
			Index: i,
			Name:  fmt.Sprintf("scene-%02d", sc.ID),
			Fn: func(ctx context.Context) error {
				// 视频（已存在则续跑复用）
				if reusable(clipPath) {
					dur, _ := e.composer.ProbeDuration(ctx, clipPath)
					clipResults[i] = domain.MediaResult{SceneID: sc.ID, Path: clipPath, DurationSec: dur, Skipped: true}
				} else {
					if _, err := e.videos.GenerateClip(ctx, port.ClipRequest{
						OutPath:     clipPath,
						Prompt:      buildVideoPrompt(sc),
						DurationSec: sc.DurationSec,
						Ratio:       series.Config.Ratio,
						Resolution:  series.Config.Resolution,
						Watermark:   true,
					}); err != nil {
						return fmt.Errorf("视频生成: %w", err)
					}
					dur, err := e.composer.ProbeDuration(ctx, clipPath)
					if err != nil {
						return fmt.Errorf("视频验收探测: %w", err)
					}
					clipResults[i] = domain.MediaResult{SceneID: sc.ID, Path: clipPath, DurationSec: dur}
				}

				// 旁白（已存在则续跑复用）
				if reusable(audioPath) {
					dur, _ := e.composer.ProbeDuration(ctx, audioPath)
					audioResults[i] = domain.MediaResult{SceneID: sc.ID, Path: audioPath, DurationSec: dur, Skipped: true}
				} else {
					if _, err := e.speech.Synthesize(ctx, port.SpeechRequest{
						OutPath:     audioPath,
						Text:        sc.Narration,
						Voice:       firstNonEmpty(series.Config.TTSVoice, e.voice),
						Instruction: firstNonEmpty(series.Config.TTSInstruction, e.instruction),
						Format:      "mp3",
					}); err != nil {
						return fmt.Errorf("旁白合成: %w", err)
					}
					dur, err := e.composer.ProbeDuration(ctx, audioPath)
					if err != nil {
						return fmt.Errorf("音频验收探测: %w", err)
					}
					audioResults[i] = domain.MediaResult{SceneID: sc.ID, Path: audioPath, DurationSec: dur}
				}
				return nil
			},
		}
	}

	batch := e.batchFor(series)
	results := batch.RunBatch(ctx, tasks)

	// 失败的镜头补登记信息，保证状态完整。
	for _, r := range CollectFailures(results) {
		sc := scenes[r.Index]
		if clipResults[r.Index].Path == "" {
			clipResults[r.Index] = domain.MediaResult{SceneID: sc.ID, Err: r.Err.Error()}
		}
		if audioResults[r.Index].Path == "" {
			audioResults[r.Index] = domain.MediaResult{SceneID: sc.ID, Err: r.Err.Error()}
		}
	}
	ep.State.Clips = clipResults
	ep.State.Audios = audioResults
	_ = e.repo.SaveEpisode(ctx, ep) // 先落盘部分进度

	if failed := CollectFailures(results); len(failed) > 0 {
		return e.fail(ctx, ep, domain.StepProduce, &FailedError{Items: failed})
	}
	if err := ValidateMedia(clipResults, audioResults, len(scenes)); err != nil {
		return e.fail(ctx, ep, domain.StepProduce, err)
	}

	ep.State.Current = domain.StepCompose
	ep.State.Mark(domain.StepProduce, domain.StatusDone, "")
	return e.repo.SaveEpisode(ctx, ep)
}

// Compose 步骤 5：归一化 + 混音拼接 + 烧录字幕，产出成片。
func (e *Engine) Compose(ctx context.Context, episodeID string) (string, error) {
	ep, series, err := e.load(ctx, episodeID)
	if err != nil {
		return "", err
	}
	if len(ep.State.Clips) == 0 || len(ep.State.Audios) == 0 {
		return "", fmt.Errorf("尚无媒体产物，请先执行 produce")
	}

	outDir := filepath.Join(ep.WorkDir, "output")
	tmpDir := filepath.Join(ep.WorkDir, "tmp")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", err
	}
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return "", err
	}

	clipsByScene := indexMedia(ep.State.Clips)
	audiosByScene := indexMedia(ep.State.Audios)
	tracks := make([]port.ClipTrack, 0, len(ep.State.Storyboard.Scenes))
	var expected float64
	for _, sc := range ep.State.Storyboard.Scenes {
		c, ok1 := clipsByScene[sc.ID]
		a, ok2 := audiosByScene[sc.ID]
		if !ok1 || !ok2 {
			return "", fmt.Errorf("镜头 #%d 缺少媒体产物", sc.ID)
		}
		tracks = append(tracks, port.ClipTrack{
			SceneID:   sc.ID,
			ClipPath:  c.Path,
			AudioPath: a.Path,
			Narration: sc.Narration,
		})
		// 成片镜头时长约等于视频与旁白的较长者。
		expected += maxFloat(c.DurationSec, a.DurationSec)
	}

	finalPath := filepath.Join(outDir, fmt.Sprintf("%s-%s.mp4", ep.ID, ratioSuffix(series.Config.Ratio)))
	ep.State.BeginAttempt(domain.StepCompose)
	if err := e.repo.SaveEpisode(ctx, ep); err != nil {
		return "", err
	}

	res, err := e.composer.Compose(ctx, port.ComposeRequest{
		WorkDir:       tmpDir,
		Tracks:        tracks,
		Ratio:         series.Config.Ratio,
		Resolution:    series.Config.Resolution,
		FinalPath:     finalPath,
		BurnSubtitles: true,
	})
	if err != nil {
		return "", e.fail(ctx, ep, domain.StepCompose, err)
	}
	if res.DurationSec <= 0 {
		return "", e.fail(ctx, ep, domain.StepCompose, fmt.Errorf("成片时长异常: %.2f", res.DurationSec))
	}
	if diff := res.DurationSec - expected; diff > 3 || diff < -3 {
		return "", e.fail(ctx, ep, domain.StepCompose, fmt.Errorf("成片时长 %.2f 与素材总时长 %.2f 偏差过大", res.DurationSec, expected))
	}

	ep.State.Outputs = appendIfMissing(ep.State.Outputs, res.FinalPath)
	ep.State.Current = domain.StepCompose
	ep.State.Mark(domain.StepCompose, domain.StatusDone, "")
	if err := e.repo.SaveEpisode(ctx, ep); err != nil {
		return "", err
	}
	return res.FinalPath, nil
}

// Export 从成片导出其他比例版本。
func (e *Engine) Export(ctx context.Context, episodeID, ratio string) (string, error) {
	ep, series, err := e.load(ctx, episodeID)
	if err != nil {
		return "", err
	}
	if len(ep.State.Outputs) == 0 {
		return "", fmt.Errorf("尚无成片，请先执行 compose")
	}
	src := ep.State.Outputs[0]
	dst := filepath.Join(ep.WorkDir, "output", fmt.Sprintf("%s-%s.mp4", ep.ID, ratioSuffix(ratio)))
	if err := e.composer.Export(ctx, port.ExportRequest{
		SrcPath:    src,
		DstPath:    dst,
		Ratio:      ratio,
		Resolution: series.Config.Resolution,
	}); err != nil {
		return "", err
	}
	ep.State.Outputs = appendIfMissing(ep.State.Outputs, dst)
	return dst, e.repo.SaveEpisode(ctx, ep)
}

// ---- 内部辅助 ----

func (e *Engine) batchFor(series *domain.Series) *Runner {
	c := e.runner.MaxConcurrency
	r := e.runner.MaxRetries
	if series.Config.MaxConcurrency > 0 {
		c = series.Config.MaxConcurrency
	}
	if series.Config.MaxRetries > 0 {
		r = series.Config.MaxRetries
	}
	runner := NewRunner(c, r)
	runner.Backoff = e.runner.Backoff // 保留引擎配置的退避基数
	return runner
}

func (e *Engine) load(ctx context.Context, episodeID string) (*domain.Episode, *domain.Series, error) {
	ep, err := e.repo.GetEpisode(ctx, episodeID)
	if err != nil {
		return nil, nil, fmt.Errorf("加载集 %s: %w", episodeID, err)
	}
	series, err := e.repo.GetSeries(ctx, ep.SeriesID)
	if err != nil {
		return nil, nil, fmt.Errorf("加载系列 %s: %w", ep.SeriesID, err)
	}
	return ep, series, nil
}

// fail 标记步骤失败、落盘错误信息并返回错误。
func (e *Engine) fail(ctx context.Context, ep *domain.Episode, step domain.StepName, cause error) error {
	ep.State.Mark(step, domain.StatusFailed, cause.Error())
	_ = e.repo.SaveEpisode(ctx, ep)
	return cause
}

// writeReviewCopy 把选定故事与分镜在集目录落一份人工审阅副本。
func (e *Engine) writeReviewCopy(ep *domain.Episode) error {
	if ep.State.Story != nil {
		md := fmt.Sprintf("# %s\n\n- 朝代：%s\n- 出处：%s\n\n%s\n",
			ep.State.Story.Title, ep.State.Story.Dynasty, ep.State.Story.Source, ep.State.Story.Content)
		if err := os.WriteFile(filepath.Join(ep.WorkDir, "story.md"), []byte(md), 0o644); err != nil {
			return err
		}
	}
	if ep.State.Storyboard != nil {
		b, err := json.MarshalIndent(ep.State.Storyboard, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(ep.WorkDir, "storyboard.json"), b, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func buildVideoPrompt(sc domain.Scene) string {
	s := strings.TrimSpace(sc.VisualPrompt)
	if sc.Camera != "" {
		s += "。运镜：" + sc.Camera
	}
	return s
}

func reusable(path string) bool {
	if fi, err := os.Stat(path); err == nil && fi.Size() > 0 {
		return true
	}
	return false
}

func indexMedia(ms []domain.MediaResult) map[int]domain.MediaResult {
	m := make(map[int]domain.MediaResult, len(ms))
	for _, v := range ms {
		m[v.SceneID] = v
	}
	return m
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func appendIfMissing(xs []string, x string) []string {
	for _, v := range xs {
		if v == x {
			return xs
		}
	}
	return append(xs, x)
}

func ratioSuffix(ratio string) string {
	return strings.ReplaceAll(ratio, ":", "x")
}
