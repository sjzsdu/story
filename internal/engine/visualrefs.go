package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sjzsdu/story/internal/domain"
	"github.com/sjzsdu/story/internal/port"
	"github.com/sjzsdu/story/internal/templates"
)

// EpisodeRefsDirName 集级视觉参考图目录名（位于 <episode WorkDir>/refs/）。
const EpisodeRefsDirName = "refs"

// mergeVisualRefs 合并系列级人物设定与集级视觉参考，供分镜校验与 produce 匹配。
// 系列人物先入列，集级参考（人物/场景）追加；同 kind+name 时集级覆盖系列级
// （单元剧可为本集重定义形象，连续剧沿用系列设定时集级不会重复输出同名项）。
func mergeVisualRefs(series *domain.Series, episodeRefs []domain.VisualRef) []domain.VisualRef {
	merged := make([]domain.VisualRef, 0, len(series.Characters)+len(episodeRefs))
	index := make(map[string]int, cap(merged))
	key := func(kind, name string) string { return kind + "\x00" + name }

	for _, ch := range series.Characters {
		if strings.TrimSpace(ch.Name) == "" {
			continue
		}
		idx := len(merged)
		merged = append(merged, domain.VisualRef{
			Kind:        domain.RefKindCharacter,
			Name:        ch.Name,
			Description: ch.Appearance,
			RefImage:    ch.RefImage,
		})
		index[key(domain.RefKindCharacter, ch.Name)] = idx
	}
	for _, r := range episodeRefs {
		r.Kind = domain.NormalizeRefKind(r.Kind)
		if strings.TrimSpace(r.Name) == "" {
			continue
		}
		if idx, ok := index[key(r.Kind, r.Name)]; ok {
			merged[idx] = r // 集级优先
			continue
		}
		index[key(r.Kind, r.Name)] = len(merged)
		merged = append(merged, r)
	}
	return merged
}

// visualRefLines 把本集视觉参考格式化为「[人物]/[场景] 名称：描述」行，重跑分镜时回灌。
func visualRefLines(refs []domain.VisualRef) []string {
	lines := make([]string, 0, len(refs))
	for _, r := range refs {
		name := strings.TrimSpace(r.Name)
		desc := strings.TrimSpace(r.Description)
		if name == "" || desc == "" {
			continue
		}
		kind := "人物"
		if domain.NormalizeRefKind(r.Kind) == domain.RefKindScene {
			kind = "场景"
		}
		lines = append(lines, fmt.Sprintf("[%s] %s：%s", kind, name, desc))
	}
	return lines
}

// preserveRefImages 分镜重跑时，按 kind+name 把旧参考图路径延续到新一轮 refs，
// 避免重新拆分分镜后已付费生成的参考图丢失关联。
func preserveRefImages(old, fresh []domain.VisualRef) []domain.VisualRef {
	if len(fresh) == 0 {
		return nil
	}
	oldImg := make(map[string]string, len(old))
	for _, r := range old {
		if r.RefImage != "" {
			oldImg[domain.NormalizeRefKind(r.Kind)+"\x00"+r.Name] = r.RefImage
		}
	}
	out := make([]domain.VisualRef, 0, len(fresh))
	for _, r := range fresh {
		r.Kind = domain.NormalizeRefKind(r.Kind)
		r.Name = strings.TrimSpace(r.Name)
		r.Description = strings.TrimSpace(r.Description)
		if r.Name == "" || r.Description == "" {
			continue
		}
		if r.RefImage == "" {
			r.RefImage = oldImg[r.Kind+"\x00"+r.Name]
		}
		out = append(out, r)
	}
	return out
}

// refImagesForScene 按画面描述中出现的参考名（人物姓名/场景名）匹配已有参考图。
func refImagesForScene(visualPrompt string, refs []domain.VisualRef) []string {
	var imgs []string
	seen := map[string]bool{}
	for _, r := range refs {
		if r.RefImage == "" || seen[r.RefImage] {
			continue
		}
		if strings.Contains(visualPrompt, r.Name) {
			imgs = append(imgs, r.RefImage)
			seen[r.RefImage] = true
		}
	}
	return imgs
}

// refPromptPrefix 生成 bl video ref 的提示词前缀，逐张声明参考图对应的人物/场景。
func refPromptPrefix(refs []domain.VisualRef, imgs []string) string {
	if len(imgs) == 0 {
		return ""
	}
	byRef := make(map[string]domain.VisualRef, len(refs))
	for _, r := range refs {
		if r.RefImage != "" {
			byRef[r.RefImage] = r
		}
	}
	var b strings.Builder
	b.WriteString("严格保持参考图中的视觉设定：")
	for i, img := range imgs {
		if r, ok := byRef[img]; ok {
			if r.Kind == domain.RefKindScene {
				fmt.Fprintf(&b, "Image %d 为场景「%s」的环境参考；", i+1, r.Name)
			} else {
				fmt.Fprintf(&b, "Image %d 为人物%s的形象参考；", i+1, r.Name)
			}
		}
	}
	b.WriteString("画面中对应人物的外貌服饰、对应场景的建筑形制与氛围必须与参考图一致。")
	return b.String()
}

