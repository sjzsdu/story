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
}
