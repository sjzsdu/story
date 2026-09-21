package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sjzsdu/story/internal/app"
	"github.com/sjzsdu/story/internal/domain"
)

var (
	seriesName       string
	seriesDynasty    string
	seriesDesc       string
	seriesRatio      string
	seriesResolution string
	seriesVisualMode string
	// §16：推荐使用 --voice 直接指定声音条目 ID；旧 --voice-profile 兼容期保留。
	seriesVoiceID     string
	seriesVoiceProf   string
	seriesVoice       string
	seriesInstruction string
	seriesConcurrency int
	seriesRetries     int
	seriesPlatforms   []string
)

var seriesCmd = &cobra.Command{
	Use:   "series",
	Short: "管理内容系列（如「鬼谷子」系列）",
}

var seriesCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "创建一个系列",
	RunE: func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(seriesName) == "" {
			return fmt.Errorf("--name 不能为空")
		}
		se, err := application.CreateSeries(rootCtx, app.CreateSeriesInput{
			Name:            seriesName,
			Dynasty:         seriesDynasty,
			Description:     seriesDesc,
			Ratio:           seriesRatio,
			Resolution:      seriesResolution,
			VisualMode:      seriesVisualMode,
			VoiceID:         seriesVoiceID,
			VoiceProfile:    seriesVoiceProf,
			Voice:           seriesVoice,
			TTSInstruction:  seriesInstruction,
			Concurrency:     seriesConcurrency,
			Retries:         seriesRetries,
			TargetPlatforms: seriesPlatforms,
		})
		if err != nil {
			return err
		}
		fmt.Printf("系列已创建: %s\n", se.ID)
		fmt.Printf("  名称: %s\n", se.Name)
		fmt.Printf("  朝代: %s\n", se.Config.Dynasty)
		fmt.Printf("  画面: %s / %s / %s\n", se.Config.Ratio, se.Config.Resolution, visualModeLabel(se.Config.VisualMode))
		fmt.Printf("  声音: %s（创建后锁定，不可改）\n", se.VoiceID)
		fmt.Printf("  并发: %d  重试: %d\n", se.Config.MaxConcurrency, se.Config.MaxRetries)
		return nil
	},
}

var seriesListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出全部系列",
	RunE: func(cmd *cobra.Command, args []string) error {
		series, err := application.ListSeries(rootCtx)
		if err != nil {
			return err
		}
		if len(series) == 0 {
			fmt.Println("（暂无系列，使用 `story series create --name <名称>` 创建）")
			return nil
		}
		fmt.Printf("%-18s %-12s %-8s %s\n", "ID", "名称", "朝代", "集数")
		for _, se := range series {
			eps, _ := application.ListEpisodes(rootCtx, se.ID)
			fmt.Printf("%-18s %-12s %-8s %d\n", se.ID, se.Name, se.Dynasty, len(eps))
		}
		return nil
	},
}

var seriesShowCmd = &cobra.Command{
	Use:   "show <series-id>",
	Short: "查看系列详情与各集进度",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		se, err := application.GetSeries(rootCtx, args[0])
		if err != nil {
			return err
		}
		printSeries(se)
		eps, err := application.ListEpisodes(rootCtx, se.ID)
		if err != nil {
			return err
		}
		fmt.Println("\n集列表:")
		if len(eps) == 0 {
			fmt.Println("  （暂无集，使用 `story episode create --series " + se.ID + " --title <标题>` 创建）")
			return nil
		}
		fmt.Printf("  %-18s %-6s %-24s %-12s %s\n", "ID", "序号", "标题", "当前步骤", "状态")
		for _, ep := range eps {
			cur := ep.State.Current
			st := ep.State.Steps[cur].Status
			if ep.State.IsDone() {
				cur, st = domain.StepCompose, domain.StatusDone
			}
			fmt.Printf("  %-18s %-6d %-24s %-12s %s\n", ep.ID, ep.Number, truncate(ep.Title, 22), cur, st)
		}
		return nil
	},
}

