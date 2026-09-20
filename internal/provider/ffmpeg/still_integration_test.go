//go:build integration

package ffmpeg

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/sjzsdu/story/internal/port"
)

// TestRenderStillIntegration 真实 ffmpeg 验证 Ken Burns 静帧渲染：
// 各种运镜产出的片段须为目标尺寸、时长精确、无音轨。
func TestRenderStillIntegration(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("未安装 ffmpeg")
	}
	root := t.TempDir()
	img := filepath.Join(root, "panel.png")
	mk := exec.Command("ffmpeg", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=1080x1920:rate=1",
		"-frames:v", "1", img)
	if out, err := mk.CombinedOutput(); err != nil {
		t.Fatalf("生成测试插画失败: %v\n%s", err, out)
	}

	c := New("ffmpeg", "ffprobe")
	ctx := context.Background()
	motions := []string{
		port.MotionPushIn, port.MotionPullOut,
		port.MotionPanLeft, port.MotionPanRight,
		port.MotionPanUp, port.MotionPanDown,
		port.MotionStatic, "", // 空值兜底
	}
	for i, m := range motions {
		out := filepath.Join(root, "scene.mp4")
		err := c.RenderStill(ctx, port.StillRequest{
			ImagePath:   img,
			OutPath:     out,
			DurationSec: 4,
			Ratio:       "9:16",
			Resolution:  "720P",
			Motion:      m,
		})
		if err != nil {
			t.Fatalf("RenderStill(%q) 失败: %v", m, err)
		}
		w, h, err := probeSize(ctx, "ffprobe", out)
		if err != nil {
			t.Fatalf("probeSize(%q): %v", m, err)
		}
		if w != 720 || h != 1280 {
			t.Fatalf("motion=%q 尺寸 %dx%d, 期望 720x1280", m, w, h)
		}
		dur, err := probeDuration(ctx, "ffprobe", out)
		if err != nil {
			t.Fatalf("probeDuration(%q): %v", m, err)
		}
		if dur < 3.95 || dur > 4.05 {
			t.Fatalf("motion=%q 时长 %.3f, 期望 4.0s", m, dur)
		}
		// 静帧片段不带音轨（旁白在 Compose 阶段混入）。
		info, err := probe(ctx, "ffprobe", out)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range info.Streams {
			if s.CodecType == "audio" {
				t.Fatalf("motion=%q 不应包含音轨", m)
			}
		}
		_ = i
	}
}
