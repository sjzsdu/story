package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/sjzsdu/story/internal/domain"
)

// 声音条目删除/更新时的业务错误。
var (
	// ErrVoiceBuiltin 内置声音不可删除（可编辑）。
	ErrVoiceBuiltin = errors.New("内置声音不可删除")
	// ErrVoiceInUse 声音被系列引用，删除会破坏引用完整性。
	ErrVoiceInUse = errors.New("声音被系列引用")
)

// ---- CRUD ----

// CreateVoice 插入声音条目；ID 冲突时返回 SQLite 主键约束错误。
func (s *Store) CreateVoice(ctx context.Context, v *domain.Voice) error {
	if v.CreatedAt.IsZero() {
		v.CreatedAt = time.Now()
	}
	v.UpdatedAt = v.CreatedAt
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO voices (id, name, voice, instruction, rate, pitch, style_note, is_builtin, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		v.ID, v.Name, v.Voice, v.Instruction, v.Rate, v.Pitch, v.StyleNote,
		btoi(v.IsBuiltin), formatTime(v.CreatedAt), formatTime(v.UpdatedAt),
	)
	if err != nil {
		return fmt.Errorf("创建声音 %s: %w", v.ID, err)
	}
	return nil
}

// GetVoice 按 ID 查询声音条目。
func (s *Store) GetVoice(ctx context.Context, id string) (*domain.Voice, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, voice, instruction, rate, pitch, style_note, is_builtin, created_at, updated_at
		 FROM voices WHERE id = ?`, id)
	v, err := scanVoice(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: 声音 %s", ErrNotFound, id)
	}
	return v, err
}

// ListVoices 列出全部声音（内置在前，按创建时间升序）。
func (s *Store) ListVoices(ctx context.Context) ([]*domain.Voice, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, voice, instruction, rate, pitch, style_note, is_builtin, created_at, updated_at
		 FROM voices ORDER BY is_builtin DESC, created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.Voice
	for rows.Next() {
		v, err := scanVoice(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// UpdateVoice 更新声音条目（除 ID 外全部字段，含 IsBuiltin 标记）。
// voice_id 在 series 上锁定，但声音条目本身可改，编辑后影响所有引用它的系列。
func (s *Store) UpdateVoice(ctx context.Context, v *domain.Voice) error {
	v.UpdatedAt = time.Now()
	res, err := s.db.ExecContext(ctx,
		`UPDATE voices SET name=?, voice=?, instruction=?, rate=?, pitch=?, style_note=?, is_builtin=?, updated_at=? WHERE id=?`,
		v.Name, v.Voice, v.Instruction, v.Rate, v.Pitch, v.StyleNote,
		btoi(v.IsBuiltin), formatTime(v.UpdatedAt), v.ID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: 声音 %s", ErrNotFound, v.ID)
	}
	return nil
}

// DeleteVoice 删除声音条目：内置（is_builtin=1）拒绝；被系列引用拒绝。
func (s *Store) DeleteVoice(ctx context.Context, id string) error {
	v, err := s.GetVoice(ctx, id)
	if err != nil {
		return err
	}
	if v.IsBuiltin {
		return fmt.Errorf("%w: %s", ErrVoiceBuiltin, id)
	}
	count, err := s.CountSeriesByVoiceID(ctx, id)
	if err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("%w: %s 被 %d 个系列引用", ErrVoiceInUse, id, count)
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM voices WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: 声音 %s", ErrNotFound, id)
	}
	return nil
}

// CountSeriesByVoiceID 统计引用某声音的系列数。
func (s *Store) CountSeriesByVoiceID(ctx context.Context, voiceID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM series WHERE voice_id = ?`, voiceID,
	).Scan(&n)
	return n, err
}

// ---- 一次性启动辅助 ----

// SeedBuiltinVoices 用 INSERT OR IGNORE 把预设列表写入 voices 表（幂等）。
// 内置条目的 ID 取自 VoiceProfile.Key；is_builtin=1。
// Bootstrap 在 Open 之后调用，错误即返回。
func SeedBuiltinVoices(ctx context.Context, s *Store, presets []domain.VoiceProfile) error {
	for _, p := range presets {
		if p.Key == "" || p.Voice == "" {
			continue
		}
		now := time.Now()
		_, err := s.db.ExecContext(ctx,
			`INSERT OR IGNORE INTO voices
			 (id, name, voice, instruction, rate, pitch, style_note, is_builtin, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`,
			p.Key, p.Name, p.Voice, p.Instruction, p.Rate, p.Pitch, p.StyleNote,
			formatTime(now), formatTime(now),
		)
		if err != nil {
			return fmt.Errorf("seed 内置声音 %s: %w", p.Key, err)
		}
	}
	return nil
}

