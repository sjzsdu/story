package domain

import "time"

// 预设语音画像 key（SeriesConfig.VoiceProfile）。
// 画像 = 系统声音 + 语速 + 音高 + 风格指令，逼近某种讲述风格。
// 注意：使用系统声音模拟风格，非真人声音克隆（§8 法律红线）。
// 升级为顶层 Voice 实体后，这些 key 同时作为内置 voices 表行的 ID。
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

// VoiceProfile 语音画像：对 TTS 合成参数的打包（值对象）。
// 作为 engine 内部解析后的 DTO；持久化顶层实体使用 Voice。
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

// Voice 声音顶层实体（与 Series 同级，§16）。
// 持久化在 voices 表；series.voice_id 引用其 ID，创建后锁定不可改。
// IsBuiltin=1 的内置条目由启动时 seed（INSERT OR IGNORE），可编辑不可删除。
type Voice struct {
	// ID slug（内置条目用预设 key；用户新增条目用 Slugify(name)+nanos 后缀）。
	ID string `json:"id"`
	// Name 显示名。
	Name string `json:"name"`
	// Voice bl 语音 ID（如 longtian_v3）。
	Voice string `json:"voice"`
	// Instruction 风格指令（部分音色不支持，provider 自动降级）。
	Instruction string `json:"instruction,omitempty"`
	// Rate 语速 0.5-2.0，0 表示用音色默认。
	Rate float64 `json:"rate,omitempty"`
	// Pitch 音高 0.5-2.0，0 表示用音色默认。
	Pitch float64 `json:"pitch,omitempty"`
	// StyleNote 风格说明（UI 展示用）。
	StyleNote string `json:"style_note,omitempty"`
	// IsBuiltin 是否内置（1=是，不可删除但可编辑）。
	IsBuiltin bool `json:"is_builtin"`
	// CreatedAt 创建时间。
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt 最后修改时间。
	UpdatedAt time.Time `json:"updated_at"`
}

// ToProfile 把 Voice 实体转为 VoiceProfile 值对象（供 engine / 试音复用）。
func (v *Voice) ToProfile() VoiceProfile {
	return VoiceProfile{
		Key:         v.ID,
		Name:        v.Name,
		Voice:       v.Voice,
		Instruction: v.Instruction,
		Rate:        v.Rate,
		Pitch:       v.Pitch,
		StyleNote:   v.StyleNote,
	}
}
