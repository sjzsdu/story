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

// RefsDirName 系列级定妆照目录名（位于 data/projects/<series>/refs/）。
const RefsDirName = "refs"

// GenerateKeyframe 为单个角色生成定妆照（水墨工笔立绘），落到系列 refs/ 目录。
// 已存在且 force=false 时跳过（幂等，支持断点）。
func (e *Engine) GenerateKeyframe(ctx context.Context, series *domain.Series, ch *domain.CharacterSetting, force bool) (string, error) {
	if e.images == nil {
		return "", fmt.Errorf("未配置图片生成能力（ImageGenerator）")
	}
	refsDir := filepath.Join(e.projectsDir, series.ID, RefsDirName)
	if err := os.MkdirAll(refsDir, 0o755); err != nil {
		return "", err
	}
	outPath := filepath.Join(refsDir, slugFileName(ch.Name)+".png")
	if !force {
		if fi, err := os.Stat(outPath); err == nil && fi.Size() > 0 {
			ch.RefImage = outPath
			return outPath, nil
		}
	}
	prompt := templates.KeyframeImagePrompt(
		firstNonEmpty(series.Config.Dynasty, series.Dynasty),
		ch.Name, ch.Identity, ch.Appearance, ch.Temperament,
	)
	req := port.ImageRequest{OutPath: outPath, Prompt: prompt, Size: "3:4"}
	if _, err := e.images.GenerateImage(ctx, req); err != nil {
		return "", fmt.Errorf("生成 %s 定妆照: %w", ch.Name, err)
	}
	ch.RefImage = outPath
	return outPath, nil
}

// GenerateSeriesKeyframes 为系列人物设定集批量生成定妆照（并发复用 runner）。
// 返回更新后的设定集（含 RefImage 路径）并写回系列。
func (e *Engine) GenerateSeriesKeyframes(ctx context.Context, seriesID string, force bool) ([]domain.CharacterSetting, error) {
	series, err := e.repo.GetSeries(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	if len(series.Characters) == 0 {
		return nil, fmt.Errorf("系列尚无人物设定，请先在分集策划中产出或手动添加")
	}
	tasks := make([]Task, len(series.Characters))
	for i := range series.Characters {
		i, ch := i, &series.Characters[i]
		tasks[i] = Task{
			Index: i,
			Name:  "keyframe-" + ch.Name,
			Fn: func(ctx context.Context) error {
				_, err := e.GenerateKeyframe(ctx, series, ch, force)
				return err
			},
		}
	}
	res := e.runner.RunBatch(ctx, tasks)
	if failed := CollectFailures(res); len(failed) > 0 {
		return nil, &FailedError{Items: failed}
	}
	series.UpdatedAt = time.Now()
	if err := e.repo.UpdateSeries(ctx, series); err != nil {
		return nil, err
	}
	return series.Characters, nil
}

// refImagesForScene 按 visual_prompt 中出现的人名匹配定妆照（要求分镜 prompt 用正史人名指代）。
func refImagesForScene(visualPrompt string, characters []domain.CharacterSetting) []string {
	if len(characters) == 0 {
		return nil
	}
	var imgs []string
	for _, ch := range characters {
		if ch.RefImage == "" {
			continue
		}
		if strings.Contains(visualPrompt, ch.Name) {
			imgs = append(imgs, ch.RefImage)
		}
	}
	return imgs
}

// refPromptPrefix 生成 bl video ref 的提示词前缀，声明每张参考图对应的人物。
func refPromptPrefix(characters []domain.CharacterSetting, imgs []string) string {
	if len(imgs) == 0 {
		return ""
	}
	byRef := make(map[string]string, len(characters))
	for _, ch := range characters {
		if ch.RefImage != "" {
			byRef[ch.RefImage] = ch.Name
		}
	}
	var b strings.Builder
	b.WriteString("严格保持参考图中人物的形象与服饰：")
	for i, img := range imgs {
		if name, ok := byRef[img]; ok {
			fmt.Fprintf(&b, "Image %d 为%s的定妆照；", i+1, name)
		}
	}
	b.WriteString("画面中人物的外貌、发型、服装必须与对应参考图一致。")
	return b.String()
}

// characterLines 把人物设定集格式化为「姓名（身份）：外貌；气质：…」行，供分镜 prompt 注入。
func characterLines(characters []domain.CharacterSetting) []string {
	if len(characters) == 0 {
		return nil
	}
	lines := make([]string, 0, len(characters))
	for _, ch := range characters {
		var b strings.Builder
		b.WriteString(ch.Name)
		if ch.Identity != "" {
			b.WriteString("（" + ch.Identity + "）")
		}
		b.WriteString("：" + ch.Appearance)
		if ch.Temperament != "" {
			b.WriteString("；气质：" + ch.Temperament)
		}
		lines = append(lines, b.String())
	}
	return lines
}

func slugFileName(name string) string {
	replacer := strings.NewReplacer(" ", "", "/", "-", "\\", "-", ":", "-", "*", "-", "?", "-", "\"", "-", "<", "-", ">", "-", "|", "-")
	return replacer.Replace(strings.TrimSpace(name))
}
