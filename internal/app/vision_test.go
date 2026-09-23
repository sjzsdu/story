package app

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sjzsdu/story/internal/capability"
	"github.com/sjzsdu/story/internal/config"
	"github.com/sjzsdu/story/internal/port"
	"github.com/sjzsdu/story/testutil/mock"
)

// newVisionApp 起一个带配置路径的 Bootstrap 实例（§22 测试共用）。
func newVisionApp(t *testing.T) *App {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Default()
	cfg.DataDir = dir
	cfg.VisionModel = "qwen3-vl-plus-test"
	a, err := Bootstrap(context.Background(), cfg, filepath.Join(dir, "story.yaml"))
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })
	return a
}

// TestCapabilityInfoHas12Entries §22 后能力目录 12 项；三个新能力的
// 运行时状态如实反映装配结果：图/视频理解默认 bailian，sfx 空槽（无实现无默认）。
func TestCapabilityInfoHas12Entries(t *testing.T) {
	a := newVisionApp(t)
	info := a.CapabilityInfo()
	if len(info.Capabilities) != 12 {
		t.Fatalf("能力数 = %d，期望 12", len(info.Capabilities))
	}
	byKey := map[string]capability.CapabilityInfo{}
	for _, c := range info.Capabilities {
		byKey[c.Key] = c
	}
	iv, ok := byKey[capability.ImageUnderstand]
	if !ok || iv.Default != "bailian" || len(iv.Providers) != 1 {
		t.Fatalf("image_understand 装配异常: %+v", iv)
	}
	vv := byKey[capability.VideoUnderstand]
	if vv.Default != "bailian" || vv.SeriesField != "" {
		t.Fatalf("video_understand 装配异常: %+v", vv)
	}
	// sfx 三处降级之二：装配层登记空表 → 目录/快照显示无实现、无默认。
	sfxInfo := byKey[capability.SFX]
	if len(sfxInfo.Providers) != 0 || sfxInfo.Default != "" {
		t.Fatalf("sfx 应为空槽: %+v", sfxInfo)
	}
}

// TestDescribeWrappersInjectVisionModel App 包装层把 cfg.VisionModel 透传进请求
// （req.Model 为空时），显式 model 不被覆盖；override 单次指定实现生效。
func TestDescribeWrappersInjectVisionModel(t *testing.T) {
	a := newVisionApp(t)
	v := &mock.Visioner{Reply: "两个古装人物对坐"}
	if err := a.Engine.ImageUnderstand.Register("mock", v); err != nil {
		t.Fatal(err)
	}
	if err := a.Engine.VideoUnderstand.Register("mock", v); err != nil {
		t.Fatal(err)
	}

	// 空 model → 注入配置值。
	if _, err := a.DescribeImage(context.Background(),
		port.DescribeImageRequest{Image: "a.png"}, "mock"); err != nil {
		t.Fatalf("DescribeImage: %v", err)
	}
	if v.ImageReq.Model != "qwen3-vl-plus-test" {
		t.Fatalf("cfg.VisionModel 未注入: %+v", v.ImageReq)
	}
	// 显式 model 不被覆盖。
	if _, err := a.DescribeVideo(context.Background(),
		port.DescribeVideoRequest{Video: "v.mp4", Model: "explicit-model"}, "mock"); err != nil {
		t.Fatalf("DescribeVideo: %v", err)
	}
	if v.VideoReq.Model != "explicit-model" {
		t.Fatalf("显式 model 被覆盖: %+v", v.VideoReq)
	}
	// sfx 三处降级之三：包装层只注入预留模型，空槽错误照常上抛（不静默）。
	_, err := a.GenerateSoundEffect(context.Background(),
		port.SoundEffectRequest{Prompt: "木门吱呀", OutPath: filepath.Join(t.TempDir(), "sfx.wav")}, "")
	if err == nil || !strings.Contains(err.Error(), "音效生成") {
		t.Fatalf("sfx 空槽应上抛能力错误，得到: %v", err)
	}
}

// TestReconfigureKeepsNewSlots 热更新后三个新槽仍在（applyProviders 覆盖它们），
// 且配置的系统默认如实落到槽上。
func TestReconfigureKeepsNewSlots(t *testing.T) {
	a := newVisionApp(t)
	next := a.Config()
	next.ImageUnderstandProvider = "unknown-vendor"
	if err := a.Reconfigure(next); err != nil {
		t.Fatalf("Reconfigure: %v", err)
	}
	// 未知配置值兜底 bailian（defaultKeyFor 语义），槽可用。
	if got := capabilityDefault(t, a.CapabilityInfo(), capability.ImageUnderstand); got != "bailian" {
		t.Fatalf("image_understand 兜底默认 = %q，期望 bailian", got)
	}
	// sfx 空表 → defaultKeyFor 返回空串 → ReplaceAll 允许空默认，不报错。
	if got := capabilityDefault(t, a.CapabilityInfo(), capability.SFX); got != "" {
		t.Fatalf("sfx 默认 = %q，期望空", got)
	}
}
