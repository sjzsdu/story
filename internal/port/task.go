package port

import "context"

// 视频异步任务状态（取值与 DashScope 对齐，实现侧需做兼容映射）。
const (
	TaskPending   = "PENDING"
	TaskRunning   = "RUNNING"
	TaskSucceeded = "SUCCEEDED"
	TaskFailed    = "FAILED"
	TaskUnknown   = "UNKNOWN"
)

// VideoTask 异步视频任务快照。
type VideoTask struct {
	TaskID   string
	Status   string
	VideoURL string
}

// TaskPoller 异步任务查询/等待。
// bl CLI 默认自带轮询，本接口保留给需要显式编排任务的场景。
type TaskPoller interface {
	GetVideoTask(ctx context.Context, taskID string) (VideoTask, error)
	WaitForVideo(ctx context.Context, taskID, outPath string) error
}
