package templates

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/sjzsdu/story/internal/domain"
)

// 本文件是创作控制参数的唯一知识源：新增一个画面选项或一个参数，
// 只在这里加一份声明式数据即可——engine / CLI / server / 前端都由本包驱动，
// 不需要知道任何具体参数名（前端靠 Catalog() 渲染控件）。
//
// 题材的故事质量不走这里：讲述结构、钩子、节奏、落点全部内建在
// prompts.go 的默认系统提示里（默认即唯一版本，见 §26 的收敛），
// 本注册表只保留「一眼能选的画面观感项」（画风 / 运镜 / 平台时长）。
//
// 三条硬约束：
//  1. 默认值一律空串。这些值经 StoryBrief/BoardBrief 进派生键，
//     internal/engine/derive.go 用 json.Marshal(params) 求 sha1，只要默认值非空，
//     存量系列的派生键立刻全变、下游被误判失效而重复调用付费模型。
//  2. 全部零值时 StoryBrief/BoardBrief 必须返回 ""（调用方据此跳过注入）。
//  3. brief 只能作为「创作要求」覆盖口味，不得推翻系统提示里的硬性规则，
//     因此文案里必须点名铁律并声明冲突时以硬性规则为准。

// MaxInstructionLen 自定义指令的最大长度（字符数，超出截断）。
const MaxInstructionLen = 500

// KnobType 参数控件类型。
type KnobType string

const (
	// KnobEnum 枚举：值必须是 Options 里的 key，空串＝跟随默认；未知值一律忽略。
	KnobEnum KnobType = "enum"
	// KnobText 自由文本。
	KnobText KnobType = "text"
)

// KnobOption 一个可选值及其对应的 prompt 片段（片段为空表示该阶段不受此值影响）。
type KnobOption struct {
	// Key 值 key（空串保留给「跟随默认」，不在 Options 中列出）。
	Key string
	// Label 中文名（前端显示）。
	Label string
	// Story 注入故事生成 prompt 的片段。
	Story string
	// Board 注入分镜拆解 prompt 的片段（可空：该值不影响切分）。
	Board string
}

// Knob 一个创作控制参数。
type Knob struct {
	Key   string
	Label string
	// Help 前端提示语（也可以写费用提醒）。
	Help string
	// CostImpact 改动本参数会触发画面/分镜重做（重新调用图片或视频模型，产生费用）。
	// 由后端显式声明，前端据此弹费用确认，不再靠 Help 里出现「费用」二字来判断。
	CostImpact bool
	// DefaultLabel 默认（空值）对应的中文名，供前端渲染「跟随默认：xxx」。
	DefaultLabel string
	Type         KnobType
	Options      []KnobOption
	// MaxLength 文本型参数的最大长度（字符数，0＝不限）；枚举型恒为 0。
	MaxLength int
	// Get/Set 让 app / engine / CLI 无需知道具体参数名即可读写 SeriesConfig——
	// 这是插件化的关键：新增参数只在本文件加一个 Knob。
	Get func(domain.SeriesConfig) string
	Set func(*domain.SeriesConfig, string)
}

// CatalogKnob 暴露给前端的参数描述（不含函数，可 JSON 序列化）。
type CatalogKnob struct {
	Key          string          `json:"key"`
	Label        string          `json:"label"`
	Help         string          `json:"help"`
	CostImpact   bool            `json:"cost_impact"`
	DefaultLabel string          `json:"default_label"`
	Type         KnobType        `json:"type"`
	MaxLength    int             `json:"max_length,omitempty"`
	Options      []CatalogOption `json:"options"`
}

// CatalogOption 暴露给前端的选项。
type CatalogOption struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// CreativeCatalog 一次性快照：前端据此渲染控件，不硬编码任何选项。
type CreativeCatalog struct {
	Knobs []CatalogKnob `json:"knobs"`
}

