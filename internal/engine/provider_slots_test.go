package engine

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sjzsdu/story/internal/domain"
	"github.com/sjzsdu/story/internal/port"
	"github.com/sjzsdu/story/testutil/mock"
)

// registerSlot 给槽补登具名实现（New 已把测试 mock 注册在 default 键上）。
func registerSlot[T any](t *testing.T, s interface {
	Register(key string, impl T) error
}, entries map[string]T) {
	t.Helper()
	for k, v := range entries {
		if err := s.Register(k, v); err != nil {
			t.Fatalf("登记 %q: %v", k, err)
		}
	}
}

// updateSeriesConfig 直接改系列配置（mock repo 支持全量更新）。
func updateSeriesConfig(t *testing.T, f *fixture, mut func(*domain.SeriesConfig)) {
	t.Helper()
	se, err := f.repo.GetSeries(context.Background(), "guiguzi")
	if err != nil {
		t.Fatal(err)
	}
	mut(&se.Config)
	if err := f.repo.UpdateSeries(context.Background(), se); err != nil {
		t.Fatal(err)
	}
}

// TestSeriesTextProviderOverrideDiverts 专项：系列覆盖 text_provider 必须真正
// 分流到对应实现（旧 RegisterProvider(any) 类型 switch 首命中即返回时，
// board/plan/tts/image/video 的 bailian 系列覆盖会静默失效）。
func TestSeriesTextProviderOverrideDiverts(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	e := f.eng

	bailianText := &mock.StoryGen{Candidates: sampleCandidates()}
	deepseekText := &mock.StoryGen{Candidates: sampleCandidates()}
	registerSlot(t, e.Text, map[string]port.StoryGenerator{
		"bailian":  bailianText,
		"deepseek": deepseekText,
	})

	// ① 系列覆盖 deepseek → 只有 deepseek 实现被调用。
	updateSeriesConfig(t, f, func(c *domain.SeriesConfig) { c.TextProvider = "deepseek" })
	if _, err := e.GenerateStory(ctx, f.epID, DeriveOptions{}); err != nil {
		t.Fatalf("deepseek 覆盖生成: %v", err)
	}
	if deepseekText.Calls != 1 || bailianText.Calls != 0 || f.stories.Calls != 0 {
		t.Fatalf("覆盖未分流: deepseek=%d bailian=%d 默认=%d",
			deepseekText.Calls, bailianText.Calls, f.stories.Calls)
	}

	// ② 清空覆盖 → 回落到系统默认（New 注册的 default 键实现）。
	updateSeriesConfig(t, f, func(c *domain.SeriesConfig) { c.TextProvider = "" })
	if _, err := e.GenerateStory(ctx, f.epID, DeriveOptions{Reroll: true}); err != nil {
		t.Fatalf("默认生成: %v", err)
	}
	if f.stories.Calls != 1 || bailianText.Calls != 0 {
		t.Fatalf("空覆盖应回默认: 默认=%d bailian=%d", f.stories.Calls, bailianText.Calls)
	}

	// ③ 系列覆盖 bailian → 只有 bailian 实现被调用。
	updateSeriesConfig(t, f, func(c *domain.SeriesConfig) { c.TextProvider = "bailian" })
	if _, err := e.GenerateStory(ctx, f.epID, DeriveOptions{Reroll: true}); err != nil {
		t.Fatalf("bailian 覆盖生成: %v", err)
	}
	if bailianText.Calls != 1 || deepseekText.Calls != 1 || f.stories.Calls != 1 {
		t.Fatalf("bailian 覆盖未分流: bailian=%d deepseek=%d 默认=%d",
			bailianText.Calls, deepseekText.Calls, f.stories.Calls)
	}

	// ④ 显式未知覆盖 → 报错（不得静默回落默认）。
	updateSeriesConfig(t, f, func(c *domain.SeriesConfig) { c.TextProvider = "iflytek" })
	if _, err := e.GenerateStory(ctx, f.epID, DeriveOptions{Reroll: true}); err == nil {
		t.Fatal("未知覆盖应报错，而非静默回退默认")
	}
}

