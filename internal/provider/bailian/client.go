// Package bailian 是基于百炼 CLI（bl）的 port 接口实现。
package bailian

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"slices"
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
	// ImageModel 图片模型，空则用 bl 默认。
	ImageModel string
	// TimeoutSec bl 单次请求超时秒数；<=0 时用 defaultTimeoutSec。
	// 视频生成等长任务在各自调用处显式传更大的 --timeout。
	TimeoutSec int
}

// defaultTimeoutSec bl 默认请求超时。实测批量生成候选故事/策划对话可超过 5 分钟
// （bl 内置超时过短会报 code 5 Request timed out），文本/图片统一放宽到 10 分钟。
const defaultTimeoutSec = 600

// NewClient 创建客户端。
func NewClient(bin, textModel, videoModel, ttsModel, imageModel string) *Client {
	if bin == "" {
		bin = "bl"
	}
	return &Client{Bin: bin, TextModel: textModel, VideoModel: videoModel, TTSModel: ttsModel, ImageModel: imageModel}
}

// run 执行 bl 子命令，返回 stdout。错误信息会带上 stderr 片段便于排查。
// 未显式带 --timeout 时自动追加（bl 内置超时对长文本/图片调用过短，会报 code 5 Request timed out）。
func (c *Client) run(ctx context.Context, args ...string) ([]byte, error) {
	args = c.withTimeout(args)
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

// withTimeout 在参数未带 --timeout 时按配置追加。
func (c *Client) withTimeout(args []string) []string {
	if slices.Contains(args, "--timeout") {
		return args
	}
	sec := c.TimeoutSec
	if sec <= 0 {
		sec = defaultTimeoutSec
	}
	return append(args, "--timeout", strconv.Itoa(sec))
}

// runJSON 执行 bl 子命令并要求 JSON 输出。
func (c *Client) runJSON(ctx context.Context, args ...string) ([]byte, error) {
	args = append(args, "--output", "json", "--quiet")
	// bl 内置 undici 300s headers 超时（--timeout 无法绕过）：非 TTY 下 --stream
	// 默认关闭，长文本生成需整包等响应头，必撞超时（UND_ERR_HEADERS_TIMEOUT）。
	// 显式开启 --stream 后逐块接收可绕过；输出形态变为 {"content": ...} 包装，
	// 由 parseChatContent 兼容。
	if len(args) >= 2 && args[0] == "text" && args[1] == "chat" && !slices.Contains(args, "--stream") {
		args = append(args, "--stream")
	}
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
