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
	"github.com/sjzsdu/story/internal/port"
)

// ErrNotFound 数据不存在（等价于 port.ErrNotFound，保持对 app 层既有判断的兼容）。
var ErrNotFound = port.ErrNotFound

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
    characters_json TEXT NOT NULL DEFAULT '[]',
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
    refs_json  TEXT NOT NULL DEFAULT '[]',
    workdir    TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(series_id, number)
);
CREATE TABLE IF NOT EXISTS plan_sessions (
    series_id     TEXT PRIMARY KEY REFERENCES series(id) ON DELETE CASCADE,
    messages_json TEXT NOT NULL,
    drafts_json   TEXT NOT NULL,
    characters_json TEXT NOT NULL DEFAULT '[]',
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS voices (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    voice       TEXT NOT NULL,
    instruction TEXT NOT NULL DEFAULT '',
    rate        REAL NOT NULL DEFAULT 0,
    pitch       REAL NOT NULL DEFAULT 0,
    style_note  TEXT NOT NULL DEFAULT '',
    is_builtin  INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
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
	// 轻量迁移：老库补列。
	for _, m := range []struct{ table, column, def string }{
		{"series", "characters_json", "TEXT NOT NULL DEFAULT '[]'"},
		{"plan_sessions", "characters_json", "TEXT NOT NULL DEFAULT '[]'"},
		{"episodes", "refs_json", "TEXT NOT NULL DEFAULT '[]'"},
		// §16：series.voice_id 引用顶层 Voice 实体，创建后锁定（UpdateSeries 不写该列）。
		{"series", "voice_id", "TEXT NOT NULL DEFAULT ''"},
	} {
		if err := ensureColumn(ctx, db, m.table, m.column, m.def); err != nil {
			_ = db.Close()
			return nil, err
		}
	}
	return &Store{db: db}, nil
}

// ensureColumn 表缺列时 ALTER TABLE 补上（幂等）。
func ensureColumn(ctx context.Context, db *sql.DB, table, column, def string) error {
	rows, err := db.QueryContext(ctx, `PRAGMA table_info(`+table+`)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notNull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk); err != nil {
			return err
		}
		if name == column {
			return rows.Err()
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `ALTER TABLE `+table+` ADD COLUMN `+column+` `+def)
	return err
}

// Close 关闭数据库。
func (s *Store) Close() error { return s.db.Close() }

// ---- Series ----

// CreateSeries 插入系列（含 voice_id；voice_id 创建后锁定，UpdateSeries 不写该列）。
func (s *Store) CreateSeries(ctx context.Context, se *domain.Series) error {
	cfg, err := json.Marshal(se.Config)
	if err != nil {
		return err
	}
	chars, err := json.Marshal(se.Characters)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO series (id, name, dynasty, description, voice_id, config_json, characters_json, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		se.ID, se.Name, se.Dynasty, se.Description, se.VoiceID, string(cfg), string(chars),
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
		`SELECT id, name, dynasty, description, voice_id, config_json, characters_json, created_at, updated_at
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
		`SELECT id, name, dynasty, description, voice_id, config_json, characters_json, created_at, updated_at
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
	chars, err := json.Marshal(se.Characters)
	if err != nil {
		return err
	}
	se.UpdatedAt = time.Now()
	res, err := s.db.ExecContext(ctx,
		`UPDATE series SET name=?, dynasty=?, description=?, config_json=?, characters_json=?, updated_at=? WHERE id=?`,
		se.Name, se.Dynasty, se.Description, string(cfg), string(chars), formatTime(se.UpdatedAt), se.ID,
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
	refs, err := json.Marshal(ep.Refs)
	if err != nil {
		return err
	}
	if ep.Refs == nil {
		refs = []byte("[]")
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO episodes (id, series_id, number, title, topic, state_json, refs_json, workdir, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ep.ID, ep.SeriesID, ep.Number, ep.Title, ep.Topic, string(state), string(refs), ep.WorkDir,
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
		`SELECT id, series_id, number, title, topic, state_json, refs_json, workdir, created_at, updated_at
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
		`SELECT id, series_id, number, title, topic, state_json, refs_json, workdir, created_at, updated_at
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
	refs, err := json.Marshal(ep.Refs)
	if err != nil {
		return err
	}
	if ep.Refs == nil {
		refs = []byte("[]")
	}
	ep.UpdatedAt = time.Now()
	res, err := s.db.ExecContext(ctx,
		`UPDATE episodes SET title=?, topic=?, state_json=?, refs_json=?, workdir=?, updated_at=? WHERE id=?`,
		ep.Title, ep.Topic, string(state), string(refs), ep.WorkDir, formatTime(ep.UpdatedAt), ep.ID,
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

// ---- 系列分集策划会话 ----

// GetPlanSession 读取系列的策划会话；不存在时返回 ErrNotFound。
func (s *Store) GetPlanSession(ctx context.Context, seriesID string) (*domain.PlanSession, error) {
	var ps domain.PlanSession
	var messagesJSON, draftsJSON, charsJSON, created, updated string
	err := s.db.QueryRowContext(ctx,
		`SELECT series_id, messages_json, drafts_json, characters_json, created_at, updated_at
		 FROM plan_sessions WHERE series_id = ?`, seriesID,
	).Scan(&ps.SeriesID, &messagesJSON, &draftsJSON, &charsJSON, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: 策划会话 %s", ErrNotFound, seriesID)
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(messagesJSON), &ps.Messages); err != nil {
		return nil, fmt.Errorf("解析策划会话消息 %s: %w", seriesID, err)
	}
	if err := json.Unmarshal([]byte(draftsJSON), &ps.Drafts); err != nil {
		return nil, fmt.Errorf("解析分集草案 %s: %w", seriesID, err)
	}
	if err := json.Unmarshal([]byte(charsJSON), &ps.Characters); err != nil {
		return nil, fmt.Errorf("解析人物设定 %s: %w", seriesID, err)
	}
	ps.CreatedAt = parseTime(created)
	ps.UpdatedAt = parseTime(updated)
	return &ps, nil
}

// SavePlanSession 创建或更新策划会话（upsert）。
func (s *Store) SavePlanSession(ctx context.Context, ps *domain.PlanSession) error {
	messages, err := json.Marshal(ps.Messages)
	if err != nil {
		return err
	}
	drafts, err := json.Marshal(ps.Drafts)
	if err != nil {
		return err
	}
	chars, err := json.Marshal(ps.Characters)
	if err != nil {
		return err
	}
	now := time.Now()
	if ps.CreatedAt.IsZero() {
		ps.CreatedAt = now
	}
	ps.UpdatedAt = now
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO plan_sessions (series_id, messages_json, drafts_json, characters_json, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(series_id) DO UPDATE SET
		   messages_json   = excluded.messages_json,
		   drafts_json     = excluded.drafts_json,
		   characters_json = excluded.characters_json,
		   updated_at      = excluded.updated_at`,
		ps.SeriesID, string(messages), string(drafts), string(chars),
		formatTime(ps.CreatedAt), formatTime(ps.UpdatedAt),
	)
	return err
}

// DeletePlanSession 删除系列的策划会话；不存在不报错。
func (s *Store) DeletePlanSession(ctx context.Context, seriesID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM plan_sessions WHERE series_id = ?`, seriesID)
	return err
}

// ---- row scanner 适配 ----

type rowScanner interface {
	Scan(dest ...any) error
}

func scanSeries(r rowScanner) (*domain.Series, error) {
	var se domain.Series
	var cfgJSON, charsJSON, created, updated string
	if err := r.Scan(&se.ID, &se.Name, &se.Dynasty, &se.Description, &se.VoiceID, &cfgJSON, &charsJSON, &created, &updated); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(cfgJSON), &se.Config); err != nil {
		return nil, fmt.Errorf("解析系列配置 %s: %w", se.ID, err)
	}
	if err := json.Unmarshal([]byte(charsJSON), &se.Characters); err != nil {
		return nil, fmt.Errorf("解析系列人物设定 %s: %w", se.ID, err)
	}
	se.CreatedAt = parseTime(created)
	se.UpdatedAt = parseTime(updated)
	return &se, nil
}

func scanEpisode(r rowScanner) (*domain.Episode, error) {
	var ep domain.Episode
	var stateJSON, refsJSON, created, updated string
	if err := r.Scan(&ep.ID, &ep.SeriesID, &ep.Number, &ep.Title, &ep.Topic,
		&stateJSON, &refsJSON, &ep.WorkDir, &created, &updated); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(stateJSON), &ep.State); err != nil {
		return nil, fmt.Errorf("解析集状态 %s: %w", ep.ID, err)
	}
	if refsJSON != "" {
		if err := json.Unmarshal([]byte(refsJSON), &ep.Refs); err != nil {
			return nil, fmt.Errorf("解析集视觉参考 %s: %w", ep.ID, err)
		}
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
