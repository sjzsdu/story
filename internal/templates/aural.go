package templates

import (
	"fmt"
	"regexp"
	"strings"
)

// 听觉门禁（§27）：口播稿是写给耳朵的，全片只被「听」一次，听不懂就是
// 永远听不懂。本文件是 StorySystemPrompt【耳朵铁律】的代码侧镜像：
// prompt 管住生成，这里管住验收——只拦「机器可判定」的硬伤，不替代人工品鉴。
//
// 设计取舍：门禁失败＝整篇重写（一次文本调用），因此规则必须保守——
// 宁可漏判（留给人工审阅兜底），不可误杀（误杀会烧钱空转）。
// 每条规则都经过反例校准，见 aural_test.go。

// numChars 编年纪年的数字特征（汉字数字 + 阿拉伯数字）。
var numChars = regexp.MustCompile(`[一二三四五六七八九十百千两零〇0-9]`)

// relativeTimeWords 相对时间表述特征字：短语含这些字时不是编年纪年，放行
// （「那一年」「次年」「头一年」「过去一年」等）。
var relativeTimeWords = []string{
	"那", "这", "同", "次", "翌", "当", "来", "去", "明", "今", "往",
	"头", "连", "经", "隔", "哪", "某", "每", "第", "余", "过", "早",
	"晚", "终", "半", "多", "几", "数", "前", "后", "近",
}

// isChronicleClause 判断一个短句是否是「编年纪年」：
// 以「年」收尾、不含相对时间特征字、形如年号+数词或含数字
// （「地皇三年」「汉高祖十二年」「公元前202年」）。
func isChronicleClause(clause string) bool {
	c := strings.TrimSpace(clause)
	if !strings.HasSuffix(c, "年") {
		return false
	}
	body := strings.TrimSuffix(c, "年")
	r := []rune(body)
	if len(r) == 0 {
		return false
	}
	if strings.Contains(body, "公元") {
		return true // 公元前202年：含「前」但有明确公元锚点，是纪年
	}
	for _, rel := range relativeTimeWords {
		if strings.Contains(body, rel) {
			return false
		}
	}
	return len(r) <= 8 || numChars.MatchString(body)
}

// bioPattern 虚词/口语字：档案句的中间段（籍贯）不应出现这些字，
// 「他是一个卖谷的人」这类叙述句由此放行。
const bioFiller = "的在了着过是个有说想看做当来去"

// isBioOpening 判断一个句子是否是「生平档案句」：
// 「刘秀是南阳蔡阳人」「刘秀，南阳蔡阳人」「刘秀生于南阳蔡阳」这类起笔。
// 按整句（未切逗号）判断，档案式「X，Y人」依赖逗号在句内。
func isBioOpening(s string) bool {
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "。"))
	r := []rune(s)
	if len(r) < 4 {
		return false
	}
	// 「X生于Y」：X 为短人名。
	if i := runeIndex(s, "生于"); i > 0 && i <= 8 {
		return true
	}
	// 档案式「X，Y人」。
	if runeAt(r, len(r)-1) == '人' {
		if j := runeIndex(s, "，"); j > 0 && j <= 8 {
			name := string(r[:j])
			place := string(r[j+1 : len(r)-1])
			place = strings.TrimSuffix(place, "人")
			if isHanName(name) && place != "" && !strings.ContainsAny(place, bioFiller) {
				return true
			}
		}
	}
	// 判断式「X是Y人」。
	if j := runeIndex(s, "是"); j > 0 && j <= 8 {
		name := string(r[:j])
		tail := string(r[j+1:])
		if !strings.HasSuffix(tail, "人") {
			return false
		}
		mid := strings.TrimSuffix(tail, "人")
		if isHanName(name) && mid != "" && len([]rune(mid)) <= 12 &&
			!strings.ContainsAny(mid, bioFiller) {
			return true
		}
	}
	return false
}

