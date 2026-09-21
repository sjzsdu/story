package engine

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/sjzsdu/story/internal/domain"
)

// ---- 派生键回归：创作参数新增后，默认（无任何创作设置）的派生键必须逐字不变 ----

type legacyStoryParams struct {
	SeriesID string `json:"series_id"`
	Topic    string `json:"topic"`
	Dynasty  string `json:"dynasty"`
}

type legacyStoryboardParams struct {
	StoryKey   string `json:"story_key"`
	Dynasty    string `json:"dynasty"`
	Ratio      string `json:"ratio"`
	Resolution string `json:"resolution"`
	VideoStyle string `json:"video_style"`
	RefsDigest string `json:"refs_digest"`
}

type legacyMediaParams struct {
	StoryboardKey string `json:"storyboard_key"`
	VisualMode    string `json:"visual_mode"`
	VideoStyle    string `json:"video_style"`
	Ratio         string `json:"ratio"`
	Resolution    string `json:"resolution"`
	VoiceID       string `json:"voice_id"`
	VoiceDigest   string `json:"voice_digest"`
}

// legacyNodeKey 复刻改动前的派生键算法（只换 params 类型），作为独立参照。
func legacyNodeKey(stage domain.Stage, parentID string, params any, attempt int) string {
	b, _ := json.Marshal(params)
	raw := fmt.Sprintf("%d|%s|%s|%s|%d", derivationSchemaVersion, stage, parentID, b, attempt)
	sum := sha1.Sum([]byte(raw))
	return fmt.Sprintf("%s-%s", stage, hex.EncodeToString(sum[:])[:12])
}

func TestDerivationKeyUnchangedForDefaultCreative(t *testing.T) {
	cases := []struct {
		name    string
		stage   domain.Stage
		parent  string
		attempt int
		now     any
		legacy  any
	}{
		{
			name: "story", stage: domain.StageStory, attempt: 0,
			now:    storyParams{SeriesID: "guiguzi", Topic: "捭阖之术", Dynasty: "战国"},
			legacy: legacyStoryParams{SeriesID: "guiguzi", Topic: "捭阖之术", Dynasty: "战国"},
		},
		{
			name: "story-reroll", stage: domain.StageStory, attempt: 3,
			now:    storyParams{SeriesID: "guiguzi", Topic: "", Dynasty: ""},
			legacy: legacyStoryParams{SeriesID: "guiguzi"},
		},
		{
			name: "storyboard", stage: domain.StageStoryboard, parent: "story-abc123", attempt: 0,
			now: storyboardParams{
				StoryKey: "story-abc123", Dynasty: "战国", Ratio: "9:16",
				Resolution: "1080P", VideoStyle: "gongbi", RefsDigest: "d1",
			},
			legacy: legacyStoryboardParams{
				StoryKey: "story-abc123", Dynasty: "战国", Ratio: "9:16",
				Resolution: "1080P", VideoStyle: "gongbi", RefsDigest: "d1",
			},
		},
		{
			name: "media", stage: domain.StageMedia, parent: "storyboard-abc123", attempt: 0,
			now: mediaParams{
				StoryboardKey: "storyboard-abc123", VisualMode: "comic", VideoStyle: "gongbi",
				Ratio: "9:16", Resolution: "1080P", VoiceID: "v1", VoiceDigest: "d2",
			},
			legacy: legacyMediaParams{
				StoryboardKey: "storyboard-abc123", VisualMode: "comic", VideoStyle: "gongbi",
				Ratio: "9:16", Resolution: "1080P", VoiceID: "v1", VoiceDigest: "d2",
			},
		},
	}
	for _, c := range cases {
		got := nodeKey(c.stage, c.parent, c.now, c.attempt)
		want := legacyNodeKey(c.stage, c.parent, c.legacy, c.attempt)
		if got != want {
			t.Fatalf("%s 派生键变了：默认配置下旧产物会被误判失效并重复调用付费模型\n got %s\nwant %s", c.name, got, want)
		}
	}
}

// setSeriesConfig 覆写系列配置并落库（模拟用户在界面上改创作设置）。
func setSeriesConfig(t *testing.T, f *fixture, mutate func(*domain.SeriesConfig)) {
	t.Helper()
	ctx := context.Background()
	series, err := f.repo.GetSeries(ctx, "guiguzi")
	if err != nil {
		t.Fatal(err)
	}
	mutate(&series.Config)
	if err := f.repo.UpdateSeries(ctx, series); err != nil {
		t.Fatal(err)
	}
}

