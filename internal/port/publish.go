package port

import (
	"context"
	"time"

	"github.com/sjzsdu/story/internal/domain"
)

// PlatformPublisher 平台发布能力（每个平台实现一个，§19）。
type PlatformPublisher interface {
	// Platform 返回该发布器支持的平台标识。
	Platform() domain.Platform

	// Upload 上传视频（可能进入草稿箱，不一定直接发布）。
	Upload(ctx context.Context, req PublishRequest) (*PublishResult, error)

	// Publish 确认发布（上传与发布分两步的平台用此步）。
	Publish(ctx context.Context, videoID string, account *domain.PlatformAccount) (*PublishResult, error)

	// Status 查询发布/审核状态。
	Status(ctx context.Context, videoID string, account *domain.PlatformAccount) (domain.PublishStatus, string, error)

	// Delete 删除已发布的视频。
	Delete(ctx context.Context, videoID string, account *domain.PlatformAccount) error

	// UploadCover 上传/替换封面图。
	UploadCover(ctx context.Context, videoID, coverPath string, account *domain.PlatformAccount) error
}

// PublishRequest 发布请求素材。
type PublishRequest struct {
	VideoPath   string
	CoverPath   string
	Title       string
	Description string
	Tags        []string
	Category    string
	ScheduledAt *time.Time
	Account     *domain.PlatformAccount
	Extra       map[string]any // 平台特有参数
}

// PublishResult 发布结果。
type PublishResult struct {
	VideoID string              // 平台侧视频 ID
	URL     string              // 发布后链接
	Status  domain.PublishStatus
	Error   string              // 平台返回的错误信息
}

// PublishOptions 发布请求选项（engine 层使用，§19）。
type PublishOptions struct {
	Platforms   []string
	Title       string
	Description string
	Tags        []string
	CoverPath   string
	Category    string
	ScheduledAt *time.Time
}
