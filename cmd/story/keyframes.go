package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var keyframesForce bool

// keyframesCmd 为系列人物设定集批量生成视觉参考图（跨集复用的角色形象约束）。
var keyframesCmd = &cobra.Command{
	Use:   "keyframes <series-id>",
	Short: "生成系列视觉参考图（人物，跨集复用；用于 video 模式参考图）",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		se, err := application.GetSeries(rootCtx, args[0])
		if err != nil {
			return err
		}
		if len(se.Characters) == 0 {
			return fmt.Errorf("系列尚无人物设定，请先在分集策划中产出或用 Web UI 编辑")
		}
		fmt.Printf(">>> 正在为 %d 个系列人物生成视觉参考图（约每张 10-30 秒，已存在且未加 --force 的会跳过）...\n", len(se.Characters))
		characters, err := application.GenerateSeriesKeyframes(rootCtx, se.ID, keyframesForce)
		if err != nil {
			return err
		}
		fmt.Println("系列视觉参考图已就绪:")
		for _, ch := range characters {
			ref := ch.RefImage
			if ref == "" {
				ref = "（未生成）"
			}
			fmt.Printf("  %-8s %-16s %s\n", ch.Name, truncate(ch.Identity, 14), ref)
		}
		fmt.Println("\n人物设定已随 series 保存；video 模式 produce 时含人物的镜头将自动以参考图约束形象。")
		return nil
	},
}

func init() {
	keyframesCmd.Flags().BoolVar(&keyframesForce, "force", false, "已存在的参考图也重新生成")
	rootCmd.AddCommand(keyframesCmd)
}
