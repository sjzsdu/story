package bailian

import (
	"context"
	"os"
	"path/filepath"

	"github.com/sjzsdu/story/internal/port"
)

// videoTimeoutSec 视频生成（含轮询与下载）的长超时。
const videoTimeoutSec = 1800

// GenerateClip 实现 port.VideoGenerator。
// 使用 bl 内置等待：命令返回时视频已下载到 OutPath。
// 携带人物定妆照（RefImages）时改走 bl video ref，保证角色形象一致。
func (c *Client) GenerateClip(ctx context.Context, req port.ClipRequest) (port.ClipResult, error) {
	if err := os.MkdirAll(filepath.Dir(req.OutPath), 0o755); err != nil {
		return port.ClipResult{}, err
	}
	if len(req.RefImages) > 0 {
		return c.generateClipRef(ctx, req)
	}

	args := []string{
		"video", "generate",
		"--prompt", req.Prompt,
		"--download", req.OutPath,
		"--watermark", "true",
		"--timeout", intArg(videoTimeoutSec),
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
	return c.verifyClip(req)
}

// generateClipRef 参考图生视频（bl video ref）。Prompt 需含 Image 1..N 标记。
func (c *Client) generateClipRef(ctx context.Context, req port.ClipRequest) (port.ClipResult, error) {
	args := []string{
		"video", "ref",
		"--prompt", req.Prompt,
		"--download", req.OutPath,
		"--timeout", intArg(videoTimeoutSec),
	}
	for _, img := range req.RefImages {
		args = append(args, "--image", img)
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
		args = append(args, "--watermark", "false")
	}
	if _, err := c.run(ctx, args...); err != nil {
		return port.ClipResult{}, err
	}
	return c.verifyClip(req)
}

func (c *Client) verifyClip(req port.ClipRequest) (port.ClipResult, error) {
	fi, err := os.Stat(req.OutPath)
	if err != nil {
		return port.ClipResult{}, err
	}
	if fi.Size() == 0 {
		return port.ClipResult{}, os.ErrInvalid
	}
	return port.ClipResult{OutPath: req.OutPath}, nil
}
