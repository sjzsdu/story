package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sjzsdu/story/internal/domain"
	"github.com/sjzsdu/story/internal/port"
	"github.com/sjzsdu/story/testutil/mock"
)

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"鬼谷子":     "guiguzi",
		"三国":      "sanguo",
		"Tang 盛唐": "tang-shengtang",
		"":        "series-",
	}
	for in, prefix := range cases {
		got := Slugify(in)
		if in == "" {
			// 空名称走时间戳兜底，只检查前缀。
			if len(got) < len("series-") || got[:7] != "series-" {
				t.Fatalf("空名称 slug 异常: %s", got)
			}
			continue
		}
		if got != prefix {
			t.Errorf("Slugify(%q) = %q, 期望 %q", in, got, prefix)
		}
	}
}

// TestResolveVoiceBuilder 覆盖造声供应商分发：空值回退 bailian，显式未知供应商必须报错。
func TestResolveVoiceBuilder(t *testing.T) {
	bl := &mock.VoiceBuild{}
	a := &App{voiceBuilders: map[string]port.VoiceBuilder{
		domain.VoiceProviderBailian: bl,
	}}

	// 空值 → 默认 bailian。
	p, b, err := a.resolveVoiceBuilder("")
	if err != nil || p != domain.VoiceProviderBailian || b != port.VoiceBuilder(bl) {
		t.Fatalf("空 provider 应回退 bailian，得到 (%q, %v, %v)", p, b, err)
	}

	// 大小写/空白容错。
	if _, _, err := a.resolveVoiceBuilder("  Bailian "); err != nil {
		t.Fatalf("大小写/空白应归一：%v", err)
	}

	// 显式未知供应商 → 报错（不得静默回退 bailian）。
	if _, _, err := a.resolveVoiceBuilder("iflytek"); err == nil {
		t.Fatal("未知供应商应报错，而非静默回退 bailian")
	}
}

// TestSaveVoiceSample 覆盖参考音频保存与归一化（§16 复刻）：
// 时长下限拦截、归一化失败传播、未装配报错、恶意文件名不逃出目标目录。
func TestSaveVoiceSample(t *testing.T) {
	t.Run("时长合法返回归一化路径", func(t *testing.T) {
		norm := &mock.AudioNorm{Duration: 12.5}
		a := &App{audioNormalizer: norm}
		got, err := a.SaveVoiceSample(context.Background(), strings.NewReader("fake-webm"), "recording.webm")
		if err != nil {
			t.Fatalf("保存参考音频失败: %v", err)
		}
		if got.DurationSec != 12.5 {
			t.Errorf("时长 = %v, 期望 12.5", got.DurationSec)
		}
		if filepath.Ext(got.Path) != ".wav" {
			t.Errorf("归一化产物应为 wav，得到 %s", got.Path)
		}
		if _, err := os.Stat(got.Path); err != nil {
			t.Errorf("归一化产物不存在: %v", err)
		}
		// 原始件应落在同一个 story-voice-samples 目录下，且保留原始录音的扩展名。
		src := norm.LastSrc()
		if filepath.Dir(src) != filepath.Dir(got.Path) {
			t.Errorf("原始件目录 %s 与产物目录 %s 不一致", filepath.Dir(src), filepath.Dir(got.Path))
		}
		if filepath.Ext(src) != ".webm" {
			t.Errorf("原始件应保留 .webm 扩展名，得到 %s", src)
		}
	})

	t.Run("时长过短被拦截", func(t *testing.T) {
		norm := &mock.AudioNorm{Duration: 1.2}
		a := &App{audioNormalizer: norm}
		if _, err := a.SaveVoiceSample(context.Background(), strings.NewReader("x"), "a.wav"); err == nil {
			t.Fatal("时长低于下限应报错")
		}
	})

	t.Run("归一化失败向上传播", func(t *testing.T) {
		norm := &mock.AudioNorm{Err: errors.New("ffmpeg 挂了")}
		a := &App{audioNormalizer: norm}
		if _, err := a.SaveVoiceSample(context.Background(), strings.NewReader("x"), "a.wav"); err == nil {
			t.Fatal("归一化失败应报错")
		}
	})

	t.Run("未装配归一化能力报错", func(t *testing.T) {
		a := &App{}
		if _, err := a.SaveVoiceSample(context.Background(), strings.NewReader("x"), "a.wav"); err == nil {
			t.Fatal("audioNormalizer 为空应报错")
		}
	})

	t.Run("恶意文件名不逃出目标目录", func(t *testing.T) {
		norm := &mock.AudioNorm{Duration: 9}
		a := &App{audioNormalizer: norm}
		got, err := a.SaveVoiceSample(context.Background(), strings.NewReader("x"), "../../etc/passwd")
		if err != nil {
			t.Fatalf("保存失败: %v", err)
		}
		wantDir := filepath.Join(os.TempDir(), "story-voice-samples")
		if filepath.Dir(got.Path) != wantDir {
			t.Errorf("产物目录 = %s, 期望 %s", filepath.Dir(got.Path), wantDir)
		}
		if src := norm.LastSrc(); filepath.Dir(src) != wantDir {
			t.Errorf("原始件目录 = %s, 期望 %s（路径穿越必须被截断）", filepath.Dir(src), wantDir)
		}
	})
}

// TestSanitizeAudioExt 覆盖扩展名白名单（不信任上传文件名）。
func TestSanitizeAudioExt(t *testing.T) {
	cases := map[string]string{
		"recording.webm":   ".webm",
		"RECORDING.WAV":    ".wav",
		"sample-v3.m4a":    ".m4a",
		"name.tar.gz":      ".gz",
		"../../etc/passwd": ".bin", // 无扩展名
		"evil.wav/../../x": ".bin", // Base 后无扩展名
		"weird.<script>":   ".bin", // 非法字符
		"toolongextension": ".bin", // 无扩展名
		"noext":            ".bin",
		"a.x":              ".x",   // 单字符扩展名合法，仅作命名提示
		"a.":               ".bin", // 只有点，无扩展名
	}
	for in, want := range cases {
		if got := sanitizeAudioExt(in); got != want {
			t.Errorf("sanitizeAudioExt(%q) = %q, 期望 %q", in, got, want)
		}
	}
}
