// Package doctor 提供 CLI 依赖健康检查能力（story doctor / story init）。
//
// 检查项：
//   - bl（百炼 CLI）
//   - ffmpeg / ffprobe
//   - python3（版本 ≥ 3.10）
//   - pip / uv（Python 包管理器）
//   - sau（social-auto-upload）
//   - Chrome 浏览器（sau 自动化依赖）
package doctor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Status 检查结果状态。
type Status int

const (
	StatusOK       Status = iota // 正常
	StatusMissing                // 未安装
	StatusWarning                 // 版本过低或其他警告
	StatusError                  // 检查出错
)

func (s Status) String() string {
	switch s {
	case StatusOK:
		return "✅"
	case StatusMissing:
		return "❌"
	case StatusWarning:
		return "⚠️"
	default:
		return "❓"
	}
}

// CheckResult 单项检查结果。
type CheckResult struct {
	Name    string  // 组件名
	Status  Status  // 状态
	Version string  // 检测到的版本
	Message string  // 附加说明（错误/警告原因）
	Fix     string  // 修复建议
}

// Doctor 全局健康检查器。
type Doctor struct {
	// BlBin bl 可执行文件路径
	BlBin string
	// FFMPEGBin ffmpeg 可执行文件路径
	FFMPEGBin string
	// PythonBin Python 解释器路径
	PythonBin string
	// SAUBin sau 可执行文件路径
	SAUBin string
}

// New 创建检查器，用配置值填充，空值用默认路径。
func New(blBin, ffmpegBin, pythonBin, sauBin string) *Doctor {
	if blBin == "" {
		blBin = "bl"
	}
	if ffmpegBin == "" {
		ffmpegBin = "ffmpeg"
	}
	if pythonBin == "" {
		pythonBin = "python3"
	}
	if sauBin == "" {
		sauBin = "sau"
	}
	return &Doctor{
		BlBin:     blBin,
		FFMPEGBin: ffmpegBin,
		PythonBin: pythonBin,
		SAUBin:    sauBin,
	}
}

// RunAll 执行全部检查，返回结果列表。
func (d *Doctor) RunAll(ctx context.Context) []*CheckResult {
	var results []*CheckResult
	results = append(results, d.CheckBL(ctx))
	results = append(results, d.CheckFFmpeg(ctx))
	results = append(results, d.CheckFFprobe(ctx))
	results = append(results, d.CheckPython(ctx))
	results = append(results, d.CheckPip(ctx))
	results = append(results, d.CheckSAU(ctx))
	results = append(results, d.CheckChrome(ctx))
	return results
}

// Summary 输出检查摘要。
func Summary(results []*CheckResult) (ok, warn, fail int) {
	for _, r := range results {
		switch r.Status {
		case StatusOK:
			ok++
		case StatusWarning:
			warn++
		case StatusMissing, StatusError:
			fail++
		}
	}
	return
}

// ---- 逐项检查 ----

// CheckBL 检查百炼 CLI。
func (d *Doctor) CheckBL(ctx context.Context) *CheckResult {
	r := &CheckResult{
		Name:    "bl (百炼 CLI)",
		Fix:     "参考 https://help.aliyun.com/zh/model-studio/getting-started/install 安装",
	}
	version, err := runVersion(ctx, d.BlBin, "--version")
	if err != nil {
		r.Status = StatusMissing
		r.Message = "未找到 bl 命令"
		return r
	}
	r.Version = version
	r.Status = StatusOK
	return r
}

// CheckFFmpeg 检查 ffmpeg。
func (d *Doctor) CheckFFmpeg(ctx context.Context) *CheckResult {
	r := &CheckResult{
		Name: "ffmpeg",
		Fix:  "brew install ffmpeg  # macOS",
	}
	// 找 ffprobe 的目录，推导 ffmpeg 路径
	bin := d.FFMPEGBin
	version, err := runVersion(ctx, bin, "-version")
	if err != nil {
		r.Status = StatusMissing
		r.Message = "未找到 ffmpeg 命令"
		return r
	}
	r.Version = parseFirstline(version)
	r.Status = StatusOK
	return r
}

// CheckFFprobe 检查 ffprobe。
func (d *Doctor) CheckFFprobe(ctx context.Context) *CheckResult {
	r := &CheckResult{
		Name: "ffprobe",
		Fix:  "brew install ffmpeg  # macOS（ffprobe 随 ffmpeg 一起安装）",
	}
	// 从 ffmpeg 路径推导 ffprobe 路径
	bin := strings.Replace(d.FFMPEGBin, "ffmpeg", "ffprobe", 1)
	if bin == d.FFMPEGBin {
		bin = "ffprobe"
	}
	version, err := runVersion(ctx, bin, "-version")
	if err != nil {
		r.Status = StatusMissing
		r.Message = "未找到 ffprobe 命令"
		return r
	}
	r.Version = parseFirstline(version)
	r.Status = StatusOK
	return r
}

