package main

import (
	"fmt"

	"github.com/sjzsdu/story/internal/domain"
	"github.com/spf13/cobra"
)

var episodeRefsForce bool

// episodeRefsCmd 为单集视觉参考（本集人物 + 跨镜重复场景）批量生成参考图。
var episodeRefsCmd = &cobra.Command{
	Use:   "episode-refs <episode-id>",
	Short: "生成本集视觉参考图（人物/场景，按集独立；用于 video 模式参考图）",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ep, err := application.GetEpisode(rootCtx, args[0])
		if err != nil {
			return err
		}
		if len(ep.Refs) == 0 {
			return fmt.Errorf("本集尚无视觉参考，请先执行 storyboard（人物/场景由分镜阶段产出）")
		}
		fmt.Printf(">>> 正在为本集 %d 条视觉参考生成参考图（已存在且未加 --force 的会跳过）...\n", len(ep.Refs))
		refs, err := application.GenerateEpisodeRefs(rootCtx, ep.ID, episodeRefsForce)
		if err != nil {
			return err
		}
		fmt.Println("本集视觉参考图已就绪:")
		for _, r := range refs {
			kind := "人物"
			if r.Kind == domain.RefKindScene {
				kind = "场景"
			}
			ref := r.RefImage
			if ref == "" {
				ref = "（未生成）"
			}
			fmt.Printf("  [%s] %-10s %s\n", kind, r.Name, ref)
		}
		fmt.Println("\n视觉参考已随集保存；video 模式 produce 时匹配到人物/场景名的镜头将自动引用参考图。")
		return nil
	},
}

func init() {
	episodeRefsCmd.Flags().BoolVar(&episodeRefsForce, "force", false, "已存在的参考图也重新生成")
	rootCmd.AddCommand(episodeRefsCmd)
}
