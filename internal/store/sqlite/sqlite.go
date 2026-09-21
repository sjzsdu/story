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
    id             TEXT PRIMARY KEY,
    series_id      TEXT NOT NULL REFERENCES series(id) ON DELETE CASCADE,
    number         INTEGER NOT NULL,
    title          TEXT NOT NULL,
    topic          TEXT NOT NULL DEFAULT '',
    instruction    TEXT NOT NULL DEFAULT '',
    state_json     TEXT NOT NULL DEFAULT '{}',
    nodes_json     TEXT NOT NULL DEFAULT '[]',
    active_node_id TEXT NOT NULL DEFAULT '',
    refs_json      TEXT NOT NULL DEFAULT '[]',
    workdir        TEXT NOT NULL,
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL,
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
    provider    TEXT NOT NULL DEFAULT 'bailian',
    voice       TEXT NOT NULL,
    model       TEXT NOT NULL DEFAULT '',
    instruction TEXT NOT NULL DEFAULT '',
    rate        REAL NOT NULL DEFAULT 0,
    pitch       REAL NOT NULL DEFAULT 0,
    style_note  TEXT NOT NULL DEFAULT '',
    is_builtin  INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS publish_jobs (
    id                TEXT PRIMARY KEY,
    episode_id        TEXT NOT NULL REFERENCES episodes(id) ON DELETE CASCADE,
    series_id         TEXT NOT NULL,
    platform          TEXT NOT NULL,
    node_id           TEXT NOT NULL DEFAULT '',
    status            TEXT NOT NULL DEFAULT 'pending',
    video_path        TEXT NOT NULL DEFAULT '',
    cover_path        TEXT NOT NULL DEFAULT '',
    title             TEXT NOT NULL DEFAULT '',
    description       TEXT NOT NULL DEFAULT '',
    tags              TEXT NOT NULL DEFAULT '[]',
    category          TEXT NOT NULL DEFAULT '',
    platform_video_id TEXT NOT NULL DEFAULT '',
    platform_url      TEXT NOT NULL DEFAULT '',
    scheduled_at      TEXT,
    attempts          INTEGER NOT NULL DEFAULT 0,
    max_retries       INTEGER NOT NULL DEFAULT 3,
    error             TEXT NOT NULL DEFAULT '',
    created_at        TEXT NOT NULL,
    updated_at        TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS platform_accounts (
    id            TEXT PRIMARY KEY,
    platform      TEXT NOT NULL,
    account_name  TEXT NOT NULL DEFAULT '',
    account_id    TEXT NOT NULL DEFAULT '',
    access_token  TEXT NOT NULL DEFAULT '',
    refresh_token TEXT NOT NULL DEFAULT '',
    token_expiry  TEXT NOT NULL DEFAULT '',
    extra         TEXT NOT NULL DEFAULT '',
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL
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
		// §17：版本树（nodes_json）与活跃节点指针（active_node_id）。
		// 旧库的 state_json 列保留为迁移前的只读历史快照，新代码不再写入。
		{"episodes", "nodes_json", "TEXT NOT NULL DEFAULT '[]'"},
		{"episodes", "active_node_id", "TEXT NOT NULL DEFAULT ''"},
		// 创作控制参数：集级附加指令（叠加在系列创作设置之上）。
		{"episodes", "instruction", "TEXT NOT NULL DEFAULT ''"},
		// §16：series.voice_id 引用顶层 Voice 实体，创建后锁定（UpdateSeries 不写该列）。
		{"series", "voice_id", "TEXT NOT NULL DEFAULT ''"},
		// §16：voices.provider 标识 TTS 供应商，旧行默认 bailian。
		{"voices", "provider", "TEXT NOT NULL DEFAULT 'bailian'"},
		// §16：voices.model 驱动音色的合成模型（造声音色必填），旧行空表示用全局 tts_model。
		{"voices", "model", "TEXT NOT NULL DEFAULT ''"},
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
//
// state_json 仅供旧库的 NOT NULL 约束占位（新集没有历史状态），
// 真正的产物状态存在 nodes_json / active_node_id（§17 版本树）。
func (s *Store) CreateEpisode(ctx context.Context, ep *domain.Episode) error {
	nodes, err := json.Marshal(ep.Nodes)
	if err != nil {
		return err
	}
	if ep.Nodes == nil {
		nodes = []byte("[]")
	}
	refs, err := json.Marshal(ep.Refs)
	if err != nil {
		return err
	}
	if ep.Refs == nil {
		refs = []byte("[]")
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO episodes (id, series_id, number, title, topic, instruction, state_json, nodes_json, active_node_id, refs_json, workdir, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, '{}', ?, ?, ?, ?, ?, ?)`,
		ep.ID, ep.SeriesID, ep.Number, ep.Title, ep.Topic, ep.Instruction, string(nodes), ep.ActiveNodeID,
		string(refs), ep.WorkDir, formatTime(ep.CreatedAt), formatTime(ep.UpdatedAt),
	)
	if err != nil {
		return fmt.Errorf("创建集 %s: %w", ep.ID, err)
	}
	return nil
}

// GetEpisode 按 ID 查询集。
func (s *Store) GetEpisode(ctx context.Context, id string) (*domain.Episode, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, series_id, number, title, topic, instruction, nodes_json, active_node_id, refs_json, workdir, created_at, updated_at
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
		`SELECT id, series_id, number, title, topic, instruction, nodes_json, active_node_id, refs_json, workdir, created_at, updated_at
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

// SaveEpisode 整体写回集（含版本树）。state_json 为迁移前的历史快照，不在此写入。
func (s *Store) SaveEpisode(ctx context.Context, ep *domain.Episode) error {
	nodes, err := json.Marshal(ep.Nodes)
	if err != nil {
		return err
	}
	if ep.Nodes == nil {
		nodes = []byte("[]")
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
		`UPDATE episodes SET title=?, topic=?, instruction=?, nodes_json=?, active_node_id=?, refs_json=?, workdir=?, updated_at=? WHERE id=?`,
		ep.Title, ep.Topic, ep.Instruction, string(nodes), ep.ActiveNodeID, string(refs), ep.WorkDir, formatTime(ep.UpdatedAt), ep.ID,
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
	var nodesJSON, refsJSON, created, updated string
	if err := r.Scan(&ep.ID, &ep.SeriesID, &ep.Number, &ep.Title, &ep.Topic, &ep.Instruction,
		&nodesJSON, &ep.ActiveNodeID, &refsJSON, &ep.WorkDir, &created, &updated); err != nil {
		return nil, err
	}
	if nodesJSON != "" {
		if err := json.Unmarshal([]byte(nodesJSON), &ep.Nodes); err != nil {
			return nil, fmt.Errorf("解析集版本树 %s: %w", ep.ID, err)
		}
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

// ListUnmigratedEpisodes 列出尚未迁移到版本树的集 ID（§17 一次性迁移用）。
// 判据：版本树为空且活跃指针为空。
func (s *Store) ListUnmigratedEpisodes(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id FROM episodes WHERE nodes_json IN ('', '[]') AND active_node_id = '' ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// LegacyEpisodeState 返回某集迁移前的 state_json 原文（§17 一次性迁移专用；
// 迁移完成后该列不再被任何代码读写，仅作历史快照保留）。
func (s *Store) LegacyEpisodeState(ctx context.Context, id string) ([]byte, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT state_json FROM episodes WHERE id = ?`, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: 集 %s", ErrNotFound, id)
	}
	if err != nil {
		return nil, err
	}
	return []byte(raw), nil
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
