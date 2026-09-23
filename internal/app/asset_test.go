package app

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sjzsdu/story/internal/config"
	"github.com/sjzsdu/story/internal/domain"
	"github.com/sjzsdu/story/testutil/mock"
)

// 素材/资产（§23）app 层测试：全程 mock 仓储 + 临时 DataDir + 假时长探测，
// 不触碰 bl（成本红线）、不依赖本机 ffprobe。

// newAssetApp 直构 App：mock 仓库、临时数据目录、注入假 assetProber。
func newAssetApp(t *testing.T) *App {
	t.Helper()
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	a := &App{Repo: mock.NewRepo()}
	a.setConfig(cfg)
	a.assetProber = func(context.Context, string) (float64, error) { return 12.5, nil }
	return a
}

// TestUploadAssetLandsInLibrary 上传链路：落盘进素材库子目录、恶意文件名不逃出、
// 未知 kind 显式拒绝（不静默回退写错目录）。
func TestUploadAssetLandsInLibrary(t *testing.T) {
	a := newAssetApp(t)
	ctx := context.Background()

	// 文件名带路径穿越成分：只取扩展名，绝不参与拼路径。
	as, err := a.UploadAsset(ctx, bytes.NewReader([]byte("fake-mp3")), "../../evil.mp3",
		domain.AssetKindBGM, "主题曲", "开场")
	if err != nil {
		t.Fatalf("UploadAsset: %v", err)
	}
	if as.Origin != "upload" {
		t.Fatalf("origin 应为 upload: %q", as.Origin)
	}
	if as.DurationSec != 12.5 {
		t.Fatalf("时长应取注入的探测结果: %v", as.DurationSec)
	}
	if as.Name != "主题曲" || as.Description != "开场" {
		t.Fatalf("名称/描述未写入: %+v", as)
	}

	dir, err := a.Config().AssetDir(domain.AssetKindBGM)
	if err != nil {
		t.Fatal(err)
	}
	dirAbs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(dirAbs, as.Path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		t.Fatalf("素材文件必须落在素材库目录内: %s（目录 %s）", as.Path, dirAbs)
	}
	if _, err := os.Stat(as.Path); err != nil {
		t.Fatalf("落盘文件缺失: %v", err)
	}

	// 未知 kind 报错，不写任何文件。
	if _, err := a.UploadAsset(ctx, bytes.NewReader(nil), "a.mp3", "evil-kind", "", ""); err == nil ||
		!strings.Contains(err.Error(), "未知素材类型") {
		t.Fatalf("未知 kind 应报「未知素材类型」: %v", err)
	}
	// ListAssets 同样拒绝未知 kind（空串＝全部）。
	if _, err := a.ListAssets(ctx, "nope"); err == nil {
		t.Fatal("ListAssets 未知 kind 应报错")
	}
	if list, err := a.ListAssets(ctx, ""); err != nil || len(list) != 1 {
		t.Fatalf("ListAssets 空 kind 应列出全部: %v, n=%d", err, len(list))
	}
}

// TestImportAssetCopiesIntoLibrary 导入链路：复制进库而非引用原路径、原文件不动、
// 缺源/源不存在报清晰错误。
func TestImportAssetCopiesIntoLibrary(t *testing.T) {
	a := newAssetApp(t)
	ctx := context.Background()

	src := filepath.Join(t.TempDir(), "mine.mp3")
	if err := os.WriteFile(src, []byte("orig"), 0o644); err != nil {
		t.Fatal(err)
	}
	as, err := a.ImportAsset(ctx, src, domain.AssetKindBGM, "", "")
	if err != nil {
		t.Fatalf("ImportAsset: %v", err)
	}
	if as.Origin != "import" {
		t.Fatalf("origin 应为 import: %q", as.Origin)
	}
	if as.Path == src {
		t.Fatal("应复制进素材库而非引用原路径")
	}
	if b, err := os.ReadFile(as.Path); err != nil || string(b) != "orig" {
		t.Fatalf("副本内容不一致: %v %q", err, b)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("原文件不应被动: %v", err)
	}
	// 名称留空回退文件名。
	if as.Name == "" {
		t.Fatal("名称留空应回退文件名")
	}

	if _, err := a.ImportAsset(ctx, "  ", domain.AssetKindBGM, "", ""); err == nil {
		t.Fatal("空源路径应报错")
	}
	if _, err := a.ImportAsset(ctx, filepath.Join(t.TempDir(), "none.mp3"), domain.AssetKindBGM, "", ""); err == nil ||
		!strings.Contains(err.Error(), "导入源文件不存在") {
		t.Fatalf("源不存在应报「导入源文件不存在」: %v", err)
	}
}

