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
	"github.com/sjzsdu/story/internal/templates"
)

// Engine 流水线引擎，编排各步骤并维护状态机。
type Engine struct {
	repo     port.Repository
	stories  port.StoryGenerator
	boards   port.StoryboardPlanner
	videos   port.VideoGenerator
	speech   port.SpeechSynthesizer
	composer port.VideoComposer
	planner  port.SeriesPlanner
	images   port.ImageGenerator

	// projectsDir 媒体项目根目录（data/projects），用于系列级产物（定妆照）落盘。
	// 构造时归一为绝对路径：定妆照会以绝对路径写进 bl 命令与持久化数据，避免受服务进程 cwd 影响。
	projectsDir string

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
	planner port.SeriesPlanner,
	images port.ImageGenerator,
	projectsDir string,
	concurrency, retries int,
	voice, instruction string,
) *Engine {
	if projectsDir != "" {
		if abs, err := filepath.Abs(projectsDir); err == nil {
			projectsDir = abs
		}
	}
	return &Engine{
		repo:        repo,
		stories:     stories,
		boards:      boards,
		videos:      videos,
		speech:      speech,
		composer:    composer,
		planner:     planner,
		images:      images,
		projectsDir: projectsDir,
		runner:      NewRunner(concurrency, retries),
		voice:       voice,
		instruction: instruction,
	}
}

// GenerateCandidates 步骤 1：生成故事。
// 2026-09-19 起取消「3 个候选人工选择」：模型只产出一篇定稿，生成成功后
// 自动完成选定（StepPick 同步置 done），下一步直接进入分镜。不满意可重跑
// 本步骤覆盖；仍保留单元素 Candidates/Pick 结构以兼容状态与旧数据。
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

	chosen := candidates[0]
	chosen.Index = 1
	idx := 1
	ep.State.Candidates = candidates
	ep.State.Selected = &idx
	ep.State.Story = &chosen
	ep.State.Mark(domain.StepGenerate, domain.StatusDone, "")
	ep.State.Mark(domain.StepPick, domain.StatusDone, "")
	ep.State.Current = domain.StepStoryboard
	if err := e.writeReviewCopy(ep); err != nil {
		return nil, err
	}
	if err := e.repo.SaveEpisode(ctx, ep); err != nil {
		return nil, err
	}
	return candidates, nil
}

// Pick 手动选定故事（index 从 1 开始）。
// 自动流程下 GenerateCandidates 已完成选定，本方法保留用于兼容旧数据
// （历史上 generate 与 pick 分离的集）与 CLI/API。
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
		Story:       *ep.State.Story,
		Dynasty:     firstNonEmpty(series.Config.Dynasty, ep.State.Story.Dynasty),
		Ratio:       series.Config.Ratio,
		Resolution:  series.Config.Resolution,
		VideoStyle:  series.Config.VideoStyle,
		Characters:  characterLines(series.Characters),
		EpisodeRefs: visualRefLines(ep.Refs),
	})
	if err != nil {
		return nil, e.fail(ctx, ep, domain.StepStoryboard, err)
	}
	if err := ValidateStoryboard(sb); err != nil {
		return nil, e.fail(ctx, ep, domain.StepStoryboard, err)
	}
	normalizeDurations(sb)
	for i := range sb.Scenes {
		sb.Scenes[i].ID = i + 1
	}

	// 本集视觉参考（人物+重复场景）：延续旧参考图关联后落集级事实源，
	// 分镜审阅副本 storyboard.json 带同一快照。
	ep.Refs = preserveRefImages(ep.Refs, sb.Refs)
	sb.Refs = ep.Refs
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

// sceneMedia 单镜的产物规划信息：路径与当前完成情况（以磁盘文件为准）。
type sceneMedia struct {
	Scene domain.Scene
	// Index 镜头在分镜中的位置（0 起），供运镜缺省轮换使用。
	Index     int
	ClipPath  string
	AudioPath string
	PanelPath string
	// ClipOK/AudioOK 画面片段与旁白音频是否已存在且非空（存在即视为已完成，不再触碰）。
	ClipOK  bool
	AudioOK bool
}

