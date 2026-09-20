package domain

// 预设语音画像 key（SeriesConfig.VoiceProfile）。
// 画像 = 系统声音 + 语速 + 音高 + 风格指令，逼近某种讲述风格。
// 注意：使用系统声音模拟风格，非真人声音克隆（§8 法律红线）。
const (
	// VoiceProfileWangliqun 王立群风格：沉稳、有书卷气、节奏从容。
	VoiceProfileWangliqun = "wangliqun"
	// VoiceProfileKaishu 凯叔风格：温暖、有亲和力、戏剧化停顿。
	VoiceProfileKaishu = "kaishu"
	// VoiceProfileYizhongtian 易中天风格：轻快、有对话感、机智。
	VoiceProfileYizhongtian = "yizhongtian"
	// VoiceProfileShuoshu 说书人风格：热血磁性、抑扬顿挫、戏剧化。
	VoiceProfileShuoshu = "shuoshu"
	// VoiceProfileCangsang 沧桑低吟：多情忧郁、有历史厚重感。
	VoiceProfileCangsang = "cangsang"
	// VoiceProfileZhixing 知性女声：沉稳权威、知性。
	VoiceProfileZhixing = "zhixing"
)

// VoiceProfile 语音画像：对 TTS 合成参数的打包。
// 预设由 voicelibrary.go 供给；用户也可自定义（存完整画像到 series config）。
type VoiceProfile struct {
	// Key 预设 key（自定义时为空或 "custom"）。
	Key string `json:"key,omitempty"`
	// Name 显示名。
	Name string `json:"name"`
	// Voice bl 语音 ID（如 longtian_v3）。
	Voice string `json:"voice"`
	// Instruction 风格指令（部分音色不支持，provider 自动降级）。
	Instruction string `json:"instruction,omitempty"`
	// Rate 语速 0.5-2.0，默认 1.0。
	Rate float64 `json:"rate,omitempty"`
	// Pitch 音高 0.5-2.0，默认 1.0。
	Pitch float64 `json:"pitch,omitempty"`
	// StyleNote 风格说明（UI 展示用）。
	StyleNote string `json:"style_note,omitempty"`
}
