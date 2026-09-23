package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sjzsdu/story/internal/domain"
	"github.com/sjzsdu/story/internal/port"
)

func openAssetTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestAssetCRUDAndListFilter(t *testing.T) {
	ctx := context.Background()
	s := openAssetTestStore(t)

	bgm := &domain.Asset{ID: "bgm-1", Kind: domain.AssetKindBGM, Name: "开场曲", Path: "/tmp/a.mp3", Origin: "upload"}
	if err := s.CreateAsset(ctx, bgm); err != nil {
		t.Fatalf("CreateAsset: %v", err)
	}
	if bgm.CreatedAt.IsZero() || bgm.UpdatedAt.IsZero() {
		t.Fatalf("时间未填充: %+v", bgm)
	}
	img := &domain.Asset{ID: "img-1", Kind: domain.AssetKindImageRef, Name: "兰若寺", Path: "/tmp/r.png"}
	if err := s.CreateAsset(ctx, img); err != nil {
		t.Fatalf("CreateAsset img: %v", err)
	}
	if err := s.CreateAsset(ctx, bgm); err == nil {
		t.Fatal("重复 ID 应报错")
	}

	got, err := s.GetAsset(ctx, "bgm-1")
	if err != nil {
		t.Fatalf("GetAsset: %v", err)
	}
	if got.Name != "开场曲" || got.Params == "" {
		t.Fatalf("读回不一致: %+v", got)
	}
	if _, err := s.GetAsset(ctx, "nope"); !errors.Is(err, port.ErrNotFound) {
		t.Fatalf("GetAsset 未找到应 ErrNotFound, got %v", err)
	}

	all, err := s.ListAssets(ctx, "")
	if err != nil || len(all) != 2 {
		t.Fatalf("ListAssets 全部: %v, n=%d", err, len(all))
	}
	onlyBGM, err := s.ListAssets(ctx, domain.AssetKindBGM)
	if err != nil || len(onlyBGM) != 1 || onlyBGM[0].ID != "bgm-1" {
		t.Fatalf("ListAssets kind 过滤: %v, %+v", err, onlyBGM)
	}

	got.Name = "开场曲·改"
	got.Description = "片头"
	if err := s.UpdateAsset(ctx, got); err != nil {
		t.Fatalf("UpdateAsset: %v", err)
	}
	got2, _ := s.GetAsset(ctx, "bgm-1")
	if got2.Name != "开场曲·改" || got2.Description != "片头" {
		t.Fatalf("更新未生效: %+v", got2)
	}
	upd := &domain.Asset{ID: "nope", Kind: domain.AssetKindBGM}
	if err := s.UpdateAsset(ctx, upd); !errors.Is(err, port.ErrNotFound) {
		t.Fatalf("UpdateAsset 未找到应 ErrNotFound, got %v", err)
	}
}

// TestAssetDeleteTwoStates 引用计数两态：0 → 删行成功；>0 → 拒删且 ErrAssetInUse。
func TestAssetDeleteTwoStates(t *testing.T) {
	ctx := context.Background()
	s := openAssetTestStore(t)

	a := &domain.Asset{ID: "bgm-used", Kind: domain.AssetKindBGM, Name: "被引用", Path: "/tmp/u.mp3"}
	if err := s.CreateAsset(ctx, a); err != nil {
		t.Fatalf("CreateAsset: %v", err)
	}

	// 态一：无引用 → 可删。
	free := &domain.Asset{ID: "bgm-free", Kind: domain.AssetKindBGM, Name: "未引用", Path: "/tmp/f.mp3"}
	if err := s.CreateAsset(ctx, free); err != nil {
		t.Fatalf("CreateAsset free: %v", err)
	}
	if err := s.DeleteAsset(ctx, "bgm-free"); err != nil {
		t.Fatalf("无引用应可删: %v", err)
	}
	if _, err := s.GetAsset(ctx, "bgm-free"); !errors.Is(err, port.ErrNotFound) {
		t.Fatalf("删除后应不存在, got %v", err)
	}

	// 建一个系列引用 bgm-used：bgm_path = "asset:bgm-used"。
	now := time.Now()
	ser := &domain.Series{ID: "s1", Name: "系列1", Config: domain.SeriesConfig{}, CreatedAt: now, UpdatedAt: now}
	ser.Config.BGMPath = domain.AssetRef("bgm-used")
	if err := s.CreateSeries(ctx, ser); err != nil {
		t.Fatalf("CreateSeries: %v", err)
	}

	n, err := s.CountAssetRefs(ctx, domain.AssetKindBGM, domain.AssetRef("bgm-used"))
	if err != nil || n != 1 {
		t.Fatalf("CountAssetRefs: n=%d err=%v", n, err)
	}
	// 精确匹配：别的引用串计 0。
	if n, _ := s.CountAssetRefs(ctx, domain.AssetKindBGM, "asset:other"); n != 0 {
		t.Fatalf("非匹配引用应为 0, got %d", n)
	}
	// image_ref：此处没有任何人物/视觉参考引用，应计 0（行级两态见
	// TestCountAssetRefsImageRefTwoStates）。
	if n, _ := s.CountAssetRefs(ctx, domain.AssetKindImageRef, domain.AssetRef("bgm-used")); n != 0 {
		t.Fatalf("image_ref 无引用应计 0, got %d", n)
	}

	// 态二：引用中拒删。
	err = s.DeleteAsset(ctx, "bgm-used")
	if !errors.Is(err, port.ErrAssetInUse) {
		t.Fatalf("引用中应 ErrAssetInUse, got %v", err)
	}
	if err == nil || !strings.Contains(err.Error(), "1 个系列引用") {
		t.Fatalf("错误文案应含引用数: %v", err)
	}
	if _, err := s.GetAsset(ctx, "bgm-used"); err != nil {
		t.Fatalf("拒删后素材应还在: %v", err)
	}

	// 解除引用 → 可删。
	ser.Config.BGMPath = ""
	if err := s.UpdateSeries(ctx, ser); err != nil {
		t.Fatalf("UpdateSeries: %v", err)
	}
	if err := s.DeleteAsset(ctx, "bgm-used"); err != nil {
		t.Fatalf("解除引用后应可删: %v", err)
	}
	if err := s.DeleteAsset(ctx, "bgm-used"); !errors.Is(err, port.ErrNotFound) {
		t.Fatalf("二次删除应 ErrNotFound, got %v", err)
	}
}

