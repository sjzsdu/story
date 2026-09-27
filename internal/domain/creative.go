package domain

// CreativeStyle 创作控制参数（系列级）。
//
// 这里只保留「一眼能选的画面观感项」：讲述本身的口味（结构、钩子、节奏、落点）
// 是项目内建的唯一默认，不再做成可调参数（见 §26 的收敛）。
//
// 全部字段零值＝空串＝内置默认，产出与历史行为完全一致：这些值不直接参与
// prompt，而是经 templates 注册表解析成「创作要求」文本（StoryBrief/BoardBrief）
// 后注入 prompt，并进派生键。因此新增字段必须带 omitempty，否则旧版本树
// 的派生键会整体改变、下游被误判为失效而重复调用付费模型。
type CreativeStyle struct {
	// Motion 运镜强度 key（标准＝空 / 强 / 弱），仅小人书（comic）模式生效。
	Motion string `json:"motion,omitempty"`
	// Duration 平台时长档位 key（全平台 4-7 分钟＝空 / 60 秒竖屏快节奏），
	// 见 templates.CreativeKnobs 的 duration；与正文篇幅正交，冲突时以更短的为准。
	Duration string `json:"duration,omitempty"`
}

// IsZero 判断是否全部为默认（未做任何设置），用于跳过组装与展示判断。
func (c CreativeStyle) IsZero() bool {
	return c == CreativeStyle{}
}