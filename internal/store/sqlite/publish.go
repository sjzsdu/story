package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/sjzsdu/story/internal/domain"
	"github.com/sjzsdu/story/internal/port"
)

// ---- PublishJob ----

// CreatePublishJob 插入发布任务。
func (s *Store) CreatePublishJob(ctx context.Context, job *domain.PublishJob) error {
	tags, _ := json.Marshal(job.Tags)
	if job.Tags == nil {
		tags = []byte("[]")
	}
	var sched *string
	if job.ScheduledAt != nil {
		s := job.ScheduledAt.UTC().Format(time.RFC3339)
		sched = &s
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO publish_jobs
		   (id, episode_id, series_id, platform, node_id, status,
		    video_path, cover_path, title, description, tags, category,
		    platform_video_id, platform_url,
		    scheduled_at, attempts, max_retries, error,
		    created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		job.ID, job.EpisodeID, job.SeriesID, string(job.Platform), job.NodeID, string(job.Status),
		job.VideoPath, job.CoverPath, job.Title, job.Description, string(tags), job.Category,
		job.PlatformVideoID, job.PlatformURL,
		sched, job.Attempts, job.MaxRetries, job.Error,
		formatTime(job.CreatedAt), formatTime(job.UpdatedAt),
	)
	if err != nil {
		return fmt.Errorf("创建发布任务 %s: %w", job.ID, err)
	}
	return nil
}

// GetPublishJob 按 ID 查询发布任务。
func (s *Store) GetPublishJob(ctx context.Context, id string) (*domain.PublishJob, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, episode_id, series_id, platform, node_id, status,
		        video_path, cover_path, title, description, tags, category,
		        platform_video_id, platform_url,
		        scheduled_at, attempts, max_retries, error,
		        created_at, updated_at
		 FROM publish_jobs WHERE id = ?`, id)
	job, err := scanPublishJob(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: 发布任务 %s", port.ErrNotFound, id)
	}
	return job, err
}

// ListPublishJobsByEpisode 列出某集的全部发布任务（按创建时间升序）。
func (s *Store) ListPublishJobsByEpisode(ctx context.Context, episodeID string) ([]*domain.PublishJob, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, episode_id, series_id, platform, node_id, status,
		        video_path, cover_path, title, description, tags, category,
		        platform_video_id, platform_url,
		        scheduled_at, attempts, max_retries, error,
		        created_at, updated_at
		 FROM publish_jobs WHERE episode_id = ? ORDER BY created_at ASC`, episodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.PublishJob
	for rows.Next() {
		job, err := scanPublishJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, job)
	}
	return out, rows.Err()
}

// UpdatePublishJob 更新发布任务。
func (s *Store) UpdatePublishJob(ctx context.Context, job *domain.PublishJob) error {
	tags, _ := json.Marshal(job.Tags)
	if job.Tags == nil {
		tags = []byte("[]")
	}
	var sched *string
	if job.ScheduledAt != nil {
		s := job.ScheduledAt.UTC().Format(time.RFC3339)
		sched = &s
	}
	job.UpdatedAt = time.Now()
	res, err := s.db.ExecContext(ctx,
		`UPDATE publish_jobs SET
		   status=?, video_path=?, cover_path=?, title=?, description=?, tags=?, category=?,
		   platform_video_id=?, platform_url=?,
		   scheduled_at=?, attempts=?, max_retries=?, error=?, updated_at=?
		 WHERE id=?`,
		string(job.Status), job.VideoPath, job.CoverPath, job.Title, job.Description, string(tags), job.Category,
		job.PlatformVideoID, job.PlatformURL,
		sched, job.Attempts, job.MaxRetries, job.Error, formatTime(job.UpdatedAt),
		job.ID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: 发布任务 %s", port.ErrNotFound, job.ID)
	}
	return nil
}

// DeletePublishJob 删除发布任务。
func (s *Store) DeletePublishJob(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM publish_jobs WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: 发布任务 %s", port.ErrNotFound, id)
	}
	return nil
}

