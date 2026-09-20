package domain

import "time"

// Series 系列（如「鬼谷子」），包含若干集。
type Series struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Dynasty     string             `json:"dynasty"`
	Description string             `json:"description"`
	Config      SeriesConfig       `json:"config"`
	Characters  []CharacterSetting `json:"characters"`
	CreatedAt   time.Time          `json:"created_at"`
	UpdatedAt   time.Time          `json:"updated_at"`
}

// SeriesConfig 系列级配置，同一系列各集共享。
type SeriesConfig struct {
	// Dynasty 默认朝代（视觉/叙事锚定）。
	Dynasty string `json:"dynasty"`
	// Ratio 画面比例：9:16 / 16:9 / 1:1 / 3:4。
	Ratio string `json:"ratio"`
	// Resolution 分辨率：720P / 1080P。
	Resolution string `json:"resolution"`
	// VideoStyle 全片统一画风 key：gongbi（默认）/ realistic / ink，见 visualstyle.go。
	VideoStyle string `json:"video_style"`
	// VisualMode 画面生产模式：comic（小人书：AI 插画 + ffmpeg Ken Burns，默认）
	// / video（AI 视频片段）。空值与未知值回退 comic。
	VisualMode string `json:"visual_mode"`
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

// 画面生产模式（SeriesConfig.VisualMode）。
const (
	// VisualModeComic 小人书模式：每镜一张 AI 插画，ffmpeg 做 Ken Burns
	// 推/拉/平移运镜渲染成片段。成本低、画风稳，与口播为主的定位匹配。
	VisualModeComic = "comic"
	// VisualModeVideo AI 视频模式：每镜调用视频生成模型产出动态片段。
	VisualModeVideo = "video"
)

// NormalizeVisualMode 归一化画面模式：空值与未知值回退默认 comic。
func NormalizeVisualMode(m string) string {
	switch m {
	case VisualModeComic, VisualModeVideo:
		return m
	default:
		return VisualModeComic
	}
}