// TestPlanSlotSeriesOverride 系列策划槽：override 取 cfg.TextProvider；
// 即便该供应商不实现 SeriesPlanner（app 把 deepseek 映射到 bl 实现），
// 槽内只要登记了该 key 就必须命中——引擎侧只认选择链，不认供应商能力。
func TestPlanSlotSeriesOverride(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	e := f.eng

	oneDraft := []domain.EpisodeDraft{{Title: "新集", Topic: "补充", Summary: "补充集"}}
	plannerA := &mock.SeriesPlanner{Result: port.SeriesPlanResult{Reply: "bailian 规划", Drafts: oneDraft}}
	plannerB := &mock.SeriesPlanner{Result: port.SeriesPlanResult{Reply: "deepseek 规划", Drafts: oneDraft}}
	registerSlot(t, e.Plan, map[string]port.SeriesPlanner{
		"bailian":  plannerA,
		"deepseek": plannerB,
	})

	// 覆盖 deepseek → plannerB。
	updateSeriesConfig(t, f, func(c *domain.SeriesConfig) { c.TextProvider = "deepseek" })
	ps, err := e.ChatSeriesPlan(ctx, "guiguzi", "再扩两集", "")
	if err != nil {
		t.Fatalf("策划会话: %v", err)
	}
	if plannerB.Calls != 1 || plannerA.Calls != 0 || f.planner.Calls != 0 {
		t.Fatalf("Plan 槽未分流: deepseek=%d bailian=%d 默认=%d",
			plannerB.Calls, plannerA.Calls, f.planner.Calls)
	}
	if len(ps.Drafts) != 1 || ps.Drafts[0].Title != "新集" {
		t.Fatalf("草案应来自命中的实现，得到 %+v", ps.Drafts)
	}

	// 清空覆盖 → 默认实现（New 注册的 default 键）。
	updateSeriesConfig(t, f, func(c *domain.SeriesConfig) { c.TextProvider = "" })
	if _, err := e.ChatSeriesPlan(ctx, "guiguzi", "换個方向", ""); err != nil {
		t.Fatalf("默认策划会话: %v", err)
	}
	if f.planner.Calls != 1 || plannerA.Calls != 0 {
		t.Fatalf("空覆盖应回默认: 默认=%d bailian=%d", f.planner.Calls, plannerA.Calls)
	}

	// 未知覆盖 → 报错。
	updateSeriesConfig(t, f, func(c *domain.SeriesConfig) { c.TextProvider = "iflytek" })
	if _, err := e.ChatSeriesPlan(ctx, "guiguzi", "再来一次", ""); err == nil {
		t.Fatal("未知覆盖应报错")
	}
}

