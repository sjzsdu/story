package capability

import (
	"strings"
	"sync"
	"testing"
)

type fakeImpl struct{ name string }

func newTestSlot(t *testing.T) *Slot[fakeImpl] {
	t.Helper()
	s := NewSlot[fakeImpl]("语音合成")
	if err := s.Register("bailian", fakeImpl{"bailian"}); err != nil {
		t.Fatalf("Register bailian: %v", err)
	}
	if err := s.Register("minimax", fakeImpl{"minimax"}); err != nil {
		t.Fatalf("Register minimax: %v", err)
	}
	if err := s.SetDefault("bailian"); err != nil {
		t.Fatalf("SetDefault: %v", err)
	}
	return s
}

// Resolve 三分支：显式 key 命中 / 空 key 走默认 / 显式 key 未登记报错。
func TestResolveThreeBranches(t *testing.T) {
	s := newTestSlot(t)

	// 分支一：显式 key 命中（系列覆盖生效）。
	got, impl, err := s.Resolve("minimax")
	if err != nil {
		t.Fatalf("Resolve(minimax): %v", err)
	}
	if got != "minimax" || impl.name != "minimax" {
		t.Fatalf("Resolve(minimax) = (%q, %+v), want minimax", got, impl)
	}

	// 分支二：空 key 走默认（未覆盖 → 系统默认）。
	got, impl, err = s.Resolve("")
	if err != nil {
		t.Fatalf("Resolve(\"\"): %v", err)
	}
	if got != "bailian" || impl.name != "bailian" {
		t.Fatalf("Resolve(\"\") = (%q, %+v), want default bailian", got, impl)
	}

	// 分支三：显式 key 未登记 → 报错（绝不静默回退默认），且列出已登记项。
	_, _, err = s.Resolve("zhipu")
	if err == nil {
		t.Fatal("Resolve(zhipu) 应报错（显式未知不允许静默回退）")
	}
	msg := err.Error()
	if !strings.Contains(msg, "zhipu") || !strings.Contains(msg, "bailian") || !strings.Contains(msg, "minimax") {
		t.Fatalf("错误文案应含未知 key 与已登记项，got: %s", msg)
	}
	if !strings.Contains(msg, "语音合成") {
		t.Fatalf("错误文案应含能力展示名，got: %s", msg)
	}
}