// ListPendingScheduledPublishJobs 返回已上传且到达定时时间的发布任务。
func (s *Store) ListPendingScheduledPublishJobs(ctx context.Context) ([]*domain.PublishJob, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, episode_id, series_id, platform, node_id, status,
		        video_path, cover_path, title, description, tags, category,
		        platform_video_id, platform_url,
		        scheduled_at, attempts, max_retries, error,
		        created_at, updated_at
		 FROM publish_jobs
		 WHERE status = 'uploaded' AND scheduled_at IS NOT NULL AND scheduled_at <= ?
		 ORDER BY scheduled_at ASC`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.PublishJob
	for rows.Next() {
		job, err := scanPublishJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, job)
	}
	return out, rows.Err()
}

// ---- PlatformAccount ----

// CreatePlatformAccount 插入平台账号。
func (s *Store) CreatePlatformAccount(ctx context.Context, a *domain.PlatformAccount) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO platform_accounts
		   (id, platform, account_name, account_id, access_token, refresh_token, token_expiry, extra, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, string(a.Platform), a.AccountName, a.AccountID, a.AccessToken, a.RefreshToken,
		formatTime(a.TokenExpiry), a.Extra,
		formatTime(a.CreatedAt), formatTime(a.UpdatedAt),
	)
	if err != nil {
		return fmt.Errorf("创建平台账号 %s: %w", a.ID, err)
	}
	return nil
}

// GetPlatformAccount 按 ID 查询平台账号。
func (s *Store) GetPlatformAccount(ctx context.Context, id string) (*domain.PlatformAccount, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, platform, account_name, account_id, access_token, refresh_token, token_expiry, extra, created_at, updated_at
		 FROM platform_accounts WHERE id = ?`, id)
	a, err := scanPlatformAccount(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: 平台账号 %s", port.ErrNotFound, id)
	}
	return a, err
}

// ListPlatformAccountsByPlatform 列出某平台的全部账号。
func (s *Store) ListPlatformAccountsByPlatform(ctx context.Context, platform domain.Platform) ([]*domain.PlatformAccount, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, platform, account_name, account_id, access_token, refresh_token, token_expiry, extra, created_at, updated_at
		 FROM platform_accounts WHERE platform = ? ORDER BY created_at ASC`, string(platform))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.PlatformAccount
	for rows.Next() {
		a, err := scanPlatformAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// UpdatePlatformAccount 更新平台账号。
func (s *Store) UpdatePlatformAccount(ctx context.Context, a *domain.PlatformAccount) error {
	a.UpdatedAt = time.Now()
	res, err := s.db.ExecContext(ctx,
		`UPDATE platform_accounts SET
		   account_name=?, account_id=?, access_token=?, refresh_token=?, token_expiry=?, extra=?, updated_at=?
		 WHERE id=?`,
		a.AccountName, a.AccountID, a.AccessToken, a.RefreshToken, formatTime(a.TokenExpiry), a.Extra,
		formatTime(a.UpdatedAt), a.ID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: 平台账号 %s", port.ErrNotFound, a.ID)
	}
	return nil
}

// DeletePlatformAccount 删除平台账号。
func (s *Store) DeletePlatformAccount(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM platform_accounts WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: 平台账号 %s", port.ErrNotFound, id)
	}
	return nil
}

// ---- scan helpers ----

func scanPublishJob(r rowScanner) (*domain.PublishJob, error) {
	var job domain.PublishJob
	var platform, status, tagsJSON, created, updated string
	var sched sql.NullString
	if err := r.Scan(
		&job.ID, &job.EpisodeID, &job.SeriesID, &platform, &job.NodeID, &status,
		&job.VideoPath, &job.CoverPath, &job.Title, &job.Description, &tagsJSON, &job.Category,
		&job.PlatformVideoID, &job.PlatformURL,
		&sched, &job.Attempts, &job.MaxRetries, &job.Error,
		&created, &updated,
	); err != nil {
		return nil, err
	}
	job.Platform = domain.Platform(platform)
	job.Status = domain.PublishStatus(status)
	if tagsJSON != "" {
		_ = json.Unmarshal([]byte(tagsJSON), &job.Tags)
	}
	if sched.Valid && sched.String != "" {
		t := parseTime(sched.String)
		job.ScheduledAt = &t
	}
	job.CreatedAt = parseTime(created)
	job.UpdatedAt = parseTime(updated)
	return &job, nil
}

func scanPlatformAccount(r rowScanner) (*domain.PlatformAccount, error) {
	var a domain.PlatformAccount
	var platform, created, updated string
	if err := r.Scan(
		&a.ID, &platform, &a.AccountName, &a.AccountID, &a.AccessToken, &a.RefreshToken,
		&a.TokenExpiry, &a.Extra, &created, &updated,
	); err != nil {
		return nil, err
	}
	a.Platform = domain.Platform(platform)
	a.CreatedAt = parseTime(created)
	a.UpdatedAt = parseTime(updated)
	return &a, nil
}
