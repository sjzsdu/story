package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	episodeSeriesID string
	episodeTitle    string
	episodeTopic    string
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
		ep, err := application.CreateEpisode(rootCtx, episodeSeriesID, episodeTitle, episodeTopic)
		if err != nil {
			return err
		}
		fmt.Printf("集已创建: %s（第 %d 集）\n", ep.ID, ep.Number)
		fmt.Printf("  标题: %s\n", ep.Title)
		if ep.Topic != "" {
			fmt.Printf("  主题: %s\n", ep.Topic)
		}
		fmt.Printf("  目录: %s\n", ep.WorkDir)
		fmt.Printf("\n下一步: story candidates %s\n", ep.ID)
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
			fmt.Printf("%-18s  E%02d  %-28s  [%s/%s]\n",
				ep.ID, ep.Number, truncate(ep.Title, 26), ep.State.Current, ep.State.Steps[ep.State.Current].Status)
		}
		return nil
	},
}

func init() {
	episodeCreateCmd.Flags().StringVar(&episodeSeriesID, "series", "", "所属系列 ID（必填）")
	episodeCreateCmd.Flags().StringVar(&episodeTitle, "title", "", "本集标题（必填）")
	episodeCreateCmd.Flags().StringVar(&episodeTopic, "topic", "", "本集主题/切入点（可空，由 AI 自由命题）")

	episodeListCmd.Flags().StringVar(&episodeSeriesID, "series", "", "系列 ID（必填）")

	episodeCmd.AddCommand(episodeCreateCmd, episodeListCmd)
	rootCmd.AddCommand(episodeCmd)
}
