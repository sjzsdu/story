// Package domain 包含流水线的纯领域模型，不依赖任何第三方包与具体实现。
package domain

import "time"

// StepName 流水线步骤名。
type StepName string

const (
	StepGenerate   StepName = "generate"   // 生成候选故事
	StepPick       StepName = "pick"       // 选定故事
	StepStoryboard StepName = "storyboard" // 分镜拆解
	StepProduce    StepName = "produce"    // 视频+语音生产
	StepCompose    StepName = "compose"    // 合成成片
)

// AllSteps 按执行顺序返回全部步骤。
func AllSteps() []StepName {
	return []StepName{StepGenerate, StepPick, StepStoryboard, StepProduce, StepCompose}
}

// StepStatus 步骤状态。
type StepStatus string

const (
	StatusPending  StepStatus = "pending"  // 未开始
	StatusRunning  StepStatus = "running"  // 执行中
	StatusReview   StepStatus = "review"   // 等待人工/Agent 验收确认
	StatusApproved StepStatus = "approved" // 验收通过
	StatusDone     StepStatus = "done"     // 已完成
	StatusFailed   StepStatus = "failed"   // 失败（可重试）
)

// StepState 单个步骤的运行时状态。
type StepState struct {
	Status    StepStatus `json:"status"`
	Attempts  int        `json:"attempts"`
	Error     string     `json:"error,omitempty"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// PipelineState 一集视频的完整流水线状态，整体序列化存入 SQLite。
type PipelineState struct {
	Current    StepName               `json:"current"`              // 当前所处步骤
	Steps      map[StepName]StepState `json:"steps"`                // 各步骤状态
	Candidates []StoryCandidate       `json:"candidates,omitempty"` // generate 产物
	Selected   *int                   `json:"selected,omitempty"`   // pick 结果（候选序号）
	Story      *StoryCandidate        `json:"story,omitempty"`      // 选中的故事（正文完整）
	Storyboard *Storyboard            `json:"storyboard,omitempty"` // 分镜
	Clips      []MediaResult          `json:"clips,omitempty"`      // 视频片段产物
	Audios     []MediaResult          `json:"audios,omitempty"`     // 旁白音频产物
	Outputs    []string               `json:"outputs,omitempty"`    // 成片路径
}

// NewPipelineState 创建初始状态。
func NewPipelineState() *PipelineState {
	steps := make(map[StepName]StepState, len(AllSteps()))
	now := time.Now()
	for _, s := range AllSteps() {
		steps[s] = StepState{Status: StatusPending, UpdatedAt: now}
	}
	return &PipelineState{Current: StepGenerate, Steps: steps}
}

// Mark 更新某步骤状态。
func (p *PipelineState) Mark(step StepName, status StepStatus, errMsg string) {
	st := p.Steps[step]
	st.Status = status
	st.UpdatedAt = time.Now()
	if errMsg != "" {
		st.Error = errMsg
	} else if status == StatusRunning || status == StatusDone || status == StatusFailed {
		st.Error = ""
	}
	p.Steps[step] = st
}

// BeginAttempt 标记步骤开始执行并累加尝试次数。
func (p *PipelineState) BeginAttempt(step StepName) {
	st := p.Steps[step]
	st.Status = StatusRunning
	st.Attempts++
	st.Error = ""
	st.UpdatedAt = time.Now()
	p.Steps[step] = st
	p.Current = step
}

// IsDone 判断整个流水线是否完成。
func (p *PipelineState) IsDone() bool {
	return p.Steps[StepCompose].Status == StatusDone
}
