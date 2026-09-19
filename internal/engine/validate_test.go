package engine

import (
	"strings"
	"testing"

	"github.com/sjzsdu/story/internal/domain"
)

func TestValidateCandidates(t *testing.T) {
	good := []domain.StoryCandidate{
		{Title: "a", Source: "《史记》", Content: "x"},
		{Title: "b", Source: "《汉书》", Content: "y"},
		{Title: "c", Source: "《三国志》", Content: "z"},
	}
	if err := ValidateCandidates(good); err != nil {
		t.Fatalf("合法候选被拒: %v", err)
	}
	if err := ValidateCandidates(good[:2]); err == nil {
		t.Fatal("2 个候选应被拒绝")
	}
	bad := good
	bad[1].Source = ""
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
	if err := ValidateStoryboard(mk(4, 9)); err != nil {
		t.Fatal("时长 9 秒应合法（上限 10 秒）")
	}
	if err := ValidateStoryboard(mk(4, 11)); err == nil {
		t.Fatal("时长 11 秒应非法")
	}
	bad := mk(4, 4)
	bad.Scenes[0].Narration = ""
	if err := ValidateStoryboard(bad); err == nil {
		t.Fatal("缺旁白应报错")
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
