package engine

import (
	"github.com/sjzsdu/story/internal/domain"
	"github.com/sjzsdu/story/internal/templates"
)

// resolveVoice 从系列配置解析旁白合成参数。
// 有预设 key 时由 voicelibrary 解析 voice/rate/pitch；否则用旧字段兼容（TTSVoice/TTSInstruction/TTSRate/TTSPitch）。
func resolveVoice(cfg domain.SeriesConfig, fallbackVoice, fallbackInstr string) (voice string, rate, pitch float64, instr string) {
	if cfg.VoiceProfile != "" {
		p := templates.MatchVoiceProfile(cfg.VoiceProfile)
		return p.Voice, p.Rate, p.Pitch, p.Instruction
	}
	voice = firstNonEmpty(cfg.TTSVoice, fallbackVoice)
	rate = cfg.TTSRate
	pitch = cfg.TTSPitch
	instr = firstNonEmpty(cfg.TTSInstruction, fallbackInstr)
	return
}
