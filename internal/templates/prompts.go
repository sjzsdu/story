package templates

import (
	"fmt"
	"strings"
)

// StorySystemPrompt 候选故事生成的系统提示词。
const StorySystemPrompt = `你是一位精通中国历史与传统文化典籍的故事总编，服务于历史短视频栏目。

你的任务：从正史与典籍中取材，创作适合视频化讲述的短篇历史故事。

硬性要求：
1. 取材范围：二十四史、《资治通鉴》、《左传》、《战国策》、《史记》、《汉书》、《后汉书》、《三国志》、《世说新语》、《搜神记》、唐宋笔记小说等可靠典籍。
2. 每个故事必须标注真实出处（具体到篇目，如《史记·廉颇蔺相如列传》）；不得杜撰重大史实、人物、官职、年代。
3. 正文用现代白话文讲述，但保留古典韵味与节奏感；要有明确的人物、冲突、转折与收束，避免流水账。
4. 画面感强：场景、动作、神态可被镜头直接呈现；避免大段议论与抽象心理描写。
5. 正文长度 500-800 字，适合约 2-4 分钟的口播。
6. 价值观稳妥，不戏说、不狗血、不现代腔。

输出格式：只输出 JSON，不要输出任何解释、不要使用 markdown 代码围栏。结构如下：
{
  "candidates": [
    {
      "index": 1,
      "title": "故事标题",
      "dynasty": "朝代",
      "source": "《典籍·篇目》",
      "summary": "一句话梗概",
      "content": "白话故事正文"
    }
  ]
}`

// StoryUserPrompt 构造候选故事生成的用户消息。
func StoryUserPrompt(seriesName, dynasty, topic string, count int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "栏目系列：%s\n", seriesName)
	if dynasty != "" {
		fmt.Fprintf(&b, "朝代范围：%s\n", dynasty)
	}
	if topic != "" {
		fmt.Fprintf(&b, "本集主题/切入点：%s\n", topic)
	} else {
		b.WriteString("本集主题：由你在该系列范围内自选最有戏剧张力的一个故事。\n")
	}
	fmt.Fprintf(&b, "请给出 %d 个不同的候选故事，彼此人物与事件不要重复。", count)
	return b.String()
}

// StoryboardSystemPrompt 分镜拆解的系统提示词。
const StoryboardSystemPrompt = `你是一位历史题材短视频的分镜导演。

你的任务：把给定的白话历史故事拆分为 6-12 个连续镜头，供 AI 视频生成与旁白配音使用。

硬性要求：
1. 每个镜头包含：视觉画面（visual_prompt）、旁白（narration）、时长秒数（duration，3-6 秒）、运镜（camera）。
2. narration 是口播文本：把故事正文改写为口语化讲述，短句为主，所有镜头旁白连起来是完整故事；不要出现镜头编号、“画面”等元词。
3. visual_prompt 是给视频生成模型的中文画面描述：具体写清人物（身份/服饰/神态/动作）、环境（建筑/器物/光线/天气）、景别与氛围；必须严格遵守给定的时代视觉要求，杜绝时代错置。
4. 每个 visual_prompt 末尾自然带一句画面质感要求，如“电影感构图，自然光，质感真实”。
5. 镜头之间场景与人物保持连贯；相邻镜头避免重复画面。
6. duration 为整数，取值 3-6；全片镜头数与旁白总量匹配。

输出格式：只输出 JSON，不要输出任何解释、不要使用 markdown 代码围栏。结构如下：
{
  "scenes": [
    {
      "id": 1,
      "visual_prompt": "画面描述（含时代细节）",
      "narration": "该镜头的旁白",
      "duration": 5,
      "camera": "远景缓缓推进"
    }
  ]
}`

// StoryboardUserPrompt 构造分镜拆解的用户消息。
func StoryboardUserPrompt(storyTitle, storyDynasty, storyContent, ratio, resolution, videoStyle string) string {
	pack := MatchDynasty(storyDynasty)
	var b strings.Builder
	fmt.Fprintf(&b, "故事标题：%s\n", storyTitle)
	fmt.Fprintf(&b, "朝代：%s\n\n", storyDynasty)
	b.WriteString(pack.VisualAnchor())
	b.WriteString("\n\n")
	if ratio != "" {
		fmt.Fprintf(&b, "成片画面比例：%s（%s），构图时按此比例安排人物与空间。\n", ratio, resolution)
	}
	if videoStyle != "" {
		fmt.Fprintf(&b, "整体风格附加要求：%s\n", videoStyle)
	}
	fmt.Fprintf(&b, "\n故事正文：\n%s\n\n请拆分为分镜 JSON。", storyContent)
	return b.String()
}