// CreativeKnobs 内置参数表，顺序即展示顺序。
func CreativeKnobs() []Knob {
	return []Knob{
		{
			Key:          "duration",
			Label:        "平台时长档位",
			Help:         "成片目标时长。留空＝全平台通用（4-7 分钟）；选 60 秒档画面费大幅下降。与正文篇幅正交，同时设置时以更短者为准。",
			DefaultLabel: "全平台通用（默认）",
			Type:         KnobEnum,
			Get:          func(c domain.SeriesConfig) string { return c.Creative.Duration },
			Set:          func(c *domain.SeriesConfig, v string) { c.Creative.Duration = v },
			Options: []KnobOption{
				{
					Key:   "d60",
					Label: "60 秒竖屏快节奏",
					Story: "目标时长 60 秒（抖音/快手竖屏快节奏档）：正文 300-420 字（约 60 秒口播），只跟一条主线、只留一个落点，出场人物不超过三个；钩子必须占满前两句，落点即结尾，砍掉一切不影响主线的交代。",
					Board: "镜头数：8-12 个（不得超过 12），单镜 5-10 秒；讲述稿很短，按口播节奏切分，宁可整句一镜也不要拆碎；narration 仍逐字沿用讲述稿原文。",
				},
			},
		},
		{
			Key:          "motion",
			Label:        "运镜强度",
			Help:         "小人书模式下插画推拉平移的幅度。改动会重新生成插画（产生费用）。",
			CostImpact:   true,
			DefaultLabel: "标准（默认）",
			Type:         KnobEnum,
			Get:          func(c domain.SeriesConfig) string { return c.Creative.Motion },
			Set:          func(c *domain.SeriesConfig, v string) { c.Creative.Motion = v },
			Options: []KnobOption{
				{Key: "strong", Label: "强"},
				{Key: "subtle", Label: "弱"},
			},
		},
		{
			Key:          "video_style",
			Label:        "画风",
			Help:         "全片统一画风。改动会使分镜与画面重做（产生费用）。",
			CostImpact:   true,
			DefaultLabel: MatchStyle(DefaultStyleKey).Name + "（默认）",
			Type:         KnobEnum,
			Get:          func(c domain.SeriesConfig) string { return c.VideoStyle },
			Set:          func(c *domain.SeriesConfig, v string) { c.VideoStyle = v },
			Options:      styleOptions(),
		},
	}
}

// styleOptions 画风选项直接从 visualstyle.go 的风格包生成，避免两处维护。
// 片段留空：画风已由分镜 prompt 的「全片统一画风」段与 provider 锚句统一供给。
func styleOptions() []KnobOption {
	opts := make([]KnobOption, 0, len(VisualStyles))
	for _, s := range VisualStyles {
		opts = append(opts, KnobOption{Key: s.Key, Label: s.Name})
	}
	return opts
}

// FindKnob 按 key 查参数。
func FindKnob(key string) (Knob, bool) {
	for _, k := range CreativeKnobs() {
		if k.Key == key {
			return k, true
		}
	}
	return Knob{}, false
}

// optionFragment 取该值在当前阶段对应的片段；值非法或该阶段不关心此值时返回空串。
func (k Knob) optionFragment(value string, board bool) string {
	if value == "" {
		return ""
	}
	for _, o := range k.Options {
		if o.Key != value {
			continue
		}
		if board {
			return o.Board
		}
		return o.Story
	}
	return ""
}

// normalize 归一化取值：枚举类参数的未知值一律当作「未设置」，与
// domain.NormalizeVisualMode 的回退精神一致（但这里是回到默认而非默认选项）。
func (k Knob) normalize(value string) string {
	if k.Type == KnobText {
		return value
	}
	if value == "" {
		return ""
	}
	for _, o := range k.Options {
		if o.Key == value {
			return value
		}
	}
	return ""
}

// SetKnob 按参数 key 写值（未知 key 报错、未知值回落默认）。
func SetKnob(cfg *domain.SeriesConfig, key, value string) error {
	k, ok := FindKnob(key)
	if !ok {
		return fmt.Errorf("未知创作参数 %q（支持：%s）", key, KnobKeys())
	}
	k.Set(cfg, k.normalize(value))
	return nil
}

// ApplyKnobs 按 map（{knobKey: value}）逐项写值，用于 API / CLI 的批量入口：
// 未知 key 报错并列出支持的 key；未知值回落「未设置」；未列出的参数保持原值。
func ApplyKnobs(cfg *domain.SeriesConfig, values map[string]string) error {
	for key, value := range values {
		if err := SetKnob(cfg, key, value); err != nil {
			return err
		}
	}
	return nil
}

