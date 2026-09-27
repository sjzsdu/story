package templates

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// 去文章腔（去 AI 味）机械验收。规则移植自 Easel（github.com/ZJU-REAL/Easel，
// Apache-2.0）text-polisher 技能的中文去 AI 感权威清单（references/zh-ai-markers.md）
// 与 novel-writer 的 slopcheck 思路，按本仓库「题材解耦铁律」落在 templates 包，
// 是 StorySystemPrompt 去 AI 腔红线的代码侧镜像：prompt 管住生成，这里管住验收。
//
// 只拦「一眼假」的机器腔，不替代人工品鉴；按口播稿定位做了裁剪（与 Easel 全表
// 的有意取舍）：说书稿允许破折号，「与此同时」「接下来」等口语转场计入文章腔
// 总数而非一票否决。规则明细见 AGENTS.md §25。

// storyHardFlaws 硬伤词：书面论述的标志性废话，说书稿出现任意一处即验收失败（零容忍）。
var storyHardFlaws = []string{
	"综上所述",
	"值得注意的是",
	"值得一提的是",
	"不难发现",
	"不可否认",
	"毋庸置疑",
	"众所周知",
}

// storyEssayFlaws 文章腔词：累计命中超过 storyEssayFlawLimit 处才失败。
var storyEssayFlaws = []string{
	"总的来说",
	"接下来",
	"让我们",
	"简而言之",
	"毫无疑问",
	"本质上",
	"换句话说",
	"某种程度上",
	"一方面",
	"与此同时",
	"这意味着",
	"值得深思",
	"令人深思",
	"发人深省",
	"标志着",
	"见证了",
	"彰显了",
	"凸显了",
	"体现了",
	"与此相关的是",
}

// slopPattern 一条正则规则：desc 供错误信息展示。
type slopPattern struct {
	desc string
	re   *regexp.Regexp
}

// storyHardPatterns 硬伤句式：AGENTS.md §7 明令禁止的评论家口吻公式。
var storyHardPatterns = []slopPattern{
	{desc: "「最狠/最可怕的地方」式评论家句式", re: regexp.MustCompile(`最(狠|可怕|残忍|残酷|讽刺|毒)的地方`)},
	{desc: "「真正……的不是……而是……」式评论家句式", re: regexp.MustCompile(`真正[^。]{0,12}的不是`)},
	{desc: "「这故事告诉我们」式说理收束", re: regexp.MustCompile(`这[故个段]事(告诉|最想告诉)`)},
}

// storyEssayPatterns 文章腔句式：计入文章腔总数。
var storyEssayPatterns = []slopPattern{
	{desc: "段首「首先/其次/再次，」序号转场", re: regexp.MustCompile(`(?m)^\s*(首先|其次|再次)[，,]`)},
	{desc: "「通过……来……」句式", re: regexp.MustCompile(`通过[^。，；]{1,12}来`)},
	{desc: "「不仅……而且/更……」句式", re: regexp.MustCompile(`不仅[^。]{0,12}(而且|更|还)`)},
	{desc: "「（经历/故事/历史）告诉我们」式升华", re: regexp.MustCompile(`(经历|故事|历史)[^。]{0,6}告诉我们`)},
}

// storyEssayFlawLimit 文章腔容忍阈值：正文命中总数超过该值即验收失败。
// 1200-1800 字的口播稿里 3 处以内属正常口语波动，再多说明在写文章、不在讲故事。
const storyEssayFlawLimit = 3

// slopHit 一条规则的命中情况。
type slopHit struct {
	name  string
	count int
}

// countWords 统计字面词命中。
func countWords(text string, words []string) []slopHit {
	hits := make([]slopHit, 0, len(words))
	for _, w := range words {
		if n := strings.Count(text, w); n > 0 {
			hits = append(hits, slopHit{name: w, count: n})
		}
	}
	return hits
}

// countPatterns 统计正则规则命中。
func countPatterns(text string, ps []slopPattern) []slopHit {
	hits := make([]slopHit, 0, len(ps))
	for _, p := range ps {
		if n := len(p.re.FindAllString(text, -1)); n > 0 {
			hits = append(hits, slopHit{name: p.desc, count: n})
		}
	}
	return hits
}

// sortHits 按命中次数降序（同次数按名称稳定排序，保证错误信息可复现）。
func sortHits(hits []slopHit) {
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].count != hits[j].count {
			return hits[i].count > hits[j].count
		}
		return hits[i].name < hits[j].name
	})
}

// formatHits 把命中项格式成「「词」×N、「词」」的形式，最多展示 max 条。
func formatHits(hits []slopHit, max int) string {
	sortHits(hits)
	if len(hits) > max {
		hits = hits[:max]
	}
	parts := make([]string, 0, len(hits))
	for _, h := range hits {
		if h.count > 1 {
			parts = append(parts, fmt.Sprintf("「%s」×%d", h.name, h.count))
		} else {
			parts = append(parts, fmt.Sprintf("「%s」", h.name))
		}
	}
	return strings.Join(parts, "、")
}

// CheckStorySlop 对一篇定稿故事做去文章腔机械验收：硬伤词/评论家句式零容忍，
// 正文文章腔合计超过阈值判废。title/summary/content 分别为标题、梗概与正文；
// 标题与梗概只查硬伤（体量小，阈值无意义），返回 nil 表示通过。
func CheckStorySlop(title, summary, content string) error {
	full := content + "\n" + summary + "\n" + title
	hard := append(countWords(full, storyHardFlaws), countPatterns(full, storyHardPatterns)...)
	if len(hard) > 0 {
		return fmt.Errorf("命中说书稿硬伤（书面文章腔/AI 腔，出现即不合格）：%s。说书稿要用讲出口的白话；请换一版重写或人工修改后重试。", formatHits(hard, 8))
	}
	essay := append(countWords(content, storyEssayFlaws), countPatterns(content, storyEssayPatterns)...)
	total := 0
	for _, h := range essay {
		total += h.count
	}
	if total > storyEssayFlawLimit {
		return fmt.Errorf("文章腔标记过多（%d 处，上限 %d）：%s。这更像书面文章而不是口播讲述稿；请换一版重写或人工修改后重试。", total, storyEssayFlawLimit, formatHits(essay, 8))
	}
	return nil
}
