package domain

import (
	"strings"
	"time"
)

// 素材/资产（Asset）——统一资源管理的第二类资源（AGENTS.md §23）。
// 与 Voice 同款：顶层持久化实体 + 独立表 + 三端入口；本轮只实现 BGM 曲库（kind=bgm），
// image_ref 等后续资源类型在第二步接入视觉参考时沿用同一张表。
type Asset struct {
	ID          string    `json:"id"`
	Kind        string    `json:"kind"` // bgm | image_ref（见下方常量）
	Name        string    `json:"name"`
	Path        string    `json:"path"` // 落盘绝对路径（App 层入库前 filepath.Abs）
	Description string    `json:"description,omitempty"`
	DurationSec float64   `json:"duration_sec,omitempty"` // 时长（秒），探测失败为 0
	Params      string    `json:"params,omitempty"`       // 附加参数 JSON（预留给第二步）
	Origin      string    `json:"origin,omitempty"`       // upload=Web 上传 / import=CLI 导入
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// 素材类型常量。当前只实现 bgm；image_ref 先占位（§23 第二步接入视觉参考）。
const (
	AssetKindBGM      = "bgm"
	AssetKindImageRef = "image_ref"
)

// NormalizeAssetKind 归一素材类型：命中已知类型原样返回；
// 空值/未知值返回空串——**由调用方报错，不静默回退**。
// （与 NormalizeVoiceProvider 的静默回退不同：素材类型决定落盘子目录 data/assets/<kind>/，
// 静默把 image_ref 回退成 bgm 会写错目录，故必须显式拒绝。）
func NormalizeAssetKind(kind string) string {
	switch strings.TrimSpace(kind) {
	case AssetKindBGM:
		return AssetKindBGM
	case AssetKindImageRef:
		return AssetKindImageRef
	default:
		return ""
	}
}

// AssetKindLabel 类型的中文展示名（CLI/Web 通用）。
func AssetKindLabel(kind string) string {
	switch kind {
	case AssetKindBGM:
		return "背景音乐"
	case AssetKindImageRef:
		return "视觉参考图"
	default:
		return kind
	}
}

// AssetKindsLabel 返回全部素材类型的可选值文案（CLI flag / 报错提示共用，
// 避免各处硬编码只列 bgm）。
func AssetKindsLabel() string {
	return AssetKindBGM + "、" + AssetKindImageRef
}

// 系列配置里素材引用的统一形态："asset:<id>"（§23）。
// SeriesConfig.BGMPath 若以此前缀开头则查 assets 表，否则按历史语义当字面路径解析。
const assetRefPrefix = "asset:"

// AssetRefPrefix 引用前缀（供 engine/app/server 判断用，不导出字面量避免各处写错）。
func AssetRefPrefix() string { return assetRefPrefix }

// IsAssetRef 判断配置值是否为素材引用（"asset:<id>"）。
func IsAssetRef(v string) bool { return strings.HasPrefix(v, assetRefPrefix) }

// AssetRef 把素材 ID 包装成配置里的引用串。
func AssetRef(id string) string { return assetRefPrefix + id }

// AssetIDFromRef 从引用串取出素材 ID；非引用形态返回空串。
func AssetIDFromRef(v string) string {
	if !IsAssetRef(v) {
		return ""
	}
	return strings.TrimPrefix(v, assetRefPrefix)
}
