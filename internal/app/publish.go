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

// buildPublishProviders 构建平台发布能力登记表（key ＝ string(domain.Platform)，§19/§21）。
// 所有平台统一走 sau CLI（social-auto-upload），无需 OAuth 凭证；
// 用户只需 pip install social-auto-upload + 各平台扫码登录一次。
func buildPublishProviders(cfg config.Config) map[string]port.PlatformPublisher {
	sauClient := sau.NewClient(cfg.SAUBin, cfg.PythonBin, 600)

	// 全部平台默认登记——sau 支持的平台都可以尝试。
	return map[string]port.PlatformPublisher{
		string(domain.PlatformDouyin):      douyin.New(sauClient),
		string(domain.PlatformKuaishou):    kuaishou.New(sauClient),
		string(domain.PlatformBilibili):    bilibili.New(sauClient, cfg.BilibiliDefaultTid),
		string(domain.PlatformXiaohongshu): xiaohongshu.New(sauClient),
		string(domain.PlatformWeixin):      tencent.New(sauClient),
	}
}

// publishProviders 返回已登记的平台发布器（从能力槽重建 domain 键视图，§19/§21）。
// 槽为空（能力未装配）时返回 nil，调用方据此报「发布能力未初始化」。
func (a *App) publishProviders() map[domain.Platform]port.PlatformPublisher {
	keys := a.publish.Keys()
	if len(keys) == 0 {
		return nil
	}
	out := make(map[domain.Platform]port.PlatformPublisher, len(keys))
	for _, k := range keys {
		if _, p, err := a.publish.Resolve(k); err == nil && p != nil {
			out[domain.Platform(k)] = p
		}
	}
	return out
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

// ListSeriesPublishJobs 列出某系列下全部集的发布任务（系列详情页汇总用）。
func (a *App) ListSeriesPublishJobs(ctx context.Context, seriesID string) ([]*domain.PublishJob, error) {
	return a.Repo.ListPublishJobsBySeries(ctx, seriesID)
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
	sauClient := sau.NewClient(a.Config().SAUBin, a.Config().PythonBin, 120)
	return sauClient.Login(ctx, platform, account)
}

// SauCheck 调用 sau 检查平台登录状态。
func (a *App) SauCheck(ctx context.Context, platform, account string) (bool, error) {
	sauClient := sau.NewClient(a.Config().SAUBin, a.Config().PythonBin, 30)
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
