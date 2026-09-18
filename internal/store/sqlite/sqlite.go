// Package sqlite 是 port.Repository 的 SQLite 实现（纯 Go 驱动，无 CGO）。
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"github.com/sjzsdu/story/internal/domain"
)

// ErrNotFound 数据不存在。
var ErrNotFound = errors.New("记录不存在")

// Store SQLite 仓储。
type Store struct {
	db *sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS series (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    dynasty     TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    config_json TEXT NOT NULL,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS episodes (
    id         TEXT PRIMARY KEY,
    series_id  TEXT NOT NULL REFERENCES series(id) ON DELETE CASCADE,
    number     INTEGER NOT NULL,
    title      TEXT NOT NULL,
    topic      TEXT NOT NULL DEFAULT '',
    state_json TEXT NOT NULL,
    workdir    TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(series_id, number)
);
`

// Open 打开（必要时创建）数据库并初始化表结构。
func Open(ctx context.Context, path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("创建数据库目录: %w", err)
		}
	}
	dsn := path + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if _, err := db.ExecContext(ctx, schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("初始化表结构: %w", err)
	}
	return &Store{db: db}, nil
}

// Close 关闭数据库。
func (s *Store) Close() error { return s.db.Close() }

// ---- Series ----

// CreateSeries 插入系列。
func (s *Store) CreateSeries(ctx context.Context, se *domain.Series) error {
	cfg, err := json.Marshal(se.Config)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO series (id, name, dynasty, description, config_json, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		se.ID, se.Name, se.Dynasty, se.Description, string(cfg),
		formatTime(se.CreatedAt), formatTime(se.UpdatedAt),
	)
	if err != nil {
		return fmt.Errorf("创建系列 %s: %w", se.ID, err)
	}
	return nil
}

// GetSeries 按 ID 查询系列。
func (s *Store) GetSeries(ctx context.Context, id string) (*domain.Series, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, dynasty, description, config_json, created_at, updated_at
		 FROM series WHERE id = ?`, id)
	se, err := scanSeries(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: 系列 %s", ErrNotFound, id)
	}
	return se, err
}

// ListSeries 列出全部系列（按创建时间升序）。
func (s *Store) ListSeries(ctx context.Context) ([]*domain.Series, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, dynasty, description, config_json, created_at, updated_at
		 FROM series ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.Series
	for rows.Next() {
		se, err := scanSeries(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, se)
	}
	return out, rows.Err()
}

// UpdateSeries 更新系列元数据与配置。
func (s *Store) UpdateSeries(ctx context.Context, se *domain.Series) error {
	cfg, err := json.Marshal(se.Config)
	if err != nil {
		return err
	}
	se.UpdatedAt = time.Now()
	res, err := s.db.ExecContext(ctx,
		`UPDATE series SET name=?, dynasty=?, description=?, config_json=?, updated_at=? WHERE id=?`,
		se.Name, se.Dynasty, se.Description, string(cfg), formatTime(se.UpdatedAt), se.ID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: 系列 %s", ErrNotFound, se.ID)
	}
	return nil
}

// DeleteSeries 删除系列（episodes 表有 ON DELETE CASCADE，集记录随之清除）。
func (s *Store) DeleteSeries(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM series WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: 系列 %s", ErrNotFound, id)
	}
	return nil
}

// ---- Episode ----

// CreateEpisode 插入集。
func (s *Store) CreateEpisode(ctx context.Context, ep *domain.Episode) error {
	state, err := json.Marshal(ep.State)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO episodes (id, series_id, number, title, topic, state_json, workdir, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ep.ID, ep.SeriesID, ep.Number, ep.Title, ep.Topic, string(state), ep.WorkDir,
		formatTime(ep.CreatedAt), formatTime(ep.UpdatedAt),
	)
	if err != nil {
		return fmt.Errorf("创建集 %s: %w", ep.ID, err)
	}
	return nil
}

// GetEpisode 按 ID 查询集。
func (s *Store) GetEpisode(ctx context.Context, id string) (*domain.Episode, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, series_id, number, title, topic, state_json, workdir, created_at, updated_at
		 FROM episodes WHERE id = ?`, id)
	ep, err := scanEpisode(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: 集 %s", ErrNotFound, id)
	}
	return ep, err
}

// ListEpisodes 列出某系列下的全部集（按序号升序）。
func (s *Store) ListEpisodes(ctx context.Context, seriesID string) ([]*domain.Episode, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, series_id, number, title, topic, state_json, workdir, created_at, updated_at
		 FROM episodes WHERE series_id = ? ORDER BY number ASC`, seriesID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.Episode
	for rows.Next() {
		ep, err := scanEpisode(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ep)
	}
	return out, rows.Err()
}

// SaveEpisode 整体写回集（含流水线状态）。
func (s *Store) SaveEpisode(ctx context.Context, ep *domain.Episode) error {
	state, err := json.Marshal(ep.State)
	if err != nil {
		return err
	}
	ep.UpdatedAt = time.Now()
	res, err := s.db.ExecContext(ctx,
		`UPDATE episodes SET title=?, topic=?, state_json=?, workdir=?, updated_at=? WHERE id=?`,
		ep.Title, ep.Topic, string(state), ep.WorkDir, formatTime(ep.UpdatedAt), ep.ID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: 集 %s", ErrNotFound, ep.ID)
	}
	return nil
}

// DeleteEpisode 删除单集。
func (s *Store) DeleteEpisode(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM episodes WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: 集 %s", ErrNotFound, id)
	}
	return nil
}

// NextEpisodeNumber 返回系列下一集序号。
func (s *Store) NextEpisodeNumber(ctx context.Context, seriesID string) (int, error) {
	var n int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(number), 0) + 1 FROM episodes WHERE series_id = ?`, seriesID,
	).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// ---- row scanner 适配 ----

type rowScanner interface {
	Scan(dest ...any) error
}

func scanSeries(r rowScanner) (*domain.Series, error) {
	var se domain.Series
	var cfgJSON, created, updated string
	if err := r.Scan(&se.ID, &se.Name, &se.Dynasty, &se.Description, &cfgJSON, &created, &updated); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(cfgJSON), &se.Config); err != nil {
		return nil, fmt.Errorf("解析系列配置 %s: %w", se.ID, err)
	}
	se.CreatedAt = parseTime(created)
	se.UpdatedAt = parseTime(updated)
	return &se, nil
}

func scanEpisode(r rowScanner) (*domain.Episode, error) {
	var ep domain.Episode
	var stateJSON, created, updated string
	if err := r.Scan(&ep.ID, &ep.SeriesID, &ep.Number, &ep.Title, &ep.Topic,
		&stateJSON, &ep.WorkDir, &created, &updated); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(stateJSON), &ep.State); err != nil {
		return nil, fmt.Errorf("解析集状态 %s: %w", ep.ID, err)
	}
	ep.CreatedAt = parseTime(created)
	ep.UpdatedAt = parseTime(updated)
	return &ep, nil
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		t = time.Now()
	}
	return t.UTC().Format(time.RFC3339)
}

func parseTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return t
}
