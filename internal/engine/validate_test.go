package engine

import (
	"strings"
	"testing"

	"github.com/sjzsdu/story/internal/domain"
)

func TestValidateCandidates(t *testing.T) {
	// 新流程为单篇定稿：1 个合法故事即通过，0 个必须拒绝。
	good := []domain.StoryCandidate{
		{Title: "a", Source: "《史记》", Content: "x"},
	}
	if err := ValidateCandidates(good); err != nil {
		t.Fatalf("合法故事被拒: %v", err)
	}
	if err := ValidateCandidates(nil); err == nil {
		t.Fatal("0 个故事应被拒绝")
	}
	bad := []domain.StoryCandidate{{Title: "b", Source: "", Content: "y"}}
	if err := ValidateCandidates(bad); err == nil || !strings.Contains(err.Error(), "出处") {
		t.Fatalf("缺出处应报错, got %v", err)
	}
}

func TestValidateSelection(t *testing.T) {
	cs := []domain.StoryCandidate{
		{Content: "a"}, {Content: "b"}, {Content: "c"},
	}
	if err := ValidateSelection(cs, 1); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSelection(cs, 0); err == nil {
		t.Fatal("序号 0 应越界")
	}
	if err := ValidateSelection(cs, 4); err == nil {
		t.Fatal("序号 4 应越界")
	}
}

func TestValidateStoryboard(t *testing.T) {
	mk := func(n, dur int) *domain.Storyboard {
		var sc []domain.Scene
		for i := 0; i < n; i++ {
			sc = append(sc, domain.Scene{ID: i + 1, VisualPrompt: "v", Narration: "n", DurationSec: dur})
		}
		return &domain.Storyboard{Scenes: sc}
	}
	if err := ValidateStoryboard(mk(4, 5)); err != nil {
		t.Fatal(err)
	}
	if err := ValidateStoryboard(mk(3, 5)); err == nil {
		t.Fatal("3 个镜头应被拒绝")
	}
	if err := ValidateStoryboard(mk(29, 5)); err == nil {
		t.Fatal("29 个镜头应被拒绝（上限 28）")
	}
	if err := ValidateStoryboard(mk(4, 9)); err != nil {
		t.Fatal("时长 9 秒应合法（上限 12 秒）")
	}
	if err := ValidateStoryboard(mk(4, 13)); err == nil {
		t.Fatal("时长 13 秒应非法")
	}
	bad := mk(4, 4)
	bad.Scenes[0].Narration = ""
	if err := ValidateStoryboard(bad); err == nil {
		t.Fatal("缺旁白应报错")
	}
}

func TestNormalizeDurations(t *testing.T) {
	sb := &domain.Storyboard{Scenes: []domain.Scene{
		{ID: 1, VisualPrompt: "v", Narration: "短旁白五字", DurationSec: 4},                 // need=3，不下调
		{ID: 2, VisualPrompt: "v", Narration: strings.Repeat("字", 36), DurationSec: 5}, // need=8，上调
		{ID: 3, VisualPrompt: "v", Narration: strings.Repeat("字", 60), DurationSec: 9}, // need=12 封顶（maxSceneDur=12）
	}}
	normalizeDurations(sb)
	if sb.Scenes[0].DurationSec != 4 {
		t.Fatalf("短旁白不应下调: %d", sb.Scenes[0].DurationSec)
	}
	if sb.Scenes[1].DurationSec != 8 {
		t.Fatalf("36 字应上调到 8s: %d", sb.Scenes[1].DurationSec)
	}
	if sb.Scenes[2].DurationSec != 12 {
		t.Fatalf("超上限应封顶 12s: %d", sb.Scenes[2].DurationSec)
	}
}

func TestValidateMedia(t *testing.T) {
	clips := []domain.MediaResult{
		{SceneID: 1, Path: "a.mp4", DurationSec: 5},
		{SceneID: 2, Path: "b.mp4", DurationSec: 5},
	}
	audios := []domain.MediaResult{
		{SceneID: 1, Path: "a.mp3", DurationSec: 4},
		{SceneID: 2, Path: "b.mp3", DurationSec: 6},
	}
	if err := ValidateMedia(clips, audios, 2); err != nil {
		t.Fatal(err)
	}
	bad := clips
	bad[0].Err = "boom"
	if err := ValidateMedia(bad, audios, 2); err == nil {
		t.Fatal("含错误登记应失败")
	}
}
