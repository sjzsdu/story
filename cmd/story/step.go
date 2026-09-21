package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sjzsdu/story/internal/domain"
)

var pickIndex int
var exportRatio string

var candidatesCmd = &cobra.Command{
	Use:   "candidates <episode-id>",
	Short: "步骤1：AI 生成本集故事（直接定稿，无需人工选择）",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println(">>> 正在生成本集故事（约需 30-60 秒，生成即定稿）...")
		candidates, err := application.Engine.GenerateCandidates(rootCtx, args[0])
		if err != nil {
			return err
		}
		s := candidates[0]
		fmt.Printf("\n《%s》  %s · %s\n", s.Title, s.Dynasty, s.Source)
		fmt.Println(wrapIndent(s.Summary, "    "))
		ep, _ := application.GetEpisode(rootCtx, args[0])
		fmt.Printf("\n审阅副本: %s\n", ep.WorkDir+"/story.md")
		fmt.Println("不满意可重新执行本命令覆盖；满意则进入下一步:")
		fmt.Printf("story storyboard %s\n", args[0])
		return nil
	},
}

var pickCmd = &cobra.Command{
	Use:        "pick <episode-id>",
	Short:      "（旧版兼容）手动选定故事；新版 candidates 已自动定稿，通常无需执行",
	Args:       cobra.ExactArgs(1),
	Deprecated: "新版 story candidates 生成后自动定稿，pick 仅用于兼容旧数据",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := application.Engine.Pick(rootCtx, args[0], pickIndex); err != nil {
			return err
		}
		ep, err := application.GetEpisode(rootCtx, args[0])
		if err != nil {
			return err
		}
		fmt.Printf("已选定 #%d 《%s》（%s · %s）\n", pickIndex, ep.State.Story.Title, ep.State.Story.Dynasty, ep.State.Story.Source)
		fmt.Printf("审阅副本: %s\n", ep.WorkDir+"/story.md")
		fmt.Printf("下一步: story storyboard %s\n", ep.ID)
		return nil
	},
}

var storyboardCmd = &cobra.Command{
	Use:   "storyboard <episode-id>",
	Short: "步骤2：将故事拆解为分镜",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println(">>> 正在拆分分镜（约需 30-60 秒）...")
		sb, err := application.Engine.PlanStoryboard(rootCtx, args[0])
		if err != nil {
			return err
		}
		total := 0
		for _, sc := range sb.Scenes {
			total += sc.DurationSec
		}
		fmt.Printf("已生成 %d 个镜头，预计总时长约 %d 秒\n", len(sb.Scenes), total)
		ep, _ := application.GetEpisode(rootCtx, args[0])
		fmt.Printf("审阅副本: %s\n", ep.WorkDir+"/storyboard.json")
		fmt.Printf("下一步: story produce %s\n", ep.ID)
		return nil
	},
}

var produceScenes string

var produceCmd = &cobra.Command{
	Use:   "produce <episode-id>",
	Short: "步骤3：并发生成画面片段与旁白（只生产未完成的镜头，支持断点续跑）",
	Long: "生产本集的画面片段与旁白。\n" +
		"默认只生产「画面或旁白尚未产出」的镜头：已成功的镜头不会被重复出图/合成，\n" +
		"因此直接重跑本命令即等价于「只重试失败镜头」。\n" +
		"用 --scenes 可只重试指定镜头，例如 --scenes 13,15,17。",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		scenes, err := parseSceneIDs(produceScenes)
		if err != nil {
			return err
		}
		ep, err := application.GetEpisode(rootCtx, args[0])
		if err != nil {
			return err
		}
		se, err := application.GetSeries(rootCtx, ep.SeriesID)
		if err != nil {
			return err
		}
		n := len(ep.State.Storyboard.Scenes)
		if len(scenes) > 0 {
			fmt.Printf(">>> 只重试镜头 %s（并发 %d，失败重试 %d 次）...\n",
				formatSceneIDs(scenes), se.Config.MaxConcurrency, se.Config.MaxRetries)
		} else {
			fmt.Printf(">>> 开始生产 %d 个镜头（并发 %d，失败重试 %d 次；已完成的镜头自动跳过）...\n",
				n, se.Config.MaxConcurrency, se.Config.MaxRetries)
		}

		if len(scenes) > 0 {
			err = application.Engine.ProduceScenes(rootCtx, args[0], scenes)
		} else {
			err = application.Engine.Produce(rootCtx, args[0])
		}
		if err != nil {
			fmt.Println("生产未全部成功，已保留进度；可重跑 produce（只会重试未完成的镜头），或用 --scenes 指定镜头。")
			return err
		}
		ep, _ = application.GetEpisode(rootCtx, args[0])
		cr, cp, cf := countMedia(ep.State.Clips)
		ar, ap, af := countMedia(ep.State.Audios)
		fmt.Printf("画面：复用 %d，本次生产 %d，未完成 %d\n", cr, cp, cf)
		fmt.Printf("旁白：复用 %d，本次生产 %d，未完成 %d\n", ar, ap, af)
		if st := ep.State.Steps[domain.StepProduce].Status; st == domain.StatusDone {
			fmt.Printf("下一步: story compose %s\n", ep.ID)
		} else {
			fmt.Printf("整集仍有镜头未完成: %s\n", ep.State.Steps[domain.StepProduce].Error)
		}
		return nil
	},
}

var composeCmd = &cobra.Command{
	Use:   "compose <episode-id>",
	Short: "步骤4：ffmpeg 合成成片（归一化+拼接+烧录硬字幕）",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println(">>> 正在合成成片...")
		final, err := application.Engine.Compose(rootCtx, args[0])
		if err != nil {
			return err
		}
		fmt.Printf("成片完成: %s\n", final)
		fmt.Printf("导出其他比例: story export %s --ratio 16:9\n", args[0])
		return nil
	},
}

