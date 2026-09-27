package main

import (
	"fmt"
	"strings"

	"github.com/sjzsdu/story/internal/domain"
	"github.com/spf13/cobra"
)

var seriesPlanCmd = &cobra.Command{
	Use:   "plan <series-id>",
	Short: "AI 分集策划：查看会话 / 对话修订草案 / 采纳为集列表（真实调用文本模型）",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		if planReset {
			if err := application.ResetSeriesPlan(rootCtx, id); err != nil {
				return err
			}
			fmt.Println("策划会话已清空（已采纳创建的集不受影响）")
			return nil
		}

		ps, err := application.GetSeriesPlan(rootCtx, id)
		if err != nil {
			return err
		}

		// --message：追加一轮对话（可带 --system-extra 附加系统级规划要求）。
		if strings.TrimSpace(planMessage) != "" {
			ps, err = application.ChatSeriesPlan(rootCtx, id, planMessage, planSystemExtra)
			if err != nil {
				return err
			}
			if n := len(ps.Messages); n > 0 {
				fmt.Printf("总编：%s\n\n", ps.Messages[n-1].Content)
			}
		}

		printPlanDrafts(ps)

		// --apply：把当前草案采纳为集列表（不触发视频生产，同标题集自动跳过）。
		if planApply {
			eps, err := application.ApplyEpisodePlan(rootCtx, id, ps.Drafts, ps.Characters)
			if err != nil {
				return err
			}
			fmt.Printf("\n已采纳：创建 %d 集，保存 %d 个人物设定\n", len(eps), len(ps.Characters))
		}
		return nil
	},
}

// printPlanDrafts 打印当前会话的全量分集草案与人物设定摘要。
func printPlanDrafts(ps *domain.PlanSession) {
	if len(ps.Messages) == 0 {
		fmt.Println("（尚未开始策划，用 --message \"...\" 描述你对这一季分集的想法）")
		return
	}
	fmt.Printf("分集草案（%d 集）：\n", len(ps.Drafts))
	for i, d := range ps.Drafts {
		fmt.Printf("  %02d.《%s》%s\n", i+1, d.Title, d.Topic)
		if d.Summary != "" {
			fmt.Printf("      %s\n", truncate(d.Summary, 80))
		}
	}
	if len(ps.Characters) > 0 {
		fmt.Printf("人物设定（%d 人）：", len(ps.Characters))
		names := make([]string, 0, len(ps.Characters))
		for _, c := range ps.Characters {
			names = append(names, c.Name)
		}
		fmt.Println(strings.Join(names, "、"))
	}
}

func init() {
	seriesPlanCmd.Flags().StringVar(&planMessage, "message", "", "本轮对话内容（如「规划 40 集，重点覆盖合纵连横全程」；留空＝只查看当前草案）")
	seriesPlanCmd.Flags().StringVar(&planSystemExtra, "system-extra", "", "本轮附加的系统级规划要求（如取材典籍优先级、叙事框架；留空＝只用默认）")
	seriesPlanCmd.Flags().BoolVar(&planApply, "apply", false, "对话后把当前草案采纳为集列表（不触发视频生产）")
	seriesPlanCmd.Flags().BoolVar(&planReset, "reset", false, "清空当前策划会话重新开始（已创建的集不受影响）")
	seriesCmd.AddCommand(seriesPlanCmd)
}
