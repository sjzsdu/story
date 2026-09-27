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
	// 只改运镜强度（纯画面参数）不得让故事/分镜重跑。
	cfg.Creative.Motion = "strong"
	if got := StoryBrief(cfg, ""); got != "" {
		t.Fatalf("只改运镜强度不应影响 StoryBrief:\n%s", got)
	}
	if got := BoardBrief(cfg, ""); got != "" {
		t.Fatalf("只改运镜强度不应影响 BoardBrief:\n%s", got)
	}
}

// 未知值回落为「未设置」，未知参数单独报错。
func TestSetKnobNormalizesUnknown(t *testing.T) {
	var cfg domain.SeriesConfig
	if err := SetKnob(&cfg, "duration", "不存在的档位"); err != nil {
		t.Fatal(err)
	}
	if cfg.Creative.Duration != "" {
		t.Fatalf("未知值应回落为空，得到 %q", cfg.Creative.Duration)
	}
	if err := SetKnob(&cfg, "nope", "x"); err == nil {
		t.Fatal("未知参数应报错")
	}
	// 已设置的参数不得被未知值擦掉。
	cfg.Creative.Motion = "strong"
	if err := SetKnob(&cfg, "motion", "unknown"); err != nil {
		t.Fatal(err)
	}
	if cfg.Creative.Motion != "" {
		t.Fatalf("非法值应归一为未设置，得到 %q", cfg.Creative.Motion)
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
	if err := SetKnob(&cfg, "duration", "d60"); err != nil {
		t.Fatal(err)
	}
	sb := StoryBrief(cfg, "本集只讲一个晚上")
	for _, want := range []string{"【创作要求】", "平台时长档位", "60 秒", "本集只讲一个晚上", "冲突时以硬性规则为准", "文言引用"} {
		if !contains(sb, want) {
			t.Fatalf("StoryBrief 缺 %q:\n%s", want, sb)
		}
	}
	bb := BoardBrief(cfg, "")
	for _, want := range []string{"narration 必须逐字沿用讲述稿原文", "visual_prompt 严禁出现任何画风/质感/媒介词", "时代视觉锚定", "不得超过 12"} {
		if !contains(bb, want) {
			t.Fatalf("BoardBrief 缺 %q:\n%s", want, bb)
		}
	}
}

// 集级附加指令有长度上限。
func TestInstructionTruncated(t *testing.T) {
	long := make([]rune, MaxInstructionLen+50)
	for i := range long {
		long[i] = '字'
	}
	got := StoryBrief(domain.SeriesConfig{}, string(long))
	if !contains(got, string(long[:MaxInstructionLen])) {
		t.Fatal("截断后未保留前 MaxInstructionLen 个字符")
	}
	if contains(got, string(long)) {
		t.Fatal("超长指令未被截断")
	}
}

// 「本版附加要求」（换一版时用户填的迭代方向）并入创作要求；
// 留空时必须与不带 note 的旧函数逐字一致，否则存量派生键会变。
func TestNoteBrief(t *testing.T) {
	cfg := domain.SeriesConfig{}
	if err := SetKnob(&cfg, "duration", "d60"); err != nil {
		t.Fatal(err)
	}

	if got, want := StoryBriefWithNote(cfg, "本集只讲一个晚上", ""), StoryBrief(cfg, "本集只讲一个晚上"); got != want {
		t.Fatalf("空 note 必须与不带 note 逐字一致:\n got %q\nwant %q", got, want)
	}
	if got, want := BoardBriefWithNote(cfg, "", ""), BoardBrief(cfg, ""); got != want {
		t.Fatalf("空 note 必须与不带 note 逐字一致:\n got %q\nwant %q", got, want)
	}

	// 只有 note、其余全默认时也要成段，并保留冲突声明。
	sb := StoryBriefWithNote(domain.SeriesConfig{}, "", "改成从行刑前一夜倒叙")
	for _, want := range []string{"【创作要求】", "本版附加要求", "改成从行刑前一夜倒叙", "冲突时以硬性规则为准"} {
		if !contains(sb, want) {
			t.Fatalf("StoryBrief 缺 %q:\n%s", want, sb)
		}
	}
	bb := BoardBriefWithNote(domain.SeriesConfig{}, "", "把朝堂争论压到三个镜头以内")
	for _, want := range []string{"本版附加要求", "把朝堂争论压到三个镜头以内", "narration 必须逐字沿用讲述稿原文"} {
		if !contains(bb, want) {
			t.Fatalf("BoardBrief 缺 %q:\n%s", want, bb)
		}
	}
	// 超长 note 同样按上限截断（上限只有一个来源）。
	long := strings.Repeat("字", MaxInstructionLen+10)
	if contains(StoryBriefWithNote(domain.SeriesConfig{}, "", long), long) {
		t.Fatal("超长 note 未被截断")
	}
}

// §26 收敛后只剩画面观感项：注册表不得再出现讲述口味类参数。
func TestOnlyVisualKnobsRemain(t *testing.T) {
	want := []string{"duration", "motion", "video_style"}
	got := CreativeKnobs()
	if len(got) != len(want) {
		t.Fatalf("参数数 %d，want %d：%s", len(got), len(want), KnobKeys())
	}
	for i, k := range got {
		if k.Key != want[i] {
			t.Fatalf("第 %d 个参数为 %s，want %s", i+1, k.Key, want[i])
		}
	}
}

// Catalog 快照默认全空，可供前端直接渲染。
func TestCatalogShape(t *testing.T) {
	c := Catalog()
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
		knob, _ := FindKnob(k.Key)
		if knob.Get(cfg) != "" {
			t.Fatalf("默认 %s 应为空串", k.Key)
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
	var cfg domain.SeriesConfig
	if err := SetKnob(&cfg, "duration", "d60"); err != nil {
		t.Fatal(err)
	}
	brief := StoryBrief(cfg, "")
	if brief == "" {
		t.Fatal("测试前提：brief 不应为空")
	}
	got := StoryboardUserPrompt("测试集", "战国", "正文", "9:16", "720P", "gongbi", brief, nil, nil)
	if wantWithBrief := strings.Replace(want, "\n讲述稿正文：", brief+"\n\n讲述稿正文：", 1); got != wantWithBrief {
		t.Fatalf("分镜 prompt 的 brief 注入位置不对:\n got %q\nwant %q", got, wantWithBrief)
	}
}

// §27 平台时长档位：d60 档在故事/分镜两阶段的 brief 片段。
func TestDurationKnobInBrief(t *testing.T) {
	var cfg domain.SeriesConfig
	if err := SetKnob(&cfg, "duration", "d60"); err != nil {
		t.Fatalf("SetKnob duration: %v", err)
	}
	sb := StoryBrief(cfg, "")
	for _, want := range []string{"平台时长档位", "60 秒", "300-420 字"} {
		if !contains(sb, want) {
			t.Fatalf("StoryBrief 缺 %q:\n%s", want, sb)
		}
	}
	bb := BoardBrief(cfg, "")
	for _, want := range []string{"8-12 个", "不得超过 12", "逐字沿用讲述稿原文"} {
		if !contains(bb, want) {
			t.Fatalf("BoardBrief 缺 %q:\n%s", want, bb)
		}
	}
	// 默认空值：brief 必须为空串（派生键稳定前提）。
	if got := StoryBrief(domain.SeriesConfig{}, ""); got != "" {
		t.Fatalf("全默认 StoryBrief 应为空串，got:\n%s", got)
	}
	// 未知值回落未设置。
	var cfg2 domain.SeriesConfig
	if err := SetKnob(&cfg2, "duration", "d90"); err != nil {
		t.Fatalf("未知值不应报错: %v", err)
	}
	if cfg2.Creative.Duration != "" {
		t.Fatalf("未知值应回落空串，got %q", cfg2.Creative.Duration)
	}
}