// 未配置默认时，空 key 也报错（而不是返回零值）。
func TestResolveWithoutDefaultErrors(t *testing.T) {
	s := NewSlot[fakeImpl]("图片生成")
	if err := s.Register("bailian", fakeImpl{"bailian"}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	_, _, err := s.Resolve("")
	if err == nil {
		t.Fatal("未设置默认时 Resolve(\"\") 应报错")
	}
	if !strings.Contains(err.Error(), "bailian") {
		t.Fatalf("错误应列出已登记项，got: %s", err)
	}
}

// nil 槽的读方法安全（可选能力枚举时用）。
func TestNilSlotReadsSafe(t *testing.T) {
	var s *Slot[fakeImpl]
	if s.Keys() != nil || s.Default() != "" || s.Has("x") {
		t.Fatal("nil 槽读方法应返回零值")
	}
	if _, _, err := s.Resolve(""); err == nil {
		t.Fatal("nil 槽 Resolve 应报错")
	}
	if s.Name() != "" {
		t.Fatal("nil 槽 Name 应为空")
	}
}

func TestRegisterValidation(t *testing.T) {
	s := NewSlot[fakeImpl]("视频生成")
	if err := s.Register("", fakeImpl{}); err == nil {
		t.Fatal("空 key 应报错")
	}
	if err := s.Register("bailian", fakeImpl{}); err != nil {
		t.Fatalf("首次 Register: %v", err)
	}
	if err := s.Register("bailian", fakeImpl{}); err == nil {
		t.Fatal("重复 Register 应报错")
	}
}

func TestSetDefaultValidates(t *testing.T) {
	s := NewSlot[fakeImpl]("视频生成")
	if err := s.Register("bailian", fakeImpl{}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	err := s.SetDefault("kling")
	if err == nil {
		t.Fatal("SetDefault 未登记 key 应报错")
	}
	if !strings.Contains(err.Error(), "bailian") {
		t.Fatalf("错误应列出可用项，got: %s", err)
	}
	if s.Default() != "" {
		t.Fatalf("失败的 SetDefault 不应改动默认，got %q", s.Default())
	}
	if err := s.SetDefault("bailian"); err != nil {
		t.Fatalf("SetDefault 已登记 key: %v", err)
	}
	if s.Default() != "bailian" {
		t.Fatalf("Default = %q, want bailian", s.Default())
	}
}

// ReplaceAll：defaultKey 必须在新表中；空 defaultKey 允许（枚举槽）；整表原子生效。
func TestReplaceAll(t *testing.T) {
	s := newTestSlot(t)

	// defaultKey 不在新表 → 报错且不改动旧表。
	if err := s.ReplaceAll(map[string]fakeImpl{"kling": {"kling"}}, "bailian"); err == nil {
		t.Fatal("defaultKey 不在新表应报错")
	}
	if got, _, err := s.Resolve(""); err != nil || got != "bailian" {
		t.Fatalf("失败的 ReplaceAll 不应改动旧表，got (%q, %v)", got, err)
	}

	// 正常替换：默认与实现表同时换新。
	if err := s.ReplaceAll(map[string]fakeImpl{"kling": {"kling"}, "bailian": {"bailian2"}}, "kling"); err != nil {
		t.Fatalf("ReplaceAll: %v", err)
	}
	if got, impl, err := s.Resolve(""); err != nil || got != "kling" || impl.name != "kling" {
		t.Fatalf("Replace后 Resolve = (%q, %+v, %v), want kling", got, impl, err)
	}
	if _, _, err := s.Resolve("minimax"); err == nil {
		t.Fatal("被替换掉的实现不应再解析得到")
	}

	// 空 defaultKey 合法（＝无默认，只做枚举）。
	if err := s.ReplaceAll(map[string]fakeImpl{"a": {"a"}}, ""); err != nil {
		t.Fatalf("空 defaultKey 应允许: %v", err)
	}
	if s.Default() != "" {
		t.Fatalf("Default = %q, want 空", s.Default())
	}
	if _, _, err := s.Resolve(""); err == nil {
		t.Fatal("无默认时 Resolve(\"\") 应报错")
	}
	if got, _, err := s.Resolve("a"); err != nil || got != "a" {
		t.Fatalf("显式 key 仍应可用, got (%q, %v)", got, err)
	}

	// 空 key 条目拒绝。
	if err := s.ReplaceAll(map[string]fakeImpl{"": {"x"}}, ""); err == nil {
		t.Fatal("新表含空 key 应报错")
	}
}

func TestKeysSorted(t *testing.T) {
	s := NewSlot[fakeImpl]("发布")
	for _, k := range []string{"weixin", "douyin", "bilibili"} {
		if err := s.Register(k, fakeImpl{k}); err != nil {
			t.Fatalf("Register %s: %v", k, err)
		}
	}
	got := strings.Join(s.Keys(), ",")
	if want := "bilibili,douyin,weixin"; got != want {
		t.Fatalf("Keys() = %q, want %q", got, want)
	}
}

// 并发：Resolve 与 ReplaceAll 并发跑，race detector 下必须干净，且解析结果不允许出现中间态。
func TestConcurrentResolveAndReplaceAll(t *testing.T) {
	s := NewSlot[fakeImpl]("语音合成")
	base := map[string]fakeImpl{"bailian": {"bailian"}}
	if err := s.ReplaceAll(base, "bailian"); err != nil {
		t.Fatalf("ReplaceAll: %v", err)
	}

	var wg sync.WaitGroup

	// 写者：反复整表替换（bailian/minimax 交替作默认）。
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 5000; i++ {
			def := "bailian"
			if i%2 == 1 {
				def = "minimax"
			}
			m := map[string]fakeImpl{"bailian": {"bailian"}, "minimax": {"minimax"}}
			if err := s.ReplaceAll(m, def); err != nil {
				t.Errorf("ReplaceAll: %v", err)
				return
			}
		}
	}()

	// 读者：空 key 解析必须总是落在默认上（不允许零值或中间态）。
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 2000; j++ {
				key, impl, err := s.Resolve("")
				if err != nil {
					t.Errorf("Resolve: %v", err)
					return
				}
				if key == "" || impl.name != key {
					t.Errorf("解析到中间态: key=%q impl=%q", key, impl.name)
					return
				}
			}
		}()
	}
	wg.Wait()
}

