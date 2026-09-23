package bailian

import (
	"context"
	"fmt"
	"strings"

	"github.com/sjzsdu/story/internal/port"
)

// 视觉理解（§22）：图像理解与视频理解共用同一条 `bl vision describe` 命令，
// 由同一个 bailian Client 实现两个 port 接口。
//
// 关键取舍：
//   - 走 run() 纯文本输出（--output 默认即 text），不走 runJSON：理解结果是自由
//     文本描述，没有稳定 JSON 结构可校验；TrimSpace 后为空视为失败。
//   - 超时沿用 run() 注入的 --timeout（默认 600s），理解类调用无需单独放宽。
//   - model 为空省略 --model，走 bl 默认 qwen3-vl-plus。

// DescribeImage 实现 port.ImageUnderstander：bl vision describe --image <path|url>。
func (c *Client) DescribeImage(ctx context.Context, req port.DescribeImageRequest) (port.DescribeResult, error) {
	if strings.TrimSpace(req.Image) == "" {
		return port.DescribeResult{}, fmt.Errorf("图像理解缺少图片路径或 URL")
	}
	args := []string{"vision", "describe", "--image", req.Image}
	return c.describe(ctx, args, req.Prompt, req.Model)
}

// DescribeVideo 实现 port.VideoUnderstander：bl vision describe --video <url|path>。
// 视频单独即可理解（--image 非必填）；若给了封面/关键帧图则一并传入。
func (c *Client) DescribeVideo(ctx context.Context, req port.DescribeVideoRequest) (port.DescribeResult, error) {
	if strings.TrimSpace(req.Video) == "" {
		return port.DescribeResult{}, fmt.Errorf("视频理解缺少视频 URL 或路径")
	}
	args := []string{"vision", "describe", "--video", req.Video}
	if img := strings.TrimSpace(req.Image); img != "" {
		args = append(args, "--image", img)
	}
	return c.describe(ctx, args, req.Prompt, req.Model)
}

// describe 执行 vision describe 并返回非空纯文本结果。
func (c *Client) describe(ctx context.Context, args []string, prompt, model string) (port.DescribeResult, error) {
	if p := strings.TrimSpace(prompt); p != "" {
		args = append(args, "--prompt", p)
	}
	args = appendModel(args, model)

	out, err := c.run(ctx, args...)
	if err != nil {
		return port.DescribeResult{}, err
	}
	text := strings.TrimSpace(string(out))
	if text == "" {
		return port.DescribeResult{}, fmt.Errorf("bl vision describe 返回空结果")
	}
	return port.DescribeResult{Text: text}, nil
}
