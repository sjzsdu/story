package engine

import (
	"strings"

	"github.com/sjzsdu/story/internal/domain"
	"github.com/sjzsdu/story/internal/port"
	"github.com/sjzsdu/story/internal/templates"
)

// PanelsDirName comic 模式每镜插画的子目录名（位于集 WorkDir/panels/）。
const PanelsDirName = "panels"

// fallbackMotions camera 缺省/无法识别时的运镜轮换表：避免全片每镜运镜
// 雷同（口播为主的静帧画面，运镜变化是主要节奏手段之一）。
var fallbackMotions = []string{
	port.MotionPushIn,
	port.MotionPanRight,
	port.MotionPullOut,
	port.MotionPanLeft,
	port.MotionPushIn,
	port.MotionPanDown,
	port.MotionPanRight,
	port.MotionPullOut,
	port.MotionPanUp,
	port.MotionPanLeft,
	port.MotionPushIn,
	port.MotionStatic,
}

// motionForScene 把分镜 camera 中文提示映射为 Ken Burns 运镜 key；
// 无法识别时按镜头序号在轮换表中取值。
func motionForScene(sc domain.Scene, index int) string {
	c := sc.Camera
	switch {
	case strings.Contains(c, "定格") || strings.Contains(c, "静止") || strings.Contains(c, "固定"):
		return port.MotionStatic
	case strings.Contains(c, "拉"):
		return port.MotionPullOut
	case strings.Contains(c, "推") || strings.Contains(c, "特写"):
		return port.MotionPushIn
	case strings.Contains(c, "左"):
		return port.MotionPanLeft
	case strings.Contains(c, "右"):
		return port.MotionPanRight
	case strings.Contains(c, "俯") || strings.Contains(c, "下移") || strings.Contains(c, "向下"):
		return port.MotionPanDown
	case strings.Contains(c, "仰") || strings.Contains(c, "上移") || strings.Contains(c, "向上"):
		return port.MotionPanUp
	default:
		if index < 0 {
			index = 0
		}
		return fallbackMotions[index%len(fallbackMotions)]
	}
}

// panelImageSize comic 模式按成片比例返回插画像素尺寸（bl --size W*H）。
// 竖屏给 1080x1920，为 Ken Burns 缩放/平移保留裁切余量；ffmpeg 侧会再
// cover 到精确比例，故返回值仅影响出图成本与清晰度。
func panelImageSize(ratio string) string {
	switch ratio {
	case "9:16":
		return "1080*1920"
	case "16:9":
		return "1920*1080"
	case "1:1":
		return "1024*1024"
	case "3:4":
		return "1080*1440"
	case "4:3":
		return "1440*1080"
	default:
		return "1080*1920"
	}
}

// buildImagePrompt 组装 comic 模式单镜插画 prompt：画面内容 + 全片统一
// 画风锚句（与视频模式同源风格包，保证两种模式画风一致）。
func buildImagePrompt(sc domain.Scene, style templates.VisualStylePack) string {
	return strings.TrimSpace(sc.VisualPrompt) + "。" + style.VideoAnchor
}
