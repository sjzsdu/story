package port

import "context"

// SpeechRequest 单段旁白合成请求。
type SpeechRequest struct {
	// OutPath 音频落盘路径。
	OutPath string
	// Text 旁白文本。
	Text string
	// Voice 音色 ID。
	Voice string
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
