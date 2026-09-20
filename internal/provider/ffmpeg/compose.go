// Package ffmpeg 是基于本地 ffmpeg/ffprobe 的 port.VideoComposer 实现。
package ffmpeg

import (
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/sjzsdu/story/internal/port"
	"github.com/sjzsdu/story/internal/subtitle"
)

// Composer 通过 ffmpeg/ffprobe 完成归一化、混音、拼接、字幕烧录与多比例导出。
type Composer struct {
	FFMPEG  string
	FFProbe string
	// subs 字幕渲染器（纯 Go 渲染 PNG，规避无 libass 的 ffmpeg 构建）。
	subs *subtitle.Renderer
}

// New 创建 Composer。
func New(ffmpegBin, ffprobeBin string) *Composer {
	if ffmpegBin == "" {
		ffmpegBin = "ffmpeg"
	}
	if ffprobeBin == "" {
		ffprobeBin = "ffprobe"
	}
	return &Composer{FFMPEG: ffmpegBin, FFProbe: ffprobeBin}
}

// WithSubtitles 注入指定字体的字幕渲染器。
func (c *Composer) WithSubtitles(r *subtitle.Renderer) *Composer {
	c.subs = r
	return c
}

// ProbeDuration 实现 port.VideoComposer。
func (c *Composer) ProbeDuration(ctx context.Context, path string) (float64, error) {
	return probeDuration(ctx, c.FFProbe, path)
}

