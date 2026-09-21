package domain

import (
	"strings"
	"time"
)

// 内置声音条目 ID（voices 表主键；§16）。
//
// 2026-09-20 修订：ID 与音色真实身份一致。初版用名人风格化名
// （wangliqun/kaishu/yizhongtian/shuoshu/cangsang/zhixing），但系统音色
// 实际调不出名人味道，名不副实；已由 RemapLegacyBuiltinVoices 一次性
// 重映射到下列真实 ID，旧条目删除。
const (
	// VoiceIDLongtian 龙天（longtian_v3）：磁性理智男。
	VoiceIDLongtian = "longtian"
	// VoiceIDLongze 龙泽（longze_v3）：温暖元气男。
	VoiceIDLongze = "longze"
	// VoiceIDLongcheng 龙橙（longcheng_v3）：智慧青年男。
	VoiceIDLongcheng = "longcheng"
	// VoiceIDLongfei 龙飞（longfei_v3）：热血磁性男。
	VoiceIDLongfei = "longfei"
	// VoiceIDLonghao 龙浩（longhao_v3）：多情忧郁男。
	VoiceIDLonghao = "longhao"
	// VoiceIDLongxiaoxia 龙小夏（longxiaoxia_v3）：沉稳权威女。
	VoiceIDLongxiaoxia = "longxiaoxia"

	// DefaultVoiceID 默认声音条目 ID（空值/旧 key 兜底）。
	DefaultVoiceID = VoiceIDLongtian
)

// TTS 供应商标识（Voice.Provider）。
// 一条声音条目只属于一个供应商（2026-09-20 决策）：Voice 字段存该供应商
// 体系内的音色 ID，参数面（Instruction/Rate/Pitch）也随供应商而异。
// 空值/未知值一律归一为 bailian（兼容 §16 旧数据）。
const (
	// VoiceProviderBailian 阿里云百炼 CosyVoice（bl speech synthesize）。
	VoiceProviderBailian = "bailian"
)

// 造声方式（VoiceBuildRequest.Kind，§16 声音设计 / 声音复刻）。
const (
	// VoiceBuildDesign 声音设计：用文字描述从零生成音色（无需录音素材）。
	VoiceBuildDesign = "design"
	// VoiceBuildClone 声音复刻：上传个人音频克隆音色。
	VoiceBuildClone = "clone"
)

// VoiceBuildModelDefault 造声（设计/复刻）默认驱动模型。
// 与 config.Default().TTSModel 一致：造出的音色必须用同一模型合成，否则必失败。
const VoiceBuildModelDefault = "cosyvoice-v3-flash"

// NormalizeVoiceProvider 归一供应商 key：去空白、小写；空或未知回退 bailian。
func NormalizeVoiceProvider(p string) string {
	p = strings.ToLower(strings.TrimSpace(p))
	switch p {
	case VoiceProviderBailian:
		return p
	case "":
		return VoiceProviderBailian
	default:
		// 未来接入新供应商时在此补 case；当前只认识百炼。
		return VoiceProviderBailian
	}
}

// SystemVoice 供应商的系统音色（浏览音色库时的列表项，值对象）。
// 由 VoiceLister port 实时获取，不落库（音色随供应商更新，避免维护硬编码清单）。
type SystemVoice struct {
	// ID 供应商体系内的音色参数值（如 longtian_v3）。
	ID string `json:"id"`
	// Name 音色显示名（如 龙天）。
	Name string `json:"name"`
	// Description 音色特质（如 磁性理智男）。
	Description string `json:"description"`
	// Language 支持语言（如 中文/英文）。
	Language string `json:"language"`
}

// VoiceProfile 语音画像：对 TTS 合成参数的打包（值对象）。
// 作为 engine 内部解析后的 DTO；持久化顶层实体使用 Voice。
type VoiceProfile struct {
	// Key 预设 key（自定义时为空或 "custom"）。
	Key string `json:"key,omitempty"`
	// Name 显示名。
	Name string `json:"name"`
	// Provider TTS 供应商标识（空值归一 bailian）。
	Provider string `json:"provider,omitempty"`
	// Voice 该供应商体系内的音色 ID（百炼下如 longtian_v3）。
	Voice string `json:"voice"`
	// Model 驱动该音色的 TTS 模型（造声音色必填：设计/复刻音色必须用
	// 造声时的 target_model 合成；空则用全局 tts_model）。
	Model string `json:"model,omitempty"`
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
	// Provider TTS 供应商标识（一条声音只属于一个供应商；空值归一 bailian）。
	Provider string `json:"provider"`
	// Voice 该供应商体系内的音色 ID（百炼下如 longtian_v3）。
	Voice string `json:"voice"`
	// Model 驱动该音色的 TTS 模型；造声音色（设计/复刻）必填，
	// 空值表示用全局 tts_model（系统音色走此路径）。
	Model string `json:"model,omitempty"`
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
		Provider:    v.Provider,
		Voice:       v.Voice,
		Model:       v.Model,
		Instruction: v.Instruction,
		Rate:        v.Rate,
		Pitch:       v.Pitch,
		StyleNote:   v.StyleNote,
	}
}
