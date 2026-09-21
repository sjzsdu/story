package ffmpeg

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sjzsdu/story/internal/port"
)

// stillFPS comic 模式静帧片段帧率，与 normalize 的 fps=30 保持一致，
// 使 RenderStill 产物与归一化中间件编码参数对齐、可直接 concat 流拷贝。
const stillFPS = 30

// stillZoom 推近/拉远的缩放倍率。1.30 表示全程放大三成：静帧画面唯一的
// 运动来源就是运镜，幅度太小观众几乎看不出变化，故取到既明显、又不至于
// 把构图裁得面目全非的力度。
const stillZoom = 1.30

// panZoom 横移/纵移时固定的缩放倍率：平移行程 = iw - iw/zoom，
// 倍率越大行程越长。1.24 使视窗横移约画面的四分之一宽度，肉眼可辨。
const panZoom = 1.24

// RenderStill 实现 port.VideoComposer：单张插画 + Ken Burns 运镜 → 视频片段。
//
// 纯本地 ffmpeg（zoompan），不调用任何模型：
// 输入图先 cover 缩放/裁剪到 2 倍目标尺寸的缓冲（任意比例输入均无黑边，
// 大缓冲抑制 zoompan 抖动），再按运镜表达式输出目标分辨率片段；无音轨，
// 旁白音频在 Compose 阶段混入。
func (c *Composer) RenderStill(ctx context.Context, req port.StillRequest) error {
	if req.DurationSec <= 0 {
		return fmt.Errorf("静帧时长非法: %d", req.DurationSec)
	}
	if _, err := os.Stat(req.ImagePath); err != nil {
		return fmt.Errorf("插画不存在: %w", err)
	}
	width, height, err := dimensions(req.Ratio, req.Resolution)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(req.OutPath), 0o755); err != nil {
		return err
	}

	frames := req.DurationSec * stillFPS
	bufW, bufH := width*2, height*2
	z, x, y := kenBurnsExpr(req.Motion, frames)

	// zoompan 工作在缓冲尺寸上，输出 s=目标尺寸；fps/像素格式与 normalize 对齐。
	filter := fmt.Sprintf(
		"scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d,setsar=1,"+
			"zoompan=z='%s':x='%s':y='%s':d=%d:s=%dx%d:fps=%d,format=yuv420p",
		bufW, bufH, bufW, bufH,
		z, x, y, frames, width, height, stillFPS,
	)

	_, err = runCmd(ctx, c.FFMPEG,
		"-y",
		"-loop", "1",
		"-i", req.ImagePath,
		"-vf", filter,
		"-c:v", "libx264", "-preset", "veryfast", "-crf", "20", "-pix_fmt", "yuv420p",
		"-r", fmt.Sprintf("%d", stillFPS),
		"-frames:v", fmt.Sprintf("%d", frames),
		"-an",
		req.OutPath,
	)
	if err != nil {
		return fmt.Errorf("静帧渲染: %w", err)
	}
	return nil
}

// kenBurnsExpr 按运镜 key 生成 zoompan 的 z/x/y 表达式。
// on/d 为 zoompan 内置的输出帧序号/总帧数；进度 e∈[0,1] 做 smoothstep
// 缓动（起止自然，不匀速突兀）。未知运镜回退缓推。
func kenBurnsExpr(motion string, frames int) (z, x, y string) {
	d := frames - 1
	if d < 1 {
		d = 1
	}
	// smoothstep：e = p*p*(3-2p)，p=on/d。
	const centerX = "iw/2-(iw/zoom/2)"
	const centerY = "ih/2-(ih/zoom/2)"
	p := fmt.Sprintf("(on/%d)", d)
	e := fmt.Sprintf("(%s*%s*(3-2*%s))", p, p, p)
	maxX := fmt.Sprintf("(iw-iw/zoom)")
	maxY := fmt.Sprintf("(ih-ih/zoom)")

	switch motion {
	case port.MotionPullOut:
		return fmt.Sprintf("%.2f-%.2f*%s", stillZoom, stillZoom-1.0, e), centerX, centerY
	case port.MotionPanRight:
		return fmt.Sprintf("%.2f", panZoom), maxX + "*" + e, centerY
	case port.MotionPanLeft:
		return fmt.Sprintf("%.2f", panZoom), maxX + "*(1-" + e + ")", centerY
	case port.MotionPanDown:
		return fmt.Sprintf("%.2f", panZoom), centerX, maxY + "*" + e
	case port.MotionPanUp:
		return fmt.Sprintf("%.2f", panZoom), centerX, maxY + "*(1-" + e + ")"
	case port.MotionStatic:
		return "1.0", "0", "0"
	case port.MotionPushIn:
		fallthrough
	default:
		// 缓推：zoom 1.0 → stillZoom，视线居中。
		return fmt.Sprintf("1.0+%.2f*%s", stillZoom-1.0, e), centerX, centerY
	}
}
