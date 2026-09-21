package engine

import (
	"context"

	"github.com/sjzsdu/story/internal/domain"
)

// LegacyChainInput 旧流水线状态（PipelineState）中与产物有关的部分。
// 仅用于 §17 的一次性旧集迁移。
type LegacyChainInput struct {
	Story      *domain.StoryCandidate
	Storyboard *domain.Storyboard
	Clips      []domain.MediaResult
	Audios     []domain.MediaResult
	Outputs    []string
}

// DeriveLegacyChain 按旧流水线状态构造一条线性版本链 story → storyboard → media → final。
//
// 派生键按当前规则计算（含语音画像解析），因此迁移后的节点与「现在重新执行该步骤」
// 命中的是同一个节点——旧集迁移后继续跑下游不会重复出图/合成。
// 节点会被追加到 ep.Nodes；返回节点列表（链顺序）与最深已完成节点 ID。
//
// 缺上游的产物会被跳过（例如只有分镜没有故事，则分镜也不建——没有父节点可挂）。
func (e *Engine) DeriveLegacyChain(ctx context.Context, ep *domain.Episode, series *domain.Series, in LegacyChainInput) ([]*domain.VersionNode, string) {
	var nodes []*domain.VersionNode
	active := ""

	var storyNode *domain.VersionNode
	if in.Story != nil {
		params := storyParams{SeriesID: series.ID, Topic: ep.Topic, Dynasty: series.Config.Dynasty}
		storyNode = ensureNode(ep, domain.StageStory, nil, params, false)
		storyNode.Story = in.Story
		storyNode.Mark(domain.NodeDone, "")
		nodes = append(nodes, storyNode)
		active = storyNode.ID
	}

	var boardNode *domain.VersionNode
	if storyNode != nil && in.Storyboard != nil {
		visualRefs := mergeVisualRefs(series, ep.Refs)
		params := storyboardParams{
			StoryKey:   storyNode.ID,
			Dynasty:    firstNonEmpty(series.Config.Dynasty, in.Story.Dynasty),
			Ratio:      series.Config.Ratio,
			Resolution: series.Config.Resolution,
			VideoStyle: series.Config.VideoStyle,
			RefsDigest: refsDigest(visualRefs),
		}
		boardNode = ensureNode(ep, domain.StageStoryboard, storyNode, params, false)
		boardNode.Storyboard = in.Storyboard
		boardNode.Refs = ep.Refs
		boardNode.Mark(domain.NodeDone, "")
		nodes = append(nodes, boardNode)
		active = boardNode.ID
	}

	var mediaNode *domain.VersionNode
	if boardNode != nil && (len(in.Clips) > 0 || len(in.Audios) > 0) {
		mode := domain.NormalizeVisualMode(series.Config.VisualMode)
		voice, model, rate, pitch, instr := e.resolveVoice(ctx, series.Config, series.VoiceID, e.voice, e.instruction)
		params := mediaParams{
			StoryboardKey: boardNode.ID,
			VisualMode:    mode,
			VideoStyle:    series.Config.VideoStyle,
			Ratio:         series.Config.Ratio,
			Resolution:    series.Config.Resolution,
			VoiceID:       series.VoiceID,
			VoiceDigest:   voiceDigest(voice, model, rate, pitch, instr),
		}
		mediaNode = ensureNode(ep, domain.StageMedia, boardNode, params, false)
		mediaNode.Clips = in.Clips
		mediaNode.Audios = in.Audios
		mediaNode.Mark(domain.NodeDone, "")
		nodes = append(nodes, mediaNode)
		active = mediaNode.ID
	}

	if mediaNode != nil && len(in.Outputs) > 0 {
		params := finalParams{
			MediaKey:      mediaNode.ID,
			Ratio:         series.Config.Ratio,
			Resolution:    series.Config.Resolution,
			BurnSubtitles: true,
		}
		finalNode := ensureNode(ep, domain.StageFinal, mediaNode, params, false)
		finalNode.Outputs = in.Outputs
		finalNode.Mark(domain.NodeDone, "")
		nodes = append(nodes, finalNode)
		active = finalNode.ID
	}

	return nodes, active
}
