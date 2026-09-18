package domain

import "time"

// Series 系列（如「鬼谷子」），包含若干集。
type Series struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Dynasty     string       `json:"dynasty"`
	Description string       `json:"description"`
	Config      SeriesConfig `json:"config"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

// SeriesConfig 系列级配置，同一系列各集共享。
type SeriesConfig struct {
	// Dynasty 默认朝代（视觉/叙事锚定）。
	Dynasty string `json:"dynasty"`
	// Ratio 画面比例：9:16 / 16:9 / 1:1 / 3:4。
	Ratio string `json:"ratio"`
	// Resolution 分辨率：720P / 1080P。
	Resolution string `json:"resolution"`
	// VideoStyle 额外的视频风格指令（可空）。
	VideoStyle string `json:"video_style"`
	// TTSVoice 旁白音色 ID。
	TTSVoice string `json:"tts_voice"`
	// TTSInstruction 旁白风格自然语言指令。
	TTSInstruction string `json:"tts_instruction"`
	// TargetPlatforms 目标发布平台（仅记录，供导出参考）。
	TargetPlatforms []string `json:"target_platforms"`
	// MaxConcurrency 单集生产的最大并发镜头数。
	MaxConcurrency int `json:"max_concurrency"`
	// MaxRetries 单镜头失败最大重试次数。
	MaxRetries int `json:"max_retries"`
}
