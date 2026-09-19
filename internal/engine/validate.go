package engine

import (
	"fmt"
	"strings"

	"github.com/sjzsdu/story/internal/domain"
)

// 验收阈值（与 AGENTS.md 中的验收标准一致）。
const (
	minCandidates = 3
	minScenes     = 4
	minSceneDur   = 3
	// maxSceneDur 与分镜 prompt 的 duration 上限一致（4-10 秒，wan3.0-video 单片段档位）。
	maxSceneDur = 10
)

// ValidateCandidates 验收 generate 步骤产物。
func ValidateCandidates(cs []domain.StoryCandidate) error {
	if len(cs) < minCandidates {
		return fmt.Errorf("候选故事数量 %d 不足，至少 %d 个", len(cs), minCandidates)
	}
	for i, c := range cs {
		if strings.TrimSpace(c.Title) == "" {
			return fmt.Errorf("候选 #%d 缺少标题", i+1)
		}
		if strings.TrimSpace(c.Source) == "" {
			return fmt.Errorf("候选《%s》缺少典籍出处", c.Title)
		}
		if strings.TrimSpace(c.Content) == "" {
			return fmt.Errorf("候选《%s》正文为空", c.Title)
		}
	}
	return nil
}

// ValidateSelection 验收 pick 步骤。
func ValidateSelection(cs []domain.StoryCandidate, index int) error {
	if index < 1 || index > len(cs) {
		return fmt.Errorf("选择序号 %d 越界（共 %d 个候选）", index, len(cs))
	}
	if strings.TrimSpace(cs[index-1].Content) == "" {
		return fmt.Errorf("选中的候选 #%d 正文为空", index)
	}
	return nil
}

// ValidateStoryboard 验收 storyboard 步骤产物。
func ValidateStoryboard(sb *domain.Storyboard) error {
	if sb == nil {
		return fmt.Errorf("分镜为空")
	}
	if len(sb.Scenes) < minScenes {
		return fmt.Errorf("镜头数量 %d 不足，至少 %d 个", len(sb.Scenes), minScenes)
	}
	for i, sc := range sb.Scenes {
		if strings.TrimSpace(sc.VisualPrompt) == "" {
			return fmt.Errorf("镜头 #%d 缺少 visual_prompt", i+1)
		}
		if strings.TrimSpace(sc.Narration) == "" {
			return fmt.Errorf("镜头 #%d 缺少 narration", i+1)
		}
		if sc.DurationSec < minSceneDur || sc.DurationSec > maxSceneDur {
			return fmt.Errorf("镜头 #%d 时长 %d 非法（需在 %d-%d 秒）", i+1, sc.DurationSec, minSceneDur, maxSceneDur)
		}
	}
	return nil
}

// ValidateMedia 验收 produce 步骤产物登记信息。
func ValidateMedia(clips, audios []domain.MediaResult, sceneCount int) error {
	if len(clips) != sceneCount {
		return fmt.Errorf("视频片段数量 %d 与镜头数 %d 不符", len(clips), sceneCount)
	}
	if len(audios) != sceneCount {
		return fmt.Errorf("旁白音频数量 %d 与镜头数 %d 不符", len(audios), sceneCount)
	}
	for _, m := range clips {
		if m.Err != "" {
			return fmt.Errorf("镜头 #%d 视频生产失败: %s", m.SceneID, m.Err)
		}
		if strings.TrimSpace(m.Path) == "" {
			return fmt.Errorf("镜头 #%d 视频路径为空", m.SceneID)
		}
		if m.DurationSec <= 0 {
			return fmt.Errorf("镜头 #%d 视频时长异常: %.2f", m.SceneID, m.DurationSec)
		}
	}
	for _, m := range audios {
		if m.Err != "" {
			return fmt.Errorf("镜头 #%d 旁白生产失败: %s", m.SceneID, m.Err)
		}
		if strings.TrimSpace(m.Path) == "" {
			return fmt.Errorf("镜头 #%d 音频路径为空", m.SceneID)
		}
		if m.DurationSec <= 0 {
			return fmt.Errorf("镜头 #%d 音频时长异常: %.2f", m.SceneID, m.DurationSec)
		}
	}
	return nil
}
