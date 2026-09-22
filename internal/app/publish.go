package app

import (
	"context"
	"fmt"
	"time"

	"github.com/sjzsdu/story/internal/config"
	"github.com/sjzsdu/story/internal/domain"
	"github.com/sjzsdu/story/internal/port"
	"github.com/sjzsdu/story/internal/provider/publish/bilibili"
	"github.com/sjzsdu/story/internal/provider/publish/douyin"
	"github.com/sjzsdu/story/internal/provider/publish/kuaishou"
	"github.com/sjzsdu/story/internal/provider/publish/sau"
	"github.com/sjzsdu/story/internal/provider/publish/tencent"
	"github.com/sjzsdu/story/internal/provider/publish/xiaohongshu"
)

// initPublishProviders 根据配置初始化平台发布器注册表（§19）。
// 所有平台统一走 sau CLI（social-auto-upload），无需 OAuth 凭证；
// 用户只需 pip install social-auto-upload + 各平台扫码登录一次。
func initPublishProviders(cfg config.Config) map[domain.Platform]port.PlatformPublisher {
	sauClient := sau.NewClient(cfg.SAUBin, cfg.PythonBin, 600)

	providers := make(map[domain.Platform]port.PlatformPublisher)

	// 全部平台默认注册——sau 支持的平台都可以尝试
	providers[domain.PlatformDouyin] = douyin.New(sauClient)
	providers[domain.PlatformKuaishou] = kuaishou.New(sauClient)
	providers[domain.PlatformBilibili] = bilibili.New(sauClient, cfg.BilibiliDefaultTid)
	providers[domain.PlatformXiaohongshu] = xiaohongshu.New(sauClient)
	providers[domain.PlatformWeixin] = tencent.New(sauClient)

	return providers
}

// publishProviders 返回已注册的平台发布器（§19）。
func (a *App) publishProviders() map[domain.Platform]port.PlatformPublisher {
	return a.publishProvidersRegistry
}

// Publish 发布一集成片到指定平台。
func (a *App) Publish(ctx context.Context, episodeID string, in PublishInput) ([]*domain.PublishJob, error) {
	providers := a.publishProviders()
	if providers == nil {
		return nil, fmt.Errorf("发布能力未初始化")
	}
	return a.Engine.Publish(ctx, episodeID, port.PublishOptions{
		Platforms:   in.Platforms,
		Title:       in.Title,
		Description: in.Description,
		Tags:        in.Tags,
		CoverPath:   in.CoverPath,
		Category:    in.Category,
		ScheduledAt: in.ScheduledAt,
	}, providers)
}

// CreatePublishDrafts 为 final 节点自动创建发布草稿。
func (a *App) CreatePublishDrafts(ctx context.Context, episodeID string) ([]*domain.PublishJob, error) {
	providers := a.publishProviders()
	if providers == nil {
		return nil, nil
	}
	return a.Engine.CreatePublishDrafts(ctx, episodeID, providers)
}

// CancelPublish 取消发布任务。
func (a *App) CancelPublish(ctx context.Context, jobID string) error {
	return a.Engine.CancelPublish(ctx, jobID)
}

// DeletePublished 删除已发布的平台视频。
func (a *App) DeletePublished(ctx context.Context, jobID string) error {
	providers := a.publishProviders()
	if providers == nil {
		return fmt.Errorf("发布能力未初始化")
	}
	return a.Engine.DeletePublished(ctx, jobID, providers)
}

// ListPublishJobs 列出某集的全部发布任务。
func (a *App) ListPublishJobs(ctx context.Context, episodeID string) ([]*domain.PublishJob, error) {
	return a.Repo.ListPublishJobsByEpisode(ctx, episodeID)
}

// GetPublishJob 获取单个发布任务详情。
func (a *App) GetPublishJob(ctx context.Context, jobID string) (*domain.PublishJob, error) {
	return a.Repo.GetPublishJob(ctx, jobID)
}

// ---- 平台账号管理 ----

// CreatePlatformAccount 创建平台账号。
func (a *App) CreatePlatformAccount(ctx context.Context, account *domain.PlatformAccount) error {
	return a.Repo.CreatePlatformAccount(ctx, account)
}

// GetPlatformAccount 获取平台账号。
func (a *App) GetPlatformAccount(ctx context.Context, id string) (*domain.PlatformAccount, error) {
	return a.Repo.GetPlatformAccount(ctx, id)
}

// ListPlatformAccounts 列出某平台的全部账号。
func (a *App) ListPlatformAccounts(ctx context.Context, platform domain.Platform) ([]*domain.PlatformAccount, error) {
	return a.Repo.ListPlatformAccountsByPlatform(ctx, platform)
}

// DeletePlatformAccount 删除平台账号。
func (a *App) DeletePlatformAccount(ctx context.Context, id string) error {
	return a.Repo.DeletePlatformAccount(ctx, id)
}

// ListRegisteredPlatforms 返回已注册发布能力的平台列表。
func (a *App) ListRegisteredPlatforms() []domain.Platform {
	providers := a.publishProviders()
	if providers == nil {
		return nil
	}
	var platforms []domain.Platform
	for p := range providers {
		platforms = append(platforms, p)
	}
	return platforms
}

// SauLogin 调用 sau 执行平台登录（扫码/Cookie 持久化）。
func (a *App) SauLogin(ctx context.Context, platform, account string) error {
	sauClient := sau.NewClient(a.Cfg.SAUBin, a.Cfg.PythonBin, 120)
	return sauClient.Login(ctx, platform, account)
}

// SauCheck 调用 sau 检查平台登录状态。
func (a *App) SauCheck(ctx context.Context, platform, account string) (bool, error) {
	sauClient := sau.NewClient(a.Cfg.SAUBin, a.Cfg.PythonBin, 30)
	return sauClient.Check(ctx, platform, account)
}

// PublishInput 发布请求参数。
type PublishInput struct {
	Platforms   []string
	Title       string
	Description string
	Tags        []string
	CoverPath   string
	Category    string
	ScheduledAt *time.Time
}
