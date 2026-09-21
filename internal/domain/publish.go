// Package domain 是纯数据层（struct + JSON tag + 少量纯函数），不依赖任何第三方包。
package domain

import "time"

// Platform 平台标识（§19）。
type Platform string

const (
	PlatformDouyin      Platform = "douyin"       // 抖音
	PlatformKuaishou    Platform = "kuaishou"     // 快手
	PlatformBilibili    Platform = "bilibili"     // B站
	PlatformXiaohongshu Platform = "xiaohongshu"  // 小红书
	PlatformWeixin      Platform = "weixin"       // 视频号
)

// PublishStatus 发布任务状态。
type PublishStatus string

const (
	PublishPending   PublishStatus = "pending"    // 待发布
	PublishUploading PublishStatus = "uploading"  // 上传中
	PublishUploaded  PublishStatus = "uploaded"   // 已上传（待确认发布 / 定时队列）
	PublishPublished PublishStatus = "published"  // 已发布
	PublishFailed    PublishStatus = "failed"     // 发布失败（可重试）
	PublishRejected  PublishStatus = "rejected"   // 平台审核拒绝（不可重试）
	PublishCanceled  PublishStatus = "canceled"   // 已取消
)

// PublishJob 一次发布任务（§19 顶层持久化实体）。
type PublishJob struct {
	ID        string        `json:"id"`
	EpisodeID string        `json:"episode_id"`
	SeriesID  string        `json:"series_id"`
	Platform  Platform      `json:"platform"`
	NodeID    string        `json:"node_id"` // 发布的 final 节点 ID
	Status    PublishStatus `json:"status"`
	// 素材
	VideoPath   string `json:"video_path"`
	CoverPath   string `json:"cover_path,omitempty"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Tags        []string `json:"tags,omitempty"`
	Category    string   `json:"category,omitempty"`
	// 平台返回
	PlatformVideoID string `json:"platform_video_id,omitempty"`
	PlatformURL     string `json:"platform_url,omitempty"`
	// 定时发布
	ScheduledAt *time.Time `json:"scheduled_at,omitempty"`
	// 重试
	Attempts   int    `json:"attempts"`
	MaxRetries int    `json:"max_retries"`
	Error      string `json:"error,omitempty"`
	// 时间
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// IsRetryable 判断当前状态是否允许重试。
func (j *PublishJob) IsRetryable() bool {
	return j.Status == PublishFailed || j.Status == PublishPending && j.Attempts > 0
}

// CanCancel 判断是否可取消（上传中、待发布、定时队列可取消）。
func (j *PublishJob) CanCancel() bool {
	return j.Status == PublishPending || j.Status == PublishUploading || j.Status == PublishUploaded
}

// PlatformAccount 平台账号凭证（§19 顶层持久化实体）。
type PlatformAccount struct {
	ID           string    `json:"id"`
	Platform     Platform  `json:"platform"`
	AccountName  string    `json:"account_name"`  // 平台侧用户名/昵称
	AccountID    string    `json:"account_id"`    // 平台侧用户 ID
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	TokenExpiry  time.Time `json:"token_expiry"`
	Extra        string    `json:"extra,omitempty"` // 平台特有参数 JSON
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// NormalizePlatform 归一化平台 key：去空白、小写；空或未知原样返回。
func NormalizePlatform(p string) Platform {
	return Platform(p)
}

// AllPlatforms 返回全部已支持的平台列表。
func AllPlatforms() []Platform {
	return []Platform{
		PlatformDouyin,
		PlatformKuaishou,
		PlatformBilibili,
		PlatformXiaohongshu,
		PlatformWeixin,
	}
}

// PlatformLabel 返回平台的中文显示名。
func PlatformLabel(p Platform) string {
	switch p {
	case PlatformDouyin:
		return "抖音"
	case PlatformKuaishou:
		return "快手"
	case PlatformBilibili:
		return "B站"
	case PlatformXiaohongshu:
		return "小红书"
	case PlatformWeixin:
		return "视频号"
	default:
		return string(p)
	}
}

// PlatformTitleLimit 各平台标题字数上限。
func PlatformTitleLimit(p Platform) int {
	switch p {
	case PlatformDouyin:
		return 55
	case PlatformKuaishou:
		return 20
	case PlatformBilibili:
		return 80
	case PlatformXiaohongshu:
		return 20
	case PlatformWeixin:
		return 30
	default:
		return 30
	}
}

// PlatformDescLimit 各平台描述字数上限。
func PlatformDescLimit(p Platform) int {
	switch p {
	case PlatformDouyin:
		return 2000
	case PlatformKuaishou:
		return 2000
	case PlatformBilibili:
		return 2000
	case PlatformXiaohongshu:
		return 1000
	case PlatformWeixin:
		return 1000
	default:
		return 1000
	}
}

// CoverSize 返回平台推荐封面尺寸。
func CoverSize(p Platform) (int, int) {
	switch p {
	case PlatformBilibili:
		return 1920, 1080 // 16:9
	case PlatformXiaohongshu:
		return 1080, 1440 // 3:4
	default:
		return 1080, 1920 // 9:16
	}
}
