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

// GenerateEpisodeRef 为单条集级视觉参考生成参考图，落到 <episode>/refs/ 目录。
// 已存在且 force=false 时跳过（幂等）。
func (e *Engine) GenerateEpisodeRef(ctx context.Context, series *domain.Series, ep *domain.Episode, ref *domain.VisualRef, force bool) (string, error) {
	img := e.resolveImage(series.Config)
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
			ref.RefImage = outPath
			return outPath, nil
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
	if _, err := img.GenerateImage(ctx, port.ImageRequest{OutPath: outPath, Prompt: prompt, Size: size}); err != nil {
		kind := "人物"
		if ref.Kind == domain.RefKindScene {
			kind = "场景"
		}
		return "", fmt.Errorf("生成%s参考图 %s: %w", kind, ref.Name, err)
	}
	ref.RefImage = outPath
	return outPath, nil
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
