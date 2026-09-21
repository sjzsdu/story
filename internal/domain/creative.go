package domain

// CreativeStyle 创作控制参数（系列级）。
//
// 全部字段零值＝空串＝内置默认，产出与历史行为完全一致：这些值不直接参与
// prompt，而是经 templates 注册表解析成「创作要求」文本（StoryBrief/BoardBrief）
// 后注入 prompt，并进派生键。因此新增字段必须带 omitempty，否则旧版本树
// 的派生键会整体改变、下游被误判为失效而重复调用付费模型。
type CreativeStyle struct {
	// Preset 套用的创作预设 key（仅作溯源与前端回填，不参与 prompt）。
	Preset string `json:"preset,omitempty"`
	// Narrative 叙事风格 key（说书人讲述＝空 / 纪录客观 / 当事人自述 / 悬疑倒叙）。
	Narrative string `json:"narrative,omitempty"`
	// Audience 目标受众 key（成人通识＝空 / 青少年 / 儿童）。
	Audience string `json:"audience,omitempty"`
	// Length 篇幅档位 key（中篇＝空 / 短篇 / 长篇）。
	Length string `json:"length,omitempty"`
	// Motion 运镜强度 key（标准＝空 / 强 / 弱），仅小人书（comic）模式生效。
	Motion string `json:"motion,omitempty"`
	// Instruction 自定义创作指令（自由文本，叠加在内置规则之上，冲突时以硬性规则为准）。
	Instruction string `json:"instruction,omitempty"`
}

// IsZero 判断是否全部为默认（未做任何设置），用于跳过组装与展示判断。
func (c CreativeStyle) IsZero() bool {
	return c == CreativeStyle{}
}
