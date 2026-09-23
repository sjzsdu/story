package main

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/sjzsdu/story/internal/domain"
)

// 素材/资产（§23 统一资源管理）CLI：list / add（导入复制进库）/ rm（引用计数拒删）。

var (
	assetKind        string
	assetName        string
	assetDesc        string
	assetImportPath  string
	assetListAllFlag bool
)

var assetCmd = &cobra.Command{
	Use:   "asset",
	Short: "管理素材库（BGM 曲目、视觉参考图等，与 Voice 同级的顶层实体，§23）",
}

var assetListCmd = &cobra.Command{
	Use:   "list",
	Short: "列出素材（默认只列背景音乐，--kind all 列全部类型）",
	RunE: func(cmd *cobra.Command, args []string) error {
		kind := assetKind
		if strings.EqualFold(kind, "all") {
			kind = "" // 空串＝全部（App.ListAssets 约定）
		}
		if k := strings.TrimSpace(kind); k != "" && domain.NormalizeAssetKind(k) == "" {
			return fmt.Errorf("未知素材类型 %q（可用: %s 或 all）", kind, domain.AssetKindsLabel())
		}
		as, err := application.ListAssets(rootCtx, kind)
		if err != nil {
			return err
		}
		if len(as) == 0 {
			fmt.Println("（暂无素材，使用 `story asset add --path <本机文件>` 导入，或在 Web 端上传）")
			return nil
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\t类型\t名称\t时长\t来源\t描述")
		for _, a := range as {
			dur := "-"
			if a.DurationSec > 0 {
				dur = fmt.Sprintf("%.1fs", a.DurationSec)
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
				a.ID, domain.AssetKindLabel(a.Kind), truncate(a.Name, 16), dur, a.Origin, truncate(a.Description, 30))
		}
		_ = w.Flush()
		return nil
	},
}

var assetAddCmd = &cobra.Command{
	Use:   "add",
	Short: "从本机文件导入一个素材（复制进素材库 data/assets/<kind>/，原文件不动）",
	RunE: func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(assetImportPath) == "" {
			return fmt.Errorf("--path 不能为空（要导入的本机文件路径）")
		}
		kind := assetKind
		if strings.TrimSpace(kind) == "" {
			kind = domain.AssetKindBGM
		}
		a, err := application.ImportAsset(rootCtx, assetImportPath, kind, assetName, assetDesc)
		if err != nil {
			return err
		}
		fmt.Printf("素材已导入: %s\n", a.ID)
		fmt.Printf("  类型: %s（%s）\n", domain.AssetKindLabel(a.Kind), a.Kind)
		fmt.Printf("  名称: %s\n", a.Name)
		fmt.Printf("  落盘: %s\n", a.Path)
		if a.DurationSec > 0 {
			fmt.Printf("  时长: %.1f 秒\n", a.DurationSec)
		}
		fmt.Printf("  引用: %s（系列设置 --bgm-asset 用它）\n", domain.AssetRef(a.ID))
		return nil
	},
}

var assetRmCmd = &cobra.Command{
	Use:   "rm <asset-id>",
	Short: "删除素材（被系列引用时拒绝，需先在系列设置里改选其他曲目）",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := application.DeleteAsset(rootCtx, args[0]); err != nil {
			return err
		}
		fmt.Printf("素材已删除: %s\n", args[0])
		return nil
	},
}

var assetShowCmd = &cobra.Command{
	Use:   "show <asset-id>",
	Short: "查看素材详情",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		a, err := application.GetAsset(rootCtx, args[0])
		if err != nil {
			return err
		}
		fmt.Printf("素材: %s（%s）\n", a.Name, a.ID)
		fmt.Printf("  类型: %s\n", domain.AssetKindLabel(a.Kind))
		fmt.Printf("  路径: %s\n", a.Path)
		if a.DurationSec > 0 {
			fmt.Printf("  时长: %.1f 秒\n", a.DurationSec)
		}
		if a.Description != "" {
			fmt.Printf("  描述: %s\n", a.Description)
		}
		fmt.Printf("  来源: %s\n", a.Origin)
		fmt.Printf("  引用: %s\n", domain.AssetRef(a.ID))
		return nil
	},
}

// resolveBGMAssetFlag 把 --bgm-asset 的取值转成系列配置里的引用串。
// 接受裸素材 ID（story asset add 输出的 id）或完整 "asset:<id>"；空串＝清除。
func resolveBGMAssetFlag(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if domain.IsAssetRef(v) {
		return v
	}
	return domain.AssetRef(v)
}

// bgmFlagsConflict create/set 共用：--bgm 与 --bgm-asset 同时出现即报错（两条通路互斥）。
func bgmFlagsConflict(cmd *cobra.Command) error {
	if cmd.Flags().Changed("bgm") && cmd.Flags().Changed("bgm-asset") {
		return fmt.Errorf("--bgm（字面路径）与 --bgm-asset（曲库素材）互斥，请只用其一")
	}
	return nil
}

// currentBGMPathValue 按 Changed 语义给出 BGMPath 提交值（含 --bgm-asset → asset: 引用转换）。
// 两条互斥通路都未出现时返回 (nil, false)。
func currentBGMPathValue(cmd *cobra.Command) (*string, bool, error) {
	if err := bgmFlagsConflict(cmd); err != nil {
		return nil, false, err
	}
	if cmd.Flags().Changed("bgm-asset") {
		v := resolveBGMAssetFlag(seriesBGMAsset)
		// 非清除操作时快速校验素材存在（app 层还会再校验一次，这里给 CLI 用户更早的反馈）。
		if v != "" {
			if _, err := application.GetAsset(rootCtx, domain.AssetIDFromRef(v)); err != nil {
				return nil, false, fmt.Errorf("素材 %s 不存在: %w；用 `story asset list` 查看可选值", domain.AssetIDFromRef(v), err)
			}
		}
		return &v, true, nil
	}
	if cmd.Flags().Changed("bgm") {
		return &seriesBGM, true, nil
	}
	return nil, false, nil
}

func init() {
	assetListCmd.Flags().StringVar(&assetKind, "kind", domain.AssetKindBGM,
		fmt.Sprintf("素材类型：%s / all（全部）", domain.AssetKindsLabel()))

	assetAddCmd.Flags().StringVar(&assetImportPath, "path", "", "要导入的本机文件路径（必填，复制进素材库）")
	assetAddCmd.Flags().StringVar(&assetKind, "kind", domain.AssetKindBGM,
		fmt.Sprintf("素材类型，默认 %s（可选 %s）", domain.AssetKindBGM, domain.AssetKindsLabel()))
	assetAddCmd.Flags().StringVar(&assetName, "name", "", "素材名称（留空用文件名）")
	assetAddCmd.Flags().StringVar(&assetDesc, "desc", "", "素材描述（仅展示）")

	assetCmd.AddCommand(assetListCmd, assetAddCmd, assetShowCmd, assetRmCmd)
	rootCmd.AddCommand(assetCmd)
}

// printBGMAssetInCreate 新建系列成功后回显 BGM 引用（避免与 printSeries 重复改造）。
func printBGMAssetInCreate(se *domain.Series) {
	if domain.IsAssetRef(se.Config.BGMPath) {
		if a, err := application.GetAsset(rootCtx, domain.AssetIDFromRef(se.Config.BGMPath)); err == nil {
			fmt.Printf("  BGM: 曲库「%s」（%s）\n", a.Name, a.ID)
		}
	}
}
