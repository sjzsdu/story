package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sjzsdu/story/internal/domain"
	"github.com/sjzsdu/story/internal/engine"
)

var (
	exportRatio   string
	produceScenes string
	fromNode      string
	rerollFlag    bool
	noteFlag      string
)

// deriveOpts 组装本次派生的选项（--from / --reroll / --note）。
func deriveOpts() engine.DeriveOptions {
	return engine.DeriveOptions{From: fromNode, Reroll: rerollFlag, Note: noteFlag}
}

var generateCmd = &cobra.Command{
	Use:     "generate <episode-id>",
	Aliases: []string{"candidates"},
	Short:   "步骤1：AI 生成本集故事（直接定稿）",
	Long: "生成本集故事定稿。同一组派生输入（系列 + 主题 + 朝代）已产出的版本会直接复用，\n" +
		"不重复调用模型；用 --reroll 可另开一版（旧版保留在版本树里）。",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println(">>> 正在生成本集故事（约需 30-60 秒，生成即定稿）...")
		node, err := application.Engine.GenerateStory(rootCtx, args[0], deriveOpts())
		if err != nil {
			return err
		}
		s := node.Story
		if s != nil {
			fmt.Printf("\n《%s》  %s · %s\n", s.Title, s.Dynasty, s.Source)
			fmt.Println(wrapIndent(s.Summary, "    "))
		}
		fmt.Printf("\n节点 %s（v%d）\n审阅副本: %s/story.md\n", node.ID, node.Attempt+1, node.Dir)
		fmt.Println("不满意可加 --reroll 另开一版；满意则进入下一步:")
		fmt.Printf("story storyboard %s\n", args[0])
		return nil
	},
}

var storyboardCmd = &cobra.Command{
	Use:   "storyboard <episode-id>",
	Short: "步骤2：将故事拆解为分镜",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println(">>> 正在拆分分镜（约需 30-60 秒）...")
		node, err := application.Engine.PlanStoryboard(rootCtx, args[0], deriveOpts())
		if err != nil {
			return err
		}
		sb := node.Storyboard
		total := 0
		for _, sc := range sb.Scenes {
			total += sc.DurationSec
		}
		fmt.Printf("已生成 %d 个镜头，预计总时长约 %d 秒\n", len(sb.Scenes), total)
		fmt.Printf("节点 %s（v%d）\n审阅副本: %s/storyboard.json\n", node.ID, node.Attempt+1, node.Dir)
		fmt.Printf("下一步: story produce %s\n", args[0])
		return nil
	},
}

var produceCmd = &cobra.Command{
	Use:   "produce <episode-id>",
	Short: "步骤3：并发生成画面片段与旁白（只生产未完成的镜头，支持断点续跑）",
	Long: "生产本集的画面片段与旁白。\n" +
		"默认只生产「画面或旁白尚未产出」的镜头：已成功的镜头不会被重复出图/合成，\n" +
		"因此直接重跑本命令即等价于「只重试失败镜头」。\n" +
		"用 --scenes 可只重试指定镜头（如 --scenes 13,15,17）；\n" +
		"用 --reroll 可基于同一分镜另开一版画面（旧版片段原样保留）。",
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
		if len(scenes) > 0 {
			fmt.Printf(">>> 只重试镜头 %s（并发 %d，失败重试 %d 次）...\n",
				formatSceneIDs(scenes), se.Config.MaxConcurrency, se.Config.MaxRetries)
		} else {
			fmt.Printf(">>> 开始生产（并发 %d，失败重试 %d 次；已完成的镜头自动跳过）...\n",
				se.Config.MaxConcurrency, se.Config.MaxRetries)
		}

		opts := deriveOpts()
		opts.Scenes = scenes
		node, err := application.Engine.Produce(rootCtx, args[0], opts)
		if err != nil {
			fmt.Println("生产未全部成功，已保留进度；可重跑 produce（只会重试未完成的镜头），或用 --scenes 指定镜头。")
			return err
		}
		cr, cp, cf := countMedia(node.Clips)
		ar, ap, af := countMedia(node.Audios)
		fmt.Printf("画面：复用 %d，本次生产 %d，未完成 %d\n", cr, cp, cf)
		fmt.Printf("旁白：复用 %d，本次生产 %d，未完成 %d\n", ar, ap, af)
		fmt.Printf("节点 %s（v%d）: %s\n", node.ID, node.Attempt+1, node.Status)
		if node.Done() {
			fmt.Printf("下一步: story compose %s\n", node.ID)
		} else if node.Error != "" {
			fmt.Printf("整集仍有镜头未完成: %s\n", node.Error)
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
		final, err := application.Engine.Compose(rootCtx, args[0], deriveOpts())
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
	Long: "从起点沿版本链往下补齐到成片。已产出的节点直接复用（零费用续跑）；\n" +
		"用 --from <node-id> 可从任意节点继续，用 --reroll 可整条链另开一版。",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println(">>> 沿版本链补齐到成片（已有节点直接复用）...")
		if err := application.Engine.Run(rootCtx, args[0], engine.DeriveOptions{From: fromNode, Reroll: rerollFlag}); err != nil {
			return err
		}
		ep, err := application.GetEpisode(rootCtx, args[0])
		if err != nil {
			return err
		}
		if n := ep.NodeByID(ep.ActiveNodeID); n != nil && len(n.Outputs) > 0 {
			fmt.Printf("\n全部完成: %s\n", n.Outputs[0])
		}
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
		dst, err := application.Engine.Export(rootCtx, args[0], exportRatio, deriveOpts())
		if err != nil {
			return err
		}
		fmt.Printf("已导出: %s\n", dst)
		return nil
	},
}

