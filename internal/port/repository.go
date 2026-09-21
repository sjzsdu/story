package port

import (
	"context"
	"errors"

	"github.com/sjzsdu/story/internal/domain"
)

// ErrNotFound 持久化层「记录不存在」的统一哨兵（各 store 实现的同名错误应等价于它）。
var ErrNotFound = errors.New("记录不存在")

// Repository 持久化端口。engine 只面向本接口，不感知 SQLite。
type Repository interface {
	// 系列
	CreateSeries(ctx context.Context, s *domain.Series) error
	GetSeries(ctx context.Context, id string) (*domain.Series, error)
	ListSeries(ctx context.Context) ([]*domain.Series, error)
	UpdateSeries(ctx context.Context, s *domain.Series) error
	// DeleteSeries 删除系列（实现应级联删除其下所有集的持久化记录）。
	DeleteSeries(ctx context.Context, id string) error

	// 集
	CreateEpisode(ctx context.Context, ep *domain.Episode) error
	GetEpisode(ctx context.Context, id string) (*domain.Episode, error)
	ListEpisodes(ctx context.Context, seriesID string) ([]*domain.Episode, error)
	// SaveEpisode 更新集（含流水线状态整体写回）。
	SaveEpisode(ctx context.Context, ep *domain.Episode) error
	// DeleteEpisode 删除单集。
	DeleteEpisode(ctx context.Context, id string) error

	// NextEpisodeNumber 返回该系列下一集的序号。
	NextEpisodeNumber(ctx context.Context, seriesID string) (int, error)

	// 系列分集策划会话（一个系列 1:1）。
	GetPlanSession(ctx context.Context, seriesID string) (*domain.PlanSession, error)
	SavePlanSession(ctx context.Context, session *domain.PlanSession) error
	DeletePlanSession(ctx context.Context, seriesID string) error

	// 声音（顶层实体，§16）：CRUD + 引用统计 + 迁移辅助。
	CreateVoice(ctx context.Context, v *domain.Voice) error
	GetVoice(ctx context.Context, id string) (*domain.Voice, error)
	ListVoices(ctx context.Context) ([]*domain.Voice, error)
	UpdateVoice(ctx context.Context, v *domain.Voice) error
	// DeleteVoice 删除声音条目；内置条目或被系列引用时应返回错误。
	DeleteVoice(ctx context.Context, id string) error
	// CountSeriesByVoiceID 统计引用某声音的系列数（删除前校验）。
	CountSeriesByVoiceID(ctx context.Context, voiceID string) (int, error)

	// 发布任务（§19）。
	CreatePublishJob(ctx context.Context, job *domain.PublishJob) error
	GetPublishJob(ctx context.Context, id string) (*domain.PublishJob, error)
	ListPublishJobsByEpisode(ctx context.Context, episodeID string) ([]*domain.PublishJob, error)
	UpdatePublishJob(ctx context.Context, job *domain.PublishJob) error
	DeletePublishJob(ctx context.Context, id string) error
	// ListPendingScheduledPublishJobs 返回已上传且到达定时时间的发布任务（定时发布调度用）。
	ListPendingScheduledPublishJobs(ctx context.Context) ([]*domain.PublishJob, error)

	// 平台账号（§19）。
	CreatePlatformAccount(ctx context.Context, a *domain.PlatformAccount) error
	GetPlatformAccount(ctx context.Context, id string) (*domain.PlatformAccount, error)
	ListPlatformAccountsByPlatform(ctx context.Context, platform domain.Platform) ([]*domain.PlatformAccount, error)
	UpdatePlatformAccount(ctx context.Context, a *domain.PlatformAccount) error
	DeletePlatformAccount(ctx context.Context, id string) error

	Close() error
}