// KnobKeys 全部参数 key（逗号分隔，供错误提示）。
func KnobKeys() string {
	keys := make([]string, 0, len(CreativeKnobs()))
	for _, k := range CreativeKnobs() {
		keys = append(keys, k.Key)
	}
	return strings.Join(keys, ", ")
}

// Catalog 生成前端渲染用的快照。
func Catalog() CreativeCatalog {
	knobs := CreativeKnobs()
	out := CreativeCatalog{
		Knobs: make([]CatalogKnob, 0, len(knobs)),
	}
	for _, k := range knobs {
		ck := CatalogKnob{
			Key:          k.Key,
			Label:        k.Label,
			Help:         k.Help,
			CostImpact:   k.CostImpact,
			DefaultLabel: k.DefaultLabel,
			Type:         k.Type,
			MaxLength:    k.MaxLength,
			Options:      make([]CatalogOption, 0, len(k.Options)),
		}
		for _, o := range k.Options {
			ck.Options = append(ck.Options, CatalogOption{Key: o.Key, Label: o.Label})
		}
		out.Knobs = append(out.Knobs, ck)
	}
	return out
}

// 冲突声明：用户要求只覆盖口味，硬性规则不动。
const storyBriefClosing = "【冲突声明】以上要求只调整讲述的口味与体量，不得违反本系统提示中的硬性规则：" +
	"文言引用全片不超过两处、每处不超过 15 字，史书冷账句必须白话转述，不得编造典籍中没有的史实与结局，结尾不得总结说理。冲突时以硬性规则为准。"

const boardBriefClosing = "【冲突声明】以上要求只调整镜头切分与文字密度，不得违反本系统提示中的硬性规则：" +
	"narration 必须逐字沿用讲述稿原文（只允许在镜头衔接处增删一两个顺承词），" +
	"visual_prompt 严禁出现任何画风/质感/媒介词，时代视觉锚定不得违反。冲突时以硬性规则为准。"

// StoryBrief 组装故事生成阶段的「创作要求」。
// 全部为默认值时返回空串（调用方据此跳过注入，保证默认产出与历史行为一致）。
func StoryBrief(cfg domain.SeriesConfig, episodeInstruction string) string {
	return buildBrief(cfg, episodeInstruction, "", false)
}

// BoardBrief 组装分镜拆解阶段的「创作要求」。同 StoryBrief，默认返回空串。
func BoardBrief(cfg domain.SeriesConfig, episodeInstruction string) string {
	return buildBrief(cfg, episodeInstruction, "", true)
}

// StoryBriefWithNote / BoardBriefWithNote 在系列与集级创作设置之外，再叠加
// 「本版附加要求」（换一版时用户填的迭代方向，只作用于这一版）。
// note 为空串时与 StoryBrief / BoardBrief 逐字一致。
func StoryBriefWithNote(cfg domain.SeriesConfig, episodeInstruction, note string) string {
	return buildBrief(cfg, episodeInstruction, note, false)
}

// BoardBriefWithNote 见 StoryBriefWithNote。
func BoardBriefWithNote(cfg domain.SeriesConfig, episodeInstruction, note string) string {
	return buildBrief(cfg, episodeInstruction, note, true)
}

func buildBrief(cfg domain.SeriesConfig, episodeInstruction, note string, board bool) string {
	var lines []string
	for _, k := range CreativeKnobs() {
		if frag := k.optionFragment(k.Get(cfg), board); frag != "" {
			lines = append(lines, "- "+k.Label+"："+frag)
		}
	}
	if s := ClipInstruction(episodeInstruction); s != "" {
		lines = append(lines, "- 本集附加指令："+s)
	}
	if s := ClipInstruction(note); s != "" {
		lines = append(lines, "- 本版附加要求（只改这一版，往这个方向迭代）："+s)
	}
	if len(lines) == 0 {
		return ""
	}
	closing := storyBriefClosing
	if board {
		closing = boardBriefClosing
	}
	return "【创作要求】\n" + strings.Join(lines, "\n") + "\n" + closing
}

// ClipInstruction 截断过长的自由文本指令（按字符数，不是字节数）。
// 组装 brief 时对「本集附加指令」与「本版附加要求」统一施加上限。
func ClipInstruction(s string) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= MaxInstructionLen {
		return s
	}
	return string([]rune(s)[:MaxInstructionLen])
}