var statusCmd = &cobra.Command{
	Use:   "status <episode-id>",
	Short: "查看一集的版本树状态",
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

var nodesCmd = &cobra.Command{
	Use:   "nodes <episode-id>",
	Short: "打印该集的版本树（节点 ID / 版本 / 状态 / 摘要）",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ep, err := application.GetEpisode(rootCtx, args[0])
		if err != nil {
			return err
		}
		printVersionTree(ep)
		return nil
	},
}

var activateCmd = &cobra.Command{
	Use:   "activate <episode-id> <node-id>",
	Short: "把某个版本节点设为活跃节点（切换成片/分镜等详情）",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := application.Engine.ActivateNode(rootCtx, args[0], args[1]); err != nil {
			return err
		}
		fmt.Printf("活跃节点已切换为 %s\n", args[1])
		return nil
	},
}

var nodeRmCmd = &cobra.Command{
	Use:   "node-rm <episode-id> <node-id>",
	Short: "删除版本节点及其全部后代与媒体文件（不可恢复）",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := application.Engine.DeleteNode(rootCtx, args[0], args[1]); err != nil {
			return err
		}
		fmt.Printf("已删除节点 %s 及其后代（含媒体文件）\n", args[1])
		return nil
	},
}

func printStatus(ep *domain.Episode) {
	fmt.Printf("集: %s  《%s》（系列 %s 第 %d 集）\n", ep.ID, ep.Title, ep.SeriesID, ep.Number)
	fmt.Printf("工作目录: %s\n", ep.WorkDir)
	printVersionTree(ep)
}

func printVersionTree(ep *domain.Episode) {
	fmt.Printf("\n版本树（活跃节点 %s）:\n", orDash(ep.ActiveNodeID))
	active := make(map[string]bool, len(ep.Nodes))
	for _, n := range ep.ActivePath() {
		active[n.ID] = true
	}
	if len(ep.Nodes) == 0 {
		fmt.Println("  （空，尚未开始）")
		return
	}
	for _, stage := range domain.AllStages() {
		fmt.Printf("  [%s]\n", domain.StageLabel(stage))
		found := false
		for _, n := range ep.Nodes {
			if n.Stage != stage {
				continue
			}
			found = true
			line := fmt.Sprintf("    %s %s  v%d  %s", nodeMark(n), n.ID, n.Attempt+1, n.Status)
			if active[n.ID] {
				line += "  ← 活跃"
			}
			if n.Runs > 1 {
				line += fmt.Sprintf("（执行 %d 次）", n.Runs)
			}
			fmt.Println(line)
			if s := nodeSummary(n); s != "" {
				fmt.Println("        " + s)
			}
			if n.Error != "" {
				fmt.Println("        错误: " + wrapIndent(n.Error, "        "))
			}
		}
		if !found {
			fmt.Println("    （无）")
		}
	}
}

func nodeMark(n *domain.VersionNode) string {
	switch n.Status {
	case domain.NodeDone:
		return "●"
	case domain.NodeRunning:
		return "◐"
	case domain.NodeFailed:
		return "✗"
	default:
		return "○"
	}
}

// nodeSummary 给出版本节点的一行人读摘要（按阶段取不同内容）。
func nodeSummary(n *domain.VersionNode) string {
	switch n.Stage {
	case domain.StageStory:
		if n.Story != nil {
			return fmt.Sprintf("《%s》（%s · %s）", n.Story.Title, n.Story.Dynasty, n.Story.Source)
		}
	case domain.StageStoryboard:
		if n.Storyboard != nil {
			return fmt.Sprintf("%d 镜", len(n.Storyboard.Scenes))
		}
	case domain.StageMedia:
		if len(n.Clips) > 0 || len(n.Audios) > 0 {
			cr, cp, cf := countMedia(n.Clips)
			ar, ap, af := countMedia(n.Audios)
			return fmt.Sprintf("画面 复用%d/新%d/未完成%d，旁白 复用%d/新%d/未完成%d",
				cr, cp, cf, ar, ap, af)
		}
	case domain.StageFinal:
		if len(n.Outputs) > 0 {
			return strings.Join(n.Outputs, "，")
		}
	}
	return ""
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
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
	exportCmd.Flags().StringVar(&exportRatio, "ratio", "", "目标比例：16:9 / 9:16 / 1:1 / 3:4")
	produceCmd.Flags().StringVar(&produceScenes, "scenes", "",
		"只生产指定镜头（如 13,15,17）；留空＝只生产所有未完成的镜头")

	// --from / --reroll：版本树通用派生选项（§17）。
	for _, c := range []*cobra.Command{generateCmd, storyboardCmd, produceCmd, composeCmd, runCmd, exportCmd} {
		c.Flags().BoolVar(&rerollFlag, "reroll", false, "另开一版（Attempt+1），不复用同派生输入的既有节点")
	}
	for _, c := range []*cobra.Command{storyboardCmd, produceCmd, composeCmd, runCmd, exportCmd} {
		c.Flags().StringVar(&fromNode, "from", "", "从指定版本节点往下派生；留空＝以集当前活跃节点为准")
	}
	// --note：本版附加要求（配合 --reroll 指定迭代方向）；合成不调模型，故不注册。
	for _, c := range []*cobra.Command{generateCmd, storyboardCmd, produceCmd} {
		c.Flags().StringVar(&noteFlag, "note", "",
			"本版附加要求（换一版时的迭代方向，如「改成倒叙」）；只作用于这一版，参与派生键")
	}

	rootCmd.AddCommand(generateCmd, storyboardCmd, produceCmd, composeCmd, runCmd, exportCmd,
		statusCmd, nodesCmd, activateCmd, nodeRmCmd)
}