// Compose 实现 port.VideoComposer。
func (c *Composer) Compose(ctx context.Context, req port.ComposeRequest) (port.ComposeResult, error) {
	if len(req.Tracks) == 0 {
		return port.ComposeResult{}, fmt.Errorf("没有可合成的镜头")
	}
	if err := os.MkdirAll(req.WorkDir, 0o755); err != nil {
		return port.ComposeResult{}, err
	}
	if err := os.MkdirAll(filepath.Dir(req.FinalPath), 0o755); err != nil {
		return port.ComposeResult{}, err
	}

	width, height, err := dimensions(req.Ratio, req.Resolution)
	if err != nil {
		return port.ComposeResult{}, err
	}

	// Stage 1：逐镜头归一化（缩放补边 + 对齐音视频时长）。
	normFiles := make([]string, len(req.Tracks))
	cueRanges := make([][2]float64, len(req.Tracks))
	var cursor float64
	for i, t := range req.Tracks {
		clipDur, err := probeDuration(ctx, c.FFProbe, t.ClipPath)
		if err != nil {
			return port.ComposeResult{}, fmt.Errorf("镜头 #%d 探测视频: %w", t.SceneID, err)
		}
		audioDur, err := probeDuration(ctx, c.FFProbe, t.AudioPath)
		if err != nil {
			return port.ComposeResult{}, fmt.Errorf("镜头 #%d 探测音频: %w", t.SceneID, err)
		}
		target := math.Round(math.Max(clipDur, audioDur)*10) / 10

		normName := fmt.Sprintf("scene-%02d.norm.mp4", t.SceneID)
		normPath := filepath.Join(req.WorkDir, normName)
		if err := c.normalize(ctx, t.ClipPath, t.AudioPath, normPath, width, height, target, clipDur); err != nil {
			return port.ComposeResult{}, fmt.Errorf("镜头 #%d 归一化: %w", t.SceneID, err)
		}
		normFiles[i] = normName // concat 清单使用相对名（cmd.Dir=WorkDir）
		cueRanges[i] = [2]float64{cursor, cursor + target}
		cursor += target
	}

	// Stage 2：拼接（所有中间件编码参数一致，可直接流拷贝）。
	listPath := filepath.Join(req.WorkDir, "concat.txt")
	if err := writeConcatList(listPath, normFiles); err != nil {
		return port.ComposeResult{}, err
	}
	concatPath := filepath.Join(req.WorkDir, "concat.mp4")
	if _, err := runCmdDir(ctx, req.WorkDir, c.FFMPEG, "-y",
		"-f", "concat", "-safe", "0",
		"-i", "concat.txt",
		"-c", "copy",
		"concat.mp4",
	); err != nil {
		return port.ComposeResult{}, fmt.Errorf("拼接片段: %w", err)
	}

	// Stage 3：烧录硬字幕（Go 渲染透明 PNG → overlay 按时间区间叠加，
	// 不依赖 ffmpeg 的 libass/freetype 编译选项）。
	if req.BurnSubtitles {
		// 同时保留一份 SRT 旁路基，便于人工审阅/平台外挂。
		if err := writeSRT(filepath.Join(req.WorkDir, "subs.srt"), req.Tracks, cueRanges); err != nil {
			return port.ComposeResult{}, err
		}
		if c.subs == nil {
			r, err := subtitle.DefaultRenderer()
			if err != nil {
				return port.ComposeResult{}, fmt.Errorf("字幕渲染初始化: %w", err)
			}
			c.subs = r
		}
		args := []string{"-y", "-i", "concat.mp4"}
		pngNames := make([]string, 0, len(req.Tracks))
		for _, t := range req.Tracks {
			text := strings.TrimSpace(t.Narration)
			pngName := fmt.Sprintf("sub-%02d.png", t.SceneID)
			if text != "" {
				png, err := c.subs.Render(text, width, height, 2)
				if err != nil {
					return port.ComposeResult{}, fmt.Errorf("渲染镜头 #%d 字幕: %w", t.SceneID, err)
				}
				if err := os.WriteFile(filepath.Join(req.WorkDir, pngName), png, 0o644); err != nil {
					return port.ComposeResult{}, err
				}
				args = append(args, "-i", pngName)
				pngNames = append(pngNames, pngName)
			}
		}

		// 组装 overlay 链：每段字幕在自己的时间区间内显示，区间边界留 10ms 防重帧。
		var graph strings.Builder
		prev := "0:v"
		inputIdx := 1
		for i, t := range req.Tracks {
			if strings.TrimSpace(t.Narration) == "" {
				continue
			}
			start, end := cueRanges[i][0]+0.01, maxf(cueRanges[i][1]-0.01, cueRanges[i][0]+0.02)
			out := "vout"
			if inputIdx != len(pngNames) {
				out = fmt.Sprintf("v%d", inputIdx)
			}
			fmt.Fprintf(&graph, "[%s][%d:v]overlay=0:0:enable='between(t,%.2f,%.2f)'[%s];",
				prev, inputIdx, start, end, out)
			prev = out
			inputIdx++
		}
		filter := strings.TrimSuffix(graph.String(), ";")

		// 输出路径须转绝对——ffmpeg 以 cmd.Dir=WorkDir(tmp/) 运行，
		// 若 FinalPath 是相对项目根的路径会被误解析到 tmp/ 下。
		finalAbs := req.FinalPath
		if !filepath.IsAbs(finalAbs) {
			if abs, err := filepath.Abs(finalAbs); err == nil {
				finalAbs = abs
			}
		}
		args = append(args,
			"-filter_complex", filter,
			"-map", "["+prev+"]", "-map", "0:a?",
			"-c:v", "libx264", "-preset", "veryfast", "-crf", "20", "-pix_fmt", "yuv420p",
			"-c:a", "copy",
			finalAbs,
		)
		if _, err := runCmdDir(ctx, req.WorkDir, c.FFMPEG, args...); err != nil {
			return port.ComposeResult{}, fmt.Errorf("烧录字幕: %w", err)
		}
	} else {
		if err := moveFile(concatPath, req.FinalPath); err != nil {
			return port.ComposeResult{}, err
		}
	}

	dur, err := probeDuration(ctx, c.FFProbe, req.FinalPath)
	if err != nil {
		return port.ComposeResult{}, err
	}
	w, h, err := probeSize(ctx, c.FFProbe, req.FinalPath)
	if err != nil {
		w, h = width, height
	}
	return port.ComposeResult{FinalPath: req.FinalPath, DurationSec: dur, Width: w, Height: h}, nil
}