// MigrateSeriesVoiceIDs 把旧 series 的 VoiceProfile/TTSVoice 反查一个 voice_id 写回。
// 平迁规则（按优先级）：
//  1. voice_id 已有 → 跳过（不动）。
//  2. VoiceProfile 非空且对应内置条目存在 → 用预设 key 作 voice_id。
//  3. TTSVoice 或 fallback 非空 → 若该 voice_id 已有匹配条目则用，否则按自定义参数建用户条目并写回 ID。
//  4. 全空 → 写默认条目 ID（wangliqun）。
//
// 不删 SeriesConfig 旧字段（兼容期保留）；engine resolveVoice 优先用 voice_id。
func MigrateSeriesVoiceIDs(ctx context.Context, s *Store, fallbackVoice, fallbackInstr string) error {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, voice_id, config_json FROM series ORDER BY created_at ASC`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type migrateRow struct {
		id      string
		voiceID string
		cfg     domain.SeriesConfig
	}
	var pending []migrateRow
	for rows.Next() {
		var id, voiceID, cfgJSON string
		if err := rows.Scan(&id, &voiceID, &cfgJSON); err != nil {
			return err
		}
		if voiceID != "" {
			continue // 已有 voice_id，跳过
		}
		var cfg domain.SeriesConfig
		if err := jsonUnmarshal(cfgJSON, &cfg); err != nil {
			return fmt.Errorf("迁移 voice_id 解析配置 %s: %w", id, err)
		}
		pending = append(pending, migrateRow{id: id, voiceID: voiceID, cfg: cfg})
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, p := range pending {
		vid, err := resolveMigrationVoiceID(ctx, s, p.cfg, fallbackVoice, fallbackInstr)
		if err != nil {
			return fmt.Errorf("迁移系列 %s 的 voice_id: %w", p.id, err)
		}
		if _, err := s.db.ExecContext(ctx,
			`UPDATE series SET voice_id = ? WHERE id = ?`, vid, p.id); err != nil {
			return fmt.Errorf("写回 series.voice_id %s: %w", p.id, err)
		}
	}
	return nil
}

// resolveMigrationVoiceID 实现平迁规则；调用方负责已存在 voice_id 跳过判断。
func resolveMigrationVoiceID(ctx context.Context, s *Store, cfg domain.SeriesConfig, fallbackVoice, fallbackInstr string) (string, error) {
	// 规则 2：预设 key 对应内置条目。
	if cfg.VoiceProfile != "" {
		if _, err := s.GetVoice(ctx, cfg.VoiceProfile); err == nil {
			return cfg.VoiceProfile, nil
		}
	}
	// 规则 3：自定义参数 → 建/取条目。
	voice := cfg.TTSVoice
	if voice == "" {
		voice = fallbackVoice
	}
	if voice != "" {
		// 先尝试按 voice + rate + pitch 匹配既有条目（避免重复建）。
		existing, _ := s.ListVoices(ctx)
		for _, v := range existing {
			if v.Voice == voice && v.Rate == cfg.TTSRate && v.Pitch == cfg.TTSPitch &&
				v.Instruction == firstNonEmptyStr(cfg.TTSInstruction, fallbackInstr) {
				return v.ID, nil
			}
		}
		// 没匹配到 → 新建用户条目。
		id := fmt.Sprintf("custom-%d", time.Now().UnixNano())
		v := &domain.Voice{
			ID:          id,
			Name:        "自定义 " + voice,
			Voice:       voice,
			Instruction: firstNonEmptyStr(cfg.TTSInstruction, fallbackInstr),
			Rate:        cfg.TTSRate,
			Pitch:       cfg.TTSPitch,
			IsBuiltin:   false,
		}
		if err := s.CreateVoice(ctx, v); err != nil {
			return "", err
		}
		return id, nil
	}
	// 规则 4：全空 → 默认条目。
	return domain.VoiceProfileWangliqun, nil
}

// ---- 辅助 ----

func scanVoice(r rowScanner) (*domain.Voice, error) {
	var v domain.Voice
	var instr, style, created, updated string
	var isBuiltin int
	if err := r.Scan(&v.ID, &v.Name, &v.Voice, &instr, &v.Rate, &v.Pitch, &style, &isBuiltin, &created, &updated); err != nil {
		return nil, err
	}
	v.Instruction = instr
	v.StyleNote = style
	v.IsBuiltin = isBuiltin != 0
	v.CreatedAt = parseTime(created)
	v.UpdatedAt = parseTime(updated)
	return &v, nil
}

// btoi bool → int（SQLite 用 INTEGER 存布尔）。
func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

// jsonUnmarshal 包装（同包共享 import，与 sqlite.go 一致使用 encoding/json）。
func jsonUnmarshal(s string, v *domain.SeriesConfig) error {
	return json.Unmarshal([]byte(s), v)
}

func firstNonEmptyStr(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
