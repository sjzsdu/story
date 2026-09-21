package domain

import "time"

// Stage 版本树的阶段：一集口播视频由四个阶段依次派生。
type Stage string

const (
	// StageStory 生成故事定稿。
	StageStory Stage = "story"
	// StageStoryboard 把故事拆分为分镜。
	StageStoryboard Stage = "storyboard"
	// StageMedia 按分镜生产每镜画面与旁白。
	StageMedia Stage = "media"
	// StageFinal 合成成片（含多比例导出）。
	StageFinal Stage = "final"
)

// AllStages 按派生顺序返回全部阶段。
func AllStages() []Stage {
	return []Stage{StageStory, StageStoryboard, StageMedia, StageFinal}
}

// StageLabel 返回阶段的中文名，供错误信息与日志使用。
func StageLabel(s Stage) string {
	switch s {
	case StageStory:
		return "生成故事"
	case StageStoryboard:
		return "拆分分镜"
	case StageMedia:
		return "生产画面与旁白"
	case StageFinal:
		return "合成成片"
	default:
		return string(s)
	}
}

// NodeStatus 版本节点状态。
type NodeStatus string

const (
	// NodePending 已创建但未执行。
	NodePending NodeStatus = "pending"
	// NodeRunning 执行中。
	NodeRunning NodeStatus = "running"
	// NodeDone 已成功产出。
	NodeDone NodeStatus = "done"
	// NodeFailed 失败（可再次执行续跑）。
	NodeFailed NodeStatus = "failed"
)

// VersionNode 版本树上的一个节点：某阶段在一组派生输入下的一次产出。
//
// ID 即派生键（内容寻址），由「派生规则版本 + 阶段 + 父节点 + 本步参数 + Attempt」
// 算出；上游任何输入变化都会得到新 ID 与新产物目录，因此旧产物天然不会被复用。
type VersionNode struct {
	// ID 节点唯一标识，等于派生键。
	ID string `json:"id"`
	// Stage 本节点所属阶段。
	Stage Stage `json:"stage"`
	// ParentID 上游节点 ID；根节点（story）为空。
	ParentID string `json:"parent_id,omitempty"`
	// Attempt 同一组派生输入下的第 n 次尝试（0 起）；>0 表示用户主动「换一版」。
	Attempt int `json:"attempt"`
	// Runs 本节点被执行的次数（失败重试与续跑都会累加）。
	Runs int `json:"runs"`
	// Note 本版附加要求：「重做（换一版）」时用户填的迭代方向，只作用于这一版。
	// 与系列创作设置（长期）、本集附加指令（整集）共同构成三层创作控制；
	// 它同时参与派生键，因此带不同 Note 的两次重做会得到两个版本，而不会被复用。
	Note string `json:"note,omitempty"`
	// Status 节点状态。
	Status NodeStatus `json:"status"`
	// Error 最近一次执行的错误信息。
	Error string `json:"error,omitempty"`
	// Dir 本节点产物目录（集工作目录下的 versions/<stage>-<key>）。
	Dir string `json:"dir"`
	// Story 故事阶段产物。
	Story *StoryCandidate `json:"story,omitempty"`
	// Storyboard 分镜阶段产物。
	Storyboard *Storyboard `json:"storyboard,omitempty"`
	// Refs 分镜阶段产出的集级视觉参考快照（事实源是 Episode.Refs，此处仅供展示）。
	Refs []VisualRef `json:"refs,omitempty"`
	// Clips 画面阶段产出的片段。
	Clips []MediaResult `json:"clips,omitempty"`
	// Audios 画面阶段产出的旁白。
	Audios []MediaResult `json:"audios,omitempty"`
	// Outputs 成片阶段产出的文件（含多比例导出）。
	Outputs []string `json:"outputs,omitempty"`
	// CreatedAt 节点创建时间。
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt 最近一次更新时间。
	UpdatedAt time.Time `json:"updated_at"`
}

