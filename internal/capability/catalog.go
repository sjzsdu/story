package capability

import "sort"

// 能力 key 常量：装配层与 server/CLI 共用同一套字面量，避免各处拼字符串。
const (
	Text            = "text"             // 故事生成
	Board           = "board"            // 分镜规划
	Plan            = "plan"             // 系列策划
	TTS             = "tts"              // 语音合成
	Image           = "image"            // 图片生成
	Video           = "video"            // 视频生成
	ImageUnderstand = "image_understand" // 图像理解
	VideoUnderstand = "video_understand" // 视频理解
	SFX             = "sfx"              // 音效生成
	VoiceBuild      = "voice_build"      // 造声
	VoiceList       = "voice_list"       // 音色库列举
	Publish         = "publish"          // 平台发布
)

// Capability 一个能力槽的静态描述（与运行时注册无关，纯目录数据）。
type Capability struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Help        string `json:"help"`
	ConfigField string `json:"config_field"` // 系统默认对应的 config 字段（yaml key），空＝无该配置项
	SeriesField string `json:"series_field"` // 系列覆盖对应的 SeriesConfig 字段（yaml key），空＝该能力不支持系列覆盖
	// Providers 该能力的已知实现 key（展示顺序；运行时以槽内实际登记为准）。
	Providers []string `json:"providers"`
}

// Provider 一个供应商/平台的展示信息。
type Provider struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Desc  string `json:"desc"`
}

// CapabilityInfo 是下发给前端/CLI 的单能力视图：静态描述 + 运行时默认与实现清单。
type CapabilityInfo struct {
	Key         string     `json:"key"`
	Label       string     `json:"label"`
	Help        string     `json:"help"`
	ConfigField string     `json:"config_field"`
	SeriesField string     `json:"series_field"`
	Default     string     `json:"default"`
	Providers   []Provider `json:"providers"`
}

// Info 是 GET /api/capabilities 与 `story capabilities` 的响应体。
type Info struct {
	Capabilities []CapabilityInfo `json:"capabilities"`
	Providers    []Provider       `json:"providers"`
}

// providerLabels 供应商/平台的展示名与说明（静态表；前端不再各处硬编码文案）。
var providerLabels = map[string]Provider{
	"bailian":     {Key: "bailian", Label: "百炼 (bl)", Desc: "阿里云百炼，系统内置默认（走 bl 自带认证）"},
	"deepseek":    {Key: "deepseek", Label: "DeepSeek", Desc: "DeepSeek 开放平台（需配置 API Key）"},
	"minimax":     {Key: "minimax", Label: "MiniMax", Desc: "MiniMax 开放平台（需配置 API Key 与 Group ID）"},
	"zhipu":       {Key: "zhipu", Label: "智谱 CogView", Desc: "智谱开放平台（需配置 API Key）"},
	"kling":       {Key: "kling", Label: "可灵 Kling", Desc: "快手可灵开放平台（需配置 Access Key 与 Secret Key）"},
	"douyin":      {Key: "douyin", Label: "抖音", Desc: "抖音开放平台（OAuth2）"},
	"kuaishou":    {Key: "kuaishou", Label: "快手", Desc: "快手开放平台（OAuth2）"},
	"bilibili":    {Key: "bilibili", Label: "B站", Desc: "B站投稿 API（OAuth2 + SESSDATA）"},
	"xiaohongshu": {Key: "xiaohongshu", Label: "小红书", Desc: "无官方发布 API，半自动模式"},
	"weixin":      {Key: "weixin", Label: "视频号", Desc: "微信开放平台 / 视频号助手"},
}

