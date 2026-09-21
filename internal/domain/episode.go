package domain

import "time"

// Episode 一集视频 = 一个版本树容器。
//
// 四个阶段（story → storyboard → media → final）依次派生，每个阶段的一次产出
// 是版本树上的一个 VersionNode；从任意节点都能继续往下派生出新的成片，
// 旧分支完整保留。上游一变派生键就变，下游不会被误复用。
type Episode struct {
	ID       string `json:"id"`
	SeriesID string `json:"series_id"`
	Number   int    `json:"number"`
	Title    string `json:"title"`
	// Topic 本集主题/切入点（可空，空则由 AI 在系列范围内自由命题）。
	Topic string `json:"topic"`
	// Instruction 本集附加创作指令（自由文本，叠加在系列创作设置之上，
	// 同样只作为 prompt 里的「创作要求」，冲突时以硬性规则为准）。
	Instruction string `json:"instruction"`
	// Refs 本集视觉参考（人物 + 跨镜重复场景），分镜阶段产出、可人工编辑；
	// 参考图按需手动生成。属集级资源，跨分镜版本共享；与系列级人物设定合并后
	// 注入分镜/produce（同名集级优先）。
	Refs []VisualRef `json:"refs,omitempty"`
	// Nodes 版本树的全部节点。
	Nodes []*VersionNode `json:"nodes"`
	// ActiveNodeID 当前活跃节点：树上被选中/最新的一条分支的末端节点。
	ActiveNodeID string `json:"active_node_id"`
	// WorkDir 该集媒体文件的工作目录（版本产物落在其 versions/ 子目录）。
	WorkDir   string    `json:"workdir"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// NewEpisode 创建一集的初始结构（空版本树）。
func NewEpisode(id, seriesID string, number int, title, topic, workDir string) *Episode {
	now := time.Now()
	return &Episode{
		ID:       id,
		SeriesID: seriesID,
		Number:   number,
		Title:    title,
		Topic:    topic,
		// 显式空切片（而非 nil）：JSON 序列化为 []，前端无需处理 null。
		Nodes:     []*VersionNode{},
		WorkDir:   workDir,
		CreatedAt: now,
		UpdatedAt: now,
	}
}
