package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sjzsdu/story/internal/port"
)

// visionCmd 图像理解 / 视频理解入口（§22）：
//
//	story vision image <path|url>   看图问答 / 画面描述
//	story vision video <url|path>   视频内容问答 / 摘要（可加 --image 关键帧）
//
// 一次性调用：不挂集、不落库、不进派生键。实现经能力槽解析
// （默认取 story.yaml 的 image_understand_provider / video_understand_provider，
// 可用 --provider 单次指定）。
var visionCmd = &cobra.Command{
	Use:   "vision",
	Short: "图像理解 / 视频理解（bl vision describe）",
	Long: `图像理解与视频理解（§22）：

  story vision image 本机截图.png --prompt "图里有几个人物"
  story vision video https://example.com/clip.mp4 --prompt "概括这段视频"

实现走能力槽：系统默认见 story capabilities，--provider 可单次覆盖。
成本提示：理解调用按 token 计费，本地开发请用 mock 或明确授权的真实调用。`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var (
	visionPrompt   string
	visionModel    string
	visionProvider string
	visionImage    string
)

var visionImageCmd = &cobra.Command{
	Use:   "image <path|url>",
	Short: "图像理解：看图问答 / 画面描述",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if application == nil {
			return fmt.Errorf("应用未初始化")
		}
		res, err := application.DescribeImage(cmd.Context(), port.DescribeImageRequest{
			Image:  args[0],
			Prompt: visionPrompt,
			Model:  visionModel,
		}, visionProvider)
		if err != nil {
			return err
		}
		fmt.Println(strings.TrimSpace(res.Text))
		return nil
	},
}

var visionVideoCmd = &cobra.Command{
	Use:   "video <url|path>",
	Short: "视频理解：视频内容问答 / 摘要",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if application == nil {
			return fmt.Errorf("应用未初始化")
		}
		res, err := application.DescribeVideo(cmd.Context(), port.DescribeVideoRequest{
			Video:  args[0],
			Image:  visionImage,
			Prompt: visionPrompt,
			Model:  visionModel,
		}, visionProvider)
		if err != nil {
			return err
		}
		fmt.Println(strings.TrimSpace(res.Text))
		return nil
	},
}

func init() {
	for _, c := range []*cobra.Command{visionImageCmd, visionVideoCmd} {
		c.Flags().StringVar(&visionPrompt, "prompt", "", "提问 / 描述要求（空则让模型自行概括）")
		c.Flags().StringVar(&visionModel, "model", "", "理解模型（空则用配置 vision_model，再空走 bl 默认）")
		c.Flags().StringVar(&visionProvider, "provider", "", "单次指定实现（空则用系统默认）")
	}
	visionVideoCmd.Flags().StringVar(&visionImage, "image", "", "可选：配套的封面/关键帧图（路径或 URL）")

	visionCmd.AddCommand(visionImageCmd, visionVideoCmd)
	rootCmd.AddCommand(visionCmd)
}
