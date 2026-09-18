package app

import "testing"

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"鬼谷子":     "guiguzi",
		"三国":      "sanguo",
		"Tang 盛唐": "tang-shengtang",
		"":        "series-",
	}
	for in, prefix := range cases {
		got := Slugify(in)
		if in == "" {
			// 空名称走时间戳兜底，只检查前缀。
			if len(got) < len("series-") || got[:7] != "series-" {
				t.Fatalf("空名称 slug 异常: %s", got)
			}
			continue
		}
		if got != prefix {
			t.Errorf("Slugify(%q) = %q, 期望 %q", in, got, prefix)
		}
	}
}
