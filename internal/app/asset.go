package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sjzsdu/story/internal/domain"
	"github.com/sjzsdu/story/internal/port"
)

// 素材/资产（§23 统一资源管理）：上传 / 导入 / 查询 / 删除。
// 与 Voice 同款顶层实体语义；本轮只做 bgm 曲库，image_ref 留给第二步。

// ErrAssetInUse 素材删除被拒（仍被系列引用）。
// 与 sqlite.ErrNotFound = port.ErrNotFound 同款别名惯例：屏蔽存储实现，
// server 用 errors.Is(err, app.ErrAssetInUse) 映射 409。
var ErrAssetInUse = port.ErrAssetInUse

// ErrAssetRefInvalid 配置里的 "asset:<id>" 引用指向不存在的素材（设值即拒，不落库）。
// server 用 errors.Is 映射 400（引用无效是客户端输入问题，不是服务端故障）。
var ErrAssetRefInvalid = errors.New("素材引用无效")

// maxAssetBytes 素材上传体积上限：50MB（BGM 曲目足够；超出视为异常请求）。
const maxAssetBytes = 50 << 20

// ListAssets 列出素材；kind 为空列出全部，非空先归一（未知 kind 报错不静默）。
func (a *App) ListAssets(ctx context.Context, kind string) ([]*domain.Asset, error) {
	n := ""
	if kind = strings.TrimSpace(kind); kind != "" {
		n = domain.NormalizeAssetKind(kind)
		if n == "" {
			return nil, fmt.Errorf("未知素材类型 %q（可用: %s）", kind, domain.AssetKindsLabel())
		}
	}
	return a.Repo.ListAssets(ctx, n)
}

// GetAsset 查询单个素材。
func (a *App) GetAsset(ctx context.Context, id string) (*domain.Asset, error) {
	as, err := a.Repo.GetAsset(ctx, id)
	return as, translateErr(err)
}

// UploadAsset 把一段上传内容落进素材库并建条目（Web 上传链路）。
// filename 只用于取扩展名（sanitizeAudioExt 先取 Base 再白名单，绝不参与拼路径）。
func (a *App) UploadAsset(ctx context.Context, r io.Reader, filename, kind, name, description string) (*domain.Asset, error) {
	dir, err := a.assetDir(kind)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("创建素材目录: %w", err)
	}
	stamp := time.Now().UnixNano()
	dst := filepath.Join(dir, fmt.Sprintf("%s-%d%s", kind, stamp, sanitizeAudioExt(filename)))
	f, err := os.Create(dst)
	if err != nil {
		return nil, fmt.Errorf("创建素材文件: %w", err)
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		_ = os.Remove(dst)
		return nil, fmt.Errorf("写入素材文件: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(dst)
		return nil, err
	}
	return a.newAssetFromLibraryFile(ctx, dst, kind, name, description, "upload")
}

// ImportAsset 把本机已有文件**复制**进素材库并建条目（CLI 导入链路）。
// 复制而非引用：素材库是唯一事实源，避免用户移动/删除原文件后引用失效。
func (a *App) ImportAsset(ctx context.Context, srcPath, kind, name, description string) (*domain.Asset, error) {
	dir, err := a.assetDir(kind)
	if err != nil {
		return nil, err
	}
	src := strings.TrimSpace(srcPath)
	if src == "" {
		return nil, fmt.Errorf("缺少源文件路径")
	}
	if !filepath.IsAbs(src) {
		if src, err = filepath.Abs(src); err != nil {
			return nil, err
		}
	}
	fi, err := os.Stat(src)
	if err != nil || fi.IsDir() {
		return nil, fmt.Errorf("导入源文件不存在: %s；请检查 --path 路径", srcPath)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("创建素材目录: %w", err)
	}
	stamp := time.Now().UnixNano()
	dst := filepath.Join(dir, fmt.Sprintf("%s-%d%s", kind, stamp, sanitizeAudioExt(src)))
	if err := copyFile(src, dst); err != nil {
		_ = os.Remove(dst)
		return nil, fmt.Errorf("复制素材进库: %w", err)
	}
	return a.newAssetFromLibraryFile(ctx, dst, kind, name, description, "import")
}

