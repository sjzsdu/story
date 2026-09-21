package port

import (
	"context"

	"github.com/sjzsdu/story/internal/domain"
)

// SpeechRequest 单段旁白合成请求。
type SpeechRequest struct {
	// OutPath 音频落盘路径。
	OutPath string
	// Text 旁白文本。
	Text string
	// Voice 音色 ID。
	Voice string
	// Model 驱动模型（造声出来的音色必须传造声时的 target_model；空则用实现默认模型）。
	Model string
	// Instruction 自然语言风格指令。
	Instruction string
	// Rate 语速 0.5-2.0，默认 1.0。
	Rate float64
	// Pitch 音高 0.5-2.0，默认 1.0。
	Pitch float64
	// Format 音频格式（mp3/wav），默认 mp3。
	Format string
}

// SpeechResult 语音合成结果。
type SpeechResult struct {
	OutPath string
}

// SpeechSynthesizer 旁白语音合成器。
type SpeechSynthesizer interface {
	Synthesize(ctx context.Context, req SpeechRequest) (SpeechResult, error)
}

// VoiceLister 系统音色列表查询（浏览音色库创建声音时使用）。
// 实现方调用供应商的列音色能力（bl --list-voices），返回指定 TTS 模型可用的
// 系统音色。该查询只取元数据、不合成语音，不产生合成费用；结果不落库
// （音色随供应商持续更新，避免维护硬编码清单）。
type VoiceLister interface {
	// ListSystemVoices 列出某 TTS 模型的系统音色；model 为空时用实现默认模型。
	ListSystemVoices(ctx context.Context, model string) ([]domain.SystemVoice, error)
}

// VoiceBuildRequest 造声请求（声音设计 / 声音复刻共用，§16）。
type VoiceBuildRequest struct {
	// Kind 造声方式：domain.VoiceBuildDesign（文字设计）/
	// domain.VoiceBuildClone（音频复刻）。
	Kind string
	// TargetModel 驱动音色的合成模型（必填）。造出的音色必须用同一模型合成，
	// 否则合成会失败；因此调用方应把它写回 Voice.Model。
	TargetModel string
	// Prefix 音色名前缀（仅字母数字、≤10 位，供应商生成完整音色名用）。
	Prefix string
	// Prompt 声音描述文本（Kind=design 必填，≤500 字，中英文）。
	Prompt string
	// PreviewText 试听文本（Kind=design 必填，≤200 字）。
	PreviewText string
	// PreviewAudioPath 试听音频落盘路径（design 返回的 base64 音频写入此处；
	// 留空则不落盘）。
	PreviewAudioPath string
	// AudioURL 复刻音频：公网可访问 URL / oss:// / data: base64（Kind=clone 用）。
	// 与 AudioPath 二选一，URL 优先。
	AudioURL string
	// AudioPath 复刻音频的本地文件路径（Kind=clone 用）；
	// 调用方/provider 负责上传为供应商可访问的 URL。
	AudioPath string
	// LanguageHints 语种提示（默认 ["zh"]）。
	LanguageHints []string
	// MaxPromptAudioLength 复刻参考音频最大时长秒（3-30，0 用供应商默认）。
	MaxPromptAudioLength float64
	// EnablePreprocess 复刻音频预处理（降噪/增强），有背景噪音时开启。
	EnablePreprocess bool
}

// VoiceBuildResult 造声结果。
type VoiceBuildResult struct {
	// VoiceID 供应商返回的音色 ID（可直接用于合成的 voice 参数）。
	VoiceID string
	// TargetModel 实际生效的驱动模型（供应商回显或请求值）。
	TargetModel string
	// PreviewAudioPath 试听音频落盘路径（design 返回；clone 可能为空）。
	PreviewAudioPath string
	// RequestID 供应商请求 ID（排查用）。
	RequestID string
}

// VoiceBuilder 供应商造声能力（声音设计 / 声音复刻，§16）。
//
// 注意：百炼 CLI（bl）没有提供造声命令，实现方需直连供应商 HTTP 接口
// （架构例外，见 AGENTS.md §2/§16）。造声按新建音色个数计费。
type VoiceBuilder interface {
	BuildVoice(ctx context.Context, req VoiceBuildRequest) (VoiceBuildResult, error)
}
