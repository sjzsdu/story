package templates

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/sjzsdu/story/internal/domain"
)

// 本文件是创作控制参数的唯一知识源：新增一个风格选项、一个参数或一个预设，
// 只在这里加一份声明式数据即可——engine / CLI / server / 前端都由本包驱动，
// 不需要知道任何具体参数名（前端靠 Catalog() 渲染控件）。
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

// Preset 创作预设：一键套用一组参数值（可再逐项微调）。
type Preset struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	Desc string `json:"desc"`
	// Values 形如 {knobKey: value}；未列出的参数一律回落默认。
	// 默认预设的 Values 必须为空——「一键套用默认 = 与历史行为完全一致」。
	Values map[string]string `json:"values"`
}

// CatalogKnob 暴露给前端的参数描述（不含函数，可 JSON 序列化）。
type CatalogKnob struct {
	Key          string          `json:"key"`
	Label        string          `json:"label"`
	Help         string          `json:"help"`
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
	Knobs         []CatalogKnob `json:"knobs"`
	Presets       []Preset      `json:"presets"`
	DefaultPreset string        `json:"default_preset"`
}

// DefaultPresetKey 默认预设（values 全空＝与历史行为一致）。
const DefaultPresetKey = "classic"

// CreativeKnobs 内置参数表，顺序即展示顺序。
func CreativeKnobs() []Knob {
	return []Knob{
		{
			Key:          "narrative",
			Label:        "叙事风格",
			Help:         "讲述的口吻与结构。留空＝说书人讲述（项目默认）。",
			DefaultLabel: "说书人讲述（默认）",
			Type:         KnobEnum,
			Get:          func(c domain.SeriesConfig) string { return c.Creative.Narrative },
			Set:          func(c *domain.SeriesConfig, v string) { c.Creative.Narrative = v },
			Options: []KnobOption{
				{
					Key:   "documentary",
					Label: "纪录客观",
					Story: "叙事风格改为纪录片旁白：冷静、克制，以史实与因果为主，不设问、不卖关子、不流露讲述者本人的情绪；可以按时间顺序推进，不必打散史料顺序。",
					Board: "讲述口吻为纪录片旁白：切分时不得改写讲述稿措辞，只在镜头衔接处补顺承词，全片保持同一个声音。",
				},
				{
					Key:   "first_person",
					Label: "当事人自述",
					Story: "叙事风格改为当事人第一人称自述：用「我」来讲，写他当时看见什么、做了什么、心里怎么想，可以写他当时并不知道、后来才明白的事；语气私密，带悔意、愕然或自嘲。",
					Board: "讲述口吻为当事人第一人称自述：切分时不得改写讲述稿措辞，只在镜头衔接处补顺承词，全片保持同一个「我」的声音。",
				},
				{
					Key:   "suspense",
					Label: "悬疑倒叙",
					Story: "叙事风格改为悬疑倒叙：第一段之内就把最反常的一幕或结局摆到听众眼前，再一层层倒回去交代经过，关键信息压到最后一刻才交代，全程不让听众松气。",
					Board: "讲述口吻为悬疑倒叙：切分时不得改写讲述稿措辞，首镜必须是最反常的那一幕，末镜必须停在钩子上。",
				},
			},
		},
		{
			Key:          "audience",
			Label:        "目标受众",
			Help:         "讲述的深浅与分寸。留空＝成人通识（项目默认）。",
			DefaultLabel: "成人通识（默认）",
			Type:         KnobEnum,
			Get:          func(c domain.SeriesConfig) string { return c.Creative.Audience },
			Set:          func(c *domain.SeriesConfig, v string) { c.Creative.Audience = v },
			Options: []KnobOption{
				{
					Key:   "teen",
					Label: "青少年",
					Story: "目标受众是青少年：把背景与人物关系交代清楚，尽量不使用文言原话，节奏更快；涉及血腥、酷刑与阴暗权谋处点到为止，不做细节铺陈。",
				},
				{
					Key:   "kid",
					Label: "儿童",
					Story: "目标受众是儿童：语言浅白，人物是非分明，冲突简单直接；严禁血腥、恐怖与成人化的阴暗情节，把故事讲到孩子能听懂、听完不害怕。",
				},
			},
		},
		{
			Key:          "length",
			Label:        "篇幅档位",
			Help:         "讲述稿字数与镜头数。留空＝中篇（1200-1800 字 / 18-26 镜）。",
			DefaultLabel: "中篇（默认）",
			Type:         KnobEnum,
			Get:          func(c domain.SeriesConfig) string { return c.Creative.Length },
			Set:          func(c *domain.SeriesConfig, v string) { c.Creative.Length = v },
			Options: []KnobOption{
				{
					Key:   "short",
					Label: "短篇",
					Story: "篇幅：正文 800-1200 字，比默认更紧凑，只保留一条主线，砍掉旁枝人物与外围事件。",
					Board: "镜头数：16-20 个（按口播节奏切分，不得少于 16）。",
				},
				{
					Key:   "long",
					Label: "长篇",
					Story: "篇幅：正文 1800-2400 字，比默认更铺得开，允许并置更多因果线索，处境与关键场面写得更细。",
					Board: "镜头数：24-28 个（不得超过 28 个）。",
				},
			},
		},
		{
			Key:          "motion",
			Label:        "运镜强度",
			Help:         "小人书模式下插画推拉平移的幅度。改动会重新生成插画（产生费用）。",
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
			DefaultLabel: MatchStyle(DefaultStyleKey).Name + "（默认）",
			Type:         KnobEnum,
			Get:          func(c domain.SeriesConfig) string { return c.VideoStyle },
			Set:          func(c *domain.SeriesConfig, v string) { c.VideoStyle = v },
			Options:      styleOptions(),
		},
		{
			Key:          "instruction",
			Label:        "自定义创作指令",
			Help:         fmt.Sprintf("自由文本，只调整口味（如「多用短句」「聚焦一个夜晚」）。上限 %d 字；不得违反硬性规则，冲突时以硬性规则为准。", MaxInstructionLen),
			DefaultLabel: "无（默认）",
			Type:         KnobText,
			MaxLength:    MaxInstructionLen,
			Get:          func(c domain.SeriesConfig) string { return c.Creative.Instruction },
			Set:          func(c *domain.SeriesConfig, v string) { c.Creative.Instruction = ClipInstruction(v) },
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

// Expand 展开「预设 + 逐项微调」为一个新的配置快照（不读现有值）：
// 先套用预设，再按 values 逐项覆盖。未知预设 / 未知参数 key 报错。
// 创建系列的入口（CLI / HTTP）用它把请求体转成 SeriesConfig 字段。
func Expand(preset string, values map[string]string) (domain.SeriesConfig, error) {
	var cfg domain.SeriesConfig
	if p := strings.TrimSpace(preset); p != "" {
		if err := ApplyPreset(&cfg, p); err != nil {
			return cfg, err
		}
	}
	if err := ApplyKnobs(&cfg, values); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// KnobKeys 全部参数 key（逗号分隔，供错误提示）。
func KnobKeys() string {
	keys := make([]string, 0, len(CreativeKnobs()))
	for _, k := range CreativeKnobs() {
		keys = append(keys, k.Key)
	}
	return strings.Join(keys, ", ")
}

// CreativePresets 内置预设。默认预设的 Values 必须为空。
func CreativePresets() []Preset {
	return []Preset{
		{
			Key:    DefaultPresetKey,
			Name:   "经典评书（默认）",
			Desc:   "项目内置的说书人讲述：成人向、中篇、标准运镜、工笔重彩。不改任何参数。",
			Values: map[string]string{},
		},
		{
			Key:  "documentary",
			Name: "纪录冷叙述",
			Desc: "纪录片旁白口吻，克制不煽情，镜头动得更轻。",
			Values: map[string]string{
				"narrative": "documentary",
				"motion":    "subtle",
			},
		},
		{
			Key:  "kids",
			Name: "儿童启蒙",
			Desc: "面向儿童：语言浅白、情节简单、镜头动得轻。",
			Values: map[string]string{
				"audience": "kid",
				"motion":   "subtle",
			},
		},
		{
			Key:  "suspense",
			Name: "悬疑倒叙",
			Desc: "先给结局再倒叙，短篇快节奏，镜头动得更强。",
			Values: map[string]string{
				"narrative": "suspense",
				"length":    "short",
				"motion":    "strong",
			},
		},
	}
}

// FindPreset 按 key 查预设。
func FindPreset(key string) (Preset, bool) {
	for _, p := range CreativePresets() {
		if p.Key == key {
			return p, true
		}
	}
	return Preset{}, false
}

// ApplyPreset 套用预设：先记下预设 key（仅溯源），再逐项写值。
// 已有参数值会被预设覆盖；预设未列出的参数保持原值不动。
func ApplyPreset(cfg *domain.SeriesConfig, key string) error {
	p, ok := FindPreset(key)
	if !ok {
		return fmt.Errorf("未知创作预设 %q", key)
	}
	// 默认预设不落 key：它等价于「未套用任何预设」，保持零值才能确保
	// 一键套用默认与历史行为逐字一致。
	if p.Key != DefaultPresetKey {
		cfg.Creative.Preset = p.Key
	}
	for k, v := range p.Values {
		if err := SetKnob(cfg, k, v); err != nil {
			return err
		}
	}
	return nil
}

// Catalog 生成前端渲染用的快照。
func Catalog() CreativeCatalog {
	knobs := CreativeKnobs()
	out := CreativeCatalog{
		Knobs:         make([]CatalogKnob, 0, len(knobs)),
		Presets:       CreativePresets(),
		DefaultPreset: DefaultPresetKey,
	}
	for _, k := range knobs {
		ck := CatalogKnob{
			Key:          k.Key,
			Label:        k.Label,
			Help:         k.Help,
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
	return buildBrief(cfg, episodeInstruction, false)
}

// BoardBrief 组装分镜拆解阶段的「创作要求」。同 StoryBrief，默认返回空串。
func BoardBrief(cfg domain.SeriesConfig, episodeInstruction string) string {
	return buildBrief(cfg, episodeInstruction, true)
}

func buildBrief(cfg domain.SeriesConfig, episodeInstruction string, board bool) string {
	var lines []string
	for _, k := range CreativeKnobs() {
		if frag := k.optionFragment(k.Get(cfg), board); frag != "" {
			lines = append(lines, "- "+k.Label+"："+frag)
		}
	}
	if s := ClipInstruction(cfg.Creative.Instruction); s != "" {
		lines = append(lines, "- 创作指令（系列级）："+s)
	}
	if s := ClipInstruction(episodeInstruction); s != "" {
		lines = append(lines, "- 本集附加指令："+s)
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
// 写库（app.UpdateSeriesCreative）与组装 brief 都用它，保证上限只有一个来源。
func ClipInstruction(s string) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= MaxInstructionLen {
		return s
	}
	return string([]rune(s)[:MaxInstructionLen])
}