// newAssetFromLibraryFile 对已落库目录的文件做时长探测（容错）并建条目。
func (a *App) newAssetFromLibraryFile(ctx context.Context, dst, kind, name, description, origin string) (*domain.Asset, error) {
	abs, err := filepath.Abs(dst)
	if err != nil {
		return nil, err
	}
	// 时长探测可注入且失败容忍：单测不依赖本机 ffprobe，探测失败置 0 不报错。
	var dur float64
	if a.assetProber != nil {
		if d, err := a.assetProber(ctx, abs); err == nil {
			dur = d
		}
	}
	as := &domain.Asset{
		ID:          fmt.Sprintf("%s-%d", kind, time.Now().UnixNano()),
		Kind:        kind,
		Name:        strings.TrimSpace(name),
		Path:        abs,
		Description: strings.TrimSpace(description),
		DurationSec: dur,
		Origin:      origin,
		CreatedAt:   time.Now(),
	}
	if as.Name == "" {
		as.Name = strings.TrimSuffix(filepath.Base(abs), filepath.Ext(abs))
	}
	if err := a.Repo.CreateAsset(ctx, as); err != nil {
		_ = os.Remove(abs)
		return nil, err
	}
	return as, nil
}

// UpdateAsset 更新素材的名称/描述（补丁语义：传 nil 的项保持原值）。
func (a *App) UpdateAsset(ctx context.Context, id string, name, description *string) error {
	as, err := a.GetAsset(ctx, id)
	if err != nil {
		return err
	}
	if name != nil {
		if strings.TrimSpace(*name) == "" {
			return fmt.Errorf("素材名称不能为空")
		}
		as.Name = strings.TrimSpace(*name)
	}
	if description != nil {
		as.Description = strings.TrimSpace(*description)
	}
	return translateErr(a.Repo.UpdateAsset(ctx, as))
}

// DeleteAsset 删除素材：先校验文件在素材库目录内 → 再删库行（引用计数 >0 拒删）→ 最后删文件。
// 库外路径一律拒绝删除（绝不触碰素材库之外的文件）。
func (a *App) DeleteAsset(ctx context.Context, id string) error {
	as, err := a.GetAsset(ctx, id)
	if err != nil {
		return err
	}
	// 1) 路径必须落在 data/assets/<kind>/ 内（防脏数据把库外文件删掉）。
	inLib, err := a.assetPathInLibrary(as)
	if err != nil {
		return err
	}
	if !inLib {
		return fmt.Errorf("拒绝删除：素材 %s 的文件不在素材库目录内（%s），请人工确认后处理", id, as.Path)
	}
	// 2) 库行删除（内含引用计数：被系列引用时返回 ErrAssetInUse，文件保持原样）。
	if err := translateErr(a.Repo.DeleteAsset(ctx, id)); err != nil {
		return err
	}
	// 3) 引用校验已过，删文件（已不存在则忽略）。
	if err := os.Remove(as.Path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("素材行已删除，但文件清理失败: %w", err)
	}
	return nil
}

// assetDir 归一 kind 并返回素材库子目录（未知 kind 报错，防穿越）。
func (a *App) assetDir(kind string) (string, error) {
	n := domain.NormalizeAssetKind(kind)
	if n == "" {
		return "", fmt.Errorf("未知素材类型 %q（可用: %s）", kind, domain.AssetKindsLabel())
	}
	return a.Config().AssetDir(n)
}

// assetPathInLibrary 判断素材文件绝对路径是否落在其类型库目录内（越权式与 serveMedia 同款）。
func (a *App) assetPathInLibrary(as *domain.Asset) (bool, error) {
	base, err := a.Config().AssetDir(as.Kind)
	if err != nil {
		return false, err
	}
	baseAbs, err := filepath.Abs(base)
	if err != nil {
		return false, err
	}
	p := as.Path
	if !filepath.IsAbs(p) {
		if p, err = filepath.Abs(p); err != nil {
			return false, err
		}
	}
	rel, err := filepath.Rel(baseAbs, p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return false, nil
	}
	return true, nil
}

// validateAssetRef 配置值若为 "asset:<id>" 则校验素材存在（设置 BGM 时的快速校验）。
// 非引用形态（字面路径）原样放行，保持 §20 历史语义。
func (a *App) validateAssetRef(ctx context.Context, value string) error {
	if !domain.IsAssetRef(value) {
		return nil
	}
	id := domain.AssetIDFromRef(value)
	if _, err := a.Repo.GetAsset(ctx, id); err != nil {
		// 双 %w：既可 errors.Is 判 ErrAssetRefInvalid（server → 400），也保留底层查询错误链。
		return fmt.Errorf("%w: 素材 %s 不存在: %w；请先在素材库上传该曲目", ErrAssetRefInvalid, id, err)
	}
	return nil
}

// copyFile 复制文件（保留可读权限；素材只需 0644）。
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
