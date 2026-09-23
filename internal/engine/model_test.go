package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/sjzsdu/story/internal/domain"
	"github.com/sjzsdu/story/internal/port"
)

// ---- 系列级模型覆盖（§24 第二步）：派生键兼容性 + 三级优先级 + provider 透传 ----

// marshalJSON 序列化辅助（派生参数字节断言用）。
func marshalJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal %T: %v", v, err)
	}
	return b
}

// TestDerivationKeyUnchangedWithoutModelOverride 守住 §17/§18 派生键铁律：
// 四个模型覆盖字段必须 omitempty，未覆盖时 json.Marshal 字节与加字段前逐位一致；
// 一旦非空必须真的进派生键（否则改了模型却被静默复用旧产物）。
// 参照物是 creative_test.go 里「改动前的 legacy 结构体」，独立于被测代码。
func TestDerivationKeyUnchangedWithoutModelOverride(t *testing.T) {
	if derivationSchemaVersion != 1 {
		t.Fatalf("derivationSchemaVersion = %d，期望 1：第二步只新增 omitempty 字段，不得递增 schema 版本",
			derivationSchemaVersion)
	}

	// ---- 零值：与 legacy 结构体逐字节一致 ----
	story := storyParams{SeriesID: "guiguzi", Topic: "捭阖之术", Dynasty: "战国"}
	legacyStory := legacyStoryParams{SeriesID: "guiguzi", Topic: "捭阖之术", Dynasty: "战国"}
	if got, want := marshalJSON(t, story), marshalJSON(t, legacyStory); !bytes.Equal(got, want) {
		t.Fatalf("story 派生参数默认字节变化：\n got %s\nwant %s", got, want)
	}

	board := storyboardParams{
		StoryKey: "story-0123456789ab", Dynasty: "战国", Ratio: "9:16",
		Resolution: "1080P", VideoStyle: "gongbi", RefsDigest: "refs-deadbeef",
	}
	legacyBoard := legacyStoryboardParams{
		StoryKey: "story-0123456789ab", Dynasty: "战国", Ratio: "9:16",
		Resolution: "1080P", VideoStyle: "gongbi", RefsDigest: "refs-deadbeef",
	}
	if got, want := marshalJSON(t, board), marshalJSON(t, legacyBoard); !bytes.Equal(got, want) {
		t.Fatalf("storyboard 派生参数默认字节变化：\n got %s\nwant %s", got, want)
	}

	media := mediaParams{
		StoryboardKey: "storyboard-0123456789ab", VisualMode: "video", VideoStyle: "gongbi",
		Ratio: "9:16", Resolution: "1080P", VoiceID: "longtian", VoiceDigest: "voice-deadbeef",
	}
	legacyMedia := legacyMediaParams{
		StoryboardKey: "storyboard-0123456789ab", VisualMode: "video", VideoStyle: "gongbi",
		Ratio: "9:16", Resolution: "1080P", VoiceID: "longtian", VoiceDigest: "voice-deadbeef",
	}
	if got, want := marshalJSON(t, media), marshalJSON(t, legacyMedia); !bytes.Equal(got, want) {
		t.Fatalf("media 派生参数默认字节变化：\n got %s\nwant %s", got, want)
	}

	// final 阶段不参与模型覆盖：零值不得出现任何 model 键。
	final := finalParams{MediaKey: "media-0123456789ab", Ratio: "9:16", Resolution: "1080P", BurnSubtitles: true}
	legacyFinal := legacyFinalParams{MediaKey: "media-0123456789ab", Ratio: "9:16", Resolution: "1080P", BurnSubtitles: true}
	if got, want := marshalJSON(t, final), marshalJSON(t, legacyFinal); !bytes.Equal(got, want) {
		t.Fatalf("final 派生参数默认字节变化：\n got %s\nwant %s", got, want)
	}
	if bytes.Contains(marshalJSON(t, final), []byte("model")) {
		t.Fatalf("final 派生参数不应含 model 键: %s", marshalJSON(t, final))
	}

	// ---- 零值：派生键与 legacy 算法一致 ----
	if got, want := nodeKey(domain.StageStory, "", story, 0), legacyNodeKey(domain.StageStory, "", legacyStory, 0); got != want {
		t.Fatalf("story 派生键变化：got %s want %s", got, want)
	}
	if got, want := nodeKey(domain.StageStoryboard, "", board, 0), legacyNodeKey(domain.StageStoryboard, "", legacyBoard, 0); got != want {
		t.Fatalf("storyboard 派生键变化：got %s want %s", got, want)
	}
	if got, want := nodeKey(domain.StageMedia, "", media, 0), legacyNodeKey(domain.StageMedia, "", legacyMedia, 0); got != want {
		t.Fatalf("media 派生键变化：got %s want %s", got, want)
	}
	if got, want := nodeKey(domain.StageFinal, "", final, 0), legacyNodeKey(domain.StageFinal, "", legacyFinal, 0); got != want {
		t.Fatalf("final 派生键变化：got %s want %s", got, want)
	}

	// ---- 非空值：必须出现在 JSON 里，且必须改变派生键 ----
	withStory := story
	withStory.Model = "deepseek-chat"
	if !bytes.Contains(marshalJSON(t, withStory), []byte(`"model":"deepseek-chat"`)) {
		t.Fatalf("story.Model 未进派生 JSON: %s", marshalJSON(t, withStory))
	}
	if nodeKey(domain.StageStory, "", withStory, 0) == legacyNodeKey(domain.StageStory, "", legacyStory, 0) {
		t.Fatal("story.Model 未改变 story 派生键（改了模型会被静默复用旧故事）")
	}

	withBoard := board
	withBoard.Model = "deepseek-chat"
	if !bytes.Contains(marshalJSON(t, withBoard), []byte(`"model":"deepseek-chat"`)) {
		t.Fatalf("storyboard.Model 未进派生 JSON: %s", marshalJSON(t, withBoard))
	}
	if nodeKey(domain.StageStoryboard, "", withBoard, 0) == legacyNodeKey(domain.StageStoryboard, "", legacyBoard, 0) {
		t.Fatal("storyboard.Model 未改变 storyboard 派生键")
	}

	baseMedia := nodeKey(domain.StageMedia, "", media, 0)
	for _, c := range []struct {
		label string
		build func(*mediaParams)
		want  string
	}{
		{"image_model", func(p *mediaParams) { p.ImgModel = "cogview-4" }, `"image_model":"cogview-4"`},
		{"video_model", func(p *mediaParams) { p.VIDModel = "kling-v1" }, `"video_model":"kling-v1"`},
		{"tts_model", func(p *mediaParams) { p.TTSModel = "speech-02-hd" }, `"tts_model":"speech-02-hd"`},
	} {
		p := media
		c.build(&p)
		if !bytes.Contains(marshalJSON(t, p), []byte(c.want)) {
			t.Fatalf("%s 未进 media 派生 JSON: %s", c.label, marshalJSON(t, p))
		}
		if nodeKey(domain.StageMedia, "", p, 0) == baseMedia {
			t.Fatalf("%s 未改变 media 派生键", c.label)
		}
	}

	// ---- SeriesConfig 零值不回填系统默认模型（回填会让存量 config_json 字节全变）----
	cfgZero := marshalJSON(t, domain.SeriesConfig{})
	for _, key := range []string{`"text_model"`, `"tts_model"`, `"image_model"`, `"video_model"`} {
		if bytes.Contains(cfgZero, []byte(key)) {
			t.Fatalf("SeriesConfig 零值出现 %s（创建/更新系列不得回填系统默认模型）: %s", key, cfgZero)
		}
	}
	cfgOn := marshalJSON(t, domain.SeriesConfig{
		TextModel: "deepseek-chat", TTSModel: "speech-02-hd",
		ImageModel: "cogview-4", VideoModel: "kling-v1",
	})
	for _, key := range []string{`"text_model"`, `"tts_model"`, `"image_model"`, `"video_model"`} {
		if !bytes.Contains(cfgOn, []byte(key)) {
			t.Fatalf("SeriesConfig 覆盖值缺 %s: %s", key, cfgOn)
		}
	}
}