// TestDeleteAssetTwoStates 删除链路三道闸：引用中拒删（文件保留）→ 解除引用可删
// （行与文件都清）→ 库外路径一律拒绝删除（绝不触碰素材库之外的文件）。
func TestDeleteAssetTwoStates(t *testing.T) {
	a := newAssetApp(t)
	ctx := context.Background()

	as, err := a.UploadAsset(ctx, bytes.NewReader([]byte("x")), "ref.mp3", domain.AssetKindBGM, "被引用", "")
	if err != nil {
		t.Fatalf("UploadAsset: %v", err)
	}
	now := time.Now()
	se := &domain.Series{
		ID: "s1", Name: "系列一",
		Config:    domain.SeriesConfig{BGMPath: domain.AssetRef(as.ID)},
		CreatedAt: now, UpdatedAt: now,
	}
	if err := a.Repo.CreateSeries(ctx, se); err != nil {
		t.Fatal(err)
	}

	// 态一：引用中拒删，且错误可 errors.Is 判别（server → 409）。
	err = a.DeleteAsset(ctx, as.ID)
	if !errors.Is(err, ErrAssetInUse) {
		t.Fatalf("引用中应 ErrAssetInUse, got %v", err)
	}
	if err == nil || !strings.Contains(err.Error(), "1 个系列引用") {
		t.Fatalf("错误文案应含引用数: %v", err)
	}
	if _, err := os.Stat(as.Path); err != nil {
		t.Fatalf("拒删时文件必须保留: %v", err)
	}

	// 解除引用 → 可删（行 + 文件都清）。
	se.Config.BGMPath = ""
	if err := a.Repo.UpdateSeries(ctx, se); err != nil {
		t.Fatal(err)
	}
	if err := a.DeleteAsset(ctx, as.ID); err != nil {
		t.Fatalf("解除引用后应可删: %v", err)
	}
	if _, err := os.Stat(as.Path); !os.IsNotExist(err) {
		t.Fatalf("删除成功后文件应被清理: %v", err)
	}
	if err := a.DeleteAsset(ctx, as.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("二次删除应 ErrNotFound, got %v", err)
	}

	// 库外路径拒删（防脏数据把库外文件删掉），文件原样保留。
	out := filepath.Join(t.TempDir(), "outside.mp3")
	if err := os.WriteFile(out, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	orphan := &domain.Asset{ID: "bgm-orphan", Kind: domain.AssetKindBGM, Name: "库外", Path: out, Origin: "import"}
	if err := a.Repo.CreateAsset(ctx, orphan); err != nil {
		t.Fatal(err)
	}
	err = a.DeleteAsset(ctx, "bgm-orphan")
	if err == nil || !strings.Contains(err.Error(), "拒绝删除") {
		t.Fatalf("库外路径应拒绝删除: %v", err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("库外文件不得被动: %v", err)
	}
}

// TestUpdateSeriesBGMValidatesAssetRef 设值校验三条链路（UpdateSeriesBGM /
// CreateSeries 走的 validateAssetRef / UpdateSeries 整对象写回）：
// 字面路径放行（§20 历史语义）、悬空引用设值即拒且不落库、音量越界仍拒（§20 回归）。
func TestUpdateSeriesBGMValidatesAssetRef(t *testing.T) {
	a := newAssetApp(t)
	ctx := context.Background()

	now := time.Now()
	se := &domain.Series{ID: "s1", Name: "系列一", CreatedAt: now, UpdatedAt: now}
	if err := a.Repo.CreateSeries(ctx, se); err != nil {
		t.Fatal(err)
	}

	// 字面路径放行。
	lit := "bgm/theme.mp3"
	if err := a.UpdateSeriesBGM(ctx, "s1", &lit, nil); err != nil {
		t.Fatalf("字面路径应放行: %v", err)
	}
	if got, _ := a.Repo.GetSeries(ctx, "s1"); got.Config.BGMPath != lit {
		t.Fatalf("字面路径未写入: %q", got.Config.BGMPath)
	}

	// 悬空引用设值即拒，且库里的值保持原样。
	bad := domain.AssetRef("bgm-nope")
	err := a.UpdateSeriesBGM(ctx, "s1", &bad, nil)
	if !errors.Is(err, ErrAssetRefInvalid) {
		t.Fatalf("悬空引用应 ErrAssetRefInvalid, got %v", err)
	}
	if err == nil || !strings.Contains(err.Error(), "请先在素材库上传该曲目") {
		t.Fatalf("错误应给修复方式: %v", err)
	}
	if got, _ := a.Repo.GetSeries(ctx, "s1"); got.Config.BGMPath != lit {
		t.Fatalf("校验失败不应落库: %q", got.Config.BGMPath)
	}

	// 存在的素材引用可设。
	as, err := a.UploadAsset(ctx, bytes.NewReader([]byte("x")), "ok.mp3", domain.AssetKindBGM, "有效曲", "")
	if err != nil {
		t.Fatal(err)
	}
	good := domain.AssetRef(as.ID)
	if err := a.UpdateSeriesBGM(ctx, "s1", &good, nil); err != nil {
		t.Fatalf("有效引用应可设: %v", err)
	}

	// UpdateSeries（Web PUT / CLI series set 整对象写回）同样校验。
	// 注：mock 的 GetSeries 返回共享指针（sqlite 是副本），先值拷贝再改，
	// 模拟 handler「读出 → 修改 → 提交」的真实链路。
	cur, _ := a.Repo.GetSeries(ctx, "s1")
	full := *cur
	full.Config.BGMPath = domain.AssetRef("bgm-ghost")
	if err := a.UpdateSeries(ctx, &full); !errors.Is(err, ErrAssetRefInvalid) {
		t.Fatalf("UpdateSeries 悬空引用应被拒, got %v", err)
	}
	if got, _ := a.Repo.GetSeries(ctx, "s1"); got.Config.BGMPath != good {
		t.Fatalf("校验失败不应落库: %q", got.Config.BGMPath)
	}

	// 音量越界仍拒（§20 回归，与素材收编无关）。
	v := 1.5
	if err := a.UpdateSeriesBGM(ctx, "s1", nil, &v); err == nil || !strings.Contains(err.Error(), "bgm_volume") {
		t.Fatalf("音量越界应报 bgm_volume 错误: %v", err)
	}
}
