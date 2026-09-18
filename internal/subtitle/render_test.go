package subtitle

import (
	"bytes"
	"image/png"
	"os"
	"testing"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
)

func TestRenderChinese(t *testing.T) {
	r, err := DefaultRenderer()
	if err != nil {
		t.Skipf("跳过：系统无可用中文字体: %v", err)
	}
	const w, h = 720, 1280
	pngBytes, err := r.Render("战国末年，群雄逐鹿。一位神秘的隐士走进了历史。", w, h, 2)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		t.Fatalf("decode png: %v", err)
	}
	b := img.Bounds()
	if b.Dx() != w || b.Dy() != h {
		t.Fatalf("尺寸 = %dx%d, 期望 %dx%d", b.Dx(), b.Dy(), w, h)
	}
	// 至少应有一批非透明像素（文字+底框）。
	var ink int
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a != 0 {
				ink++
			}
		}
	}
	if ink < 1000 {
		t.Fatalf("非透明像素 %d 过少，疑似未渲染出文字", ink)
	}
	// 留一份样例供目视检查。
	if err := os.WriteFile("/tmp/story-sub-demo.png", pngBytes, 0o644); err != nil {
		t.Log(err)
	}
}

func TestWrapText(t *testing.T) {
	r, err := DefaultRenderer()
	if err != nil {
		t.Skipf("跳过：系统无可用中文字体: %v", err)
	}
	face, err := opentype.NewFace(r.font, &opentype.FaceOptions{
		Size: 48, DPI: 72, Hinting: font.HintingFull,
	})
	if err != nil {
		t.Fatal(err)
	}
	lines := wrapText(face, "一二三四五六七八九十一二三四五六七八九十", 300, 2)
	if len(lines) != 2 {
		t.Fatalf("行数 = %d, 期望 2: %v", len(lines), lines)
	}
}