// 并发 Register/SetDefault 的基本 race 覆盖。
func TestConcurrentRegister(t *testing.T) {
	s := NewSlot[fakeImpl]("图片生成")
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := string(rune('a' + i))
			if err := s.Register(key, fakeImpl{key}); err != nil {
				t.Errorf("Register %s: %v", key, err)
			}
		}(i)
	}
	wg.Wait()
	if len(s.Keys()) != 16 {
		t.Fatalf("Keys = %v, want 16 项", s.Keys())
	}
}

// 目录不变式：12 项能力、字段成对、已知 key 都有展示文案。
func TestCatalogInvariants(t *testing.T) {
	cat := Catalog()
	if len(cat) != 12 {
		t.Fatalf("Catalog 项数 = %d, want 12", len(cat))
	}
	seen := map[string]bool{}
	for _, c := range cat {
		if c.Key == "" || c.Label == "" || c.Help == "" {
			t.Fatalf("能力 %q 缺少 key/label/help", c.Key)
		}
		if seen[c.Key] {
			t.Fatalf("能力 key 重复: %q", c.Key)
		}
		seen[c.Key] = true
		// 有系统默认配置项的，也应有对应系列字段（除非该能力不支持系列覆盖）。
		if c.SeriesField != "" && c.ConfigField == "" {
			t.Fatalf("能力 %q 有系列字段却无配置字段", c.Key)
		}
		for _, pk := range c.Providers {
			if _, ok := providerLabels[pk]; !ok {
				t.Fatalf("能力 %q 的 provider %q 未收录展示名", c.Key, pk)
			}
		}
	}
	// 能力字段与目录对齐（前端靠这两个字段定位下拉绑定项）。
	for _, c := range cat {
		switch c.Key {
		case Text, Board, Plan:
			if c.ConfigField != "text_provider" || c.SeriesField != "text_provider" {
				t.Fatalf("能力 %q 字段 = (%q,%q), want text_provider/text_provider", c.Key, c.ConfigField, c.SeriesField)
			}
		case TTS, Image, Video:
			want := c.Key + "_provider"
			if c.SeriesField != want {
				t.Fatalf("能力 %q 系列字段 = %q, want %q", c.Key, c.SeriesField, want)
			}
		case VoiceBuild, VoiceList, Publish:
			if c.ConfigField != "" || c.SeriesField != "" {
				t.Fatalf("能力 %q 不应有配置/系列字段", c.Key)
			}
		case ImageUnderstand, VideoUnderstand, SFX:
			// §22：有系统默认配置字段，但本轮不做系列级覆盖。
			want := c.Key + "_provider"
			if c.ConfigField != want || c.SeriesField != "" {
				t.Fatalf("能力 %q 字段 = (%q,%q), want (%q,空)", c.Key, c.ConfigField, c.SeriesField, want)
			}
		}
	}
	// §22：bl 尚无音效命令 → sfx 是唯一允许空 Providers 的能力（槽位已立、实现未到）。
	for _, c := range cat {
		if c.Key == SFX {
			if len(c.Providers) != 0 {
				t.Fatalf("sfx 目录 Providers = %v, want 空（首个 provider 落地后本断言随特性调整）", c.Providers)
			}
			continue
		}
		if len(c.Providers) == 0 {
			t.Fatalf("能力 %q 目录 Providers 为空", c.Key)
		}
	}
}

func TestOrderProviders(t *testing.T) {
	var c Capability
	for _, cc := range Catalog() {
		if cc.Key == Publish {
			c = cc
		}
	}
	got := orderProviders(c, []string{"weixin", "douyin", "unknown-p", "bilibili"})
	want := []string{"douyin", "bilibili", "weixin", "unknown-p"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("orderProviders = %v, want %v", got, want)
	}
	// 无静态顺序 → 整体升序。
	got = orderProviders(Capability{}, []string{"b", "a"})
	if strings.Join(got, ",") != "a,b" {
		t.Fatalf("无顺序时 = %v, want a,b", got)
	}
}