// Mark 更新节点状态与错误信息。
func (n *VersionNode) Mark(status NodeStatus, errMsg string) {
	n.Status = status
	n.UpdatedAt = time.Now()
	if errMsg != "" {
		n.Error = errMsg
	} else if status == NodeRunning || status == NodeDone {
		n.Error = ""
	}
}

// BeginRun 标记节点开始执行并累加执行次数。
func (n *VersionNode) BeginRun() {
	n.Runs++
	n.Status = NodeRunning
	n.Error = ""
	n.UpdatedAt = time.Now()
}

// Done 判断节点是否已成功产出。
func (n *VersionNode) Done() bool { return n.Status == NodeDone }

// ---- Episode 上的版本树操作（纯函数）----

// NodeByID 按 ID 查找节点，找不到返回 nil。
func (ep *Episode) NodeByID(id string) *VersionNode {
	for _, n := range ep.Nodes {
		if n.ID == id {
			return n
		}
	}
	return nil
}

// Children 返回某节点的直接下游节点。
func (ep *Episode) Children(id string) []*VersionNode {
	var out []*VersionNode
	for _, n := range ep.Nodes {
		if n.ParentID == id {
			out = append(out, n)
		}
	}
	return out
}

// ActivePath 返回活跃路径（从根到 Episode.ActiveNodeID 的正序节点列表）。
func (ep *Episode) ActivePath() []*VersionNode {
	if ep.ActiveNodeID == "" {
		return nil
	}
	var rev []*VersionNode
	// 防御环：最多走 len(Nodes) 步。
	for id, steps := ep.ActiveNodeID, 0; id != "" && steps <= len(ep.Nodes); steps++ {
		n := ep.NodeByID(id)
		if n == nil {
			break
		}
		rev = append(rev, n)
		id = n.ParentID
	}
	out := make([]*VersionNode, 0, len(rev))
	for i := len(rev) - 1; i >= 0; i-- {
		out = append(out, rev[i])
	}
	return out
}

// ActiveNodeOfStage 返回活跃路径上指定阶段的节点；没有则返回 nil。
func (ep *Episode) ActiveNodeOfStage(stage Stage) *VersionNode {
	path := ep.ActivePath()
	for i := len(path) - 1; i >= 0; i-- {
		if path[i].Stage == stage {
			return path[i]
		}
	}
	return nil
}

// MaxAttempt 返回指定父节点下某阶段已存在的最大 Attempt；不存在时返回 -1。
// 「换一版」即在它的基础上 +1。
func (ep *Episode) MaxAttempt(parentID string, stage Stage) int {
	max := -1
	for _, n := range ep.Nodes {
		if n.ParentID == parentID && n.Stage == stage && n.Attempt > max {
			max = n.Attempt
		}
	}
	return max
}

// AddNode 追加节点；ID 已存在时原地替换（派生键相同即同一份产物）。
func (ep *Episode) AddNode(n *VersionNode) {
	for i, old := range ep.Nodes {
		if old.ID == n.ID {
			ep.Nodes[i] = n
			return
		}
	}
	ep.Nodes = append(ep.Nodes, n)
}

// RemoveSubtree 从树上移除某节点及其全部后代，返回被移除的节点（顺序为树中顺序）。
func (ep *Episode) RemoveSubtree(id string) []*VersionNode {
	dead := map[string]bool{id: true}
	for changed := true; changed; {
		changed = false
		for _, n := range ep.Nodes {
			if !dead[n.ID] && n.ParentID != "" && dead[n.ParentID] {
				dead[n.ID] = true
				changed = true
			}
		}
	}
	var removed, kept []*VersionNode
	for _, n := range ep.Nodes {
		if dead[n.ID] {
			removed = append(removed, n)
			continue
		}
		kept = append(kept, n)
	}
	ep.Nodes = kept
	return removed
}