// TestResolveVoiceModelPriority 守住旁白模型三级优先级（voice.Model > 系列 > 空）。
func TestResolveVoiceModelPriority(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	if err := f.repo.CreateVoice(ctx, &domain.Voice{
		ID: "clone-voice", Name: "克隆音", Provider: domain.VoiceProviderBailian,
		Voice: "s1_clone_v1", Model: "cosyvoice-v3.5-plus",
	}); err != nil {
		t.Fatalf("CreateVoice 克隆音: %v", err)
	}
	if err := f.repo.CreateVoice(ctx, &domain.Voice{
		ID: "plain-voice", Name: "普通音", Provider: domain.VoiceProviderBailian, Voice: "longtian_v3",
	}); err != nil {
		t.Fatalf("CreateVoice 普通音: %v", err)
	}

	override := domain.SeriesConfig{TTSModel: "series-tts-model"}

	// ① 条目 Model 非空必用（造声音色必须用其驱动模型，压过系列覆盖）。
	voice, model, _, _, _ := f.eng.resolveVoice(ctx, override, "clone-voice", "fallback_v", "fallback_i")
	if voice != "s1_clone_v1" || model != "cosyvoice-v3.5-plus" {
		t.Fatalf("规则1 voice=%q model=%q，期望 s1_clone_v1 / cosyvoice-v3.5-plus", voice, model)
	}

	// ② 条目 Model 为空 → 落系列覆盖。
	voice, model, _, _, _ = f.eng.resolveVoice(ctx, override, "plain-voice", "fallback_v", "fallback_i")
	if voice != "longtian_v3" || model != "series-tts-model" {
		t.Fatalf("规则1 回落 voice=%q model=%q", voice, model)
	}

	// ③ 无 voice_id（旧字段路径）→ 音色裸读、模型直接用系列覆盖。
	voice, model, _, _, _ = f.eng.resolveVoice(ctx, override, "", "fallback_v", "fallback_i")
	if voice != "fallback_v" || model != "series-tts-model" {
		t.Fatalf("规则3 voice=%q model=%q", voice, model)
	}

	// ④ 全空 → 模型为空串（＝provider 构造期系统默认），音色兜底 fallback。
	voice, model, _, _, _ = f.eng.resolveVoice(ctx, domain.SeriesConfig{}, "", "fallback_v", "fallback_i")
	if voice != "fallback_v" || model != "" {
		t.Fatalf("全空 voice=%q model=%q，期望 fallback_v / 空串", voice, model)
	}

	// ⑤ 旧数据 VoiceProfile 路径：内置条目不带 Model → 落系列覆盖。
	withProfile := domain.SeriesConfig{VoiceProfile: "longtian", TTSModel: "series-tts-model"}
	voice, model, _, _, _ = f.eng.resolveVoice(ctx, withProfile, "", "fallback_v", "fallback_i")
	if voice != "longtian_v3" || model != "series-tts-model" {
		t.Fatalf("规则2 voice=%q model=%q", voice, model)
	}
}

