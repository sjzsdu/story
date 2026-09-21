package ffmpeg

import (
	"strings"
	"testing"

	"github.com/sjzsdu/story/internal/port"
)

// TestKenBurnsExprStandardUnchanged 锁定标准档（MotionStrength 为空）的运镜表达式：
// 创作参数里的「运镜强度」若不设置，渲染结果必须与历史版本逐字一致。
func TestKenBurnsExprStandardUnchanged(t *testing.T) {
	const (
		centerX = "iw/2-(iw/zoom/2)"
		centerY = "ih/2-(ih/zoom/2)"
		e       = "((on/29)*(on/29)*(3-2*(on/29)))"
		maxX    = "(iw-iw/zoom)"
		maxY    = "(ih-ih/zoom)"
	)
	cases := []struct {
		motion  string
		z, x, y string
	}{
		{port.MotionPushIn, "1.0+0.30*" + e, centerX, centerY},
		{"", "1.0+0.30*" + e, centerX, centerY}, // 未知/空运镜回退缓推
		{port.MotionPullOut, "1.30-0.30*" + e, centerX, centerY},
		{port.MotionPanRight, "1.24", maxX + "*" + e, centerY},
		{port.MotionPanLeft, "1.24", maxX + "*(1-" + e + ")", centerY},
		{port.MotionPanDown, "1.24", centerX, maxY + "*" + e},
		{port.MotionPanUp, "1.24", centerX, maxY + "*(1-" + e + ")"},
		{port.MotionStatic, "1.0", "0", "0"},
	}
	for _, c := range cases {
		for _, strength := range []string{"", "unknown"} {
			z, x, y := kenBurnsExpr(c.motion, 30, strength)
			if z != c.z || x != c.x || y != c.y {
				t.Fatalf("运镜 %q 强度 %q 表达式变了:\n got %q %q %q\nwant %q %q %q",
					c.motion, strength, z, x, y, c.z, c.x, c.y)
			}
		}
	}
}

// TestMotionStrengthScalesAmplitude 验证强度档只改幅度、不改运镜方向。
func TestMotionStrengthScalesAmplitude(t *testing.T) {
	stdZoom, stdPan := motionZooms("")
	if stdZoom != stillZoom || stdPan != panZoom {
		t.Fatalf("标准档必须等于历史倍率: %v/%v", stdZoom, stdPan)
	}
	strongZoom, strongPan := motionZooms(port.MotionStrengthStrong)
	subtleZoom, subtlePan := motionZooms(port.MotionStrengthSubtle)
	if !(strongZoom > stdZoom && stdZoom > subtleZoom) {
		t.Fatalf("推拉倍率未按强度递增: %v / %v / %v", strongZoom, stdZoom, subtleZoom)
	}
	if !(strongPan > stdPan && stdPan > subtlePan) {
		t.Fatalf("平移倍率未按强度递增: %v / %v / %v", strongPan, stdPan, subtlePan)
	}

	z, _, _ := kenBurnsExpr(port.MotionPushIn, 30, port.MotionStrengthStrong)
	if !strings.HasPrefix(z, "1.0+0.42*") {
		t.Fatalf("强档推近表达式异常: %s", z)
	}
	z, _, _ = kenBurnsExpr(port.MotionPushIn, 30, port.MotionStrengthSubtle)
	if !strings.HasPrefix(z, "1.0+0.16*") {
		t.Fatalf("弱档推近表达式异常: %s", z)
	}
	// 定格与幅度无关。
	if z, x, y := kenBurnsExpr(port.MotionStatic, 30, port.MotionStrengthStrong); z != "1.0" || x != "0" || y != "0" {
		t.Fatalf("定格不应受强度影响: %q %q %q", z, x, y)
	}
}
