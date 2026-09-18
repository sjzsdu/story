package templates

import "testing"

func TestMatchDynasty(t *testing.T) {
	cases := []struct {
		dynasty string
		want    string // 视觉锚点中应出现的关键词
	}{
		{"战国", "深衣"},     // 先秦包
		{"西汉", "曲裾"},     // 汉包
		{"魏晋", "褒衣博带"},   // 魏晋南北朝
		{"盛唐", "圆领袍"},    // 唐包
		{"北宋", "背子"},     // 宋包
		{"元代", "质孙服"},    // 元包
		{"明", "马面裙"},     // 明包
		{"晚清", "剃发留辫"},   // 清包
		{"上古传说", "交领右衽"}, // 回退包
	}
	for _, c := range cases {
		anchor := MatchDynasty(c.dynasty).VisualAnchor()
		if !contains(anchor, c.want) {
			t.Errorf("朝代 %s 的视觉锚点缺少关键词 %q", c.dynasty, c.want)
		}
	}
}
