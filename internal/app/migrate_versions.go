package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sjzsdu/story/internal/domain"
	"github.com/sjzsdu/story/internal/engine"
	sqlitestore "github.com/sjzsdu/story/internal/store/sqlite"
)

// legacyPipelineState 旧版（§17 之前）持久化在 episodes.state_json 里的流水线状态。
// 只用于一次性迁移读取，不再写入。
type legacyPipelineState struct {
	Candidates []domain.StoryCandidate `json:"candidates,omitempty"`
	Selected   *int                    `json:"selected,omitempty"`
	Story      *domain.StoryCandidate  `json:"story,omitempty"`
	Storyboard *domain.Storyboard      `json:"storyboard,omitempty"`
	Clips      []domain.MediaResult    `json:"clips,omitempty"`
	Audios     []domain.MediaResult    `json:"audios,omitempty"`
	Outputs    []string                `json:"outputs,omitempty"`
}

// MigrateEpisodesToVersionTree 把旧布局的集迁移到版本树（幂等，§17）。
//
// 对每个尚未迁移的集：把旧 state_json 投影成一条线性链（story → storyboard →
// media → final），按当前派生规则算出节点键，然后把旧布局的产物目录搬进
// versions/<节点键>/。rename 失败（跨设备等）时该节点回退为「Dir 指向旧路径」，
// 原文件原地保留，不报错不丢数据。refs/ 永不搬动（集级共享）。
func MigrateEpisodesToVersionTree(ctx context.Context, a *App, store *sqlitestore.Store) error {
	ids, err := store.ListUnmigratedEpisodes(ctx)
	if err != nil {
		return fmt.Errorf("查询待迁移的集: %w", err)
	}
	for _, id := range ids {
		if err := migrateEpisode(ctx, a, store, id); err != nil {
			return fmt.Errorf("迁移集 %s: %w", id, err)
		}
	}
	return nil
}

func migrateEpisode(ctx context.Context, a *App, store *sqlitestore.Store, id string) error {
	raw, err := store.LegacyEpisodeState(ctx, id)
	if err != nil {
		return err
	}
	var legacy legacyPipelineState
	if len(raw) > 0 {
		if uerr := json.Unmarshal(raw, &legacy); uerr != nil {
			return fmt.Errorf("解析旧流水线状态: %w", uerr)
		}
	}
	// 旧版停在 generate 与 pick 之间的数据：故事内容取自被选中的候选。
	if legacy.Story == nil && legacy.Selected != nil && len(legacy.Candidates) > 0 {
		if i := *legacy.Selected; i >= 1 && i <= len(legacy.Candidates) {
			s := legacy.Candidates[i-1]
			legacy.Story = &s
		}
	}
	// 无任何产物的集（含尚未开工的新集）：无需搬文件，保持空树。
	if legacy.Story == nil && legacy.Storyboard == nil && len(legacy.Clips) == 0 &&
		len(legacy.Audios) == 0 && len(legacy.Outputs) == 0 {
		return nil
	}

	ep, err := a.GetEpisode(ctx, id)
	if err != nil {
		return err
	}
	series, err := a.GetSeries(ctx, ep.SeriesID)
	if err != nil {
		return err
	}

	nodes, active := a.Engine.DeriveLegacyChain(ctx, ep, series, engine.LegacyChainInput{
		Story:      legacy.Story,
		Storyboard: legacy.Storyboard,
		Clips:      legacy.Clips,
		Audios:     legacy.Audios,
		Outputs:    legacy.Outputs,
	})
	if len(nodes) == 0 {
		return nil
	}

	// 1) 先落库元数据（Dir 指向新版本目录）。
	ep.ActiveNodeID = active
	if err := a.Repo.SaveEpisode(ctx, ep); err != nil {
		return err
	}

	// 2) 再搬文件；rename 失败则回退该节点的 Dir 到旧位置（文件原地保留）。
	// 两种情况下都要把节点登记的产物路径改挂到最终 Dir 下（rename 后旧路径已失效），
	// 因此循环结束后必须再落一次库。
	for _, n := range nodes {
		if !moveNodeFiles(ep.WorkDir, n) {
			n.Dir = ep.WorkDir
		}
		rebaseNodePaths(n.Dir, n)
	}
	return a.Repo.SaveEpisode(ctx, ep)
}

// moveNodeFiles 把节点在旧布局中的产物搬进 node.Dir；全部成功返回 true。
func moveNodeFiles(workDir string, n *domain.VersionNode) bool {
	if err := os.MkdirAll(n.Dir, 0o755); err != nil {
		return false
	}
	for _, rel := range legacyPathsOf(n.Stage) {
		src := filepath.Join(workDir, rel)
		if _, err := os.Stat(src); err != nil {
			continue // 该产物本就不存在（如中途停在分镜）
		}
		if err := os.Rename(src, filepath.Join(n.Dir, rel)); err != nil {
			return false
		}
	}
	return true
}

// legacyPathsOf 返回某阶段在旧布局（集工作目录根下）的产物相对路径。
func legacyPathsOf(stage domain.Stage) []string {
	switch stage {
	case domain.StageStory:
		return []string{"story.md"}
	case domain.StageStoryboard:
		return []string{"storyboard.json"}
	case domain.StageMedia:
		return []string{"clips", "audio", engine.PanelsDirName}
	case domain.StageFinal:
		return []string{"tmp", "output"}
	default:
		return nil
	}
}

// rebaseNodePaths 把节点里登记的产物绝对路径改挂到 base 目录下（rename 失败时的回退）。
func rebaseNodePaths(base string, n *domain.VersionNode) {
	for i := range n.Clips {
		n.Clips[i].Path = rebasePath(base, "clips", n.Clips[i].Path)
	}
	for i := range n.Audios {
		n.Audios[i].Path = rebasePath(base, "audio", n.Audios[i].Path)
	}
	for i := range n.Outputs {
		n.Outputs[i] = rebasePath(base, "output", n.Outputs[i])
	}
}

func rebasePath(base, sub, old string) string {
	if strings.TrimSpace(old) == "" {
		return old
	}
	return filepath.Join(base, sub, filepath.Base(old))
}
