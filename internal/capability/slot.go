// Package capability 是零依赖的「能力注册表」：按能力槽（text/board/plan/tts/image/
// video/image_understand/video_understand/sfx/voice_build/voice_list/publish）登记若干
// 具名实现，并沿「系列覆盖 → 系统默认」两级链路解析出当次该用的实现。
//
// 本包不 import engine/port/provider/store 的任何一方（纯 Go + 标准库），engine、app、
// server、cmd 全部依赖它；provider 的具体实现只在装配层（internal/app、cmd/story）注册进来，
// 与 AGENTS.md §3 的 Ports & Adapters 铁律一致。
//
// 设计要点：
//   - 泛型 Slot[T]：一张表管一种接口类型，取代过去 engine 里手写 map + `RegisterProvider(any)`
//     的类型 switch（那种写法首个命中即返回，导致系列覆盖大面积静默失效）。
//   - 显式未知一律报错并列出已登记项，绝不静默回退默认（否则「系列覆盖了但没生效」无从发现）；
//     只有 key 为空才是「未指定 → 用默认」的合法回退。
//   - ReplaceAll 整表原子替换，用于设置热更新（Reconfigure）。
package capability

import (
	"fmt"
	"sort"
	"sync"
)

// DefaultKey 是默认保留 key：装配层在尚不知配置取值时先用它登记默认实现，
// 之后 ReplaceAll 会升级为具名 key（如 "bailian"）。
const DefaultKey = "default"

// Slot 一张能力槽：具名实现表 + 默认 key。
//
// 零值不可用（name 为空），请用 NewSlot 创建；写方法带值接收者是错误用法（改不到锁），
// 因此全部方法用指针接收者。读方法对 nil 槽安全（返回空），便于对可选能力做枚举。
type Slot[T any] struct {
	name string

	mu       sync.RWMutex
	impls    map[string]T
	defaultK string
}

// NewSlot 创建能力槽。name 是中文展示名（仅用于错误文案），如「语音合成」。
func NewSlot[T any](name string) *Slot[T] {
	return &Slot[T]{name: name, impls: map[string]T{}}
}

// Name 返回能力槽展示名。
func (s *Slot[T]) Name() string {
	if s == nil {
		return ""
	}
	return s.name
}

// Register 登记一个实现。key 为空或重复登记都报错。
// 同 key 重复 Register 是装配 bug（配置里出现了重复的供应商取值），故显式报错。
func (s *Slot[T]) Register(key string, impl T) error {
	if s == nil {
		return fmt.Errorf("能力槽未初始化")
	}
	if key == "" {
		return fmt.Errorf("能力「%s」的实现 key 不能为空", s.name)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.impls[key]; ok {
		return fmt.Errorf("能力「%s」重复登记实现 %q", s.name, key)
	}
	s.impls[key] = impl
	return nil
}

// SetDefault 设置默认实现 key，key 必须已登记。
func (s *Slot[T]) SetDefault(key string) error {
	if s == nil {
		return fmt.Errorf("能力槽未初始化")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.impls[key]; !ok {
		return fmt.Errorf("能力「%s」默认实现 %q 未登记（可用：%s）", s.name, key, joinKeys(s.impls))
	}
	s.defaultK = key
	return nil
}

// Default 返回当前默认 key（空串＝尚未设置）。
func (s *Slot[T]) Default() string {
	if s == nil {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.defaultK
}

// Has 判断 key 是否已登记。
func (s *Slot[T]) Has(key string) bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.impls[key]
	return ok
}

// Keys 返回全部已登记 key（升序）。nil 槽返回空切片。
func (s *Slot[T]) Keys() []string {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return sortedKeys(s.impls)
}

// Resolve 沿「系列覆盖 → 系统默认」两级链路解析当次实现：
//
//	key 非空：必须已登记，否则报错（显式指定却没接上，绝不静默回退）并列出已登记项；
//	key 为空：取默认 key；默认也未设置则报错。
//
// 返回值带解析后的实际 key（非空 key 时等于入参，为空时等于默认 key），便于日志与测试断言。
func (s *Slot[T]) Resolve(key string) (string, T, error) {
	var zero T
	if s == nil {
		return "", zero, fmt.Errorf("能力槽未初始化")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	try := key
	if try == "" {
		try = s.defaultK
	}
	if try == "" {
		return "", zero, fmt.Errorf("能力「%s」未配置默认实现，且未指定实现（可用：%s）",
			s.name, joinKeys(s.impls))
	}
	impl, ok := s.impls[try]
	if !ok {
		if key != "" {
			return "", zero, fmt.Errorf("能力「%s」未登记实现 %q（已登记：%s）",
				s.name, try, joinKeys(s.impls))
		}
		return "", zero, fmt.Errorf("能力「%s」默认实现 %q 未登记（已登记：%s）",
			s.name, try, joinKeys(s.impls))
	}
	return try, impl, nil
}

// ReplaceAll 整表原子替换（用于设置热更新）：一次写锁内换掉实现表与默认 key，
// 并发 Resolve 不会看到中间态。defaultKey 非空时必须出现在新表中；
// 空 defaultKey 允许（＝暂无默认，用于只做枚举的槽，如 publish）。
func (s *Slot[T]) ReplaceAll(impls map[string]T, defaultKey string) error {
	if s == nil {
		return fmt.Errorf("能力槽未初始化")
	}
	next := make(map[string]T, len(impls))
	for k, v := range impls {
		if k == "" {
			return fmt.Errorf("能力「%s」的实现 key 不能为空", s.name)
		}
		next[k] = v
	}
	if defaultKey != "" {
		if _, ok := next[defaultKey]; !ok {
			return fmt.Errorf("能力「%s」默认实现 %q 不在新登记表中（可用：%s）",
				s.name, defaultKey, joinKeys(next))
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.impls = next
	s.defaultK = defaultKey
	return nil
}

// sortedKeys 返回升序 key 切片（map 遍历无序，统一排序保证稳定）。
func sortedKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// joinKeys 把 key 集合排序后拼成「a、b、c」（空表返回「（无）」）。
// map 遍历无序，这里统一排序，保证错误文案稳定可断言。
func joinKeys[T any](m map[string]T) string {
	if len(m) == 0 {
		return "（无）"
	}
	keys := sortedKeys(m)
	out := ""
	for i, k := range keys {
		if i > 0 {
			out += "、"
		}
		out += k
	}
	return out
}