// Catalog 返回能力目录（12 项，顺序即展示顺序）。
// 目录是静态数据；运行时各能力登记了哪些实现、默认是谁，由 app 从槽里读出来填充。
func Catalog() []Capability {
	return []Capability{
		{
			Key: Text, Label: "故事生成", Help: "把题材/主题生成为定稿口播稿。",
			ConfigField: "text_provider", SeriesField: "text_provider",
			Providers: []string{"bailian", "deepseek"},
		},
		{
			Key: Board, Label: "分镜规划", Help: "把故事拆成镜头（visual_prompt / narration / duration / camera）。",
			ConfigField: "text_provider", SeriesField: "text_provider",
			Providers: []string{"bailian", "deepseek"},
		},
		{
			Key: Plan, Label: "系列策划", Help: "AI 分集策划会话（草案集数与简介）。",
			ConfigField: "text_provider", SeriesField: "text_provider",
			Providers: []string{"bailian", "deepseek"},
		},
		{
			Key: TTS, Label: "语音合成", Help: "旁白 TTS 与声音试听。",
			ConfigField: "tts_provider", SeriesField: "tts_provider",
			Providers: []string{"bailian", "minimax"},
		},
		{
			Key: Image, Label: "图片生成", Help: "小人书插画、定妆照/视觉参考图。",
			ConfigField: "image_provider", SeriesField: "image_provider",
			Providers: []string{"bailian", "zhipu"},
		},
		{
			Key: Video, Label: "视频生成", Help: "AI 视频片段与视觉参考视频。",
			ConfigField: "video_provider", SeriesField: "video_provider",
			Providers: []string{"bailian", "kling"},
		},
		{
			// §22：图像/视频理解本轮只接百炼（bl vision describe）；无系列覆盖。
			Key: ImageUnderstand, Label: "图像理解", Help: "看图问答/画面描述（bl vision describe）。",
			ConfigField: "image_understand_provider",
			Providers:   []string{"bailian"},
		},
		{
			Key: VideoUnderstand, Label: "视频理解", Help: "视频内容问答/摘要（bl vision describe）。",
			ConfigField: "video_understand_provider",
			Providers:   []string{"bailian"},
		},
		{
			// §22：bl 尚无任何音效命令 → 只立接口与槽位，暂不注册 provider
			//（Providers 为空，运行时 Resolve 会报「未配置默认实现」而非静默失败）。
			Key: SFX, Label: "音效生成", Help: "按描述生成音效（暂未接入供应商，仅预留能力槽）。",
			ConfigField: "sfx_provider",
			Providers:   []string{},
		},
		{
			Key: VoiceBuild, Label: "造声", Help: "声音设计 / 声音复刻（新建音色）。",
			Providers: []string{"bailian"},
		},
		{
			Key: VoiceList, Label: "音色库列举", Help: "列出供应商可用音色（只取元数据，不计费）。",
			Providers: []string{"bailian"},
		},
		{
			Key: Publish, Label: "平台发布", Help: "把成片发布到各短视频/中视频平台。",
			Providers: []string{"douyin", "kuaishou", "bilibili", "xiaohongshu", "weixin"},
		},
	}
}

// ProviderInfo 返回供应商展示信息；未收录的 key 用 key 本身当展示名（不报错，
// 供应商可能由用户自定义登记）。
func ProviderInfo(key string) Provider {
	if p, ok := providerLabels[key]; ok {
		return p
	}
	return Provider{Key: key, Label: key}
}

// KnownProviders 返回静态表里的全部供应商/平台（按 key 升序）。
func KnownProviders() []Provider {
	keys := make([]string, 0, len(providerLabels))
	for k := range providerLabels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]Provider, 0, len(keys))
	for _, k := range keys {
		out = append(out, providerLabels[k])
	}
	return out
}

// orderProviders 把运行时登记的 key 按目录里的已知顺序排列，未知 key 追加在末尾（各自升序）。
// 目录里没登记顺序的（静态 Providers 为空）直接整体升序。
func orderProviders(c Capability, keys []string) []string {
	rank := make(map[string]int, len(c.Providers))
	for i, k := range c.Providers {
		rank[k] = i
	}
	known := make([]string, 0, len(keys))
	unknown := make([]string, 0, len(keys))
	for _, k := range keys {
		if _, ok := rank[k]; ok {
			known = append(known, k)
		} else {
			unknown = append(unknown, k)
		}
	}
	sort.Slice(known, func(i, j int) bool { return rank[known[i]] < rank[known[j]] })
	sort.Strings(unknown)
	return append(known, unknown...)
}
