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
// 规则 1 是新数据路径，2 是迁移前的旧数据 fallback，保证旧 series 无 voice_id 时仍可工作。
// 作为方法是为了直接用 e.repo.GetVoice 查表（engine 已在 New 时注入 repo）。
func (e *Engine) resolveVoice(ctx context.Context, cfg domain.SeriesConfig, voiceID, fallbackVoice, fallbackInstr string) (voice string, rate, pitch float64, instr string) {
	// 规则 1：优先按 voice_id 查表。
	if voiceID != "" {
		if v, err := e.repo.GetVoice(ctx, voiceID); err == nil && v != nil {
			return v.Voice, v.Rate, v.Pitch, v.Instruction
		}
	}
	// 规则 2：旧字段兼容——预设 key 优先于裸 TTSVoice。
	if cfg.VoiceProfile != "" {
		p := templates.MatchVoiceProfile(cfg.VoiceProfile)
		return p.Voice, p.Rate, p.Pitch, p.Instruction
	}
	// 规则 3：旧字段裸读，最后兜底默认音色与指令。
	voice = firstNonEmpty(cfg.TTSVoice, fallbackVoice)
	rate = cfg.TTSRate
	pitch = cfg.TTSPitch
	instr = firstNonEmpty(cfg.TTSInstruction, fallbackInstr)
	return
}