// planScenes 按镜头顺序规划产物路径与完成情况。
func planScenes(ep *domain.Episode) []sceneMedia {
	scenes := ep.State.Storyboard.Scenes
	clipsDir := filepath.Join(ep.WorkDir, "clips")
	audioDir := filepath.Join(ep.WorkDir, "audio")
	panelsDir := filepath.Join(ep.WorkDir, PanelsDirName)
	plan := make([]sceneMedia, len(scenes))
	for i, sc := range scenes {
		m := sceneMedia{
			Scene:     sc,
			Index:     i,
			ClipPath:  filepath.Join(clipsDir, fmt.Sprintf("scene-%02d.mp4", sc.ID)),
			AudioPath: filepath.Join(audioDir, fmt.Sprintf("scene-%02d.mp3", sc.ID)),
			PanelPath: filepath.Join(panelsDir, fmt.Sprintf("scene-%02d.png", sc.ID)),
		}
		m.ClipOK = reusable(m.ClipPath)
		m.AudioOK = reusable(m.AudioPath)
		plan[i] = m
	}
	return plan
}

// Produce 步骤 4：生产每个镜头的画面片段与旁白音频（支持断点续跑）。
//
// 默认只对「未完成」的镜头建任务：画面与旁白都已存在的镜头连任务都不建，
// 不产生任何模型费用；未完成的镜头逐项复用已有产物（插画/片段/旁白），
// 因此重复执行本步骤等价于「只重试失败镜头」，绝不会重跑已成功的镜头。
//
// 返回 error 当且仅当本次请求生产的镜头里有失败；整集仍有未完成镜头时
// 步骤状态标记为 failed 并列出剩余镜头（便于再次续跑）。
func (e *Engine) Produce(ctx context.Context, episodeID string) error {
	return e.produce(ctx, episodeID, nil)
}

// ProduceScenes 只生产指定序号的镜头（单镜/多镜重试），其余镜头一概不碰
// （只在结果里登记磁盘上已有产物）。传空切片等同于 Produce。
func (e *Engine) ProduceScenes(ctx context.Context, episodeID string, sceneIDs []int) error {
	if len(sceneIDs) == 0 {
		return e.produce(ctx, episodeID, nil)
	}
	return e.produce(ctx, episodeID, sceneIDs)
}