// TestCreativeSettingsAffectDerivation 验证创作参数的派生语义：
// 改叙事风格 → story 与 storyboard 都要换版本；只改画风 → story 不重跑；
// 只改运镜强度 → 只有 media 换版本。
func TestCreativeSettingsAffectDerivation(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	story1, err := f.eng.GenerateStory(ctx, f.epID, DeriveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	board1, err := f.eng.PlanStoryboard(ctx, f.epID, DeriveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if f.stories.LastRequest.Brief != "" || f.boards.LastRequest.Brief != "" {
		t.Fatalf("默认配置不得注入创作要求: %q / %q", f.stories.LastRequest.Brief, f.boards.LastRequest.Brief)
	}

	// 改叙事风格：口吻变了，故事与分镜都必须换版本并重新调用模型。
	setSeriesConfig(t, f, func(c *domain.SeriesConfig) { c.Creative.Narrative = "suspense" })
	story2, err := f.eng.GenerateStory(ctx, f.epID, DeriveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if story2.ID == story1.ID {
		t.Fatal("改叙事风格必须派生新的 story 节点")
	}
	if f.stories.Calls != 2 {
		t.Fatalf("改叙事风格应重新调用故事模型: %d", f.stories.Calls)
	}
	if !strings.Contains(f.stories.LastRequest.Brief, "悬疑倒叙") {
		t.Fatalf("创作要求未透传到故事模型: %q", f.stories.LastRequest.Brief)
	}
	board2, err := f.eng.PlanStoryboard(ctx, f.epID, DeriveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if board2.ID == board1.ID {
		t.Fatal("改叙事风格必须派生新的 storyboard 节点")
	}
	if !strings.Contains(f.boards.LastRequest.Brief, "悬疑倒叙") {
		t.Fatalf("创作要求未透传到分镜模型: %q", f.boards.LastRequest.Brief)
	}

	// 只改画风：故事不该白重跑（画风只影响分镜与画面）。
	setSeriesConfig(t, f, func(c *domain.SeriesConfig) { c.VideoStyle = "ink" })
	storiesBefore := f.stories.Calls
	story3, err := f.eng.GenerateStory(ctx, f.epID, DeriveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if story3.ID != story2.ID || f.stories.Calls != storiesBefore {
		t.Fatalf("只改画风不应重跑故事: %s → %s, calls=%d", story2.ID, story3.ID, f.stories.Calls)
	}
	board3, err := f.eng.PlanStoryboard(ctx, f.epID, DeriveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if board3.ID == board2.ID {
		t.Fatal("改画风必须派生新的 storyboard 节点")
	}

	// 只改运镜强度：只有 media 换版本（分镜与故事都不动）。
	media1, err := f.eng.Produce(ctx, f.epID, DeriveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	setSeriesConfig(t, f, func(c *domain.SeriesConfig) { c.Creative.Motion = "strong" })
	media2, err := f.eng.Produce(ctx, f.epID, DeriveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if media2.ID == media1.ID {
		t.Fatal("改运镜强度必须派生新的 media 节点，否则旧片段会被直接复用、改了不生效")
	}
	ep := episodeOf(t, f)
	if got := ep.ActiveNodeOfStage(domain.StageStoryboard).ID; got != board3.ID {
		t.Fatalf("改运镜强度不应影响分镜: %s ≠ %s", got, board3.ID)
	}
	if got := ep.ActiveNodeOfStage(domain.StageStory).ID; got != story3.ID {
		t.Fatalf("改运镜强度不应影响故事: %s ≠ %s", got, story3.ID)
	}
	for i := range media2.Clips {
		if media2.Clips[i].Skipped {
			t.Fatalf("新版本镜头不应被标记为复用: %+v", media2.Clips[i])
		}
	}
}

// TestEpisodeInstructionChangesDerivation 验证集级附加指令同样进派生键。
func TestEpisodeInstructionChangesDerivation(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	story1, err := f.eng.GenerateStory(ctx, f.epID, DeriveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	ep := episodeOf(t, f)
	ep.Instruction = "本集只讲一个晚上，不写少年经历。"
	if err := f.repo.SaveEpisode(ctx, ep); err != nil {
		t.Fatal(err)
	}
	story2, err := f.eng.GenerateStory(ctx, f.epID, DeriveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if story2.ID == story1.ID {
		t.Fatal("集级附加指令必须派生新的 story 节点")
	}
	if !strings.Contains(f.stories.LastRequest.Brief, "本集只讲一个晚上") {
		t.Fatalf("集级附加指令未透传: %q", f.stories.LastRequest.Brief)
	}
	if _, err := f.eng.PlanStoryboard(ctx, f.epID, DeriveOptions{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.boards.LastRequest.Brief, "本集只讲一个晚上") {
		t.Fatalf("集级附加指令未透传到分镜: %q", f.boards.LastRequest.Brief)
	}
}
