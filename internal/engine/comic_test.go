package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/sjzsdu/story/internal/domain"
	"github.com/sjzsdu/story/internal/port"
	"github.com/sjzsdu/story/internal/templates"
)

func TestMotionForScene(t *testing.T) {
	cases := []struct {
		camera string
		index  int
		want   string
	}{
		{"缓缓拉远", 0, port.MotionPullOut},
		{"推近至张仪面部", 0, port.MotionPushIn},
		{"左移扫过宴席", 0, port.MotionPanLeft},
		{"向右横移", 0, port.MotionPanRight},
		{"俯视整个厅堂", 0, port.MotionPanDown},
		{"仰拍高台", 0, port.MotionPanUp},
		{"定格在璧玉", 0, port.MotionStatic},
		{"", 0, port.MotionPushIn}, // 缺省走轮换表首项
		{"远景", 2, port.MotionPullOut},
		{"远景", 3, port.MotionPanLeft},
	}
	for _, tc := range cases {
		sc := domain.Scene{Camera: tc.camera}
		if got := motionForScene(sc, tc.index); got != tc.want {
			t.Errorf("camera=%q index=%d → %s, want %s", tc.camera, tc.index, got, tc.want)
		}
	}
}

func TestPanelImageSize(t *testing.T) {
	if got := panelImageSize("9:16"); got != "1080*1920" {
		t.Fatalf("9:16 → %s", got)
	}
	if got := panelImageSize("16:9"); got != "1920*1080" {
		t.Fatalf("16:9 → %s", got)
	}
	if got := panelImageSize("未知比例"); got != "1080*1920" {
		t.Fatalf("未知比例应回退竖屏尺寸, got %s", got)
	}
}

func TestBuildImagePromptInjectsAnchor(t *testing.T) {
	sc := domain.Scene{VisualPrompt: "楚相府堂，宴席灯火"}
	got := buildImagePrompt(sc, templates.MatchStyle(""))
	if !strings.Contains(got, "楚相府堂") {
		t.Fatal("prompt 应包含画面内容")
	}
	if !strings.Contains(got, "工笔重彩") {
		t.Fatalf("prompt 应追加统一画风锚句: %s", got)
	}
}

func TestNormalizeVisualMode(t *testing.T) {
	if domain.NormalizeVisualMode("") != domain.VisualModeComic {
		t.Fatal("空值应默认 comic")
	}
	if domain.NormalizeVisualMode("video") != domain.VisualModeVideo {
		t.Fatal("video 应保留")
	}
	if domain.NormalizeVisualMode("奇怪值") != domain.VisualModeComic {
		t.Fatal("未知值应回退 comic")
	}
}

// TestProduceComicMode 验证 comic 模式：出插画 + 本地静帧渲染，
// 不调用视频生成；续跑时插画与片段均复用。
func TestProduceComicMode(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	// 切换系列到小人书模式（默认即 comic，这里显式设置以固定意图）。
	se, err := f.repo.GetSeries(ctx, "guiguzi")
	if err != nil {
		t.Fatal(err)
	}
	se.Config.VisualMode = domain.VisualModeComic
	if err := f.repo.UpdateSeries(ctx, se); err != nil {
		t.Fatal(err)
	}

	if _, err := f.eng.GenerateStory(ctx, f.epID, DeriveOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.eng.PlanStoryboard(ctx, f.epID, DeriveOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.eng.Produce(ctx, f.epID, DeriveOptions{}); err != nil {
		t.Fatalf("comic produce: %v", err)
	}

	if calls := f.videos.CallsCount(); calls != 0 {
		t.Fatalf("comic 模式不应调用视频生成, calls=%d", calls)
	}
	if calls := f.images.CallsCount(); calls != 4 {
		t.Fatalf("4 镜应生成 4 张插画, calls=%d", calls)
	}
	if calls := f.composr.StillCalls(); calls != 4 {
		t.Fatalf("应渲染 4 个静帧片段, calls=%d", calls)
	}
	for _, r := range f.composr.StillRequests {
		if r.Motion == "" {
			t.Fatal("静帧渲染请求缺少运镜 key")
		}
		if r.ImagePath == "" || r.OutPath == "" {
			t.Fatal("静帧渲染请求路径不完整")
		}
	}

	// 续跑：插画与片段均已存在，不应再产生任何模型调用或渲染。
	if _, err := f.eng.Produce(ctx, f.epID, DeriveOptions{}); err != nil {
		t.Fatalf("comic 续跑: %v", err)
	}
	if calls := f.images.CallsCount(); calls != 4 {
		t.Fatalf("续跑不应重新出图, calls=%d", calls)
	}
	if calls := f.composr.StillCalls(); calls != 4 {
		t.Fatalf("续跑不应重新渲染, calls=%d", calls)
	}
	if calls := f.videos.CallsCount(); calls != 0 {
		t.Fatalf("续跑不应调用视频生成, calls=%d", calls)
	}
}
