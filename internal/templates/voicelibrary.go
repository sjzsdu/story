package templates

import "github.com/sjzsdu/story/internal/domain"

// VoicePresets 内置声音条目定义（经 SeedBuiltinVoices 写入 voices 表）。
//
// 2026-09-20 修订：只描述系统音色的**真实身份与本来特质**，不再用
// 名人风格化名（如「王立群风格」），也不附加调不出对应味道的模拟参数——
// 名实必须相符。语速/音高/风格指令一律留空走音色默认；想要变体风格，
// 由用户在声音页复制条目自行调整。
// 声音清单以 bl speech synthesize --list-voices --model cosyvoice-v3-flash 为准。
var VoicePresets = []domain.VoiceProfile{
	{
		Key:       domain.VoiceIDLongtian,
		Name:      "龙天",
		Voice:     "longtian_v3",
		Provider:  domain.VoiceProviderBailian,
		StyleNote: "磁性理智男",
	},
	{
		Key:       domain.VoiceIDLongze,
		Name:      "龙泽",
		Voice:     "longze_v3",
		Provider:  domain.VoiceProviderBailian,
		StyleNote: "温暖元气男",
	},
	{
		Key:       domain.VoiceIDLongcheng,
		Name:      "龙橙",
		Voice:     "longcheng_v3",
		Provider:  domain.VoiceProviderBailian,
		StyleNote: "智慧青年男",
	},
	{
		Key:       domain.VoiceIDLongfei,
		Name:      "龙飞",
		Voice:     "longfei_v3",
		Provider:  domain.VoiceProviderBailian,
		StyleNote: "热血磁性男",
	},
	{
		Key:       domain.VoiceIDLonghao,
		Name:      "龙浩",
		Voice:     "longhao_v3",
		Provider:  domain.VoiceProviderBailian,
		StyleNote: "多情忧郁男",
	},
	{
		Key:       domain.VoiceIDLongxiaoxia,
		Name:      "龙小夏",
		Voice:     "longxiaoxia_v3",
		Provider:  domain.VoiceProviderBailian,
		StyleNote: "沉稳权威女",
	},
}

// MatchVoiceProfile 按 key 查找内置声音；空值/未知值回退默认（龙天）。
func MatchVoiceProfile(key string) domain.VoiceProfile {
	for _, p := range VoicePresets {
		if p.Key == key {
			return p
		}
	}
	return MatchVoiceProfile(domain.DefaultVoiceID)
}

// VoiceProfileByKey 返回内置声音列表（供 API 列表用）。
func VoiceProfileList() []domain.VoiceProfile {
	return VoicePresets
}
