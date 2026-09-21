package engine

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sjzsdu/story/internal/domain"
)

// VersionsDirName 集工作目录下存放各版本产物的子目录名（§17 版本树）。
const VersionsDirName = "versions"

// derivationSchemaVersion 派生规则版本。
//
// 当 prompt 模板、参数语义或产物布局发生不向后兼容的变化时递增，使所有既有
// 派生键失效——旧产物目录因此不会再被复用，等价于"一次性安全失效"。
const derivationSchemaVersion = 1

// DeriveOptions 一次派生请求的选项。
type DeriveOptions struct {
	// From 起始父节点 ID；留空表示以 Episode.ActiveNodeID 为准。
	From string
	// Reroll 为 true 时强制开新版本（Attempt+1）；false 时复用同一组派生输入的
	// 既有节点——节点已完成则直接复用不再调用模型，未完成则续跑。
	Reroll bool
	// Scenes 仅 produce 使用：只生产指定镜头序号；留空表示只生产未完成的镜头。
	Scenes []int
	// Note 本版附加要求：「重做（换一版）」时用户填的迭代方向，只作用于这一版。
	// 必须进派生键——否则用户填了方向却命中同一派生键，会被静默复用、
	// 表现为「提示词不生效」。空串时不影响任何既有派生键。
	Note string
}

// ---- 各阶段的派生参数（参与派生键计算）----

type storyParams struct {
	SeriesID string `json:"series_id"`
	Topic    string `json:"topic"`
	Dynasty  string `json:"dynasty"`
	// Brief 系列/本集的创作要求文本（templates.StoryBrief）。
	// 必须 omitempty：默认（无任何创作设置）时为空串，才能保证存量系列的派生键不变。
	Brief string `json:"brief,omitempty"`
	// Note 本版附加要求（换一版时用户填的迭代方向）；同上，必须 omitempty。
	Note string `json:"note,omitempty"`
}

type storyboardParams struct {
	StoryKey   string `json:"story_key"`
	Dynasty    string `json:"dynasty"`
	Ratio      string `json:"ratio"`
	Resolution string `json:"resolution"`
	VideoStyle string `json:"video_style"`
	RefsDigest string `json:"refs_digest"`
	// Brief 分镜阶段的创作要求文本（templates.BoardBrief）；同上，必须 omitempty。
	Brief string `json:"brief,omitempty"`
	// Note 本版附加要求（换一版时用户填的迭代方向）；同上，必须 omitempty。
	Note string `json:"note,omitempty"`
}

type mediaParams struct {
	StoryboardKey string `json:"storyboard_key"`
	VisualMode    string `json:"visual_mode"`
	VideoStyle    string `json:"video_style"`
	Ratio         string `json:"ratio"`
	Resolution    string `json:"resolution"`
	VoiceID       string `json:"voice_id"`
	VoiceDigest   string `json:"voice_digest"`
	// Motion 运镜强度 key（小人书模式生效）；同上，必须 omitempty。
	Motion string `json:"motion,omitempty"`
	// Note 本版附加要求（换一版画面时用户填的迭代方向，追加到每镜画面描述）；同上。
	Note string `json:"note,omitempty"`
}

type finalParams struct {
	MediaKey      string `json:"media_key"`
	Ratio         string `json:"ratio"`
	Resolution    string `json:"resolution"`
	BurnSubtitles bool   `json:"burn_subtitles"`
}

// nodeKey 计算派生键：内容寻址，只哈希输入不哈希输出（bl 输出不确定）。
func nodeKey(stage domain.Stage, parentID string, params any, attempt int) string {
	b, err := json.Marshal(params)
	if err != nil {
		b = []byte(fmt.Sprintf("%v", params))
	}
	raw := fmt.Sprintf("%d|%s|%s|%s|%d", derivationSchemaVersion, stage, parentID, b, attempt)
	sum := sha1.Sum([]byte(raw))
	return fmt.Sprintf("%s-%s", stage, hex.EncodeToString(sum[:])[:12])
}

// nodeDir 返回节点产物目录：集工作目录下的 versions/<节点 ID>（换一版追加 -a<n>）。
func nodeDir(ep *domain.Episode, n *domain.VersionNode) string {
	name := n.ID
	if n.Attempt > 0 {
		name = fmt.Sprintf("%s-a%d", n.ID, n.Attempt)
	}
	return filepath.Join(ep.WorkDir, VersionsDirName, name)
}

// ensureNode 按派生键在树上取得（或新建）节点。
// 派生键相同即同一份产物：reroll 为 false 时命中既有节点，避免重复计费。
func ensureNode(ep *domain.Episode, stage domain.Stage, parent *domain.VersionNode, params any, reroll bool) *domain.VersionNode {
	parentID := ""
	if parent != nil {
		parentID = parent.ID
	}
	attempt := 0
	if reroll {
		attempt = ep.MaxAttempt(parentID, stage) + 1
	}
	id := nodeKey(stage, parentID, params, attempt)
	if n := ep.NodeByID(id); n != nil {
		return n
	}
	now := time.Now()
	n := &domain.VersionNode{
		ID:        id,
		Stage:     stage,
		ParentID:  parentID,
		Attempt:   attempt,
		Status:    domain.NodePending,
		CreatedAt: now,
		UpdatedAt: now,
	}
	n.Dir = nodeDir(ep, n)
	ep.AddNode(n)
	return n
}

// resolveParent 解析本次派生的父节点：want 为父节点应处的阶段。
//
// from 非空时严格按它取（阶段不符即报错）；为空时取活跃路径上对应阶段的节点。
func resolveParent(ep *domain.Episode, from string, want domain.Stage) (*domain.VersionNode, error) {
	if from != "" {
		n := ep.NodeByID(from)
		if n == nil {
			return nil, fmt.Errorf("版本节点 %s 不存在", from)
		}
		if n.Stage != want {
			return nil, fmt.Errorf("当前选中的是「%s」节点，本步骤需要「%s」节点",
				domain.StageLabel(n.Stage), domain.StageLabel(want))
		}
		return n, nil
	}
	n := ep.ActiveNodeOfStage(want)
	if n == nil {
		return nil, fmt.Errorf("尚无「%s」节点，请先执行该步骤", domain.StageLabel(want))
	}
	return n, nil
}

// stageIndex 返回阶段在派生顺序中的下标；未知阶段返回 -1。
func stageIndex(s domain.Stage) int {
	for i, st := range domain.AllStages() {
		if st == s {
			return i
		}
	}
	return -1
}

// refsDigest 把视觉参考压成稳定摘要（顺序无关），参与分镜派生键。
func refsDigest(refs []domain.VisualRef) string {
	lines := make([]string, 0, len(refs))
	for _, r := range refs {
		lines = append(lines, strings.Join([]string{string(r.Kind), r.Name, r.Description, r.RefImage}, "\x1f"))
	}
	sort.Strings(lines)
	return shortHash(strings.Join(lines, "\x1e"))
}

// voiceDigest 把解析出的语音参数压成摘要，参与画面阶段派生键：
// 换声音条目或改其参数后重新生产会得到新版本，而不是复用旧旁白。
func voiceDigest(voice, model string, rate, pitch float64, instruction string) string {
	return shortHash(fmt.Sprintf("%s|%s|%.3f|%.3f|%s", voice, model, rate, pitch, instruction))
}

func shortHash(s string) string {
	sum := sha1.Sum([]byte(s))
	return hex.EncodeToString(sum[:])[:12]
}
