//go:build integration

// 真实 ffmpeg/ffprobe 集成测试（默认不运行）：
//
//	go test -tags=integration ./internal/provider/ffmpeg/
package ffmpeg

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/sjzsdu/story/internal/port"
)

func makeFixture(t *testing.T, dir string) (clips, audios []string) {
	t.Helper()
	specs := []struct {
		name      string
		videoDur  string
		videoSize string
		audioDur  string
	}{
		{"scene-01", "3", "540x960", "4"}, // 音频长 → 冻结末帧
		{"scene-02", "4", "540x960", "3"}, // 视频长 → 补静音
	}
	for _, s := range specs {
		clip := filepath.Join(dir, s.name+".mp4")
		audio := filepath.Join(dir, s.name+".mp3")

		mk := exec.Command("ffmpeg", "-y",
			"-f", "lavfi", "-i", "testsrc2=size="+s.videoSize+":rate=30:duration="+s.videoDur,
			"-pix_fmt", "yuv420p", clip)
		if out, err := mk.CombinedOutput(); err != nil {
			t.Fatalf("生成测试视频失败: %v\n%s", err, out)
		}
		mkA := exec.Command("ffmpeg", "-y",
			"-f", "lavfi", "-i", "sine=frequency=440:duration="+s.audioDur,
			audio)
		if out, err := mkA.CombinedOutput(); err != nil {
			t.Fatalf("生成测试音频失败: %v\n%s", err, out)
		}
		clips = append(clips, clip)
		audios = append(audios, audio)
	}
	return clips, audios
}

func TestComposeIntegration(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("未安装 ffmpeg")
	}
	root := t.TempDir()
	clips, audios := makeFixture(t, root)

	c := New("ffmpeg", "ffprobe")
	ctx := context.Background()

	tracks := []port.ClipTrack{
		{SceneID: 1, ClipPath: clips[0], AudioPath: audios[0], Narration: "战国末年，群雄并起。"},
		{SceneID: 2, ClipPath: clips[1], AudioPath: audios[1], Narration: "纵横之术，由此而生。"},
	}
	final := filepath.Join(root, "output", "guiguzi-e01-9x16.mp4")
	res, err := c.Compose(ctx, port.ComposeRequest{
		WorkDir:       filepath.Join(root, "tmp"),
		Tracks:        tracks,
		Ratio:         "9:16",
		Resolution:    "720P",
		FinalPath:     final,
		BurnSubtitles: true,
	})
	if err != nil {
		t.Fatalf("Compose 失败: %v", err)
	}
	if fi, err := os.Stat(final); err != nil || fi.Size() == 0 {
		t.Fatalf("成片不存在或为空: %v", err)
	}
	if res.Width != 720 || res.Height != 1280 {
		t.Fatalf("成片尺寸 = %dx%d, 期望 720x1280", res.Width, res.Height)
	}
	// 两镜头目标时长分别 4s/4s，总 8s。
	if res.DurationSec < 7.5 || res.DurationSec > 8.5 {
		t.Fatalf("成片时长 = %.2f, 期望约 8s", res.DurationSec)
	}

	exported := filepath.Join(root, "output", "guiguzi-e01-16x9.mp4")
	if err := c.Export(ctx, port.ExportRequest{
		SrcPath: final, DstPath: exported, Ratio: "16:9", Resolution: "720P",
	}); err != nil {
		t.Fatalf("Export 失败: %v", err)
	}
	w, h, err := probeSize(ctx, "ffprobe", exported)
	if err != nil {
		t.Fatal(err)
	}
	if w != 1280 || h != 720 {
		t.Fatalf("导出尺寸 = %dx%d, 期望 1280x720", w, h)
	}
}
