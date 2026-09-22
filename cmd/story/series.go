package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sjzsdu/story/internal/app"
	"github.com/sjzsdu/story/internal/domain"
	"github.com/sjzsdu/story/internal/templates"
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
	// 创作控制参数：--creative 可重复（key=value，天然插件化，新增参数不必加 flag）；
	// --preset / --story-instruction / --video-style 是常用项的语法糖。
	seriesCreative   []string
	seriesPreset     string
	seriesStoryInstr string
	seriesVideoStyle string
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
		knobs, err := creativeKnobs(cmd, false)
		if err != nil {
			return err
		}
		creative, videoStyle, err := app.ExpandCreative(seriesPreset, knobs)
		if err != nil {
			return err
		}
		se, err := application.CreateSeries(rootCtx, app.CreateSeriesInput{
			Name:           seriesName,
			Dynasty:        seriesDynasty,
			Description:    seriesDesc,
			Ratio:          seriesRatio,
			Resolution:     seriesResolution,
			VisualMode:     seriesVisualMode,
			VoiceID:        seriesVoiceID,
			VoiceProfile:   seriesVoiceProf,
			Voice:          seriesVoice,
			TTSInstruction: seriesInstruction,
			Concurrency:    seriesConcurrency,
			Retries:        seriesRetries,
			VideoStyle:     videoStyle,
			Creative:       creative,
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
		printCreative(se)
		return nil
	},
}

var seriesSetCmd = &cobra.Command{
	Use:   "set <series-id>",
	Short: "修改系列的创作设置（预设 / 参数 / 自定义指令；声音与画面模式创建后锁定）",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		knobs, err := creativeKnobs(cmd, true)
		if err != nil {
			return err
		}
		preset := seriesPreset
		if !cmd.Flags().Changed("preset") {
			preset = "" // 未显式指定预设＝不套用（补丁语义）
		}
		if len(knobs) == 0 && preset == "" {
			return fmt.Errorf("没有要修改的项：请用 --preset / --creative key=value / --story-instruction / --video-style")
		}
		se, err := application.UpdateSeriesCreative(rootCtx, args[0], preset, knobs)
		if err != nil {
			return err
		}
		printSeries(se)
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
		fmt.Printf("  %-18s %-6s %-24s %s\n", "ID", "序号", "标题", "进度")
		for _, ep := range eps {
			fmt.Printf("  %-18s %-6d %-24s %s\n", ep.ID, ep.Number, truncate(ep.Title, 22), episodeProgress(ep))
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
	printCreative(se)
}

// printCreative 打印系列的创作设置。参数名与可选值来自 templates 注册表，
// 这里只列出「已显式设置」的项（空值＝跟随内置默认，不必刷屏）。
func printCreative(se *domain.Series) {
	cfg := se.Config
	var lines []string
	for _, k := range templates.CreativeKnobs() {
		v := k.Get(cfg)
		if v == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s：%s", k.Label, knobValueLabel(k, v)))
	}
	if len(lines) == 0 {
		fmt.Println("  创作设置: 全部跟随内置默认")
		return
	}
	if cfg.Creative.Preset != "" {
		if p, ok := templates.FindPreset(cfg.Creative.Preset); ok {
			// 预设只是起点，参数还能逐项微调：两者不一致时如实说明。
			if presetMatches(p, cfg) {
				fmt.Printf("  创作设置（预设「%s」）:\n", p.Name)
			} else {
				fmt.Printf("  创作设置（预设「%s」基础上微调）:\n", p.Name)
			}
		}
	} else {
		fmt.Println("  创作设置:")
	}
	for _, l := range lines {
		fmt.Printf("    %s\n", l)
	}
}

// presetMatches 判断当前设置是否与预设完全一致（没有任何微调）。
// 默认预设的 values 为空，因此「全部未设置」也会判为一致。
func presetMatches(p templates.Preset, cfg domain.SeriesConfig) bool {
	for _, k := range templates.CreativeKnobs() {
		if k.Get(cfg) != p.Values[k.Key] {
			return false
		}
	}
	return true
}

// knobValueLabel 把参数的原始值转成中文名（枚举查选项；文本原样截断）。
func knobValueLabel(k templates.Knob, value string) string {
	if k.Type == templates.KnobText {
		return truncate(value, 40)
	}
	for _, o := range k.Options {
		if o.Key == value {
			return o.Label
		}
	}
	return value
}

// creativeKnobs 组装创作参数：--creative key=value（可重复）+ 常用项语法糖。
// changedOnly 为 true（series set 的补丁语义）时，只有出现在命令行上的语法糖才写入；
// 为 false（series create）时空值一律跳过，保证「未设置＝零值＝现状」。
func creativeKnobs(cmd *cobra.Command, changedOnly bool) (map[string]string, error) {
	knobs, err := parseCreativeFlags(seriesCreative)
	if err != nil {
		return nil, err
	}
	add := func(flag, key, value string) {
		if changedOnly {
			if !cmd.Flags().Changed(flag) {
				return
			}
		} else if strings.TrimSpace(value) == "" {
			return
		}
		knobs[key] = value
	}
	add("video-style", "video_style", seriesVideoStyle)
	add("story-instruction", "instruction", seriesStoryInstr)
	return knobs, nil
}

// parseCreativeFlags 解析可重复的 --creative key=value。
func parseCreativeFlags(pairs []string) (map[string]string, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(pairs))
	for _, p := range pairs {
		key, value, ok := strings.Cut(p, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return nil, fmt.Errorf("--creative 需写成 key=value，收到 %q", p)
		}
		out[key] = value
	}
	return out, nil
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
	registerCreativeFlags(seriesCreateCmd)

	registerCreativeFlags(seriesSetCmd)

	seriesCmd.AddCommand(seriesCreateCmd, seriesSetCmd, seriesListCmd, seriesShowCmd)
	rootCmd.AddCommand(seriesCmd)
}

// registerCreativeFlags 注册创作设置相关 flag（create 与 set 共用同一组变量）。
func registerCreativeFlags(cmd *cobra.Command) {
	cmd.Flags().StringArrayVar(&seriesCreative, "creative", nil,
		"创作参数，可重复，格式 key=value（如 --creative narrative=suspense）；可用 key 见 story series show 或 Web 端")
	cmd.Flags().StringVar(&seriesPreset, "preset", "", "创作预设 key：classic（默认）/ documentary / kids / suspense / teen / first_person / long_form")
	cmd.Flags().StringVar(&seriesStoryInstr, "story-instruction", "", "自定义创作指令（自由文本，上限 500 字；与硬性规则冲突时以硬性规则为准）")
	cmd.Flags().StringVar(&seriesVideoStyle, "video-style", "", "全片画风 key（选项由 templates 的风格包给出，如 gongbi/ink）")
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
