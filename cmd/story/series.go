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
	// --video-style 是常用项的语法糖。
	seriesCreative   []string
	seriesVideoStyle string
	// 成片 BGM（§20 + §23）：--bgm 路径（相对系列目录），--bgm-asset 曲库素材 ID，
	// 两者互斥；--bgm-volume 音量（0＝未设置）。
	seriesBGM       string
	seriesBGMAsset  string
	seriesBGMVolume float64
	// 系列级 Provider 覆盖（§21）：空＝跟随系统默认；四个能力各自独立。
	seriesTextProvider  string
	seriesTTSProvider   string
	seriesImageProvider string
	seriesVideoProvider string
	// 系列级模型覆盖（第二步统一资源管理）：空＝跟随系统默认。
	seriesTextModel  string
	seriesTTSModel   string
	seriesImageModel string
	seriesVideoModel string
	// 系列级「规划要求」：分集策划时自动注入首轮上下文（--planning-brief）。
	seriesPlanningBrief string
	// series plan 子命令参数。
	planMessage     string
	planSystemExtra string
	planApply       bool
	planReset       bool
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
		creative, videoStyle, err := app.ExpandCreative(knobs)
		if err != nil {
			return err
		}
		// 成片 BGM：--bgm / --bgm-asset 互斥（§23），asset 引用在此转换与校验。
		bgmPath, _, err := currentBGMPathValue(cmd)
		if err != nil {
			return err
		}
		bgm := ""
		if bgmPath != nil {
			bgm = *bgmPath
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
			BGMPath:        bgm,
			BGMVolume:      seriesBGMVolume,
			// 系列级 Provider 覆盖（§21）：空＝跟随系统默认。
			TextProvider:  strings.TrimSpace(seriesTextProvider),
			TTSProvider:   strings.TrimSpace(seriesTTSProvider),
			ImageProvider: strings.TrimSpace(seriesImageProvider),
			VideoProvider: strings.TrimSpace(seriesVideoProvider),
			// 系列级模型覆盖：空＝跟随系统默认（绝不回填系统默认模型值）。
			TextModel:  strings.TrimSpace(seriesTextModel),
			TTSModel:   strings.TrimSpace(seriesTTSModel),
			ImageModel: strings.TrimSpace(seriesImageModel),
			VideoModel: strings.TrimSpace(seriesVideoModel),
			// 分集策划的长期规划要求：每次策划自动注入首轮上下文。
			PlanningBrief: seriesPlanningBrief,
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
		printProviders(se)
		printModels(se)
		printCreative(se)
		printBGMAssetInCreate(se)
		return nil
	},
}

