package domain

// StoryCandidate AI 生成的候选故事（也是选中后承载完整故事的结构）。
type StoryCandidate struct {
	// Index 在候选列表中的序号（从 1 开始，供用户选择）。
	Index int `json:"index"`
	// Title 故事标题。
	Title string `json:"title"`
	// Dynasty 朝代，如「战国」「唐」。
	Dynasty string `json:"dynasty"`
	// Source 典籍出处，如「《史记·项羽本纪》」。
	Source string `json:"source"`
	// Summary 一句话梗概。
	Summary string `json:"summary"`
	// Content 白话故事正文（具备画面感与叙事冲突）。
	Content string `json:"content"`
	// PlatformPack 各平台原生发布物料（§27），key 为平台标识
	// （douyin/kuaishou/bilibili/xiaohongshu/weixin，与 domain.Platform 一致）。
	// 随故事一次调用顺带产出，零额外成本；可能缺平台或缺字段——消费方
	// （engine.Publish / Web 发布面板）必须逐字段回退到旧默认逻辑，不因缺失报错。
	PlatformPack map[string]*PlatformMeta `json:"platform_pack,omitempty"`
}

// PlatformMeta 一个平台的原生发布物料。
type PlatformMeta struct {
	// Title 为该平台口吻重写的标题（非截断，已按平台字数上限生成）。
	Title string `json:"title"`
	// Description 该平台口吻的简介（含话题引导或信息量补全）。
	Description string `json:"description"`
	// Tags 该平台格式的话题标签（不带 # 前缀，发布侧按平台格式渲染）。
	Tags []string `json:"tags,omitempty"`
}
