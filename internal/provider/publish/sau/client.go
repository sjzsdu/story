// Package sau 是 social-auto-upload CLI（sau）的 Go 包装层。
//
// 设计与 bailian provider（bl CLI）同款：Go 通过 os/exec 调用 sau，
// 解析 stdout/stderr，返回结构化结果。sau 底层用 Playwright 浏览器自动化
// 完成登录/上传，Go 层无需感知浏览器细节。
//
// 用户前置条件：
//   - Python 3.10+ 与 pip/uv
//   - pip install social-auto-upload（或 uv add social-auto-upload）
//   - Chrome 浏览器（sau 首次登录时扫码，Cookie 自动持久化）
package sau

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Client 封装对 sau CLI 的调用。
type Client struct {
	// Bin sau 可执行文件路径或命令名（默认 "sau"）。
	Bin string
	// PythonBin Python 解释器路径（默认 "python3"）；若 sau 已在 PATH 可忽略。
	PythonBin string
	// TimeoutSec 单次 sau 调用超时秒数（上传视频可能较慢，默认 600s）。
	TimeoutSec int
}

// NewClient 创建 sau 客户端。
func NewClient(bin, pythonBin string, timeoutSec int) *Client {
	if bin == "" {
		bin = "sau"
	}
	if pythonBin == "" {
		pythonBin = "python3"
	}
	if timeoutSec <= 0 {
		timeoutSec = 600
	}
	return &Client{
		Bin:        bin,
		PythonBin:  pythonBin,
		TimeoutSec: timeoutSec,
	}
}

// ---- 命令执行 ----

// Result sau 命令执行结果。
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Run 执行 sau 子命令，等待完成并返回结果。
func (c *Client) Run(ctx context.Context, args ...string) (*Result, error) {
	timeout := time.Duration(c.TimeoutSec) * time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, c.Bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	result := &Result{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		}
		return result, fmt.Errorf("sau %s 失败: %w\nstdout: %s\nstderr: %s",
			strings.Join(args, " "), err, stdout.String(), stderr.String())
	}
	return result, nil
}

// ---- 登录管理 ----

// Login 执行平台登录（扫码/Cookie 持久化）。
// platform: douyin / kuaishou / xiaohongshu / bilibili / tencent
// account:  账号标识（自定义名称，如 "company_douyin"）。
func (c *Client) Login(ctx context.Context, platform, account string) error {
	_, err := c.Run(ctx, platform, "login", "--account", account)
	return err
}

// Check 检查指定平台账号的登录状态。
func (c *Client) Check(ctx context.Context, platform, account string) (bool, error) {
	result, err := c.Run(ctx, platform, "check", "--account", account)
	if err != nil {
		return false, err
	}
	// sau check 成功返回 exit 0，失败返回非 0
	output := strings.ToLower(result.Stdout + result.Stderr)
	return strings.Contains(output, "success") || strings.Contains(output, "valid") || result.ExitCode == 0, nil
}

// ---- 视频上传 ----

// UploadVideoRequest 视频上传请求。
type UploadVideoRequest struct {
	Platform  string   // douyin / kuaishou / xiaohongshu / bilibili / tencent
	Account   string   // sau --account 标识
	FilePath  string   // 视频文件路径
	Title     string   // 标题
	Desc      string   // 描述/简介
	Tags      []string // 标签（部分平台支持）
	CoverPath string   // 封面图路径（可选）
	// Bilibili 特有
	Tid int // B站分区 ID（仅 bilibili）
	// 定时发布
	ScheduledTime string // 定时发布时间（sau 格式，可选）
}

// UploadResult 上传结果。
type UploadResult struct {
	Success    bool
	Message    string
	VideoURL   string // 发布后的视频链接（如果平台返回）
	VideoID    string // 平台侧视频 ID
	RawOutput  string // sau 原始输出
}

// UploadVideo 调用 sau 上传视频到指定平台。
func (c *Client) UploadVideo(ctx context.Context, req UploadVideoRequest) (*UploadResult, error) {
	args := c.buildUploadArgs(req)
	result, err := c.Run(ctx, args...)
	if err != nil {
		return &UploadResult{
			Success:   false,
			Message:   err.Error(),
			RawOutput: result.Stdout + "\n" + result.Stderr,
		}, err
	}
	return c.parseUploadResult(result), nil
}

