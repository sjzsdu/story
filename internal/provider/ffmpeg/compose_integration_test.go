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
	"strings"
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

// hasAudioStream 用 ffprobe 确认文件含音轨。
func hasAudioStream(t *testing.T, path string) bool {
	t.Helper()
	out, err := exec.Command("ffprobe", "-v", "error",
		"-select_streams", "a", "-show_entries", "stream=codec_type",
		"-of", "csv=p=0", path).Output()
	if err != nil {
		t.Fatalf("ffprobe 音轨探测失败: %v", err)
	}
	return strings.Contains(string(out), "audio")
}

// TestComposeWithBGMIntegration 真实混音（§20）：BGM 循环铺满成片，
// 输出时长必须与不带 BGM 时一致（±0.2s），且仍含音轨；烧字幕与不烧字幕两条分支都覆盖。
func TestComposeWithBGMIntegration(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("未安装 ffmpeg")
	}
	root := t.TempDir()
	clips, audios := makeFixture(t, root)

	// 一段 3s 短音乐（成片约 8s，验证 -stream_loop 循环铺满且不改变总时长）。
	bgm := filepath.Join(root, "bgm.mp3")
	mk := exec.Command("ffmpeg", "-y",
		"-f", "lavfi", "-i", "sine=frequency=220:duration=3", bgm)
	if out, err := mk.CombinedOutput(); err != nil {
		t.Fatalf("生成测试音乐失败: %v\n%s", err, out)
	}

	c := New("ffmpeg", "ffprobe")
	ctx := context.Background()
	tracks := []port.ClipTrack{
		{SceneID: 1, ClipPath: clips[0], AudioPath: audios[0], Narration: "战国末年，群雄并起。"},
		{SceneID: 2, ClipPath: clips[1], AudioPath: audios[1], Narration: "纵横之术，由此而生。"},
	}

	// 基线：不带 BGM。
	baseFinal := filepath.Join(root, "output", "base-9x16.mp4")
	base, err := c.Compose(ctx, port.ComposeRequest{
		WorkDir:       filepath.Join(root, "tmp-base"),
		Tracks:        tracks,
		Ratio:         "9:16",
		Resolution:    "720P",
		FinalPath:     baseFinal,
		BurnSubtitles: true,
	})
	if err != nil {
		t.Fatalf("基线 Compose 失败: %v", err)
	}

	// 带 BGM（烧字幕分支）。
	final := filepath.Join(root, "output", "bgm-9x16.mp4")
	res, err := c.Compose(ctx, port.ComposeRequest{
		WorkDir:       filepath.Join(root, "tmp-bgm"),
		Tracks:        tracks,
		Ratio:         "9:16",
		Resolution:    "720P",
		FinalPath:     final,
		BurnSubtitles: true,
		BGMPath:       bgm,
		BGMVolume:     0.5,
	})
	if err != nil {
		t.Fatalf("带 BGM 的 Compose 失败: %v", err)
	}
	if fi, err := os.Stat(final); err != nil || fi.Size() == 0 {
		t.Fatalf("带 BGM 的成片不存在或为空: %v", err)
	}
	if diff := res.DurationSec - base.DurationSec; diff > 0.2 || diff < -0.2 {
		t.Fatalf("BGM 改变了成片时长: 基线 %.2f, 带 BGM %.2f（差 %.2f）", base.DurationSec, res.DurationSec, diff)
	}
	if !hasAudioStream(t, final) {
		t.Fatal("带 BGM 的成片缺音轨")
	}

	// 带 BGM（不烧字幕分支：concat 直接进混音，不走 moveFile 提前结束）。
	finalNoSub := filepath.Join(root, "output", "bgm-nosub-9x16.mp4")
	resNoSub, err := c.Compose(ctx, port.ComposeRequest{
		WorkDir:       filepath.Join(root, "tmp-bgm-nosub"),
		Tracks:        tracks,
		Ratio:         "9:16",
		Resolution:    "720P",
		FinalPath:     finalNoSub,
		BurnSubtitles: false,
		BGMPath:       bgm,
	})
	if err != nil {
		t.Fatalf("不烧字幕 + BGM 的 Compose 失败: %v", err)
	}
	if fi, err := os.Stat(finalNoSub); err != nil || fi.Size() == 0 {
		t.Fatalf("成片不存在或为空: %v", err)
	}
	if diff := resNoSub.DurationSec - base.DurationSec; diff > 0.2 || diff < -0.2 {
		t.Fatalf("BGM 改变了成片时长: 基线 %.2f, 不烧字幕带 BGM %.2f（差 %.2f）",
			base.DurationSec, resNoSub.DurationSec, diff)
	}
	if !hasAudioStream(t, finalNoSub) {
		t.Fatal("成片缺音轨")
	}
}
