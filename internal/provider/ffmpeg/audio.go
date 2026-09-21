package ffmpeg

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// 复刻参考音频归一化参数（§16 声音复刻：先归一化再提交供应商）。
//
// 为什么必须转码：浏览器录音在 Chrome/Firefox 下是 `audio/webm;codecs=opus`、
// Safari 下是 `audio/mp4`，用户上传件还可能是 48kHz 立体声 m4a；供应商复刻接口
// 只接受常见音频容器的 16kHz 以上单声道/双声道音频，webm/opus 直接不认。
// 统一转 16kHz 单声道 16-bit PCM wav 后，两种来源走同一条提交路径。
const (
	// audioSampleRate 归一化目标采样率（Hz）。供应商要求 ≥16kHz，取下限即可省带宽。
	audioSampleRate = 16000
	// audioChannels 归一化目标声道数（单声道足够表征音色）。
	audioChannels = 1
)

// NormalizeAudio 实现 port.AudioNormalizer：把任意来源音频转成 16kHz 单声道 PCM wav。
//
// `-vn` 丢弃可能存在的视频轨（用户上传 mv/mp4 时常见）；
// 编码用 pcm_s16le（wav 最通用的 16-bit 小端 PCM），无需额外编码器依赖。
// 返回转码后时长（秒），供调用方做「3-30 秒」长度校验与 UI 展示。
func (c *Composer) NormalizeAudio(ctx context.Context, src, dst string) (float64, error) {
	if _, err := os.Stat(src); err != nil {
		return 0, fmt.Errorf("参考音频不可用: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return 0, err
	}
	args := []string{
		"-y",
		"-i", src,
		"-vn",
		"-ac", fmt.Sprintf("%d", audioChannels),
		"-ar", fmt.Sprintf("%d", audioSampleRate),
		"-c:a", "pcm_s16le",
		dst,
	}
	if _, err := runCmd(ctx, c.FFMPEG, args...); err != nil {
		return 0, fmt.Errorf("归一化参考音频: %w", err)
	}
	return probeDuration(ctx, c.FFProbe, dst)
}