var seriesSetCmd = &cobra.Command{
	Use:   "set <series-id>",
	Short: "修改系列的创作设置（画面参数；声音与画面模式创建后锁定）",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		knobs, err := creativeKnobs(cmd, true)
		if err != nil {
			return err
		}
		// 成片 BGM（§20 + §23）：只有显式出现在命令行上的 flag 才提交（补丁语义）。
		bgmPath, bgmChanged, err := currentBGMPathValue(cmd)
		if err != nil {
			return err
		}
		volChanged := cmd.Flags().Changed("bgm-volume")
		// Provider 覆盖（§21）：同样按 Changed 区分「未提供＝保持原值」与
		// 「显式空串＝清除该项、回到系统默认」。
		providerChanged := map[string]bool{
			"text-provider":  cmd.Flags().Changed("text-provider"),
			"tts-provider":   cmd.Flags().Changed("tts-provider"),
			"image-provider": cmd.Flags().Changed("image-provider"),
			"video-provider": cmd.Flags().Changed("video-provider"),
			// 模型覆盖（第二步）：与 Provider 覆盖同一套 Changed 补丁语义。
			"text-model":  cmd.Flags().Changed("text-model"),
			"tts-model":   cmd.Flags().Changed("tts-model"),
			"image-model": cmd.Flags().Changed("image-model"),
			"video-model": cmd.Flags().Changed("video-model"),
		}
		anyProvider := providerChanged["text-provider"] || providerChanged["tts-provider"] ||
			providerChanged["image-provider"] || providerChanged["video-provider"] ||
			providerChanged["text-model"] || providerChanged["tts-model"] ||
			providerChanged["image-model"] || providerChanged["video-model"]
		briefChanged := cmd.Flags().Changed("planning-brief")
		if len(knobs) == 0 && !bgmChanged && !volChanged && !anyProvider && !briefChanged {
			return fmt.Errorf("没有要修改的项：请用 --creative key=value / --video-style / --bgm 或 --bgm-asset / --bgm-volume / --planning-brief / --text-provider / --tts-provider / --image-provider / --video-provider / --text-model / --tts-model / --image-model / --video-model")
		}
		if len(knobs) > 0 {
			if _, err := application.UpdateSeriesCreative(rootCtx, args[0], knobs); err != nil {
				return err
			}
		}
		if anyProvider || briefChanged {
			se, err := application.GetSeries(rootCtx, args[0])
			if err != nil {
				return err
			}
			if providerChanged["text-provider"] {
				se.Config.TextProvider = strings.TrimSpace(seriesTextProvider)
			}
			if providerChanged["tts-provider"] {
				se.Config.TTSProvider = strings.TrimSpace(seriesTTSProvider)
			}
			if providerChanged["image-provider"] {
				se.Config.ImageProvider = strings.TrimSpace(seriesImageProvider)
			}
			if providerChanged["video-provider"] {
				se.Config.VideoProvider = strings.TrimSpace(seriesVideoProvider)
			}
			if providerChanged["text-model"] {
				se.Config.TextModel = strings.TrimSpace(seriesTextModel)
			}
			if providerChanged["tts-model"] {
				se.Config.TTSModel = strings.TrimSpace(seriesTTSModel)
			}
			if providerChanged["image-model"] {
				se.Config.ImageModel = strings.TrimSpace(seriesImageModel)
			}
			if providerChanged["video-model"] {
				se.Config.VideoModel = strings.TrimSpace(seriesVideoModel)
			}
			// 规划要求：显式空串＝清除。
			if briefChanged {
				se.Config.PlanningBrief = strings.TrimSpace(seriesPlanningBrief)
			}
			// 与 server updateSeries 同一条链路（补丁后整条写回）。
			if err := application.UpdateSeries(rootCtx, se); err != nil {
				return err
			}
		}
		if bgmChanged || volChanged {
			var path *string
			var volume *float64
			if bgmChanged {
				path = bgmPath
			}
			if volChanged {
				volume = &seriesBGMVolume
			}
			if err := application.UpdateSeriesBGM(rootCtx, args[0], path, volume); err != nil {
				return err
			}
		}
		se, err := application.GetSeries(rootCtx, args[0])
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
	if se.Config.BGMPath != "" {
		// §23：asset:<id> 引用显示曲库素材名，字面路径原样显示。
		bgmLabel := se.Config.BGMPath
		if domain.IsAssetRef(se.Config.BGMPath) {
			aid := domain.AssetIDFromRef(se.Config.BGMPath)
			bgmLabel = "曲库素材 " + aid
			if a, err := application.GetAsset(rootCtx, aid); err == nil {
				bgmLabel = fmt.Sprintf("曲库「%s」（%s）", a.Name, a.ID)
			}
		}
		if se.Config.BGMVolume > 0 {
			fmt.Printf("  BGM: %s（音量 %.2f）\n", bgmLabel, se.Config.BGMVolume)
		} else {
			fmt.Printf("  BGM: %s（音量 默认 0.18）\n", bgmLabel)
		}
	} else {
		fmt.Println("  BGM: 未设置")
	}
	if se.VoiceID != "" {
		fmt.Printf("  声音: %s（锁定）\n", se.VoiceID)
	} else {
		// 兼容旧 series（迁移前）：显示旧字段
		fmt.Printf("  音色: %s（旧字段，重启后会迁移到 voice_id）\n", se.Config.TTSVoice)
	}
	if se.Config.PlanningBrief != "" {
		fmt.Printf("  规划要求: %s\n", truncate(se.Config.PlanningBrief, 60))
	}
	printProviders(se)
	printModels(se)
	printCreative(se)
}

// providerLabel 渲染某项 Provider 覆盖：空＝跟随系统默认。
func providerLabel(v string) string {
	if strings.TrimSpace(v) == "" {
		return "跟随系统默认"
	}
	return v
}

// printProviders 打印系列的四项 Provider 覆盖（§21 两级选择链的第一级）。
func printProviders(se *domain.Series) {
	fmt.Printf("  Provider 覆盖: 故事/分镜/策划=%s  语音=%s  图片=%s  视频=%s\n",
		providerLabel(se.Config.TextProvider),
		providerLabel(se.Config.TTSProvider),
		providerLabel(se.Config.ImageProvider),
		providerLabel(se.Config.VideoProvider),
	)
}

