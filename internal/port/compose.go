package port

import "context"

// ClipTrack 一个镜头的合成输入：视频片段 + 旁白音频 + 旁白文本（用于字幕）。
type ClipTrack struct {
	SceneID   int
	ClipPath  string
	AudioPath string
	Narration string
}

// ComposeRequest 成片合成请求。
type ComposeRequest struct {
	// WorkDir 中间文件目录（归一化片段、SRT、concat 清单）。
	WorkDir string
	// Tracks 按顺序排列的镜头轨道。
	Tracks []ClipTrack
	// Ratio 目标比例 9:16 / 16:9 / 1:1 / 3:4。
	Ratio string
	// Resolution 720P / 1080P。
	Resolution string
	// FinalPath 成片输出路径。
	FinalPath string
	// BurnSubtitles 是否烧录硬字幕。
	BurnSubtitles bool
}

// ComposeResult 合成结果。
type ComposeResult struct {
	FinalPath   string
	DurationSec float64
	Width       int
	Height      int
}

// ExportRequest 从已成片导出其他比例版本（模糊背景填充，不重新生成视频）。
type ExportRequest struct {
	SrcPath    string
	DstPath    string
	Ratio      string
	Resolution string
}

// VideoComposer 视频后处理：归一化、混音、拼接、字幕、多比例导出。
type VideoComposer interface {
	Compose(ctx context.Context, req ComposeRequest) (ComposeResult, error)
	Export(ctx context.Context, req ExportRequest) error
	// ProbeDuration 探测媒体文件时长（秒），供验收使用。
	ProbeDuration(ctx context.Context, path string) (float64, error)
}