// TestCountAssetRefsImageRefTwoStates 覆盖 image_ref 的行级引用计数两态（§24 第二步）：
// series.characters_json 与 episodes.refs_json 的 ref_image 精确匹配计数、
// 引用中拒删（文案按 kind 区分解除方式），解除全部引用后归 0 可删且行消失。
func TestCountAssetRefsImageRefTwoStates(t *testing.T) {
	ctx := context.Background()
	s := openAssetTestStore(t)

	if err := s.CreateAsset(ctx, &domain.Asset{
		ID: "img-used", Kind: domain.AssetKindImageRef, Name: "张仪",
		Path: "/tmp/img-used.png", Origin: "generate",
	}); err != nil {
		t.Fatalf("CreateAsset: %v", err)
	}
	ref := domain.AssetRef("img-used")

	// 零引用计数。
	if n, err := s.CountAssetRefs(ctx, domain.AssetKindImageRef, ref); err != nil || n != 0 {
		t.Fatalf("零引用计数: n=%d err=%v", n, err)
	}

	// 系列人物设定集引用 +1。
	now := time.Now()
	ser := &domain.Series{ID: "s-img", Name: "系列图", CreatedAt: now, UpdatedAt: now}
	ser.Characters = []domain.CharacterSetting{{Name: "张仪", RefImage: ref}}
	if err := s.CreateSeries(ctx, ser); err != nil {
		t.Fatalf("CreateSeries: %v", err)
	}
	if n, err := s.CountAssetRefs(ctx, domain.AssetKindImageRef, ref); err != nil || n != 1 {
		t.Fatalf("系列人物引用计数: n=%d err=%v", n, err)
	}

	// 集视觉参考引用再 +1。
	ep := domain.NewEpisode("s-img-e01", "s-img", 1, "题", "", "/tmp/s-img-e01")
	ep.Refs = []domain.VisualRef{{Kind: "character", Name: "张仪", RefImage: ref}}
	if err := s.CreateEpisode(ctx, ep); err != nil {
		t.Fatalf("CreateEpisode: %v", err)
	}
	if n, err := s.CountAssetRefs(ctx, domain.AssetKindImageRef, ref); err != nil || n != 2 {
		t.Fatalf("系列+集两级引用计数: n=%d err=%v", n, err)
	}

	// 态二：引用中拒删，文案按 image_ref kind 给出解除方式。
	err := s.DeleteAsset(ctx, "img-used")
	if !errors.Is(err, port.ErrAssetInUse) {
		t.Fatalf("引用中应 ErrAssetInUse, got %v", err)
	}
	if err == nil || !strings.Contains(err.Error(), "系列人物设定 / 集视觉参考") {
		t.Fatalf("image_ref 拒删文案应含解除方式: %v", err)
	}
	if _, err := s.GetAsset(ctx, "img-used"); err != nil {
		t.Fatalf("拒删后素材行应保留: %v", err)
	}

	// 解除两级引用 → 归 0 → 可删且行消失。
	ser.Characters = nil
	if err := s.UpdateSeries(ctx, ser); err != nil {
		t.Fatalf("UpdateSeries: %v", err)
	}
	ep.Refs = nil
	if err := s.SaveEpisode(ctx, ep); err != nil {
		t.Fatalf("SaveEpisode: %v", err)
	}
	if n, err := s.CountAssetRefs(ctx, domain.AssetKindImageRef, ref); err != nil || n != 0 {
		t.Fatalf("解除后计数应 0: n=%d err=%v", n, err)
	}
	if err := s.DeleteAsset(ctx, "img-used"); err != nil {
		t.Fatalf("解除后应可删: %v", err)
	}
	if _, err := s.GetAsset(ctx, "img-used"); !errors.Is(err, port.ErrNotFound) {
		t.Fatalf("删除后应不存在, got %v", err)
	}
}
