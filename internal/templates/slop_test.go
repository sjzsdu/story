package templates

import (
	"strings"
	"testing"
)

// 合法样例：说书人白话，无文章腔标记。
const (
	okTitle   = "舌头还在吗"
	okSummary = "张仪被打得半死，回家先问舌头还在不在。"
	okContent = "张仪被人按在地上，打了几百板子。他抬起头，问妻子的第一句话不是喊疼，而是舌头还在不在。妻子哭着点头，他笑了一声，说还在就够。"
)

func TestCheckStorySlopPass(t *testing.T) {
	if err := CheckStorySlop(okTitle, okSummary, okContent); err != nil {
		t.Fatalf("干净的说书稿不应被拒: %v", err)
	}
}

func TestCheckStorySlopHardWordsZeroTolerance(t *testing.T) {
	// 每个硬伤词出现一次即拒。
	for _, w := range storyHardFlaws {
		content := strings.Replace(okContent, "他抬起头", w+"，他抬起头", 1)
		err := CheckStorySlop(okTitle, okSummary, content)
		if err == nil {
			t.Fatalf("硬伤词「%s」应导致验收失败", w)
		}
		if !strings.Contains(err.Error(), "硬伤") {
			t.Fatalf("硬伤词错误信息应含「硬伤」: %v", err)
		}
	}
}

func TestCheckStorySlopHardPatterns(t *testing.T) {
	cases := map[string]string{
		"最狠的地方":    "这故事最狠的地方不在刑罚，而在他问的那句话。",
		"真正……的不是":  "真正可怕的不是板子，是那句话。",
		"这故事告诉我们": "这故事告诉我们，舌头比命重要。",
	}
	for name, frag := range cases {
		if err := CheckStorySlop(okTitle, okSummary, okContent+frag); err == nil {
			t.Fatalf("评论家句式「%s」应导致验收失败", name)
		}
	}
}

func TestCheckStorySlopEssayThreshold(t *testing.T) {
	// 正文里恰好 storyEssayFlawLimit 处 → 通过（容忍少量口语波动）。
	exact := okContent
	for _, w := range []string{"总的来说", "毫无疑问", "与此同时"} {
		exact += "他提起" + w + "还是走了。"
	}
	if err := CheckStorySlop(okTitle, okSummary, exact); err != nil {
		t.Fatalf("文章腔恰好 %d 处应放行: %v", storyEssayFlawLimit, err)
	}
	// 正文里超过阈值 → 拒，且错误信息应含阈值说明。
	over := okContent
	for i := 0; i <= storyEssayFlawLimit; i++ {
		over += "接下来，我们看看下一段。"
	}
	err := CheckStorySlop(okTitle, okSummary, over)
	if err == nil {
		t.Fatalf("文章腔超过 %d 处应导致验收失败", storyEssayFlawLimit)
	}
	if !strings.Contains(err.Error(), "文章腔") {
		t.Fatalf("错误信息应说明文章腔原因: %v", err)
	}
}

func TestCheckStorySlopTitleAndSummaryHardOnly(t *testing.T) {
	// 标题/梗概命中硬伤 → 拒。
	if err := CheckStorySlop("值得注意的是", okSummary, okContent); err == nil {
		t.Fatal("标题命中硬伤词应导致验收失败")
	}
	if err := CheckStorySlop(okTitle, "综上所述，这事发生在战国。", okContent); err == nil {
		t.Fatal("梗概命中硬伤词应导致验收失败")
	}
	// 标题/梗概的文章腔不计入正文阈值（正文干净即放行）。
	if err := CheckStorySlop(okTitle, "总的来说，他赢了。", okContent); err != nil {
		t.Fatalf("标题/梗概的文章腔不应计入正文阈值: %v", err)
	}
}