// clipModels / speechModels / imageModels 抽出各 provider 请求的模型名，供逐项断言。
func clipModels(rs []port.ClipRequest) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.Model
	}
	return out
}

func speechModels(rs []port.SpeechRequest) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.Model
	}
	return out
}

func imageModels(rs []port.ImageRequest) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.Model
	}
	return out
}

// assertAllModels 断言这一批（本轮新增的）请求模型名全部等于 want。
func assertAllModels(t *testing.T, what, want string, got []string) {
	t.Helper()
	if len(got) == 0 {
		t.Fatalf("%s：本轮没有新请求", what)
	}
	for i, m := range got {
		if m != want {
			t.Fatalf("%s 第 %d 个请求 model = %q，期望 %q", what, i+1, m, want)
		}
	}
}

// TestSeriesModelOverridesReachProviders 端到端验证四类系列级模型覆盖：
// 空值一律不下发（走 provider 系统默认），设置后逐路透传，并且确实改变派生键。
func TestSeriesModelOverridesReachProviders(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	// ---- 阶段一：零值——所有请求 Model 为空 ----
	prepareStoryboard(t, f)
	if f.stories.LastRequest.Model != "" {
		t.Fatalf("零值 story Model = %q，期望空", f.stories.LastRequest.Model)
	}
	if f.boards.LastRequest.Model != "" {
		t.Fatalf("零值 storyboard Model = %q，期望空", f.boards.LastRequest.Model)
	}
	if _, err := f.eng.Produce(ctx, f.epID, DeriveOptions{}); err != nil {
		t.Fatalf("produce: %v", err)
	}
	assertAllModels(t, "零值视频", "", clipModels(f.videos.Requests))
	assertAllModels(t, "零值旁白", "", speechModels(f.speech.Requests))

	story1 := activeStageNode(t, f, domain.StageStory).ID
	board1 := activeStageNode(t, f, domain.StageStoryboard).ID
	media1 := activeStageNode(t, f, domain.StageMedia).ID

	// ---- 阶段二：设置四类系列级模型覆盖 ----
	setSeriesConfig(t, f, func(c *domain.SeriesConfig) {
		c.TextModel = "deepseek-chat"
		c.ImageModel = "cogview-4"
		c.VideoModel = "kling-v1"
		c.TTSModel = "speech-02-hd"
	})

	story2, err := f.eng.GenerateStory(ctx, f.epID, DeriveOptions{})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if story2.ID == story1 {
		t.Fatal("改文本模型后 story 应换版本")
	}
	if f.stories.LastRequest.Model != "deepseek-chat" {
		t.Fatalf("story Model = %q，期望 deepseek-chat", f.stories.LastRequest.Model)
	}

	board2, err := f.eng.PlanStoryboard(ctx, f.epID, DeriveOptions{})
	if err != nil {
		t.Fatalf("storyboard: %v", err)
	}
	if board2.ID == board1 {
		t.Fatal("改文本模型后 storyboard 应换版本")
	}
	if f.boards.LastRequest.Model != "deepseek-chat" {
		t.Fatalf("storyboard Model = %q，期望 deepseek-chat", f.boards.LastRequest.Model)
	}

	vidBase, spBase := len(f.videos.Requests), len(f.speech.Requests)
	media2, err := f.eng.Produce(ctx, f.epID, DeriveOptions{})
	if err != nil {
		t.Fatalf("produce: %v", err)
	}
	if media2.ID == media1 {
		t.Fatal("上游换版本后 media 应换版本")
	}
	assertAllModels(t, "视频", "kling-v1", clipModels(f.videos.Requests[vidBase:]))
	assertAllModels(t, "旁白", "speech-02-hd", speechModels(f.speech.Requests[spBase:]))

	// ---- 阶段三：只改 video_model —— 仅 media 换版本，证明媒体模型真的进 media 派生键 ----
	setSeriesConfig(t, f, func(c *domain.SeriesConfig) { c.VideoModel = "kling-v2" })
	vidBase = len(f.videos.Requests)
	media3, err := f.eng.Produce(ctx, f.epID, DeriveOptions{})
	if err != nil {
		t.Fatalf("produce: %v", err)
	}
	if media3.ID == media2.ID {
		t.Fatal("只改 video_model 也应派生新 media 节点（VIDModel 未进派生键）")
	}
	assertAllModels(t, "换模型后的视频", "kling-v2", clipModels(f.videos.Requests[vidBase:]))

	// ---- 阶段四：切 comic 模式 → image_model 进插画请求 ----
	setSeriesConfig(t, f, func(c *domain.SeriesConfig) { c.VisualMode = domain.VisualModeComic })
	imgBase := len(f.images.Requests)
	media4, err := f.eng.Produce(ctx, f.epID, DeriveOptions{})
	if err != nil {
		t.Fatalf("produce comic: %v", err)
	}
	if media4.ID == media3.ID {
		t.Fatal("改画面模式后 media 应换版本")
	}
	assertAllModels(t, "小人书插画", "cogview-4", imageModels(f.images.Requests[imgBase:]))

	// ---- 阶段五：系列定妆照 / 集级视觉参考图同样吃 image_model ----
	setCharacters(t, f, sampleCharacters())
	kfBase := len(f.images.Requests)
	if _, err := f.eng.GenerateSeriesKeyframes(ctx, "guiguzi", false); err != nil {
		t.Fatalf("生成定妆照: %v", err)
	}
	assertAllModels(t, "系列定妆照", "cogview-4", imageModels(f.images.Requests[kfBase:]))

	ep := episodeOf(t, f)
	ep.Refs = []domain.VisualRef{
		{Kind: domain.RefKindCharacter, Name: "聂小倩", Description: "素白襦裙外罩浅青纱衫"},
		{Kind: domain.RefKindScene, Name: "兰若寺大殿", Description: "破败古寺大殿，冷青色月光"},
	}
	if err := f.repo.SaveEpisode(ctx, ep); err != nil {
		t.Fatal(err)
	}
	erBase := len(f.images.Requests)
	if _, err := f.eng.GenerateEpisodeRefs(ctx, f.epID, false); err != nil {
		t.Fatalf("生成集级视觉参考: %v", err)
	}
	assertAllModels(t, "集级视觉参考图", "cogview-4", imageModels(f.images.Requests[erBase:]))

	// ---- 阶段六：试音走语音画像自带的 Model（三级优先级最高一档）----
	pvBase := len(f.speech.Requests)
	out := filepath.Join(t.TempDir(), "preview.mp3")
	if _, err := f.eng.PreviewVoice(ctx, domain.VoiceProfile{
		Voice: "longtian_v3", Model: "cosyvoice-v3.5-plus",
	}, "试音文本", out); err != nil {
		t.Fatalf("PreviewVoice: %v", err)
	}
	assertAllModels(t, "试音", "cosyvoice-v3.5-plus", speechModels(f.speech.Requests[pvBase:]))
}
