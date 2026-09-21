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

// Ken Burns 运镜 key（comic 小人书模式：单张静帧渲染成运动片段）。
const (
	MotionPushIn   = "push_in"  // 缓推（向画面中心放大）
	MotionPullOut  = "pull_out" // 拉远
	MotionPanLeft  = "pan_left" // 向左横移
	MotionPanRight = "pan_right"
	MotionPanUp    = "pan_up"
	MotionPanDown  = "pan_down"
	MotionStatic   = "static" // 定格（极轻微缩放或不动）
)

// 运镜强度（StillRequest.MotionStrength）：留空＝标准档，幅度与历史行为一致。
const (
	MotionStrengthStrong = "strong" // 幅度更大
	MotionStrengthSubtle = "subtle" // 幅度更小
)

// StillRequest 静帧渲染请求：把一张插画按指定运镜渲染为同规格视频片段
// （comic 模式用，纯本地 ffmpeg，不产生模型费用）。
type StillRequest struct {
	ImagePath   string
	OutPath     string
	DurationSec int
	Ratio       string // 9:16 / 16:9 / 1:1 / 3:4
	Resolution  string // 720P / 1080P
	Motion      string // 见 Motion* 常量，空值由实现兜底
	// MotionStrength 运镜幅度档：见 MotionStrength* 常量，空值＝标准档（默认）。
	// 定格（MotionStatic）时无意义，实现忽略。
	MotionStrength string
}

// VideoComposer 视频后处理：归一化、混音、拼接、字幕、多比例导出。
type VideoComposer interface {
	Compose(ctx context.Context, req ComposeRequest) (ComposeResult, error)
	Export(ctx context.Context, req ExportRequest) error
	// RenderStill 单张静帧 + Ken Burns 运镜 → 视频片段（无音轨，音频在 Compose 阶段混入）。
	RenderStill(ctx context.Context, req StillRequest) error
	// ProbeDuration 探测媒体文件时长（秒），供验收使用。
	ProbeDuration(ctx context.Context, path string) (float64, error)
}

// AudioNormalizer 把任意来源音频归一化为供应商可用的统一格式。
//
// 场景（§16 声音复刻）：浏览器录音产出 `audio/webm;codecs=opus`（Chrome/Firefox）
// 或 `audio/mp4`（Safari），用户上传件格式与采样率也五花八门，供应商复刻接口
// 不接受这些形态，故统一转 16kHz 单声道 PCM wav 后再提交。
// 实现方为 ffmpeg provider（与 VideoComposer 同一实例）。
type AudioNormalizer interface {
	// NormalizeAudio 把 srcPath 转码为 16kHz 单声道 16-bit PCM wav（dstPath），
	// 并返回转码后的时长（秒），供调用方做长度校验与 UI 提示。
	NormalizeAudio(ctx context.Context, srcPath, dstPath string) (float64, error)
}
