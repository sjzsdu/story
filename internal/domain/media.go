package domain

// MediaResult 单个镜头的媒体产物（视频片段或旁白音频）。
type MediaResult struct {
	// SceneID 对应镜头序号。
	SceneID int `json:"scene_id"`
	// Path 产物文件绝对路径。
	Path string `json:"path"`
	// DurationSec 经 ffprobe 探测的实际时长（秒）。
	DurationSec float64 `json:"duration_sec"`
	// Skipped 为 true 表示断点续跑时复用了已有文件。
	Skipped bool `json:"skipped,omitempty"`
	// Err 该镜头生产失败时的错误信息。
	Err string `json:"err,omitempty"`
}
