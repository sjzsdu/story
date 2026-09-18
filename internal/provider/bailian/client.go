// Package bailian 是基于百炼 CLI（bl）的 port 接口实现。
package bailian

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Client 封装对 bl 命令行的调用。零 HTTP 依赖，全部通过 os/exec 完成。
type Client struct {
	// Bin bl 可执行文件路径或命令名。
	Bin string
	// TextModel 文本模型，空则用 bl 默认。
	TextModel string
	// VideoModel 视频模型，空则用 bl 默认。
	VideoModel string
	// TTSModel 语音模型，空则用 bl 默认。
	TTSModel string
}

// NewClient 创建客户端。
func NewClient(bin, textModel, videoModel, ttsModel string) *Client {
	if bin == "" {
		bin = "bl"
	}
	return &Client{Bin: bin, TextModel: textModel, VideoModel: videoModel, TTSModel: ttsModel}
}

// run 执行 bl 子命令，返回 stdout。错误信息会带上 stderr 片段便于排查。
func (c *Client) run(ctx context.Context, args ...string) ([]byte, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, c.Bin, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		if len(msg) > 800 {
			msg = msg[:800] + "..."
		}
		return stdout.Bytes(), fmt.Errorf("bl %s: %w; %s", strings.Join(args, " "), err, msg)
	}
	return stdout.Bytes(), nil
}

// runJSON 执行 bl 子命令并要求 JSON 输出。
func (c *Client) runJSON(ctx context.Context, args ...string) ([]byte, error) {
	args = append(args, "--output", "json", "--quiet")
	return c.run(ctx, args...)
}

// intArg 整数转字符串参数。
func intArg(n int) string {
	return strconv.Itoa(n)
}

// appendModel 在 model 非空时追加 --model 参数。
func appendModel(args []string, model string) []string {
	if strings.TrimSpace(model) != "" {
		return append(args, "--model", model)
	}
	return args
}
