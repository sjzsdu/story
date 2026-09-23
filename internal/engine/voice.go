package engine

import (
	"context"

	"github.com/sjzsdu/story/internal/domain"
	"github.com/sjzsdu/story/internal/templates"
)

// resolveVoice 从系列引用的声音条目解析旁白合成参数（§16）。
// 解析顺序：
//  1. series.VoiceID 命中 voices 表 → 用条目参数。
//  2. 旧字段兼容（VoiceProfile/TTSVoice/TTSInstruction/TTSRate/TTSPitch）：
//     VoiceProfile 命中内置条目则用之，否则直接读旧字段。
//  3. 全空 → fallbackVoice（默认 longtian_v3）+ fallbackInstr。
//
// 模型三级优先级（第二步系列级模型覆盖）：
// voice.Model（造声音色必用其驱动模型）非空必用 → 系列 cfg.TTSModel → 空
//（空＝provider 构造期的系统 tts_model 默认）。前两级都不允许被静默覆盖。
// 规则 1 是新数据路径，2 是迁移前的旧数据 fallback，保证旧 series 无 voice_id 时仍可工作。
// 作为方法是为了直接用 e.repo.GetVoice 查表（engine 已在 New 时注入 repo）。
func (e *Engine) resolveVoice(ctx context.Context, cfg domain.SeriesConfig, voiceID, fallbackVoice, fallbackInstr string) (voice, model string, rate, pitch float64, instr string) {
	// 规则 1：优先按 voice_id 查表（条目 Model 非空必用，否则落系列覆盖）。
	if voiceID != "" {
		if v, err := e.repo.GetVoice(ctx, voiceID); err == nil && v != nil {
			return v.Voice, firstNonEmpty(v.Model, cfg.TTSModel), v.Rate, v.Pitch, v.Instruction
		}
	}
	// 规则 2：旧字段兼容——预设 key 优先于裸 TTSVoice（模型同款两级回退）。
	if cfg.VoiceProfile != "" {
		p := templates.MatchVoiceProfile(cfg.VoiceProfile)
		return p.Voice, firstNonEmpty(p.Model, cfg.TTSModel), p.Rate, p.Pitch, p.Instruction
	}
	// 规则 3：旧字段裸读，最后兜底默认音色与指令；模型直接用系列覆盖（空＝系统默认）。
	voice = firstNonEmpty(cfg.TTSVoice, fallbackVoice)
	model = cfg.TTSModel
	rate = cfg.TTSRate
	pitch = cfg.TTSPitch
	instr = firstNonEmpty(cfg.TTSInstruction, fallbackInstr)
	return
}
