package port

import "context"

// ClipRequest 单个视频片段生成请求。
type ClipRequest struct {
	// OutPath 视频落盘路径。
	OutPath string
	// Prompt 画面描述。
	Prompt string
	// DurationSec 期望时长（秒）。
	DurationSec int
	// Ratio 画面比例，如 9:16。
	Ratio string
	// Resolution 720P / 1080P。
	Resolution string
	// ImagePath 参考图路径（可空，空为文生视频）。
	ImagePath string
	// Watermark 是否保留 AI 生成水印（合规要求，默认 true）。
	Watermark bool
}

// ClipResult 片段生成结果。
type ClipResult struct {
	OutPath string
	TaskID  string
}

// VideoGenerator 视频片段生成器。
// 实现应当同步等待任务完成并把文件下载到 OutPath。
type VideoGenerator interface {
	GenerateClip(ctx context.Context, req ClipRequest) (ClipResult, error)
}
