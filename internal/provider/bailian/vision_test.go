package bailian

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/sjzsdu/story/internal/port"
)

// fakeBL 造一个假的 bl 可执行脚本：把收到的参数写进 args.txt 供断言，
// 并按环境变量控制的文本回显 stdout。零真实调用（成本红线）。
func fakeBL(t *testing.T, stdout string) (*Client, string) {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "bl")
	argsFile := filepath.Join(dir, "args.txt")
	if runtime.GOOS == "windows" {
		t.Skip("假 bl 脚本用 sh 编写，Windows 跳过")
	}
	script := "#!/bin/sh\nprintf '%s' \"$@\" > " + argsFile + "\n" +
		"printf '%s\\n' " + shellQuote(stdout)
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	c := NewClient(bin, "", "", "", "", "", "")
	return c, argsFile
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func readArgs(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestDescribeImageArgsAndResult(t *testing.T) {
	c, argsFile := fakeBL(t, "  一位老者立于竹林。 \n")
	res, err := c.DescribeImage(context.Background(), port.DescribeImageRequest{
		Image:  "/tmp/a.png",
		Prompt: "图里有谁",
		Model:  "qwen3-vl-plus",
	})
	if err != nil {
		t.Fatalf("DescribeImage: %v", err)
	}
	if res.Text != "一位老者立于竹林。" {
		t.Fatalf("结果未 TrimSpace: %q", res.Text)
	}
	args := readArgs(t, argsFile)
	for _, want := range []string{"vision", "describe", "--image", "/tmp/a.png",
		"--prompt", "图里有谁", "--model", "qwen3-vl-plus"} {
		if !strings.Contains(args, want) {
			t.Fatalf("参数缺 %q，实际: %q", want, args)
		}
	}
}

func TestDescribeImageEmptyModelOmitsFlag(t *testing.T) {
	c, argsFile := fakeBL(t, "ok")
	if _, err := c.DescribeImage(context.Background(),
		port.DescribeImageRequest{Image: "https://x/a.png"}); err != nil {
		t.Fatal(err)
	}
	args := readArgs(t, argsFile)
	if strings.Contains(args, "--model") {
		t.Fatalf("model 空时不应带 --model: %q", args)
	}
	if strings.Contains(args, "--prompt") {
		t.Fatalf("prompt 空时不应带 --prompt: %q", args)
	}
}

func TestDescribeVideoArgsAndOptionalImage(t *testing.T) {
	c, argsFile := fakeBL(t, "视频摘要")
	res, err := c.DescribeVideo(context.Background(), port.DescribeVideoRequest{
		Video: "https://x/v.mp4",
	})
	if err != nil {
		t.Fatalf("DescribeVideo: %v", err)
	}
	if res.Text != "视频摘要" {
		t.Fatalf("text = %q", res.Text)
	}
	args := readArgs(t, argsFile)
	if !strings.Contains(args, "--video") || !strings.Contains(args, "https://x/v.mp4") {
		t.Fatalf("缺 --video: %q", args)
	}
	// --image 可选：未提供时绝不出现（--help 示例证实 video 可单独调用）。
	if strings.Contains(args, "--image") {
		t.Fatalf("未提供 image 不应带 --image: %q", args)
	}

	// 提供关键帧图则一并传入。
	c2, argsFile2 := fakeBL(t, "ok")
	if _, err := c2.DescribeVideo(context.Background(), port.DescribeVideoRequest{
		Video: "v.mp4", Image: "kf.png",
	}); err != nil {
		t.Fatal(err)
	}
	if args2 := readArgs(t, argsFile2); !strings.Contains(args2, "--image") {
		t.Fatalf("提供 image 应带 --image: %q", args2)
	}
}

func TestDescribeValidationAndEmptyResult(t *testing.T) {
	c, _ := fakeBL(t, "x")
	if _, err := c.DescribeImage(context.Background(), port.DescribeImageRequest{}); err == nil ||
		!strings.Contains(err.Error(), "图像理解缺少图片") {
		t.Fatalf("空 image 应报校验错误, err=%v", err)
	}
	if _, err := c.DescribeVideo(context.Background(), port.DescribeVideoRequest{}); err == nil ||
		!strings.Contains(err.Error(), "视频理解缺少视频") {
		t.Fatalf("空 video 应报校验错误, err=%v", err)
	}
	// 空白输出 → 明确报错而非返回空成功。
	c2, _ := fakeBL(t, "   \n")
	if _, err := c2.DescribeImage(context.Background(),
		port.DescribeImageRequest{Image: "a.png"}); err == nil ||
		!strings.Contains(err.Error(), "返回空结果") {
		t.Fatalf("空结果应报错, err=%v", err)
	}
}