// normalize 把单个镜头统一到目标分辨率，并用旁白对齐时长。
// 音频更长时冻结末帧（tpad clone），视频更长时补静音（apad + -t 截断）。
func (c *Composer) normalize(ctx context.Context, clip, audio, out string, width, height int, target, clipDur float64) error {
	videoChain := fmt.Sprintf(
		"[0:v]scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(%d-iw)/2:(%d-ih)/2:color=black,setsar=1,fps=30",
		width, height, width, height, width, height,
	)
	if target > clipDur+0.05 {
		videoChain += fmt.Sprintf(",tpad=stop_mode=clone:stop=%.2f", target-clipDur)
	}
	videoChain += "[v]"
	audioChain := fmt.Sprintf("[1:a]aresample=48000,apad,atrim=0:%.2f,asetpts=PTS-STARTPTS[a]", target)

	args := []string{
		"-y",
		"-i", clip,
		"-i", audio,
		"-filter_complex", videoChain + ";" + audioChain,
		"-map", "[v]", "-map", "[a]",
		"-c:v", "libx264", "-preset", "veryfast", "-crf", "20", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-ar", "48000", "-b:a", "128k",
		"-t", fmt.Sprintf("%.2f", target),
		out,
	}
	_, err := runCmd(ctx, c.FFMPEG, args...)
	return err
}

// Export 从成片以模糊背景填充方式导出其他比例版本。
func (c *Composer) Export(ctx context.Context, req port.ExportRequest) error {
	width, height, err := dimensions(req.Ratio, req.Resolution)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(req.DstPath), 0o755); err != nil {
		return err
	}
	filter := fmt.Sprintf(
		"[0:v]split=2[bg][fg];"+
			"[bg]scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d,gblur=sigma=30[bg2];"+
			"[fg]scale=%d:%d:force_original_aspect_ratio=decrease[fgs];"+
			"[bg2][fgs]overlay=(%d-w)/2:(%d-h)/2[v]",
		width, height, width, height,
		width, height,
		width, height,
	)
	_, err = runCmd(ctx, c.FFMPEG, "-y",
		"-i", req.SrcPath,
		"-filter_complex", filter,
		"-map", "[v]", "-map", "0:a?",
		"-c:v", "libx264", "-preset", "veryfast", "-crf", "20", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-b:a", "128k",
		req.DstPath,
	)
	return err
}

// dimensions 比例+分辨率档位 → 像素宽高。
// resolution 档位指长边：1080P → 长边 1920，720P → 长边 1280。
func dimensions(ratio, resolution string) (int, int, error) {
	r := ratio
	if r == "" {
		r = "9:16"
	}
	p := resolution
	if p == "" {
		p = "1080P"
	}
	long := 1920
	if strings.EqualFold(p, "720P") {
		long = 1280
	}
	switch r {
	case "9:16":
		return long * 9 / 16, long, nil // 1080P → 1080x1920，720P → 720x1280
	case "16:9":
		return long, long * 9 / 16, nil
	case "1:1":
		return long, long, nil
	case "3:4":
		return long * 3 / 4, long, nil // 1080P → 1440x1920
	case "4:3":
		return long, long * 3 / 4, nil
	default:
		return 0, 0, fmt.Errorf("不支持的画面比例: %s", ratio)
	}
}

func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func writeConcatList(path string, files []string) error {
	var b strings.Builder
	for _, f := range files {
		fmt.Fprintf(&b, "file '%s'\n", f)
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func writeSRT(path string, tracks []port.ClipTrack, ranges [][2]float64) error {
	var b strings.Builder
	for i, t := range tracks {
		text := strings.TrimSpace(t.Narration)
		if text == "" {
			continue
		}
		fmt.Fprintf(&b, "%d\n%s --> %s\n%s\n\n",
			i+1,
			srtTime(ranges[i][0]),
			srtTime(ranges[i][1]),
			text,
		)
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func srtTime(sec float64) string {
	if sec < 0 {
		sec = 0
	}
	ms := int(math.Round((sec - math.Floor(sec)) * 1000))
	if ms == 1000 {
		sec += 1
		ms = 0
	}
	totalSec := int(sec)
	h := totalSec / 3600
	m := (totalSec % 3600) / 60
	s := totalSec % 60
	return fmt.Sprintf("%02d:%02d:%02d,%03d", h, m, s, ms)
}

func moveFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	// 跨文件系统时回退到复制。
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return os.Remove(src)
}