func (e *Engine) produce(ctx context.Context, episodeID string, sceneIDs []int) error {
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
	// comic 小人书模式：每镜一张插画落 panels/，再本地渲染成 clips/ 片段。
	mode := domain.NormalizeVisualMode(series.Config.VisualMode)
	panelsDir := filepath.Join(ep.WorkDir, PanelsDirName)
	if mode == domain.VisualModeComic {
		if err := os.MkdirAll(panelsDir, 0o755); err != nil {
			return err
		}
	}

	plan := planScenes(ep)
	selected, err := selectScenes(plan, sceneIDs)
	if err != nil {
		return err
	}

	ep.State.BeginAttempt(domain.StepProduce)
	if err := e.repo.SaveEpisode(ctx, ep); err != nil {
		return err
	}

	n := len(plan)
	clipResults := make([]domain.MediaResult, n)
	audioResults := make([]domain.MediaResult, n)
	// 全片唯一视觉风格：每镜视频 prompt 统一追加风格锚句，不信任 LLM 在
	// visual_prompt 中自由书写画风词（防止镜头间写实/动漫漂移）。
	style := templates.MatchStyle(series.Config.VideoStyle)
	// 两级视觉参考合并（系列人物 + 本集人物/场景，同名集级优先），
	// video 模式据此匹配参考图；comic 模式文字约束已在分镜 prompt 生效。
	visualRefs := mergeVisualRefs(series, ep.Refs)

	tasks := make([]Task, 0, n)
	taskScene := make([]int, 0, n) // 任务下标 → plan 下标
	for i, m := range plan {
		if !selected[i] {
			// 本次不生产的镜头：只登记磁盘上已有产物供 compose 使用，不触碰生成。
			if m.ClipOK {
				dur, _ := e.composer.ProbeDuration(ctx, m.ClipPath)
				clipResults[i] = domain.MediaResult{SceneID: m.Scene.ID, Path: m.ClipPath, DurationSec: dur, Skipped: true}
			}
			if m.AudioOK {
				dur, _ := e.composer.ProbeDuration(ctx, m.AudioPath)
				audioResults[i] = domain.MediaResult{SceneID: m.Scene.ID, Path: m.AudioPath, DurationSec: dur, Skipped: true}
			}
			continue
		}
		idx := i
		sc := m.Scene
		taskScene = append(taskScene, i)
		tasks = append(tasks, Task{
			Index: len(tasks),
			Name:  fmt.Sprintf("scene-%02d", sc.ID),
			Fn: func(ctx context.Context) error {
				// 画面与旁白互不影响：画面失败也照常合成旁白，
				// 避免下次重试时把已付费的插画再生成一遍。
				clipRes, clipErr := e.produceClip(ctx, series, mode, visualRefs, style, m)
				if clipErr != nil {
					clipResults[idx] = domain.MediaResult{SceneID: sc.ID, Err: "画面: " + clipErr.Error()}
				} else {
					clipResults[idx] = clipRes
				}
				audioRes, audioErr := e.produceAudio(ctx, series, m)
				if audioErr != nil {
					audioResults[idx] = domain.MediaResult{SceneID: sc.ID, Err: "旁白: " + audioErr.Error()}
				} else {
					audioResults[idx] = audioRes
				}
				switch {
				case clipErr != nil && audioErr != nil:
					return fmt.Errorf("画面: %v；旁白: %v", clipErr, audioErr)
				case clipErr != nil:
					return fmt.Errorf("画面: %w", clipErr)
				case audioErr != nil:
					return fmt.Errorf("旁白: %w", audioErr)
				}
				return nil
			},
		})
	}

	batch := e.batchFor(series)
	results := batch.RunBatch(ctx, tasks)
	failed := CollectFailures(results)

	// 没跑到的任务（如被取消）补登记错误，保证状态完整。
	for _, r := range failed {
		i := taskScene[r.Index]
		sc := plan[i].Scene
		if clipResults[i].Path == "" && clipResults[i].Err == "" {
			clipResults[i] = domain.MediaResult{SceneID: sc.ID, Err: r.Err.Error()}
		}
		if audioResults[i].Path == "" && audioResults[i].Err == "" {
			audioResults[i] = domain.MediaResult{SceneID: sc.ID, Err: r.Err.Error()}
		}
	}
	ep.State.Clips = clipResults
	ep.State.Audios = audioResults

	// 手动停止：保留已完成的产物，便于之后直接续跑（不当作失败）。
	if ctx.Err() != nil {
		ep.State.Mark(domain.StepProduce, domain.StatusFailed, "已手动停止（已完成的产物已保留，可续跑）")
		_ = e.repo.SaveEpisode(context.WithoutCancel(ctx), ep)
		return ctx.Err()
	}
	_ = e.repo.SaveEpisode(ctx, ep) // 先落盘部分进度

	// 剩余未完成的镜头 = 本次失败的 + 本次未请求生产的。
	remaining := make([]int, 0, n)
	remainingSelected := make([]int, 0, n)
	for i, m := range plan {
		if clipResults[i].Path != "" && audioResults[i].Path != "" {
			continue
		}
		remaining = append(remaining, m.Scene.ID)
		if selected[i] {
			remainingSelected = append(remainingSelected, m.Scene.ID)
		}
	}

	if len(remainingSelected) > 0 {
		cause := error(&FailedError{Items: failed})
		if len(failed) == 0 {
			cause = fmt.Errorf("镜头 %s 生产未完成", joinSceneIDs(remainingSelected))
		}
		if len(remaining) > 0 {
			cause = fmt.Errorf("%w\n整集仍未完成的镜头：%s（再次执行生产只会重试这些）", cause, joinSceneIDs(remaining))
		}
		return e.fail(ctx, ep, domain.StepProduce, cause)
	}
	if len(remaining) > 0 {
		// 本次请求的镜头都成功了，但整集还没齐：不报错（用户请求的动作本身成功），
		// 仅把步骤标记为 failed 并列出剩余镜头，便于继续续跑。
		msg := fmt.Sprintf("本次生产的镜头已全部完成；整集仍有 %d 镜未完成：%s（再次执行生产只会重试这些）",
			len(remaining), joinSceneIDs(remaining))
		ep.State.Mark(domain.StepProduce, domain.StatusFailed, msg)
		return e.repo.SaveEpisode(ctx, ep)
	}

	if err := ValidateMedia(clipResults, audioResults, n); err != nil {
		return e.fail(ctx, ep, domain.StepProduce, err)
	}

	ep.State.Current = domain.StepCompose
	ep.State.Mark(domain.StepProduce, domain.StatusDone, "")
	return e.repo.SaveEpisode(ctx, ep)
}