// TestPreviewVoiceUsesTTSSlotDefault 试音不绑系列：PreviewVoice 走 TTS 槽
// Resolve("")（系统默认），SetDefault 换默认后必须立刻生效。
func TestPreviewVoiceUsesTTSSlotDefault(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	e := f.eng

	bailianTTS := &mock.SpeechGen{}
	minimaxTTS := &mock.SpeechGen{}
	registerSlot(t, e.TTS, map[string]port.SpeechSynthesizer{
		"bailian": bailianTTS,
		"minimax": minimaxTTS,
	})
	if err := e.TTS.SetDefault("bailian"); err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(t.TempDir(), "preview.mp3")
	if _, err := e.PreviewVoice(ctx, domain.VoiceProfile{Voice: "longtian_v3"}, "试音", out); err != nil {
		t.Fatalf("试音: %v", err)
	}
	if bailianTTS.Calls != 1 || minimaxTTS.Calls != 0 || f.speech.Calls != 0 {
		t.Fatalf("试音未走 TTS 默认槽: bailian=%d minimax=%d 旧默认=%d",
			bailianTTS.Calls, minimaxTTS.Calls, f.speech.Calls)
	}

	// 换默认 → 下一次试音立刻分流到新实现。
	if err := e.TTS.SetDefault("minimax"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.PreviewVoice(ctx, domain.VoiceProfile{Voice: "longtian_v3"}, "再试", out); err != nil {
		t.Fatalf("换默认后试音: %v", err)
	}
	if minimaxTTS.Calls != 1 || bailianTTS.Calls != 1 {
		t.Fatalf("换默认未生效: minimax=%d bailian=%d", minimaxTTS.Calls, bailianTTS.Calls)
	}
}

// ---- §22：三个新能力槽（无系列覆盖，override 是单次指定）----

// TestVisionAndSFXSlotResolve 新槽三分支：具名覆盖命中 / 空 override 走默认 /
// 未知报错含能力中文名与已登记项；sfx 空槽调用报「未配置默认实现」而非静默成功。
func TestVisionAndSFXSlotResolve(t *testing.T) {
	f := setup(t)
	e := f.eng

	// New 只给新能力建空槽（13 参签名不含这三个实现），这里模拟 Bootstrap 登记。
	vision := &mock.Visioner{Reply: "两个古装人物对坐"}
	registerSlot(t, e.ImageUnderstand, map[string]port.ImageUnderstander{
		"bailian": vision, "alt": vision,
	})
	registerSlot(t, e.VideoUnderstand, map[string]port.VideoUnderstander{"bailian": vision})
	if err := e.ImageUnderstand.SetDefault("bailian"); err != nil {
		t.Fatal(err)
	}
	if err := e.VideoUnderstand.SetDefault("bailian"); err != nil {
		t.Fatal(err)
	}
	// sfx 刻意不登记（bl 无音效命令 → 空槽）。

	// ① 空 override → 默认实现，请求透传到位。
	res, err := e.DescribeImage(context.Background(), port.DescribeImageRequest{
		Image: "a.png", Prompt: "描述", Model: "qwen3-vl-plus",
	}, "")
	if err != nil {
		t.Fatalf("默认槽调用: %v", err)
	}
	if res.Text != "两个古装人物对坐" || vision.ImageCalls != 1 {
		t.Fatalf("结果/调用次数异常: %+v calls=%d", res, vision.ImageCalls)
	}
	if vision.ImageReq.Model != "qwen3-vl-plus" || vision.ImageReq.Prompt != "描述" {
		t.Fatalf("请求未透传: %+v", vision.ImageReq)
	}

	// ② 具名覆盖命中（两个登记项同一实例，用 VideoUnderstand 区分流）。
	if _, err := e.DescribeVideo(context.Background(), port.DescribeVideoRequest{
		Video: "https://example.com/v.mp4",
	}, "bailian"); err != nil {
		t.Fatalf("覆盖调用: %v", err)
	}
	if vision.VideoCalls != 1 {
		t.Fatalf("video 调用次数 = %d", vision.VideoCalls)
	}

	// ③ 未知覆盖 → 报错且列出已登记项。
	if _, err := e.DescribeImage(context.Background(),
		port.DescribeImageRequest{Image: "a.png"}, "openai"); err == nil {
		t.Fatal("未知覆盖应报错")
	} else if !strings.Contains(err.Error(), "图像理解") ||
		!strings.Contains(err.Error(), "bailian") {
		t.Fatalf("错误信息应含能力中文名与已登记项: %v", err)
	}

	// sfx 三处降级之一：空槽 + 空 override → 「未配置默认实现」，错误含能力中文名。
	_, err = e.GenerateSoundEffect(context.Background(),
		port.SoundEffectRequest{Prompt: "木门吱呀", OutPath: filepath.Join(t.TempDir(), "sfx.wav")}, "")
	if err == nil {
		t.Fatal("空槽调用应报错")
	}
	if !strings.Contains(err.Error(), "音效生成") ||
		!strings.Contains(err.Error(), "未配置默认实现") {
		t.Fatalf("sfx 空槽错误信息异常: %v", err)
	}
}

// TestSFXSlotAfterRegistration 登记 sfx 实现后入口正常工作（首个 provider 落地的通路保障）。
func TestSFXSlotAfterRegistration(t *testing.T) {
	f := setup(t)
	sfx := &mock.SfxGen{}
	registerSlot(t, f.eng.SFX, map[string]port.SoundEffectGenerator{"mock-sfx": sfx})
	if err := f.eng.SFX.SetDefault("mock-sfx"); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "sfx.wav")
	res, err := f.eng.GenerateSoundEffect(context.Background(),
		port.SoundEffectRequest{Prompt: "木门吱呀", OutPath: out}, "")
	if err != nil {
		t.Fatalf("登记后调用: %v", err)
	}
	if res.OutPath != out || sfx.Calls != 1 {
		t.Fatalf("结果异常: %+v calls=%d", res, sfx.Calls)
	}
}

