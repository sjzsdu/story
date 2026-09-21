package templates

import (
	"strings"
	"testing"

	"github.com/sjzsdu/story/internal/domain"
)

// 默认（不做任何设置）必须产出与历史行为完全一致的 prompt：
// brief 为空串，调用方据此跳过注入。
func TestCreativeBriefEmptyByDefault(t *testing.T) {
	var cfg domain.SeriesConfig
	if got := StoryBrief(cfg, ""); got != "" {
		t.Fatalf("默认 StoryBrief 应为空串，得到:\n%s", got)
	}
	if got := BoardBrief(cfg, ""); got != "" {
		t.Fatalf("默认 BoardBrief 应为空串，得到:\n%s", got)
	}
	// 定时仅运镜强度（纯画面参数）不得让故事/分镜重跑。
	cfg.Creative.Motion = "strong"
	if got := StoryBrief(cfg, ""); got != "" {
		t.Fatalf("只改运镜强度不应影响 StoryBrief:\n%s", got)
	}
	if got := BoardBrief(cfg, ""); got != "" {
		t.Fatalf("只改运镜强度不应影响 BoardBrief:\n%s", got)
	}
}

// 默认预设的 values 必须全空，套用后不改变任何参数。
func TestDefaultPresetIsNoop(t *testing.T) {
	var cfg domain.SeriesConfig
	if err := ApplyPreset(&cfg, DefaultPresetKey); err != nil {
		t.Fatal(err)
	}
	if cfg.VideoStyle != "" || !cfg.Creative.IsZero() {
		t.Fatalf("默认预设不得写入任何参数: %+v", cfg)
	}
}

// 每个预设的 values 经 Set 能回读，且预设 key 被记录。
func TestPresetsRoundTrip(t *testing.T) {
	for _, p := range CreativePresets() {
		if p.Key == DefaultPresetKey {
			continue
		}
		if len(p.Values) == 0 {
			t.Fatalf("预设 %s 的 values 不应为空", p.Key)
		}
		var cfg domain.SeriesConfig
		if err := ApplyPreset(&cfg, p.Key); err != nil {
			t.Fatalf("套用 %s 失败: %v", p.Key, err)
		}
		if cfg.Creative.Preset != p.Key {
			t.Fatalf("预设 key 未记录: %q", cfg.Creative.Preset)
		}
		for knobKey, want := range p.Values {
			k, ok := FindKnob(knobKey)
			if !ok {
				t.Fatalf("预设 %s 引用了未知参数 %q", p.Key, knobKey)
			}
			if got := k.Get(cfg); got != want {
				t.Fatalf("预设 %s 的 %s = %q，want %q", p.Key, knobKey, got, want)
			}
			if k.Type != KnobEnum {
				continue
			}
			valid := false
			for _, o := range k.Options {
				if o.Key == want {
					valid = true
				}
			}
			if !valid {
				t.Fatalf("预设 %s 的 %s=%q 不是合法选项", p.Key, knobKey, want)
			}
		}
	}
}

// 未知值回落为「未设置」，未知参数单独报错。
func TestSetKnobNormalizesUnknown(t *testing.T) {
	var cfg domain.SeriesConfig
	if err := SetKnob(&cfg, "narrative", "不存在的风格"); err != nil {
		t.Fatal(err)
	}
	if cfg.Creative.Narrative != "" {
		t.Fatalf("未知值应回落为空，得到 %q", cfg.Creative.Narrative)
	}
	if err := SetKnob(&cfg, "nope", "x"); err == nil {
		t.Fatal("未知参数应报错")
	}
	if err := ApplyPreset(&cfg, "nope"); err == nil {
		t.Fatal("未知预设应报错")
	}
	// 已设置的参数不得被未知值擦掉。
	cfg.Creative.Audience = "teen"
	if err := SetKnob(&cfg, "audience", "unknown"); err != nil {
		t.Fatal(err)
	}
	if cfg.Creative.Audience != "" {
		t.Fatalf("非法值应归一为未设置，得到 %q", cfg.Creative.Audience)
	}
}

// 画风参数复用既有 SeriesConfig.VideoStyle 字段，选项从风格包生成。
func TestVideoStyleKnobUsesExistingField(t *testing.T) {
	var cfg domain.SeriesConfig
	if err := SetKnob(&cfg, "video_style", "ink"); err != nil {
		t.Fatal(err)
	}
	if cfg.VideoStyle != "ink" {
		t.Fatalf("画风未写入 SeriesConfig.VideoStyle: %q", cfg.VideoStyle)
	}
	k, _ := FindKnob("video_style")
	if len(k.Options) != len(VisualStyles) {
		t.Fatalf("画风选项数 %d，want %d", len(k.Options), len(VisualStyles))
	}
	if k.DefaultLabel != MatchStyle(DefaultStyleKey).Name+"（默认）" {
		t.Fatalf("画风默认名未与风格包对齐: %q", k.DefaultLabel)
	}
}

// brief 必须点名硬性规则并声明冲突时以规则为准。
func TestCreativeBriefDeclaresConflict(t *testing.T) {
	cfg := domain.SeriesConfig{}
	cfg.Creative.Narrative = "suspense"
	cfg.Creative.Instruction = "多用短句"
	sb := StoryBrief(cfg, "本集只讲一个晚上")
	for _, want := range []string{"【创作要求】", "悬疑倒叙", "多用短句", "本集只讲一个晚上", "冲突时以硬性规则为准", "文言引用"} {
		if !contains(sb, want) {
			t.Fatalf("StoryBrief 缺 %q:\n%s", want, sb)
		}
	}
	bb := BoardBrief(cfg, "")
	for _, want := range []string{"narration 必须逐字沿用讲述稿原文", "visual_prompt 严禁出现任何画风/质感/媒介词", "时代视觉锚定", "多用短句"} {
		if !contains(bb, want) {
			t.Fatalf("BoardBrief 缺 %q:\n%s", want, bb)
		}
	}
	// 篇幅档位的镜头数必须封在分镜校验上限（28）之内。
	cfg = domain.SeriesConfig{}
	cfg.Creative.Length = "long"
	if !contains(BoardBrief(cfg, ""), "不得超过 28") {
		t.Fatalf("长篇档位未封顶:\n%s", BoardBrief(cfg, ""))
	}
}