// selectScenes 决定本次要生产哪些镜头：sceneIDs 为 nil 表示只选未完成的镜头；
// 显式给出时严格只选给定镜头（序号不存在直接报错）。
func selectScenes(plan []sceneMedia, sceneIDs []int) ([]bool, error) {
	selected := make([]bool, len(plan))
	if sceneIDs == nil {
		for i, m := range plan {
			selected[i] = !m.ClipOK || !m.AudioOK
		}
		return selected, nil
	}
	want := make(map[int]bool, len(sceneIDs))
	for _, id := range sceneIDs {
		want[id] = true
	}
	found := make(map[int]bool, len(sceneIDs))
	for i, m := range plan {
		if want[m.Scene.ID] {
			selected[i] = true
			found[m.Scene.ID] = true
		}
	}
	for id := range want {
		if !found[id] {
			return nil, fmt.Errorf("分镜中没有镜头 #%d", id)
		}
	}
	return selected, nil
}

// produceClip 生产单镜画面：已存在片段直接复用；comic 模式出插画后本地运镜渲染，
// video 模式调视频模型（有匹配参考图时走参考图路径）。
func (e *Engine) produceClip(ctx context.Context, series *domain.Series, mode string, visualRefs []domain.VisualRef, style templates.VisualStylePack, m sceneMedia) (domain.MediaResult, error) {
	sc := m.Scene
	if reusable(m.ClipPath) {
		dur, _ := e.composer.ProbeDuration(ctx, m.ClipPath)
		return domain.MediaResult{SceneID: sc.ID, Path: m.ClipPath, DurationSec: dur, Skipped: true}, nil
	}
	if mode == domain.VisualModeComic {
		// comic 小人书模式：AI 出插画（已出则续用）→ ffmpeg Ken Burns
		// 本地渲染成同规格片段；除出图外不产生任何模型费用。
		if !reusable(m.PanelPath) {
			if e.images == nil {
				return domain.MediaResult{}, fmt.Errorf("未配置图片生成能力（ImageGenerator），无法使用 comic 模式")
			}
			if _, err := e.images.GenerateImage(ctx, port.ImageRequest{
				OutPath: m.PanelPath,
				Prompt:  buildImagePrompt(sc, style),
				Size:    panelImageSize(series.Config.Ratio),
			}); err != nil {
				return domain.MediaResult{}, fmt.Errorf("插画生成: %w", err)
			}
		}
		if err := e.composer.RenderStill(ctx, port.StillRequest{
			ImagePath:   m.PanelPath,
			OutPath:     m.ClipPath,
			DurationSec: sc.DurationSec,
			Ratio:       series.Config.Ratio,
			Resolution:  series.Config.Resolution,
			Motion:      motionForScene(sc, m.Index),
		}); err != nil {
			return domain.MediaResult{}, fmt.Errorf("静帧运镜渲染: %w", err)
		}
	} else {
		// video 模式：AI 视频生成；画面含已生成参考图的人物/场景时，
		// 走 bl video ref 保持形象与环境一致。
		prompt := buildVideoPrompt(sc, style)
		refImgs := refImagesForScene(sc.VisualPrompt, visualRefs)
		if len(refImgs) > 0 {
			prompt = refPromptPrefix(visualRefs, refImgs) + prompt
		}
		if _, err := e.videos.GenerateClip(ctx, port.ClipRequest{
			OutPath:     m.ClipPath,
			Prompt:      prompt,
			RefImages:   refImgs,
			DurationSec: sc.DurationSec,
			Ratio:       series.Config.Ratio,
			Resolution:  series.Config.Resolution,
			Watermark:   true,
		}); err != nil {
			return domain.MediaResult{}, fmt.Errorf("视频生成: %w", err)
		}
	}
	dur, err := e.composer.ProbeDuration(ctx, m.ClipPath)
	if err != nil {
		return domain.MediaResult{}, fmt.Errorf("视频验收探测: %w", err)
	}
	return domain.MediaResult{SceneID: sc.ID, Path: m.ClipPath, DurationSec: dur}, nil
}