// CheckPython 检查 Python 3（要求 ≥ 3.10）。
func (d *Doctor) CheckPython(ctx context.Context) *CheckResult {
	r := &CheckResult{
		Name: "python3",
		Fix:  "brew install python@3  # macOS，或从 https://www.python.org/downloads/ 下载",
	}
	version, err := runVersion(ctx, d.PythonBin, "--version")
	if err != nil {
		r.Status = StatusMissing
		r.Message = "未找到 python3 命令"
		return r
	}
	r.Version = parseFirstline(version)
	// 提取版本号，如 "Python 3.12.1" → 3.12
	major, minor := parsePythonVersion(r.Version)
	if major < 3 || (major == 3 && minor < 10) {
		r.Status = StatusWarning
		r.Message = fmt.Sprintf("版本 %s 偏低，sau 需要 Python ≥ 3.10", r.Version)
		return r
	}
	r.Status = StatusOK
	return r
}

// CheckPip 检查 pip 或 uv。
func (d *Doctor) CheckPip(ctx context.Context) *CheckResult {
	r := &CheckResult{
		Name: "pip / uv",
		Fix:  "python3 -m ensurepip  # 或 curl -LsSf https://astral.sh/uv/install.sh | sh",
	}
	// 优先检查 uv
	if _, err := runVersion(ctx, "uv", "--version"); err == nil {
		r.Version = "uv"
		r.Status = StatusOK
		r.Message = "使用 uv 管理 Python 包"
		return r
	}
	// 回退检查 pip
	if _, err := runVersion(ctx, d.PythonBin, "-m", "pip", "--version"); err == nil {
		r.Version = "pip"
		r.Status = StatusOK
		r.Message = "使用 pip 管理 Python 包"
		return r
	}
	r.Status = StatusMissing
	r.Message = "未找到 pip 或 uv"
	return r
}

// CheckSAU 检查 social-auto-upload。
func (d *Doctor) CheckSAU(ctx context.Context) *CheckResult {
	r := &CheckResult{
		Name: "sau (social-auto-upload)",
		Fix:  "执行 story init 自动安装",
	}
	// sau 不支持 --version，用 --help 检测（exit 0 = 可执行）
	_, err := runVersion(ctx, d.SAUBin, "--help")
	if err != nil {
		// 尝试 ~/.local/bin/sau（story init 安装的 wrapper）
		home, _ := os.UserHomeDir()
		localBin := home + "/.local/bin/sau"
		_, err = runVersion(ctx, localBin, "--help")
	}
	if err != nil {
		r.Status = StatusMissing
		r.Message = "未找到 sau 命令（可执行 story init 自动安装）"
		return r
	}
	r.Version = "已安装"
	r.Status = StatusOK
	return r
}

// CheckChrome 检查 Chrome 浏览器（sau 浏览器自动化依赖）。
func (d *Doctor) CheckChrome(ctx context.Context) *CheckResult {
	r := &CheckResult{
		Name: "Chrome 浏览器",
		Fix:  "从 https://www.google.com/chrome/ 下载安装，版本 ≥ 144",
	}
	var bin string
	switch runtime.GOOS {
	case "darwin":
		bin = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
	case "linux":
		bin = "google-chrome"
	case "windows":
		bin = "C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe"
	default:
		r.Status = StatusWarning
		r.Message = "未知操作系统，无法自动检测 Chrome"
		return r
	}

	version, err := runVersion(ctx, bin, "--version")
	if err != nil {
		r.Status = StatusMissing
		r.Message = "未找到 Chrome 浏览器"
		return r
	}
	r.Version = parseFirstline(version)
	// 提取版本号
	ver := extractChromeVersion(r.Version)
	if ver > 0 && ver < 144 {
		r.Status = StatusWarning
		r.Message = fmt.Sprintf("Chrome %d 版本偏低，建议 ≥ 144", ver)
		return r
	}
	r.Status = StatusOK
	return r
}

// ---- 辅助函数 ----

func runVersion(ctx context.Context, name string, args ...string) (string, error) {
	ctx2, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx2, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func parseFirstline(s string) string {
	if i := strings.IndexByte(s, '\n'); i > 0 {
		return s[:i]
	}
	return s
}

var pythonVersionRe = regexp.MustCompile(`(\d+)\.(\d+)`)

func parsePythonVersion(s string) (major, minor int) {
	m := pythonVersionRe.FindStringSubmatch(s)
	if len(m) < 3 {
		return 0, 0
	}
	major, _ = strconv.Atoi(m[1])
	minor, _ = strconv.Atoi(m[2])
	return
}

var chromeVersionRe = regexp.MustCompile(`(\d+)`)

func extractChromeVersion(s string) int {
	m := chromeVersionRe.FindString(s)
	if m == "" {
		return 0
	}
	v, _ := strconv.Atoi(m)
	return v
}