// ---- §23 第二步：视觉参考收编为 image_ref 素材 ----

// assetsRoot 返回引擎侧素材库根目录 data/assets/（由 projectsDir=…/data/projects 推导，
// 与 config.AssetsDir 同源）。projectsDir 为空（未配置媒体根）时返回空串＝不登记。
func (e *Engine) assetsRoot() string {
	if e.projectsDir == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(e.projectsDir), "assets")
}

// adoptImageRefAsset 把刚生成的参考图收编进素材库并返回生效引用（§23 第二步，选型 A）：
//
//  1. prevRef 已是有效 "asset:<id>"（素材行为 image_ref）→ 覆盖写同一素材文件、
//     保持 id 稳定：防孤儿行，且 refsDigest 不因 force 重生而漂移；
//  2. 否则复制进 data/assets/image_ref/（ImportAsset 同款「复制而非引用」语义）并建条目，
//     返回 "asset:<id>"；
//  3. 任何登记失败都不阻断链路——删除副本、返回字面路径 outPath（引用仍可用，只是不入库）。
//
// outPath 原文件保留（与 ImportAsset 一致）：集/系列 refs 目录的字面路径引用
//（历史数据）继续有效，存量零迁移。
func (e *Engine) adoptImageRefAsset(ctx context.Context, outPath, prevRef, name, description string) string {
	literal := outPath
	// 未配置媒体根（e.g. 某些单测直构）：跳过登记，返回字面路径。
	root := e.assetsRoot()
	if root == "" {
		return literal
	}
	kind := domain.AssetKindImageRef

	// 路 1：复用既有素材行（force 重生 / 重复登记）。
	if domain.IsAssetRef(prevRef) {
		if as, err := e.repo.GetAsset(ctx, domain.AssetIDFromRef(prevRef)); err == nil &&
			as != nil && as.Kind == kind && as.Path != "" {
			if err := os.MkdirAll(filepath.Dir(as.Path), 0o755); err == nil {
				if err := copyFileContents(outPath, as.Path); err == nil {
					return prevRef
				}
			}
			// 覆盖失败不致命：继续走下面的新建路径（旧素材行仍在，由引用计数保护）。
		}
	}

	// 路 2：复制进素材库 + 登记条目。
	dir := filepath.Join(root, kind)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return literal
	}
	ext := refFileExt(outPath)
	dst := filepath.Join(dir, fmt.Sprintf("%s-%d%s", kind, time.Now().UnixNano(), ext))
	if err := copyFileContents(outPath, dst); err != nil {
		return literal
	}
	now := time.Now()
	as := &domain.Asset{
		ID:          fmt.Sprintf("%s-%d", kind, now.UnixNano()),
		Kind:        kind,
		Name:        strings.TrimSpace(name),
		Path:        dst,
		Description: strings.TrimSpace(description),
		Origin:      "generate",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if as.Name == "" {
		as.Name = strings.TrimSuffix(filepath.Base(dst), filepath.Ext(dst))
	}
	if err := e.repo.CreateAsset(ctx, as); err != nil {
		// 登记失败不阻断生成链路：清掉副本，回退字面路径引用。
		_ = os.Remove(dst)
		return literal
	}
	return domain.AssetRef(as.ID)
}

// refFileExt 从原路径取扩展名：先取 Base 再白名单（字母数字、长度 ≤6），
// 不合格一律 .png——绝不把不可信成分拼进素材库路径。
func refFileExt(p string) string {
	ext := strings.ToLower(filepath.Ext(filepath.Base(p)))
	core := strings.TrimPrefix(ext, ".")
	if core == "" || len(core) > 6 {
		return ".png"
	}
	for _, r := range core {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			return ".png"
		}
	}
	return ext
}

// copyFileContents 复制文件内容（素材只需 0644；dst 已存在于 force 覆盖场景）。
func copyFileContents(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o644)
}

// resolveRefImages 把匹配到的参考图引用解引用成可供模型消费的本地路径（§23 第二步）：
// "asset:<id>" → 查素材库取落盘绝对路径（不存在/文件丢失均报清晰中文错误，绝不静默跳过，
// 否则形象一致性会悄悄失效）；字面路径原样透传（不 stat，存量数据零迁移）。
// 只解析本次匹配到的项。
func (e *Engine) resolveRefImages(ctx context.Context, imgs []string) ([]string, error) {
	out := make([]string, 0, len(imgs))
	for _, img := range imgs {
		if !domain.IsAssetRef(img) {
			out = append(out, img)
			continue
		}
		id := domain.AssetIDFromRef(img)
		as, err := e.repo.GetAsset(ctx, id)
		if err != nil || as == nil {
			return nil, fmt.Errorf("视觉参考图素材引用悬空: %s（素材 %s 不存在）；请重新生成对应参考图，或在素材库补齐后重试", img, id)
		}
		if fi, err := os.Stat(as.Path); err != nil || fi.IsDir() {
			return nil, fmt.Errorf("视觉参考图素材文件缺失: %s（素材 %s，期望文件 %s）；请重新生成对应参考图", img, id, as.Path)
		}
		out = append(out, as.Path)
	}
	return out, nil
}

