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

	Close() error
}