// isHanName 短纯汉字人名（含间隔号）。
func isHanName(s string) bool {
	if s == "" {
		return false
	}
	if !regexp.MustCompile(`^[\p{Han}·]+$`).MatchString(s) {
		return false
	}
	return len([]rune(s)) <= 8
}

// runeIndex 按 rune 计算子串首次出现位置（未出现返回 -1）。
func runeIndex(s, sub string) int {
	b := strings.Index(s, sub)
	if b < 0 {
		return -1
	}
	return len([]rune(s[:b]))
}

// runeAt 取第 i 个 rune（i 已由调用方保证在界内）。
func runeAt(r []rune, i int) rune { return r[i] }

// sentences 把正文切成句子（按句号/问叹号/换行），只保留非空句。
func sentences(text string) []string {
	fields := strings.FieldsFunc(text, func(r rune) bool {
		return r == '。' || r == '？' || r == '?' || r == '！' || r == '!' || r == '\n'
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if s := strings.TrimSpace(f); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// clauses 把一个句子切成子句（按逗号/分号），保留顺序。
func clauses(s string) []string {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == '，' || r == ',' || r == '；' || r == ';'
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if c := strings.TrimSpace(f); c != "" {
			out = append(out, c)
		}
	}
	return out
}

// closingDeathVerbs 结尾「无关死亡事件句」的死亡/覆灭动词特征字。
const closingDeathVerbs = "死卒殁薨诛族灭戮"

// closingSubjectPronouns 结尾句含这些人称/指称时视为「与主角相关」，放行。
const closingSubjectPronouns = "他她我你谁"

// CheckStoryAural 对一篇定稿口播稿做听觉门禁验收，返回 nil 表示通过。
// 两条规则（均为保守启发式）：
//  1. 开场：前两句内不得以编年纪年或生平档案句起笔（背景滑落禁令）；
//  2. 结尾：最后一句不得是编年纪年句，也不得是「某路人死在某地」式
//     无关事件句（短句 + 第三者死亡/覆灭动词 + 无人物指称）。
func CheckStoryAural(title, content string) error {
	text := strings.TrimSpace(content)
	if text == "" {
		return nil // 空正文由字段验收负责，这里不重复报
	}
	// —— 开场检查：只看前两句 —— //
	ss := sentences(text)
	head := ss
	if len(head) > 2 {
		head = head[:2]
	}
	for i, s := range head {
		for _, c := range clauses(s) {
			if isChronicleClause(c) {
				return fmt.Errorf("开场第 %d 句以编年纪年起笔（「%s」）——耳朵听不出时间流动，属背景滑落；钩子三句内必须钉在「人正在做什么」上。请换一版重写或人工修改。", i+1, c)
			}
		}
		if isBioOpening(s) {
			return fmt.Errorf("开场第 %d 句是生平档案句（「%s」）——禁止从生平、籍贯起笔。请换一版重写或人工修改。", i+1, s)
		}
	}
	// —— 结尾检查：最后一句 —— //
	last := ss[len(ss)-1]
	for _, c := range clauses(last) {
		if isChronicleClause(c) {
			return fmt.Errorf("结尾落在编年纪年句上（「%s」）——最后一句必须落在主角的动作、决定或悬念上。请换一版重写或人工修改。", c)
		}
	}
	runes := len([]rune(last))
	if runes <= 22 && strings.ContainsAny(last, closingDeathVerbs) &&
		!strings.ContainsAny(last, closingSubjectPronouns) &&
		!strings.Contains(last, "？") && !strings.Contains(last, "?") {
		return fmt.Errorf("结尾是无关事件句（「%s」）——用听众不认识的第三者事件收尾，情绪会直接断电；想交代后续战局，要么把因果说透，要么不写。请换一版重写或人工修改。", last)
	}
	_ = title // 标题另有钩子规则，听觉门禁暂不校验标题（防误杀）
	return nil
}