// produceAudio 合成单镜旁白：已存在则复用。
func (e *Engine) produceAudio(ctx context.Context, series *domain.Series, m sceneMedia) (domain.MediaResult, error) {
	sc := m.Scene
	if reusable(m.AudioPath) {
		dur, _ := e.composer.ProbeDuration(ctx, m.AudioPath)
		return domain.MediaResult{SceneID: sc.ID, Path: m.AudioPath, DurationSec: dur, Skipped: true}, nil
	}
	// 解析语音画像：§16 起优先按 series.voice_id 查表，回退旧字段。
	voice, model, rate, pitch, instr := e.resolveVoice(ctx, series.Config, series.VoiceID, e.voice, e.instruction)
	if _, err := e.speech.Synthesize(ctx, port.SpeechRequest{
		OutPath:     m.AudioPath,
		Text:        sc.Narration,
		Voice:       voice,
		Model:       model,
		Instruction: instr,
		Rate:        rate,
		Pitch:       pitch,
		Format:      "mp3",
	}); err != nil {
		return domain.MediaResult{}, fmt.Errorf("旁白合成: %w", err)
	}
	dur, err := e.composer.ProbeDuration(ctx, m.AudioPath)
	if err != nil {
		return domain.MediaResult{}, fmt.Errorf("音频验收探测: %w", err)
	}
	return domain.MediaResult{SceneID: sc.ID, Path: m.AudioPath, DurationSec: dur}, nil
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

// PreviewVoice 用指定语音画像合成一段样音（试音用，不计入流水线状态）。
func (e *Engine) PreviewVoice(ctx context.Context, p domain.VoiceProfile, text, outPath string) (string, error) {
	if text == "" {
		text = "话说天下大势，分久必合，合久必分。"
	}
	_, err := e.speech.Synthesize(ctx, port.SpeechRequest{
		OutPath:     outPath,
		Text:        text,
		Voice:       p.Voice,
		Model:       p.Model,
		Instruction: p.Instruction,
		Rate:        p.Rate,
		Pitch:       p.Pitch,
		Format:      "mp3",
	})
	return outPath, err
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

// buildVideoPrompt 组装单镜视频生成 prompt：画面内容 + 运镜，末尾固定追加
// 全片统一风格锚句（结尾位置对视频模型权重最高）。
func buildVideoPrompt(sc domain.Scene, style templates.VisualStylePack) string {
	s := strings.TrimSpace(sc.VisualPrompt)
	if sc.Camera != "" {
		s += "。运镜：" + sc.Camera
	}
	s += "。" + style.VideoAnchor
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

// joinSceneIDs 把镜头序号拼成「#13、#15」形式，便于在错误信息里人读。
func joinSceneIDs(ids []int) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = fmt.Sprintf("#%d", id)
	}
	return strings.Join(parts, "、")
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
