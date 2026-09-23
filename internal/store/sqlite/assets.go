package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/sjzsdu/story/internal/domain"
	"github.com/sjzsdu/story/internal/port"
)

// 素材/资产（§23 统一资源管理）CRUD + 引用计数。
// 样板对齐 voices.go：引用中拒删用 %w 包装 port.ErrAssetInUse，server 以 errors.Is 映射 409。

// ---- CRUD ----

// CreateAsset 插入素材条目；ID 冲突时返回主键约束错误。
func (s *Store) CreateAsset(ctx context.Context, a *domain.Asset) error {
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now()
	}
	a.UpdatedAt = a.CreatedAt
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO assets (id, kind, name, path, description, duration_sec, params, origin, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.Kind, a.Name, a.Path, a.Description, a.DurationSec, emptyJSON(a.Params), a.Origin,
		formatTime(a.CreatedAt), formatTime(a.UpdatedAt),
	)
	if err != nil {
		return fmt.Errorf("创建素材 %s: %w", a.ID, err)
	}
	return nil
}

// GetAsset 按 ID 查询素材。
func (s *Store) GetAsset(ctx context.Context, id string) (*domain.Asset, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, kind, name, path, description, duration_sec, params, origin, created_at, updated_at
		 FROM assets WHERE id = ?`, id)
	a, err := scanAsset(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: 素材 %s", ErrNotFound, id)
	}
	return a, err
}

// ListAssets 列出素材；kind 为空列出全部，非空按类型过滤（创建时间升序）。
func (s *Store) ListAssets(ctx context.Context, kind string) ([]*domain.Asset, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, kind, name, path, description, duration_sec, params, origin, created_at, updated_at
		 FROM assets WHERE ? = '' OR kind = ? ORDER BY created_at ASC`, kind, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*domain.Asset
	for rows.Next() {
		a, err := scanAsset(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// UpdateAsset 更新素材（除 ID 外全部字段）。
func (s *Store) UpdateAsset(ctx context.Context, a *domain.Asset) error {
	a.UpdatedAt = time.Now()
	res, err := s.db.ExecContext(ctx,
		`UPDATE assets SET kind=?, name=?, path=?, description=?, duration_sec=?, params=?, origin=?, updated_at=? WHERE id=?`,
		a.Kind, a.Name, a.Path, a.Description, a.DurationSec, emptyJSON(a.Params), a.Origin,
		formatTime(a.UpdatedAt), a.ID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: 素材 %s", ErrNotFound, a.ID)
	}
	return nil
}

// DeleteAsset 删除素材：仍被系列引用时拒绝（%w 包装 port.ErrAssetInUse）。
func (s *Store) DeleteAsset(ctx context.Context, id string) error {
	a, err := s.GetAsset(ctx, id)
	if err != nil {
		return err
	}
	count, err := s.CountAssetRefs(ctx, a.Kind, domain.AssetRef(id))
	if err != nil {
		return err
	}
	if count > 0 {
		// 文案按 kind 区分：bgm 是系列配置引用（守住既有测试），image_ref 是行级引用
		//（系列人物设定 / 集视觉参考），提示的解除方式不同。
		if a.Kind == domain.AssetKindImageRef {
			return fmt.Errorf("%w: %s 被引用 %d 处（系列人物设定 / 集视觉参考），请先解除引用",
				port.ErrAssetInUse, id, count)
		}
		return fmt.Errorf("%w: %s 被 %d 个系列引用", port.ErrAssetInUse, id, count)
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM assets WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: 素材 %s", ErrNotFound, id)
	}
	return nil
}

// CountAssetRefs 统计引用了该素材的行数（value 为完整引用串 "asset:<id>"，精确匹配）。
//   - bgm：series.config_json 的 $.bgm_path 精确匹配（json_each/json_extract；非 LIKE、非裸 ID）；
//   - image_ref：行级计数——series.characters_json 的 $.ref_image +
//     episodes.refs_json 的 $.ref_image 各自 EXISTS 精确匹配后求和（json_valid 守卫坏 JSON）。
//     行内多处引用同一素材只计 1（拒删语义只需知道「有没有人在用」）。
func (s *Store) CountAssetRefs(ctx context.Context, kind, value string) (int, error) {
	switch kind {
	case domain.AssetKindBGM:
		var n int
		err := s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM series WHERE json_extract(config_json, '$.bgm_path') = ?`, value,
		).Scan(&n)
		return n, err
	case domain.AssetKindImageRef:
		var n int
		err := s.db.QueryRowContext(ctx,
			`SELECT
			   (SELECT COUNT(*) FROM series
			      WHERE json_valid(characters_json)
			        AND EXISTS (SELECT dr.value FROM json_each(characters_json) dr
			                     WHERE json_extract(dr.value, '$.ref_image') = ?))
			 + (SELECT COUNT(*) FROM episodes
			      WHERE json_valid(refs_json)
			        AND EXISTS (SELECT er.value FROM json_each(refs_json) er
			                     WHERE json_extract(er.value, '$.ref_image') = ?))`,
			value, value,
		).Scan(&n)
		return n, err
	default:
		return 0, nil
	}
}

// ---- 辅助 ----

func scanAsset(r rowScanner) (*domain.Asset, error) {
	var a domain.Asset
	var desc, params, origin, created, updated string
	if err := r.Scan(&a.ID, &a.Kind, &a.Name, &a.Path, &desc, &a.DurationSec, &params, &origin, &created, &updated); err != nil {
		return nil, err
	}
	a.Description = desc
	a.Params = params
	a.Origin = origin
	a.CreatedAt = parseTime(created)
	a.UpdatedAt = parseTime(updated)
	return &a, nil
}

// emptyJSON 空 params 存合法 JSON 空对象。
func emptyJSON(s string) string {
	if s == "" {
		return "{}"
	}
	return s
}