func (c *Client) buildUploadArgs(req UploadVideoRequest) []string {
	args := []string{req.Platform, "upload-video",
		"--account", req.Account,
		"--file", req.FilePath,
		"--title", req.Title,
	}
	if req.Desc != "" {
		args = append(args, "--desc", req.Desc)
	}
	if len(req.Tags) > 0 {
		args = append(args, "--tags", strings.Join(req.Tags, ","))
	}
	if req.CoverPath != "" {
		args = append(args, "--cover", req.CoverPath)
	}
	if req.Tid > 0 && req.Platform == "bilibili" {
		args = append(args, "--tid", fmt.Sprintf("%d", req.Tid))
	}
	if req.ScheduledTime != "" {
		args = append(args, "--scheduled-time", req.ScheduledTime)
	}
	return args
}

func (c *Client) parseUploadResult(result *Result) *UploadResult {
	out := &UploadResult{
		Success:   result.ExitCode == 0,
		RawOutput: result.Stdout + "\n" + result.Stderr,
	}
	if result.ExitCode != 0 {
		out.Message = strings.TrimSpace(result.Stderr)
		if out.Message == "" {
			out.Message = strings.TrimSpace(result.Stdout)
		}
		return out
	}
	// 尝试从输出中提取 URL（各平台格式不同，做最佳努力）
	output := result.Stdout + "\n" + result.Stderr
	out.Message = strings.TrimSpace(output)

	// 常见 URL 模式
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "http://") || strings.HasPrefix(line, "https://") {
			out.VideoURL = line
			break
		}
		// 抖音: "视频已发布: https://..."
		if idx := strings.Index(line, "https://"); idx >= 0 {
			out.VideoURL = strings.TrimSpace(line[idx:])
			break
		}
	}

	return out
}

// ---- 图文上传（小红书等） ----

// UploadNoteRequest 图文笔记上传请求。
type UploadNoteRequest struct {
	Platform string
	Account  string
	Images   []string // 图片路径列表
	Title    string
	Note     string   // 正文
	Tags     []string
}

// UploadNote 调用 sau 上传图文笔记。
func (c *Client) UploadNote(ctx context.Context, req UploadNoteRequest) (*UploadResult, error) {
	args := []string{req.Platform, "upload-note",
		"--account", req.Account,
		"--title", req.Title,
		"--note", req.Note,
	}
	args = append(args, "--images", strings.Join(req.Images, " "))
	if len(req.Tags) > 0 {
		args = append(args, "--tags", strings.Join(req.Tags, ","))
	}

	result, err := c.Run(ctx, args...)
	if err != nil {
		return &UploadResult{
			Success:   false,
			Message:   err.Error(),
			RawOutput: result.Stdout + "\n" + result.Stderr,
		}, err
	}
	return c.parseUploadResult(result), nil
}

// ---- 账号管理辅助 ----

// AccountInfo sau 账号信息（从 check 命令解析）。
type AccountInfo struct {
	Name     string
	Platform string
	Valid    bool
	Username string // 平台侧用户名（如果 check 输出包含）
}

// CheckAllPlatforms 批量检查所有平台的登录状态。
func (c *Client) CheckAllPlatforms(ctx context.Context, account string, platforms []string) []*AccountInfo {
	var infos []*AccountInfo
	for _, p := range platforms {
		valid, _ := c.Check(ctx, p, account)
		infos = append(infos, &AccountInfo{
			Name:     account,
			Platform: p,
			Valid:    valid,
		})
	}
	return infos
}

// SupportedPlatforms sau 支持的平台列表。
var SupportedPlatforms = []string{
	"douyin", "kuaishou", "xiaohongshu", "bilibili", "tencent",
	"baijiahao", "weibo", "hupu", "youtube", "tiktok",
}

// PlatformLabel 平台中文显示名。
func PlatformLabel(p string) string {
	labels := map[string]string{
		"douyin":      "抖音",
		"kuaishou":    "快手",
		"xiaohongshu": "小红书",
		"bilibili":    "B站",
		"tencent":     "视频号",
		"baijiahao":   "百家号",
		"weibo":       "微博",
		"hupu":        "虎扑",
		"youtube":     "YouTube",
		"tiktok":      "TikTok",
	}
	if l, ok := labels[p]; ok {
		return l
	}
	return p
}

// Unused 避免编译警告。
var _ = json.Valid
