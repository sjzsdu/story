package ffmpeg

import (
	"strings"
	"testing"
)

// TestBGMMixFilter 锁定成片 BGM 混音滤镜的关键参数（§20）。
func TestBGMMixFilter(t *testing.T) {
	// 音量：0（未设置）归一为默认 0.18；显式音量原样写出。
	if got := bgmMixFilter(0, 60); !strings.Contains(got, "volume=0.18") {
		t.Fatalf("音量 0 应归一为默认 0.18: %s", got)
	}
	if got := bgmMixFilter(0.5, 60); !strings.Contains(got, "volume=0.5,") {
		t.Fatalf("显式音量应原样写出: %s", got)
	}

	// 淡入固定 1.5s，BGM 与旁白统一 48kHz。
	got := bgmMixFilter(0.18, 60)
	if !strings.Contains(got, "afade=t=in:st=0:d=1.5") {
		t.Fatalf("缺少开头淡入 1.5s: %s", got)
	}
	if !strings.Contains(got, "aresample=48000") {
		t.Fatalf("BGM 未重采样到 48kHz: %s", got)
	}

	// 淡出：起点 = max(0, dur-3)，时长 = min(3, dur)。
	// 成片极短（<3s，即起点被夹到 0）时淡出从 0 开始、时长＝成片时长。
	short := bgmMixFilter(0.18, 2)
	if !strings.Contains(short, "afade=t=out:st=0.00:d=2.00") {
		t.Fatalf("极短成片的淡出应夹到 st=0、d=成片时长: %s", short)
	}
	// dur 在 3~4s 之间时起点同样从 0 附近起算（不为负）。
	mid := bgmMixFilter(0.18, 3.5)
	if !strings.Contains(mid, "afade=t=out:st=0.50:d=3.00") {
		t.Fatalf("3.5s 成片的淡出参数异常: %s", mid)
	}
	long := bgmMixFilter(0.18, 8)
	if !strings.Contains(long, "afade=t=out:st=5.00:d=3.00") {
		t.Fatalf("8s 成片的淡出应为 st=5、d=3: %s", long)
	}

	// 总时长跟随旁白轨（不加 BGM 时一致），并关闭 amix 自动归一（需 ffmpeg ≥4.4）。
	if !strings.Contains(long, "amix=inputs=2:duration=first:normalize=0[aout]") {
		t.Fatalf("amix 参数异常（必须 duration=first 保证时长不变）: %s", long)
	}
	if !strings.Contains(long, "[0:a][bgm]") {
		t.Fatalf("混音输入应为旁白轨 + BGM: %s", long)
	}
}
