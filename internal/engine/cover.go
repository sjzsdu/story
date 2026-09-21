package engine

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// ExtractCoverFromVideo 从视频中截取封面帧（§19 自动截帧）。
// timestampSec=0 时截取第一帧；否则截取指定时间点的帧。
func ExtractCoverFromVideo(ctx context.Context, videoPath, outPath string, timestampSec float64) error {
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	args := []string{
		"-i", videoPath,
		"-ss", fmt.Sprintf("%.2f", timestampSec),
		"-vframes", "1",
		"-q:v", "2", // 高质量 JPEG
		"-y",
		outPath,
	}
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg 截帧失败: %w\n%s", err, output)
	}
	return nil
}

// ExtractCovers 提取多个封面帧：第一帧 + 按指定时间点截帧（§19）。
// sceneTimestamps 是每个镜头的时间戳（秒），用于截取高潮帧。
// 返回截取的封面文件路径列表。
func ExtractCovers(ctx context.Context, videoPath, coversDir string, sceneTimestamps []float64) ([]string, error) {
	if err := os.MkdirAll(coversDir, 0o755); err != nil {
		return nil, err
	}
	var paths []string

	// 第一帧
	firstFrame := filepath.Join(coversDir, "cover-first.jpg")
	if err := ExtractCoverFromVideo(ctx, videoPath, firstFrame, 0); err != nil {
		return nil, fmt.Errorf("截取第一帧: %w", err)
	}
	paths = append(paths, firstFrame)

	// 按指定时间点截帧
	for i, ts := range sceneTimestamps {
		outPath := filepath.Join(coversDir, fmt.Sprintf("cover-scene-%02d.jpg", i+1))
		if err := ExtractCoverFromVideo(ctx, videoPath, outPath, ts); err != nil {
			continue // 单帧截取失败不阻塞
		}
		paths = append(paths, outPath)
	}
	return paths, nil
}
