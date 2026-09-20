package templates

import "testing"

func TestMatchStyle(t *testing.T) {
	if got := MatchStyle("gongbi").Key; got != "gongbi" {
		t.Fatalf("gongbi = %q", got)
	}
	if got := MatchStyle("realistic").Key; got != "realistic" {
		t.Fatalf("realistic = %q", got)
	}
	// 空 key 与未知 key 均回退默认工笔风格（与栏目定妆照配套）。
	for _, key := range []string{"", "unknown", "电影感"} {
		if got := MatchStyle(key).Key; got != DefaultStyleKey {
			t.Fatalf("MatchStyle(%q) = %q, want %q", key, got, DefaultStyleKey)
		}
	}
}

func TestStylePackAnchorAndKeyframe(t *testing.T) {
	p := MatchStyle("gongbi")
	if p.VideoAnchor == "" || p.Brief == "" || p.KeyframeClause == "" {
		t.Fatal("风格包字段不得为空")
	}
	got := p.KeyframePrompt("战国", "张仪", "纵横家", "绀青色深衣", "锐利诙谐")
	for _, want := range []string{"张仪", "纵横家", "绀青色深衣", "工笔重彩"} {
		if !contains(got, want) {
			t.Fatalf("定妆照 prompt 缺 %q: %s", want, got)
		}
	}
}

func TestStoryboardPromptInjectsStyle(t *testing.T) {
	got := StoryboardUserPrompt("测试集", "战国", "正文", "9:16", "720P", "gongbi", nil, nil)
	if !contains(got, "全片统一画风") || !contains(got, "工笔重彩") {
		t.Fatalf("分镜 prompt 未注入工笔风格:\n%s", got)
	}
	if contains(got, "整体风格附加要求") {
		t.Fatal("旧的自由风格文本段应已移除")
	}
}

func TestStoryboardPromptInjectsEpisodeRefs(t *testing.T) {
	refs := []string{"[人物] 聂小倩：素白襦裙", "[场景] 兰若寺大殿：破败古寺，冷青月光"}
	got := StoryboardUserPrompt("聂小倩", "清", "正文", "9:16", "1080P", "gongbi", nil, refs)
	if !contains(got, "本集已有视觉参考") || !contains(got, "兰若寺大殿") {
		t.Fatalf("分镜 prompt 未回灌本集视觉参考:\n%s", got)
	}
}

func TestSceneRefPrompt(t *testing.T) {
	p := MatchStyle("gongbi")
	got := p.SceneRefPrompt("清", "兰若寺大殿", "破败古寺大殿，冷青色月光")
	for _, want := range []string{"兰若寺大殿", "破败古寺大殿", "空镜", "工笔重彩"} {
		if !contains(got, want) {
			t.Fatalf("场景参考 prompt 缺 %q: %s", want, got)
		}
	}
}
