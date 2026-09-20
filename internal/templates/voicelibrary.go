package templates

import "github.com/sjzsdu/story/internal/domain"

// VoicePresets 预设语音画像库。
// 使用百炼系统声音 + 语速/音高参数模拟讲述风格，非真人声音克隆。
// 声音列表见 bl speech synthesize --list-voices --model cosyvoice-v3-flash。
var VoicePresets = []domain.VoiceProfile{
	{
		Key:        domain.VoiceProfileWangliqun,
		Name:       "王立群风格",
		Voice:      "longtian_v3",
		Rate:       0.9,
		Pitch:      1.0,
		StyleNote:  "磁性理智男，语速沉稳，有书卷气，节奏从容",
		Instruction: "",
	},
	{
		Key:        domain.VoiceProfileKaishu,
		Name:       "凯叔风格",
		Voice:      "longze_v3",
		Rate:       0.85,
		Pitch:      1.0,
		StyleNote:  "温暖元气男，语速偏慢，有亲和力和戏剧化停顿",
		Instruction: "",
	},
	{
		Key:        domain.VoiceProfileYizhongtian,
		Name:       "易中天风格",
		Voice:      "longcheng_v3",
		Rate:       1.05,
		Pitch:      1.0,
		StyleNote:  "智慧青年男，语速轻快，有对话感和机智",
		Instruction: "",
	},
	{
		Key:        domain.VoiceProfileShuoshu,
		Name:       "说书人风格",
		Voice:      "longfei_v3",
		Rate:       1.0,
		Pitch:      1.0,
		StyleNote:  "热血磁性男，抑扬顿挫，戏剧化讲述",
		Instruction: "",
	},
	{
		Key:        domain.VoiceProfileCangsang,
		Name:       "沧桑低吟",
		Voice:      "longhao_v3",
		Rate:       0.85,
		Pitch:      0.95,
		StyleNote:  "多情忧郁男，语速慢，有历史厚重感和沧桑",
		Instruction: "",
	},
	{
		Key:        domain.VoiceProfileZhixing,
		Name:       "知性女声",
		Voice:      "longxiaoxia_v3",
		Rate:       0.95,
		Pitch:      1.0,
		StyleNote:  "沉稳权威女，知性，节奏稳定",
		Instruction: "",
	},
}

// MatchVoiceProfile 按预设 key 查找语音画像；空值/未知值回退王立群风格（默认）。
func MatchVoiceProfile(key string) domain.VoiceProfile {
	for _, p := range VoicePresets {
		if p.Key == key {
			return p
		}
	}
	// 空值回退默认
	for _, p := range VoicePresets {
		if p.Key == domain.VoiceProfileWangliqun {
			return p
		}
	}
	return VoicePresets[0]
}

// VoiceProfileByKey 返回预设 key 列表（供 API 列表用）。
func VoiceProfileList() []domain.VoiceProfile {
	return VoicePresets
}