// 自由文本指令有长度上限。
func TestInstructionTruncated(t *testing.T) {
	long := make([]rune, MaxInstructionLen+50)
	for i := range long {
		long[i] = '字'
	}
	cfg := domain.SeriesConfig{}
	cfg.Creative.Instruction = string(long)
	got := StoryBrief(cfg, "")
	if !contains(got, string(long[:MaxInstructionLen])) {
		t.Fatal("截断后未保留前 MaxInstructionLen 个字符")
	}
	if contains(got, string(long)) {
		t.Fatal("超长指令未被截断")
	}
}

// Catalog 快照默认全空，可供前端直接渲染。
func TestCatalogShape(t *testing.T) {
	c := Catalog()
	if c.DefaultPreset != DefaultPresetKey {
		t.Fatalf("默认预设 key = %q", c.DefaultPreset)
	}
	if len(c.Knobs) != len(CreativeKnobs()) {
		t.Fatalf("参数数 %d，want %d", len(c.Knobs), len(CreativeKnobs()))
	}
	seen := map[string]bool{}
	for _, k := range c.Knobs {
		if k.Key == "" || k.Label == "" || k.DefaultLabel == "" {
			t.Fatalf("参数描述不完整: %+v", k)
		}
		if seen[k.Key] {
			t.Fatalf("参数 key 重复: %s", k.Key)
		}
		seen[k.Key] = true
		if k.Type == KnobEnum && len(k.Options) == 0 {
			t.Fatalf("枚举参数 %s 无选项", k.Key)
		}
		for _, o := range k.Options {
			if o.Key == "" || o.Label == "" {
				t.Fatalf("参数 %s 的选项描述不完整: %+v", k.Key, o)
			}
			// 「空值＝跟随默认」由前端渲染，选项表里不得混入默认项。
			if o.Label == k.DefaultLabel {
				t.Fatalf("参数 %s 的选项 %q 与默认名重复", k.Key, o.Key)
			}
		}
	}
	var cfg domain.SeriesConfig
	for _, k := range c.Knobs {
		if k.Type != KnobText && k.Options == nil {
			t.Fatalf("参数 %s 的选项不得为 null", k.Key)
		}
		switch k.Key {
		case "narrative", "audience", "length":
			knob, _ := FindKnob(k.Key)
			if knob.Get(cfg) != "" {
				t.Fatalf("默认 %s 应为空串", k.Key)
			}
		}
	}
}

// brief 为空时两个 UserPrompt 的输出必须与「没有 brief 这个概念」时逐字相同——
// 这是派生键稳定的前提（引擎把 brief 写进 story/storyboard 派生参数）。
func TestUserPromptDefaultBytesUnchanged(t *testing.T) {
	if got, want := StoryUserPrompt("鬼谷子", "战国", "张仪", ""), "栏目系列：鬼谷子\n朝代范围：战国\n本集主题/切入点：张仪\n请直接确定一个最好的故事并写出定稿口播稿（只输出一个，不要给候选）。"; got != want {
		t.Fatalf("故事 prompt 默认输出变了:\n got %q\nwant %q", got, want)
	}
	if got, want := StoryUserPrompt("鬼谷子", "", "", ""), "栏目系列：鬼谷子\n本集主题：由你在该系列范围内自选最有戏剧张力的一个故事。\n请直接确定一个最好的故事并写出定稿口播稿（只输出一个，不要给候选）。"; got != want {
		t.Fatalf("故事 prompt 空朝代/主题分支变了:\n got %q\nwant %q", got, want)
	}

	want := "故事标题：测试集\n朝代：战国\n\n" + MatchDynasty("战国").VisualAnchor() + "\n\n" +
		"【全片统一画风】" + MatchStyle("gongbi").Brief +
		"\n所有镜头必须保持同一画种；visual_prompt 中不要书写任何画风词，画风由后期统一施加。\n\n" +
		"成片画面比例：9:16（720P），由后期统一构图，visual_prompt 无需书写。\n" +
		"\n讲述稿正文：\n正文\n\n请按口播节奏拆分为分镜 JSON：narration 逐字沿用讲述稿原文（首镜钩子、末镜留白或留钩子），visual_prompt 只写画面内容、不含任何画风词。"
	if got := StoryboardUserPrompt("测试集", "战国", "正文", "9:16", "720P", "gongbi", "", nil, nil); got != want {
		t.Fatalf("分镜 prompt 默认输出变了:\n got %q\nwant %q", got, want)
	}

	// 有 brief 时只允许在「讲述稿正文」之前插入该段，其余逐字不动。
	brief := StoryBrief(domain.SeriesConfig{Creative: domain.CreativeStyle{Narrative: "suspense"}}, "")
	if brief == "" {
		t.Fatal("测试前提：brief 不应为空")
	}
	got := StoryboardUserPrompt("测试集", "战国", "正文", "9:16", "720P", "gongbi", brief, nil, nil)
	if wantWithBrief := strings.Replace(want, "\n讲述稿正文：", brief+"\n\n讲述稿正文：", 1); got != wantWithBrief {
		t.Fatalf("分镜 prompt 的 brief 注入位置不对:\n got %q\nwant %q", got, wantWithBrief)
	}
}