// replaceRefImages 返回 refs 的副本，把与 from[i] 相等的 RefImage 替换为 to[i]。
// 供 refPromptPrefix 按「解引用后的路径」反查条目——asset: 引用与字面路径都能命中，
// 参考图 prompt 前缀不受收编影响（纯函数，不改原切片）。
func replaceRefImages(refs []domain.VisualRef, from, to []string) []domain.VisualRef {
	if len(from) != len(to) || len(from) == 0 {
		return refs
	}
	m := make(map[string]string, len(from))
	for i := range from {
		m[from[i]] = to[i]
	}
	out := make([]domain.VisualRef, len(refs))
	copy(out, refs)
	for i := range out {
		if repl, ok := m[out[i].RefImage]; ok {
			out[i].RefImage = repl
		}
	}
	return out
}

// GenerateEpisodeRef 为单条集级视觉参考生成参考图，落到 <episode>/refs/ 目录。
// 已存在且 force=false 时跳过（幂等）。
func (e *Engine) GenerateEpisodeRef(ctx context.Context, series *domain.Series, ep *domain.Episode, ref *domain.VisualRef, force bool) (string, error) {
	img, err := e.resolveImage(series.Config)
	if err != nil {
		return "", err
	}
	if img == nil {
		return "", fmt.Errorf("未配置图片生成能力（ImageGenerator）")
	}
	refsDir := filepath.Join(ep.WorkDir, EpisodeRefsDirName)
	if err := os.MkdirAll(refsDir, 0o755); err != nil {
		return "", err
	}
	outPath := filepath.Join(refsDir, slugFileName(ref.Name)+".png")
	if !force {
		if fi, err := os.Stat(outPath); err == nil && fi.Size() > 0 {
			// 已生成过：仅当尚未登记时回填生效引用，保住已登记的 "asset:<id>"（同 GenerateKeyframe）。
			if strings.TrimSpace(ref.RefImage) == "" {
				ref.RefImage = e.adoptImageRefAsset(ctx, outPath, "", ref.Name, ref.Description)
			}
			return ref.RefImage, nil
		}
	}
	style := templates.MatchStyle(series.Config.VideoStyle)
	dynasty := firstNonEmpty(series.Config.Dynasty, series.Dynasty)
	var prompt, size string
	if ref.Kind == domain.RefKindScene {
		prompt = style.SceneRefPrompt(dynasty, ref.Name, ref.Description)
		// 场景图按成片比例出图（竖屏为主），与镜头构图一致。
		size = firstNonEmpty(series.Config.Ratio, "9:16")
	} else {
		prompt = style.KeyframePrompt(dynasty, ref.Name, "", ref.Description, "")
		size = "3:4"
	}
	// 覆盖前的旧引用：force 重新生成时凭它复用同一素材行（保持 id 稳定）。
	prevRef := strings.TrimSpace(ref.RefImage)
	if _, err := img.GenerateImage(ctx, port.ImageRequest{
		OutPath: outPath, Prompt: prompt, Size: size,
		Model: series.Config.ImageModel, // 系列级图片模型覆盖，空＝provider 系统默认
	}); err != nil {
		kind := "人物"
		if ref.Kind == domain.RefKindScene {
			kind = "场景"
		}
		return "", fmt.Errorf("生成%s参考图 %s: %w", kind, ref.Name, err)
	}
	// §23 第二步：生成成功后收编进素材库（登记失败不阻断，回退字面路径）。
	ref.RefImage = e.adoptImageRefAsset(ctx, outPath, prevRef, ref.Name, ref.Description)
	return ref.RefImage, nil
}

// GenerateEpisodeRefs 为集级视觉参考批量生成参考图（并发复用 runner）。
// 文本约束在分镜阶段已零成本生效；参考图按需手动触发，失败可重跑（幂等）。
func (e *Engine) GenerateEpisodeRefs(ctx context.Context, episodeID string, force bool) ([]domain.VisualRef, error) {
	ep, series, err := e.load(ctx, episodeID)
	if err != nil {
		return nil, err
	}
	if len(ep.Refs) == 0 {
		return nil, fmt.Errorf("本集尚无视觉参考，请先拆分分镜（人物/场景由分镜阶段产出）")
	}
	tasks := make([]Task, len(ep.Refs))
	for i := range ep.Refs {
		i, ref := i, &ep.Refs[i]
		tasks[i] = Task{
			Index: i,
			Name:  "ref-" + ref.Kind + "-" + ref.Name,
			Fn: func(ctx context.Context) error {
				_, err := e.GenerateEpisodeRef(ctx, series, ep, ref, force)
				return err
			},
		}
	}
	res := e.batchFor(series).RunBatch(ctx, tasks)
	if failed := CollectFailures(res); len(failed) > 0 {
		_ = e.repo.SaveEpisode(ctx, ep) // 先落盘已成功部分
		return nil, &FailedError{Items: failed}
	}
	ep.UpdatedAt = time.Now()
	if err := e.repo.SaveEpisode(ctx, ep); err != nil {
		return nil, err
	}
	return ep.Refs, nil
}
