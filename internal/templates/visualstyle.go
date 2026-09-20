package templates

import "fmt"

// VisualStylePack 全片统一视觉风格定义。
//
// 风格协调机制：分镜 LLM 只描述画面内容（人物/动作/环境/光影），
// 画风词一律由本包统一供给——Brief 约束分镜文本，VideoAnchor 由
// engine 追加到每个视频生成 prompt（双保险），KeyframeClause 约束
// 同系列定妆照，保证「定妆照 ↔ 视频画面 ↔ 全系列各集」同一画种。
type VisualStylePack struct {
	Key  string
	Name string
	// Brief 注入分镜 prompt 的风格定义（供 LLM 遵守）。
	Brief string
	// VideoAnchor 追加到每个视频生成 prompt 末尾的固定锚句。
	VideoAnchor string
	// KeyframeClause 人物参考图 prompt 中的画风从句。
	KeyframeClause string
	// SceneClause 场景环境参考图 prompt 中的画风从句。
	SceneClause string
}

// DefaultStyleKey 系列未显式配置时的默认风格（与栏目工笔定妆照配套）。
const DefaultStyleKey = "gongbi"

// VisualStyles 可选视觉风格，顺序即展示顺序。
var VisualStyles = []VisualStylePack{
	{
		Key:  "gongbi",
		Name: "工笔重彩国风动画",
		Brief: "全片统一为「工笔重彩国风手绘动画」：绢本设色质感，线条工细流畅，" +
			"设色典雅浓郁（石青、朱砂、赭石、石绿、金粉），人物面部端正细腻、五官比例稳定，" +
			"背景同样用工笔重彩手绘，不允许照片质感。",
		VideoAnchor: "全片统一画风：工笔重彩国风手绘动画，绢本设色，线条工细，设色典雅浓郁（石青、朱砂、赭石、金粉），" +
			"人物与背景同为手绘质感；严禁写实真人、照片质感、3D渲染、日漫、水彩等异质风格。",
		KeyframeClause: "工笔重彩国风动画人物设定图，绢本设色，线条工细流畅，设色典雅浓郁，面部刻画端正细腻",
		SceneClause:    "工笔重彩国风动画场景设定图，绢本设色，线条工细，设色典雅浓郁（石青、朱砂、赭石、石绿）",
	},
	{
		Key:  "realistic",
		Name: "写实真人历史正剧",
		Brief: "全片统一为「写实真人历史正剧电影」质感：真人演员、实拍布景、电影级布光，" +
			"皮肤与织物为真实物理质感，色彩克制厚重，不允许任何动画、插画、卡通感。",
		VideoAnchor: "全片统一画风：写实真人历史正剧电影质感，真人演员，实拍布景，电影级自然光，皮肤织物真实物理细节，" +
			"色彩克制厚重；严禁动画、插画、卡通、3D渲染感。",
		KeyframeClause: "写实真人历史正剧人物定妆照，电影级布光，真实皮肤与织物质感，色调克制厚重",
		SceneClause:    "写实真人历史正剧场景概念图，电影级自然光，真实建筑与器物质感，色调克制厚重",
	},
	{
		Key:  "ink",
		Name: "水墨写意动画",
		Brief: "全片统一为「水墨写意动画」：宣纸水墨大写意，泼墨晕染，大量留白，" +
			"人物造型简劲夸张但神韵为先，黑白灰为主、淡赭轻点，不允许工笔重彩、写实或 3D 质感。",
		VideoAnchor: "全片统一画风：水墨写意手绘动画，宣纸质感，泼墨晕染，大面积留白，黑白灰为主淡赭轻点，" +
			"笔墨气韵生动；严禁工笔重彩、写实真人、3D渲染、日漫风格。",
		KeyframeClause: "水墨写意国风动画人物设定图，宣纸泼墨，大写意笔触，黑白灰为主淡赭轻点，神韵为先",
		SceneClause:    "水墨写意国风动画场景设定图，宣纸泼墨，大写意笔触，大面积留白，黑白灰为主淡赭轻点",
	},
}

// MatchStyle 按 key 返回风格包；空或未知 key 回退默认工笔风格。
func MatchStyle(key string) VisualStylePack {
	for _, p := range VisualStyles {
		if p.Key == key {
			return p
		}
	}
	return VisualStyles[0] // gongbi
}

// KeyframePrompt 按本风格渲染人物定妆照的图片生成 prompt。
func (p VisualStylePack) KeyframePrompt(dynasty, name, identity, appearance, temperament string) string {
	if dynasty == "" {
		dynasty = "古代"
	}
	return fmt.Sprintf(
		"中国古代历史人物立绘，%s：%s时期历史人物%s（%s）。形象：%s。气质：%s。"+
			"全身站姿，正面微侧，双手自然，纯净留白背景，衣纹线条流畅，适合作为人物形象参考图，高清细节。",
		p.KeyframeClause, dynasty, name, identity, appearance, temperament)
}

// SceneRefPrompt 按本风格渲染场景/环境参考图的图片生成 prompt（空镜设定图）。
func (p VisualStylePack) SceneRefPrompt(dynasty, name, description string) string {
	if dynasty == "" {
		dynasty = "古代"
	}
	return fmt.Sprintf(
		"中国古代历史场景环境设定图，%s：%s时期「%s」。环境：%s。"+
			"空镜全景，画面中不出现人物，建筑器物符合时代特征，氛围统一，适合作为场景视觉参考图，高清细节。",
		p.SceneClause, dynasty, name, description)
}
