package domain

// 视觉参考类别（VisualRef.Kind）。
const (
	// RefKindCharacter 人物：跨镜头必须保持一致的角色外貌/服饰。
	RefKindCharacter = "character"
	// RefKindScene 场景/环境：在多个镜头重复出现的地点（建筑形制、光线、氛围须一致）。
	RefKindScene = "scene"
)

// VisualRef 视觉参考：对图片/视频生成的一致性约束。
//
// 分两级：系列级（Series.Characters 派生，跨集复用）与集级（Episode.Refs，
// 单元剧如聊斋每集独立人物/场景）。每一条包含：
//   - Description 文字约束（零成本）：分镜 prompt 要求逐字复制，是一致性的基础；
//   - RefImage 可选参考图（按张计费，手动生成）：视频模式 produce 时喂给视频模型。
type VisualRef struct {
	// Kind 类别：character（人物）/ scene（场景）。空值归一为 character。
	Kind string `json:"kind"`
	// Name 参考名称：人物用姓名（如「聂小倩」），场景用地点名（如「兰若寺」）。
	// 分镜 visual_prompt 必须用此名指代，produce 据此匹配参考图。
	Name string `json:"name"`
	// Description 固定视觉描述：人物为外貌/服饰/年龄感；场景为建筑形制/光线/氛围。
	// 同一条参考在所有镜头中必须逐字一致。
	Description string `json:"description"`
	// RefImage 参考图绝对路径（生成后回填；可为空）。
	// 系列级位于 data/projects/<series>/refs/，集级位于 <episode>/refs/。
	RefImage string `json:"ref_image,omitempty"`
}

// NormalizeRefKind 归一参考类别：空值/未知值回退 character。
func NormalizeRefKind(k string) string {
	if k == RefKindScene {
		return RefKindScene
	}
	return RefKindCharacter
}