var runCmd = &cobra.Command{
	Use:   "run <episode-id>",
	Short: "一键执行 generate → storyboard → produce → compose（供 Agent 自动驱动）",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ep, err := application.GetEpisode(rootCtx, args[0])
		if err != nil {
			return err
		}
		// 历史数据可能停在 generate 之后、未 pick：Story 为空时直接重新生成定稿。
		if ep.State.Story == nil {
			fmt.Println(">>> [1/4] 生成本集故事...")
			if _, err := application.Engine.GenerateCandidates(rootCtx, args[0]); err != nil {
				return err
			}
		}
		if ep.State.Storyboard == nil || ep.State.Steps[domain.StepStoryboard].Status != domain.StatusDone {
			fmt.Println(">>> [2/4] 拆分分镜...")
			if _, err := application.Engine.PlanStoryboard(rootCtx, args[0]); err != nil {
				return err
			}
		}
		if ep.State.Steps[domain.StepProduce].Status != domain.StatusDone {
			fmt.Println(">>> [3/4] 生产画面与旁白...")
			if err := application.Engine.Produce(rootCtx, args[0]); err != nil {
				return err
			}
		}
		fmt.Println(">>> [4/4] 合成成片...")
		final, err := application.Engine.Compose(rootCtx, args[0])
		if err != nil {
			return err
		}
		fmt.Printf("\n全部完成: %s\n", final)
		return nil
	},
}

var exportCmd = &cobra.Command{
	Use:   "export <episode-id>",
	Short: "从成片导出其他画面比例（模糊背景填充）",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if exportRatio == "" {
			return fmt.Errorf("--ratio 不能为空，可选 16:9 / 9:16 / 1:1 / 3:4")
		}
		dst, err := application.Engine.Export(rootCtx, args[0], exportRatio)
		if err != nil {
			return err
		}
		fmt.Printf("已导出: %s\n", dst)
		return nil
	},
}

var statusCmd = &cobra.Command{
	Use:   "status <episode-id>",
	Short: "查看一集的流水线状态",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ep, err := application.GetEpisode(rootCtx, args[0])
		if err != nil {
			return err
		}
		printStatus(ep)
		return nil
	},
}

func printStatus(ep *domain.Episode) {
	fmt.Printf("集: %s  《%s》（系列 %s 第 %d 集）\n", ep.ID, ep.Title, ep.SeriesID, ep.Number)
	fmt.Printf("工作目录: %s\n\n", ep.WorkDir)
	fmt.Println("流水线:")
	for _, step := range domain.AllSteps() {
		st := ep.State.Steps[step]
		mark := "○"
		switch st.Status {
		case domain.StatusDone, domain.StatusApproved:
			mark = "●"
		case domain.StatusRunning:
			mark = "◐"
		case domain.StatusFailed:
			mark = "✗"
		case domain.StatusReview:
			mark = "?"
		}
		line := fmt.Sprintf("  %s %-11s %s", mark, step, st.Status)
		if st.Attempts > 1 {
			line += fmt.Sprintf("（第 %d 次尝试）", st.Attempts)
		}
		fmt.Println(line)
		if st.Error != "" {
			fmt.Println("      错误: " + wrapIndent(st.Error, "      "))
		}
	}
	if ep.State.Story != nil {
		fmt.Printf("\n故事: 《%s》（%s · %s）\n", ep.State.Story.Title, ep.State.Story.Dynasty, ep.State.Story.Source)
	}
	if ep.State.Storyboard != nil {
		fmt.Printf("分镜: %d 个\n", len(ep.State.Storyboard.Scenes))
	}
	if len(ep.State.Outputs) > 0 {
		fmt.Println("\n成片:")
		for _, o := range ep.State.Outputs {
			fmt.Println("  " + o)
		}
	}
}

func wrapIndent(s, indent string) string {
	s = strings.ReplaceAll(s, "\n", "\n"+indent)
	return s
}

// parseSceneIDs 解析 --scenes，接受 "13,15,17" / "13 15 17" / "13、15" 等写法。
func parseSceneIDs(s string) ([]int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == '，' || r == '、' || r == ' '
	})
	ids := make([]int, 0, len(fields))
	for _, f := range fields {
		id, err := strconv.Atoi(strings.TrimSpace(f))
		if err != nil || id < 1 {
			return nil, fmt.Errorf("--scenes 中的镜头序号非法: %q", f)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// formatSceneIDs 把镜头序号拼成 "#13、#15" 形式。
func formatSceneIDs(ids []int) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = fmt.Sprintf("#%d", id)
	}
	return strings.Join(parts, "、")
}

// countMedia 按「复用 / 本次生产 / 未完成」统计一组产物。
func countMedia(ms []domain.MediaResult) (reused, produced, failed int) {
	for _, m := range ms {
		switch {
		case m.Skipped:
			reused++
		case m.Path != "":
			produced++
		default:
			failed++
		}
	}
	return reused, produced, failed
}

func init() {
	pickCmd.Flags().IntVar(&pickIndex, "index", 0, "故事序号（仅旧版多候选数据需要）")
	exportCmd.Flags().StringVar(&exportRatio, "ratio", "", "目标比例：16:9 / 9:16 / 1:1 / 3:4")
	produceCmd.Flags().StringVar(&produceScenes, "scenes", "",
		"只生产指定镜头（如 13,15,17）；留空＝只生产所有未完成的镜头")

	rootCmd.AddCommand(candidatesCmd, pickCmd, storyboardCmd, produceCmd, composeCmd, runCmd, exportCmd, statusCmd)
}
