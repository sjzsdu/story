package bailian

import (
	"context"
	"os"
	"path/filepath"

	"github.com/sjzsdu/story/internal/port"
)

// GenerateClip 实现 port.VideoGenerator。
// 使用 bl 内置等待：命令返回时视频已下载到 OutPath。
func (c *Client) GenerateClip(ctx context.Context, req port.ClipRequest) (port.ClipResult, error) {
	if err := os.MkdirAll(filepath.Dir(req.OutPath), 0o755); err != nil {
		return port.ClipResult{}, err
	}

	args := []string{
		"video", "generate",
		"--prompt", req.Prompt,
		"--download", req.OutPath,
		"--watermark", "true",
	}
	args = appendModel(args, c.VideoModel)

	if req.ImagePath != "" {
		args = append(args, "--image", req.ImagePath)
	}
	if req.Ratio != "" {
		args = append(args, "--ratio", req.Ratio)
	}
	if req.Resolution != "" {
		args = append(args, "--resolution", req.Resolution)
	}
	if req.DurationSec > 0 {
		args = append(args, "--duration", intArg(req.DurationSec))
	}
	if !req.Watermark {
		// 默认合规保留 AI 水印；仅在显式要求时关闭。
		args = append(args, "--watermark", "false")
	}

	if _, err := c.run(ctx, args...); err != nil {
		return port.ClipResult{}, err
	}
	fi, err := os.Stat(req.OutPath)
	if err != nil {
		return port.ClipResult{}, err
	}
	if fi.Size() == 0 {
		return port.ClipResult{}, os.ErrInvalid
	}
	return port.ClipResult{OutPath: req.OutPath}, nil
}