func printSeries(se *domain.Series) {
	fmt.Printf("系列: %s（%s）\n", se.Name, se.ID)
	fmt.Printf("  朝代: %s\n", se.Config.Dynasty)
	if se.Description != "" {
		fmt.Printf("  简介: %s\n", se.Description)
	}
	fmt.Printf("  画面: %s / %s / %s\n", se.Config.Ratio, se.Config.Resolution, visualModeLabel(se.Config.VisualMode))
	fmt.Printf("  并发上限: %d  重试次数: %d\n", se.Config.MaxConcurrency, se.Config.MaxRetries)
	if se.VoiceID != "" {
		fmt.Printf("  声音: %s（锁定）\n", se.VoiceID)
	} else {
		// 兼容旧 series（迁移前）：显示旧字段
		fmt.Printf("  音色: %s（旧字段，重启后会迁移到 voice_id）\n", se.Config.TTSVoice)
	}
	if len(se.Config.TargetPlatforms) > 0 {
		fmt.Printf("  目标平台: %s\n", strings.Join(se.Config.TargetPlatforms, ", "))
	}
}

func init() {
	seriesCreateCmd.Flags().StringVar(&seriesName, "name", "", "系列名称（必填），如「鬼谷子」")
	seriesCreateCmd.Flags().StringVar(&seriesDynasty, "dynasty", "", "默认朝代，如「战国」")
	seriesCreateCmd.Flags().StringVar(&seriesDesc, "desc", "", "系列简介")
	seriesCreateCmd.Flags().StringVar(&seriesRatio, "ratio", "", "画面比例：9:16 / 16:9 / 1:1 / 3:4（默认 9:16）")
	seriesCreateCmd.Flags().StringVar(&seriesResolution, "resolution", "", "分辨率：720P / 1080P（默认 1080P）")
	seriesCreateCmd.Flags().StringVar(&seriesVisualMode, "visual-mode", "", "画面模式：comic 小人书插画+运镜（默认，省钱）/ video AI 视频")
	// §16：声音顶层实体化后，--voice 指定声音条目 ID（推荐）；--voice-profile/--voice-raw 兼容旧请求体。
	seriesCreateCmd.Flags().StringVar(&seriesVoiceID, "voice", "", "声音条目 ID（用 `story voice list` 查看可选值，推荐）。未指定时按平迁规则建/取一个")
	seriesCreateCmd.Flags().StringVar(&seriesVoiceProf, "voice-profile", "", "（旧）预设语音画像 key：wangliqun/kaishu/yizhongtian/shuoshu/cangsang/zhixing；--voice 未指定时按此建/取内置条目")
	seriesCreateCmd.Flags().StringVar(&seriesVoice, "voice-raw", "", "（旧）百炼 TTS 音色 ID，如 longtian_v3；--voice 与 --voice-profile 均未指定时按此建用户条目")
	seriesCreateCmd.Flags().StringVar(&seriesInstruction, "instruction", "", "（旧）TTS 风格指令，配合 --voice-raw 用")
	seriesCreateCmd.Flags().IntVar(&seriesConcurrency, "concurrency", 0, "单集最大并发镜头数（默认 3）")
	seriesCreateCmd.Flags().IntVar(&seriesRetries, "retries", 0, "失败重试次数（默认 3）")
	seriesCreateCmd.Flags().StringSliceVar(&seriesPlatforms, "platforms", nil, "目标平台，逗号分隔，如 douyin,kuaishou,bilibili")

	seriesCmd.AddCommand(seriesCreateCmd, seriesListCmd, seriesShowCmd)
	rootCmd.AddCommand(seriesCmd)
}

// visualModeLabel 展示用的画面模式中文名（空值按默认 comic 显示）。
func visualModeLabel(m string) string {
	if domain.NormalizeVisualMode(m) == domain.VisualModeVideo {
		return "AI 视频"
	}
	return "小人书插画"
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
