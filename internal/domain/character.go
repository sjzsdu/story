package domain

// CharacterSetting 系列人物设定：保证同一人物跨集、跨镜头形象一致的第一层约束。
// Appearance 字段会被分镜 prompt 逐字注入，要求所有镜头原样复用。
type CharacterSetting struct {
	// Name 人物名（正史人名，如 张仪）。分镜 visual_prompt 必须用此名指代。
	Name string `json:"name"`
	// Identity 身份 / 角色，如「秦国相国，纵横家」。
	Identity string `json:"identity"`
	// Appearance 外貌与服饰的固定描述（一句，含服饰、发式、年龄感、体态）。
	Appearance string `json:"appearance"`
	// Temperament 气质与神态基调，如「沉毅多智，眉宇含锋」。
	Temperament string `json:"temperament"`
	// RefImage 定妆照文件名（存于系列 refs/ 目录，生成后回填；可为空）。
	RefImage string `json:"ref_image,omitempty"`
}
