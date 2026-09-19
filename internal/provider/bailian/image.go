package bailian

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sjzsdu/story/internal/port"
)

// GenerateImage 实现 port.ImageGenerator（bl image generate）。
func (c *Client) GenerateImage(ctx context.Context, req port.ImageRequest) (port.ImageResult, error) {
	if err := os.MkdirAll(filepath.Dir(req.OutPath), 0o755); err != nil {
		return port.ImageResult{}, err
	}
	// bl 以 out-dir + out-prefix 落盘（自动扩展名），先下到临时目录再改名。
	tmpDir, err := os.MkdirTemp("", "story-img-*")
	if err != nil {
		return port.ImageResult{}, err
	}
	defer os.RemoveAll(tmpDir)
	prefix := "img"

	args := []string{
		"image", "generate",
		"--prompt", req.Prompt,
		"--out-dir", tmpDir,
		"--out-prefix", prefix,
	}
	if req.Size != "" {
		args = append(args, "--size", req.Size)
	}
	if req.NegativePrompt != "" {
		args = append(args, "--negative-prompt", req.NegativePrompt)
	}
	args = appendModel(args, c.ImageModel)

	if _, err := c.run(ctx, args...); err != nil {
		return port.ImageResult{}, err
	}
	produced, err := filepath.Glob(filepath.Join(tmpDir, prefix+"*"))
	if err != nil || len(produced) == 0 {
		return port.ImageResult{}, fmt.Errorf("图片生成未产出文件: %v", err)
	}
	if err := os.Rename(produced[0], req.OutPath); err != nil {
		// 跨设备时退化为复制删除。
		data, rerr := os.ReadFile(produced[0])
		if rerr != nil {
			return port.ImageResult{}, rerr
		}
		if werr := os.WriteFile(req.OutPath, data, 0o644); werr != nil {
			return port.ImageResult{}, werr
		}
		_ = os.Remove(produced[0])
	}
	return port.ImageResult{OutPath: req.OutPath}, nil
}
