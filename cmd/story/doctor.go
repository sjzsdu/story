package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/sjzsdu/story/internal/doctor"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "检查所有 CLI 依赖的健康状态",
	Long: `逐项检查 story 流水线所需的全部外部依赖：
  - bl（百炼 CLI）
  - ffmpeg / ffprobe
  - python3（≥ 3.10）
  - pip / uv
  - sau（social-auto-upload）
  - Chrome 浏览器

每项显示状态（✅/❌/⚠️）、版本号和修复建议。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if application == nil {
			return fmt.Errorf("应用未初始化")
		}
		cfg := application.Cfg

		d := doctor.New(cfg.BLBin, cfg.FFMPEGBin, cfg.PythonBin, cfg.SAUBin)
		results := d.RunAll(cmd.Context())

		fmt.Println("story 依赖健康检查")
		fmt.Println("═══════════════════════════════════════")
		for _, r := range results {
			status := r.Status.String()
			version := r.Version
			if version == "" {
				version = "-"
			}
			fmt.Printf("%s %-28s  %s\n", status, r.Name, version)
			if r.Message != "" {
				fmt.Printf("   %s\n", r.Message)
			}
			if r.Status != doctor.StatusOK && r.Fix != "" {
				fmt.Printf("   修复: %s\n", r.Fix)
			}
		}

		ok, warn, fail := doctor.Summary(results)
		fmt.Println("═══════════════════════════════════════")
		fmt.Printf("共 %d 项: ✅ %d 正常  ⚠️ %d 警告  ❌ %d 缺失\n",
			len(results), ok, warn, fail)

		if fail > 0 {
			fmt.Println("\n💡 缺失的依赖会影响对应功能，请按修复建议安装后重试")
		} else if warn > 0 {
			fmt.Println("\n💡 部分依赖版本偏低，功能可用但可能不稳定")
		} else {
			fmt.Println("\n🎉 所有依赖就绪！")
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}
