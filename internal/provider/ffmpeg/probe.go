package ffmpeg

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// ffprobeJSON ffprobe -print_format json 的最小解析结构。
type ffprobeJSON struct {
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
	Streams []struct {
		CodecType string `json:"codec_type"`
		Duration  string `json:"duration"`
		Width     int    `json:"width"`
		Height    int    `json:"height"`
	} `json:"streams"`
}

func runCmd(ctx context.Context, bin string, args ...string) ([]byte, error) {
	return runCmdDir(ctx, "", bin, args...)
}

func runCmdDir(ctx context.Context, dir, bin string, args ...string) ([]byte, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, bin, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if len(msg) > 800 {
			msg = msg[:800] + "..."
		}
		return stdout.Bytes(), fmt.Errorf("%s %s: %w; %s", bin, strings.Join(args, " "), err, msg)
	}
	return stdout.Bytes(), nil
}

// probe 调用 ffprobe 返回解析结果。
func probe(ctx context.Context, ffprobeBin, path string) (*ffprobeJSON, error) {
	raw, err := runCmd(ctx, ffprobeBin,
		"-v", "error",
		"-print_format", "json",
		"-show_format", "-show_streams",
		path,
	)
	if err != nil {
		return nil, err
	}
	var out ffprobeJSON
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("解析 ffprobe 结果: %w", err)
	}
	return &out, nil
}

// probeDuration 取 format.duration，缺失时回退到视频流时长。
func probeDuration(ctx context.Context, ffprobeBin, path string) (float64, error) {
	out, err := probe(ctx, ffprobeBin, path)
	if err != nil {
		return 0, err
	}
	if d, err := strconv.ParseFloat(out.Format.Duration, 64); err == nil && d > 0 {
		return d, nil
	}
	for _, s := range out.Streams {
		if d, err := strconv.ParseFloat(s.Duration, 64); err == nil && d > 0 {
			return d, nil
		}
	}
	return 0, fmt.Errorf("无法探测时长: %s", path)
}

// probeSize 取视频流宽高。
func probeSize(ctx context.Context, ffprobeBin, path string) (int, int, error) {
	out, err := probe(ctx, ffprobeBin, path)
	if err != nil {
		return 0, 0, err
	}
	for _, s := range out.Streams {
		if s.CodecType == "video" && s.Width > 0 && s.Height > 0 {
			return s.Width, s.Height, nil
		}
	}
	return 0, 0, fmt.Errorf("未找到视频流: %s", path)
}