// printModels 打印系列的四项模型覆盖（第二步统一资源管理）：
// 空＝跟随系统默认（复用 providerLabel 的展示语义）。
func printModels(se *domain.Series) {
	fmt.Printf("  模型覆盖: 文本=%s  语音=%s  图片=%s  视频=%s\n",
		providerLabel(se.Config.TextModel),
		providerLabel(se.Config.TTSModel),
		providerLabel(se.Config.ImageModel),
		providerLabel(se.Config.VideoModel),
	)
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
	fmt.Println("  创作设置:")
	for _, l := range lines {
		fmt.Printf("    %s\n", l)
	}
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
	registerBGMFlags(seriesCreateCmd)
	registerProviderFlags(seriesCreateCmd)

	registerCreativeFlags(seriesSetCmd)
	registerBGMFlags(seriesSetCmd)
	registerProviderFlags(seriesSetCmd)

	seriesCmd.AddCommand(seriesCreateCmd, seriesSetCmd, seriesListCmd, seriesShowCmd)
	rootCmd.AddCommand(seriesCmd)
}

// registerBGMFlags 注册成片 BGM 相关 flag（create 与 set 共用同一组变量，§20）。
func registerBGMFlags(cmd *cobra.Command) {
	// 成片 BGM（§20 + §23）：create 与 set 共用同一组变量，两通路互斥。
	cmd.Flags().StringVar(&seriesBGM, "bgm", "",
		"成片背景音乐路径（相对系列目录 data/projects/<系列ID>/，如 bgm/theme.mp3；留空＝无 BGM；与 --bgm-asset 互斥）")
	cmd.Flags().StringVar(&seriesBGMAsset, "bgm-asset", "",
		"成片背景音乐的曲库素材 ID（§23，`story asset list` 查看；显式空串＝清除；与 --bgm 互斥）")
	cmd.Flags().Float64Var(&seriesBGMVolume, "bgm-volume", 0,
		"BGM 音量 0..1（含 0 与 1；0＝未设置，用默认 0.18）")
}

// registerCreativeFlags 注册创作设置相关 flag（create 与 set 共用同一组变量）。
func registerCreativeFlags(cmd *cobra.Command) {
	cmd.Flags().StringArrayVar(&seriesCreative, "creative", nil,
		"创作参数，可重复，格式 key=value（如 --creative duration=d60）；可用 key 见 story series show 或 Web 端")
	cmd.Flags().StringVar(&seriesVideoStyle, "video-style", "", "全片画风 key（选项由 templates 的风格包给出，如 gongbi/ink）")
}

// registerProviderFlags 注册系列级 Provider 覆盖 flag（create 与 set 共用同一组变量，§21）。
// 留空＝跟随系统默认；set 用 `Changed` 区分「未提供（保持原值）」与「显式空串（清除）」。
func registerProviderFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&seriesTextProvider, "text-provider", "",
		"系列级故事/分镜/策划供应商覆盖（空＝跟随系统默认；可用值见 `story capabilities`）")
	cmd.Flags().StringVar(&seriesTTSProvider, "tts-provider", "",
		"系列级语音合成供应商覆盖（空＝跟随系统默认）")
	cmd.Flags().StringVar(&seriesImageProvider, "image-provider", "",
		"系列级图片生成供应商覆盖（空＝跟随系统默认）")
	cmd.Flags().StringVar(&seriesVideoProvider, "video-provider", "",
		"系列级视频生成供应商覆盖（空＝跟随系统默认）")
	// 系列级模型覆盖（第二步统一资源管理）：与 Provider 覆盖同款 Changed 补丁语义。
	cmd.Flags().StringVar(&seriesTextModel, "text-model", "",
		"系列级文本模型覆盖（故事/分镜共用；空＝跟随系统默认）")
	cmd.Flags().StringVar(&seriesTTSModel, "tts-model", "",
		"系列级旁白模型覆盖（声音条目自带模型时优先；空＝跟随系统默认）")
	cmd.Flags().StringVar(&seriesImageModel, "image-model", "",
		"系列级图片模型覆盖（插画/定妆照；空＝跟随系统默认）")
	cmd.Flags().StringVar(&seriesVideoModel, "video-model", "",
		"系列级视频模型覆盖（空＝跟随系统默认）")
	// 系列级「规划要求」：分集策划时自动注入首轮上下文；set 用 Changed 补丁语义。
	cmd.Flags().StringVar(&seriesPlanningBrief, "planning-brief", "",
		"分集策划的长期规划要求（目标集数/取材范围/叙事主线等；留空＝跟随系统默认，显式空串＝清除）")
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