// TestResolveHelpersOverrideAndUnknown 覆盖命中 / 空值回默认 / 未知报错（错误信息须含已登记项）。
func TestResolveHelpersOverrideAndUnknown(t *testing.T) {
	f := setup(t)
	e := f.eng

	dsText := &mock.StoryGen{}
	dsBoard := &mock.BoardPlanner{Storyboard: sampleStoryboard()}
	mmTTS := &mock.SpeechGen{}
	zpImg := &mock.ImageGen{}
	klVid := &mock.VideoGen{}
	registerSlot(t, e.Text, map[string]port.StoryGenerator{"deepseek": dsText})
	registerSlot(t, e.Board, map[string]port.StoryboardPlanner{"deepseek": dsBoard})
	registerSlot(t, e.TTS, map[string]port.SpeechSynthesizer{"minimax": mmTTS})
	registerSlot(t, e.Image, map[string]port.ImageGenerator{"zhipu": zpImg})
	registerSlot(t, e.Video, map[string]port.VideoGenerator{"kling": klVid})

	override := domain.SeriesConfig{
		TextProvider:  "deepseek",
		TTSProvider:   "minimax",
		ImageProvider: "zhipu",
		VideoProvider: "kling",
	}
	if impl, err := e.resolveText(override); err != nil || impl != port.StoryGenerator(dsText) {
		t.Fatalf("resolveText 覆盖: (%v, %v)", impl, err)
	}
	if impl, err := e.resolveBoard(override); err != nil || impl != port.StoryboardPlanner(dsBoard) {
		t.Fatalf("resolveBoard 覆盖: (%v, %v)", impl, err)
	}
	if impl, err := e.resolveTTS(override); err != nil || impl != port.SpeechSynthesizer(mmTTS) {
		t.Fatalf("resolveTTS 覆盖: (%v, %v)", impl, err)
	}
	if impl, err := e.resolveImage(override); err != nil || impl != port.ImageGenerator(zpImg) {
		t.Fatalf("resolveImage 覆盖: (%v, %v)", impl, err)
	}
	if impl, err := e.resolveVideo(override); err != nil || impl != port.VideoGenerator(klVid) {
		t.Fatalf("resolveVideo 覆盖: (%v, %v)", impl, err)
	}

	// 空值 → New 注册的 default 实现。
	empty := domain.SeriesConfig{}
	if impl, err := e.resolveText(empty); err != nil || impl != port.StoryGenerator(f.stories) {
		t.Fatalf("resolveText 默认: (%v, %v)", impl, err)
	}
	if impl, err := e.resolveTTS(empty); err != nil || impl != port.SpeechSynthesizer(f.speech) {
		t.Fatalf("resolveTTS 默认: (%v, %v)", impl, err)
	}
	if impl, err := e.resolveVideo(empty); err != nil || impl != port.VideoGenerator(f.videos) {
		t.Fatalf("resolveVideo 默认: (%v, %v)", impl, err)
	}

	// 未知 → 报错，且错误信息列出已登记项（便于排查配置拼写）。
	bad := domain.SeriesConfig{TTSProvider: "iflytek"}
	if _, err := e.resolveTTS(bad); err == nil {
		t.Fatal("未知覆盖应报错")
	} else if !strings.Contains(err.Error(), "minimax") || !strings.Contains(err.Error(), "语音合成") {
		t.Fatalf("错误信息应含能力中文名与已登记项，得到: %v", err)
	}
}
