// Package subtitle 用纯 Go 把中文旁白渲染为透明 PNG（不依赖 ffmpeg libass）。
// provider/ffmpeg 再通过内置 overlay 滤镜按时间区间叠加到画面上。
package subtitle

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"strings"
	"unicode"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// defaultFontCandidates macOS 常见中文字体（按优先级）。
var defaultFontCandidates = []string{
	"/System/Library/Fonts/PingFang.ttc",
	"/System/Library/Fonts/Hiragino Sans GB.ttc",
	"/System/Library/Fonts/STHeiti Medium.ttc",
	"/System/Library/Fonts/Supplemental/Songti.ttc",
	"/System/Library/Fonts/Supplemental/Arial Unicode.ttf",
}

// Renderer 字幕渲染器，加载一次字体后重复使用。
type Renderer struct {
	font *opentype.Font
}

// NewRenderer 从字体文件加载（.ttf/.otf/.ttc，ttc 优先选常规体中文面）。
func NewRenderer(fontPath string) (*Renderer, error) {
	data, err := os.ReadFile(fontPath)
	if err != nil {
		return nil, fmt.Errorf("读取字体 %s: %w", fontPath, err)
	}
	f, err := parseFont(data)
	if err != nil {
		return nil, fmt.Errorf("解析字体 %s: %w", fontPath, err)
	}
	return &Renderer{font: f}, nil
}

// DefaultRenderer 自动探测系统中文字体。
func DefaultRenderer() (*Renderer, error) {
	for _, p := range defaultFontCandidates {
		if _, err := os.Stat(p); err != nil {
			continue
		}
		if r, err := NewRenderer(p); err == nil {
			return r, nil
		}
	}
	return nil, fmt.Errorf("未找到可用中文字体，可在配置 subtitle_font 中指定")
}

// Render 把文本渲染到 width×height 的透明图上，返回 PNG 字节。
// 文本自动按宽度换行（中文按字断行），最多 maxLines 行。
func (r *Renderer) Render(text string, width, height int, maxLines int) ([]byte, error) {
	if maxLines <= 0 {
		maxLines = 2
	}
	scale := float64(height) / 1920
	fontSize := 76 * scale
	px := func(v float64) int { return int(v*scale + 0.5) }

	face, err := opentype.NewFace(r.font, &opentype.FaceOptions{
		Size:    fontSize,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return nil, err
	}
	defer face.Close()

	marginX := px(110)
	maxTextWidth := width - 2*marginX
	lines := wrapText(face, text, maxTextWidth, maxLines)

	lineHeight := fontSize * 1.35
	blockHeight := float64(len(lines)) * lineHeight
	blockBottom := float64(height) - float64(px(170))
	blockTop := blockBottom - blockHeight

	// 计算文本块最大宽度以居中。
	maxW := 0
	widths := make([]int, len(lines))
	for i, ln := range lines {
		w := measure(face, ln).Ceil()
		widths[i] = w
		if w > maxW {
			maxW = w
		}
	}

	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// 半透明黑色底框，提升可读性。
	padX, padY := px(34), px(22)
	box := image.Rect(
		(width-maxW)/2-padX,
		int(blockTop)-padY,
		(width+maxW)/2+padX,
		int(blockBottom)+padY,
	)
	draw.Draw(img, box, &image.Uniform{color.RGBA{0, 0, 0, 110}}, image.Point{}, draw.Over)

	ascent := face.Metrics().Ascent.Ceil()
	for i, ln := range lines {
		x := (width - widths[i]) / 2
		baseline := int(blockTop) + ascent + int(float64(i)*lineHeight)
		drawStringOutlined(img, face, ln, x, baseline, px(6))
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// drawStringOutlined 以黑色描边 + 白色填充绘制一行文字。
func drawStringOutlined(dst *image.RGBA, face font.Face, s string, x, y, outlinePx int) {
	d := &font.Drawer{
		Face: face,
		Dst:  dst,
		Src:  image.NewUniform(color.RGBA{0, 0, 0, 255}),
		Dot:  fixed.Point26_6{X: fixed.I(x), Y: fixed.I(y)},
	}
	for dx := -outlinePx; dx <= outlinePx; dx++ {
		for dy := -outlinePx; dy <= outlinePx; dy++ {
			if dx == 0 && dy == 0 {
				continue
			}
			d.Dot = fixed.Point26_6{X: fixed.I(x + dx), Y: fixed.I(y + dy)}
			d.DrawString(s)
		}
	}
	d.Src = image.NewUniform(color.RGBA{255, 255, 255, 255})
	d.Dot = fixed.Point26_6{X: fixed.I(x), Y: fixed.I(y)}
	d.DrawString(s)
}

// measure 测量字符串在指定字体下的像素宽度。
func measure(face font.Face, s string) fixed.Int26_6 {
	d := font.Drawer{Face: face}
	return d.MeasureString(s)
}

// wrapText 按像素宽度逐字折行，超过 maxLines 时最后一行加省略号。
func wrapText(face font.Face, text string, maxWidthPx, maxLines int) []string {
	maxWidth := fixed.I(maxWidthPx)
	var lines []string
	var buf []rune
	flush := func() {
		lines = append(lines, string(buf))
		buf = buf[:0]
	}

	for _, r := range text {
		if r == '\n' {
			flush()
			continue
		}
		buf = append(buf, r)
		if measure(face, string(buf)) > maxWidth {
			// 当前字超宽：当前字留在新行，已积累的成行。
			overflow := buf[len(buf)-1]
			buf = buf[:len(buf)-1]
			if len(buf) > 0 {
				flush()
			}
			buf = append(buf, overflow)
		}
	}
	if len(buf) > 0 {
		flush()
	}
	if len(lines) == 0 {
		return nil
	}
	if len(lines) > maxLines {
		kept := append([]string(nil), lines[:maxLines]...)
		last := []rune(kept[maxLines-1])
		for len(last) > 0 && measure(face, string(last)+"…") > maxWidth {
			last = last[:len(last)-1]
		}
		kept[maxLines-1] = string(last) + "…"
		return kept
	}
	return lines
}

// parseFont 支持 ttc 集合（优先中文常规体）与单体 ttf/otf。
func parseFont(data []byte) (*opentype.Font, error) {
	if coll, err := opentype.ParseCollection(data); err == nil {
		n := coll.NumFonts()
		var buf sfnt.Buffer
		// 优先选择 PingFang SC / 冬青黑体等常规体。
		for i := 0; i < n; i++ {
			f, err := coll.Font(i)
			if err != nil {
				continue
			}
			name, _ := f.Name(&buf, sfnt.NameIDFamily)
			if containsCJKFamily(name) {
				sub, _ := f.Name(&buf, sfnt.NameIDSubfamily)
				if sub == "Regular" || sub == "常规体" || sub == "" {
					return f, nil
				}
			}
		}
		if f, err := coll.Font(0); err == nil {
			return f, nil
		}
	}
	return opentype.Parse(data)
}

func containsCJKFamily(name string) bool {
	for _, r := range name {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	for _, kw := range []string{"PingFang", "Heiti", "Hiragino Sans GB", "Songti"} {
		if strings.Contains(name, kw) {
			return true
		}
	}
	return false
}
