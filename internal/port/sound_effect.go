package port

import "context"

// SoundEffectRequest 音效生成请求（§22）。
type SoundEffectRequest struct {
	// Prompt 音效描述（必填），如「木门吱呀推开，伴随轻微回响」。
	Prompt string
	// DurationSec 目标时长（秒）；<=0 用实现方默认。
	DurationSec float64
	// OutPath 音频落盘路径（必填）。
	OutPath string
	// Model 模型；空则省略（走实现方默认）。
	Model string
}

// SoundEffectResult 音效生成结果。
type SoundEffectResult struct {
	OutPath     string
	DurationSec float64
}

// SoundEffectGenerator 音效生成能力（§22）。
//
// 注意：当前 bl 没有任何音效命令，本接口只立契约与能力槽（sfx），装配层不注册
// 任何 provider；调用会得到能力槽「未配置默认实现」的明确错误，而不是静默失败。
type SoundEffectGenerator interface {
	GenerateSoundEffect(ctx context.Context, req SoundEffectRequest) (SoundEffectResult, error)
}
