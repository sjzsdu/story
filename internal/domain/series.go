package domain

import "time"

// Series 系列（如「鬼谷子」），包含若干集。
type Series struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Dynasty     string `json:"dynasty"`
	Description string `json:"description"`
	// VoiceID 引用的声音条目 ID（顶层 Voice 实体，§16）。
	// 创建系列时选定，之后锁定不可改（store.UpdateSeries SQL 不含该列）。
	VoiceID    string             `json:"voice_id"`
	Config     SeriesConfig       `json:"config"`
	Characters []CharacterSetting `json:"characters"`
	CreatedAt  time.Time          `json:"created_at"`
	UpdatedAt  time.Time          `json:"updated_at"`
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
	// TTSVoice 旁白音色 ID（自定义模式时直接指定；预设模式由 VoiceProfile 驱动）。
	TTSVoice string `json:"tts_voice"`
	// TTSInstruction 旁白风格自然语言指令（部分音色不支持，provider 自动降级）。
	TTSInstruction string `json:"tts_instruction"`
	// TTSRate 语速 0.5-2.0，默认 1.0。
	TTSRate float64 `json:"tts_rate,omitempty"`
	// TTSPitch 音高 0.5-2.0，默认 1.0。
	TTSPitch float64 `json:"tts_pitch,omitempty"`
	// VoiceProfile 旁白语音画像 key（旧字段；内置 key 如 longtian/longze/longcheng）。
	// 空值时使用 TTSVoice（兼容旧数据）；非空时由 voicelibrary.go 解析覆盖 TTSVoice/Rate/Pitch。
	VoiceProfile string `json:"voice_profile,omitempty"`
	// MaxConcurrency 单集生产的最大并发镜头数。
	MaxConcurrency int `json:"max_concurrency"`
	// MaxRetries 单镜头失败最大重试次数。
	MaxRetries int `json:"max_retries"`
	// Creative 创作控制参数（叙事风格/受众/篇幅/运镜强度/自定义指令）。
	// 零值＝内置默认，产出与历史行为一致；见 creative.go 与 templates/creative.go。
	// 用 omitzero（非 omitempty）——omitempty 对结构体无效，旧系列的 config_json
	// 会凭空多出 "creative":{}。
	Creative CreativeStyle `json:"creative,omitzero"`

	// ---- 系列级 Provider 覆盖（空＝用系统默认，见 app.resolveProviders） ----
	// TextProvider 文本生成供应商覆盖（bailian / deepseek）。
	TextProvider string `json:"text_provider,omitempty"`
	// TTSProvider 语音合成供应商覆盖（bailian / minimax）。
	TTSProvider string `json:"tts_provider,omitempty"`
	// ImageProvider 图片生成供应商覆盖（bailian / zhipu）。
	ImageProvider string `json:"image_provider,omitempty"`
	// VideoProvider 视频生成供应商覆盖（bailian / kling）。
	VideoProvider string `json:"video_provider,omitempty"`

	// ---- 系列级模型覆盖（第二步统一资源管理；空＝跟随系统默认，见 app.resolveProviders） ----
	// 四个字段与上方四个 Provider 字段同构：Provider 决定用哪家供应商，
	// Model 决定用该供应商的哪个模型；都为空时用 config 里的系统默认。
	// 必须 omitempty——创建/更新绝不回填系统默认值，否则存量系列 config_json
	// 字节变化、派生键全变（§17/§18 派生键铁律）。
	// TextModel 文本模型覆盖（故事 / 分镜 / 策划共用文本 provider 的模型）。
	TextModel string `json:"text_model,omitempty"`
	// TTSModel 旁白模型覆盖。优先级：voice.Model 非空必用 → 本字段 → 空（provider 默认）。
	TTSModel string `json:"tts_model,omitempty"`
	// ImageModel 图片模型覆盖（插画 / 定妆照 / 视觉参考图）。
	ImageModel string `json:"image_model,omitempty"`
	// VideoModel 视频模型覆盖（AI 视频片段）。
	VideoModel string `json:"video_model,omitempty"`

	// ---- 成片 BGM 背景音乐（§20）----
	// PlanningBrief 系列级「规划要求」：分集策划时自动注入首轮上下文的自由文本
	// （如目标集数、取材范围、叙事主线偏好），每次策划都生效，不必重复交代。
	// 普通 SeriesConfig 字段（语义同 bgm_path），不走 CreativeKnob 插件通道；
	// omitempty 保证存量系列 config_json 字节不变、派生键不受影响（§17/§18）。
	PlanningBrief string `json:"planning_brief,omitempty"`
	// BGMPath 背景音乐曲目路径；空＝无 BGM。
	// 约定：相对路径相对系列目录 data/projects/<series-id>/（能过 serveSeriesMedia
	// 白名单、Web 可试听）；也接受绝对路径（此时 Web 不提供试听）。
	// 普通 SeriesConfig 字段（语义同 subtitle_font），不走 CreativeKnob 插件通道。
	BGMPath string `json:"bgm_path,omitempty"`
	// BGMVolume 0..1 相对音量；0 或未设置＝用默认 0.18（旁白为主体，BGM 只做底噪）。
	// 归一化（0→0.18）在 ffmpeg provider 做，此处存原始值以保证派生键稳定。
	BGMVolume float64 `json:"bgm_volume,omitempty"`
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
