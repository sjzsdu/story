package main

import (
	"fmt"

	"github.com/sjzsdu/story/internal/domain"
	"github.com/spf13/cobra"
)

var (
	episodeSeriesID    string
	episodeTitle       string
	episodeTopic       string
	episodeInstruction string
)

var episodeCmd = &cobra.Command{
	Use:   "episode",
	Short: "管理系列下的集（每集一条完整流水线）",
}

var episodeCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "在系列下创建一集",
	RunE: func(cmd *cobra.Command, args []string) error {
		if episodeSeriesID == "" {
			return fmt.Errorf("--series 不能为空")
		}
		if episodeTitle == "" {
			return fmt.Errorf("--title 不能为空")
		}
		ep, err := application.CreateEpisode(rootCtx, episodeSeriesID, episodeTitle, episodeTopic, episodeInstruction)
		if err != nil {
			return err
		}
		fmt.Printf("集已创建: %s（第 %d 集）\n", ep.ID, ep.Number)
		fmt.Printf("  标题: %s\n", ep.Title)
		if ep.Topic != "" {
			fmt.Printf("  主题: %s\n", ep.Topic)
		}
		if ep.Instruction != "" {
			fmt.Printf("  本集附加指令: %s\n", ep.Instruction)
		}
		fmt.Printf("  目录: %s\n", ep.WorkDir)
		fmt.Printf("\n下一步: story generate %s\n", ep.ID)
		return nil
	},
}

var episodeListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出系列下的全部集",
	RunE: func(cmd *cobra.Command, args []string) error {
		if episodeSeriesID == "" {
			return fmt.Errorf("--series 不能为空")
		}
		eps, err := application.ListEpisodes(rootCtx, episodeSeriesID)
		if err != nil {
			return err
		}
		if len(eps) == 0 {
			fmt.Println("（该系列下暂无集）")
			return nil
		}
		for _, ep := range eps {
			fmt.Printf("%-18s  E%02d  %-28s  [%s]\n",
				ep.ID, ep.Number, truncate(ep.Title, 26), episodeProgress(ep))
			if ep.Instruction != "" {
				fmt.Printf("%-18s        附加指令: %s\n", "", truncate(ep.Instruction, 60))
			}
		}
		return nil
	},
}

// episodeProgress 用版本树里最深的一条链概括该集进度，如「成片 ● / 画面 ◐」。
func episodeProgress(ep *domain.Episode) string {
	if len(ep.Nodes) == 0 {
		return "未开始"
	}
	byStage := make(map[domain.Stage]*domain.VersionNode, len(ep.Nodes))
	for _, n := range ep.Nodes {
		cur, ok := byStage[n.Stage]
		if !ok || n.Attempt > cur.Attempt {
			byStage[n.Stage] = n
		}
	}
	latest := domain.StageStory
	for _, st := range domain.AllStages() {
		if _, ok := byStage[st]; ok {
			latest = st
		}
	}
	return fmt.Sprintf("%s %s", domain.StageLabel(latest), byStage[latest].Status)
}

func init() {
	episodeCreateCmd.Flags().StringVar(&episodeSeriesID, "series", "", "所属系列 ID（必填）")
	episodeCreateCmd.Flags().StringVar(&episodeTitle, "title", "", "本集标题（必填）")
	episodeCreateCmd.Flags().StringVar(&episodeTopic, "topic", "", "本集主题/切入点（可空，由 AI 自由命题）")
	episodeCreateCmd.Flags().StringVar(&episodeInstruction, "instruction", "", "本集附加创作指令（可空，叠加在系列创作设置之上）")

	episodeListCmd.Flags().StringVar(&episodeSeriesID, "series", "", "系列 ID（必填）")

	episodeCmd.AddCommand(episodeCreateCmd, episodeListCmd)
	rootCmd.AddCommand(episodeCmd)
}
