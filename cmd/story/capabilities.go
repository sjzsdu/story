package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// capabilitiesCmd 列出能力注册表（§21）：每项能力登记了哪些供应商实现、
// 当前系统默认是谁、对应的配置字段与系列覆盖字段。
// 与 `GET /api/capabilities`、Web 设置页同源（都读 app.CapabilityInfo）。
var capabilitiesCmd = &cobra.Command{
	Use:   "capabilities",
	Short: "列出全部能力槽的可用供应商与系统默认",
	Long: `列出能力注册表：每项能力（故事生成 / 分镜规划 / 系列策划 / 语音合成 /
图片生成 / 视频生成 / 图像理解 / 视频理解 / 音效生成 / 造声 / 音色库列举 /
平台发布）登记了哪些实现、系统默认是谁，以及对应的配置字段（story.yaml）
与系列覆盖字段。

两级选择链：系列配置里的 provider 覆盖优先，未指定时用系统默认。
修改系统默认：Web 设置页或 story.yaml 的 *_provider 字段；
系列覆盖：story series create/set 的 --*-provider flag。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if application == nil {
			return fmt.Errorf("应用未初始化")
		}
		info := application.CapabilityInfo()

		fmt.Printf("%-14s %-10s %-22s %s\n", "能力", "系统默认", "配置字段 / 系列字段", "可用实现")
		fmt.Println(strings.Repeat("─", 96))
		for _, c := range info.Capabilities {
			avail := make([]string, 0, len(c.Providers))
			for _, p := range c.Providers {
				avail = append(avail, p.Key)
			}
			def := c.Default
			if def == "" {
				def = "（未设置）"
			}
			fields := strings.TrimSpace(c.ConfigField + " / " + c.SeriesField)
			if fields == "/" || strings.TrimSpace(c.ConfigField) == "" {
				fields = "（无配置项，按能力指定）"
			}
			fmt.Printf("%-14s %-10s %-22s %s\n", c.Label, def, fields, strings.Join(avail, ", "))
			if c.Help != "" {
				fmt.Printf("  └ %s\n", c.Help)
			}
		}

		fmt.Println()
		fmt.Println("供应商/平台说明:")
		for _, p := range info.Providers {
			fmt.Printf("  %-14s %s\n", p.Label, p.Desc)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(capabilitiesCmd)
}
