package domain

// Storyboard 分镜脚本。
type Storyboard struct {
	Scenes []Scene `json:"scenes"`
}

// Scene 单个镜头。
type Scene struct {
	// ID 镜头序号（从 1 开始）。
	ID int `json:"id"`
	// VisualPrompt 视频生成用的画面描述（已做朝代视觉锚定）。
	VisualPrompt string `json:"visual_prompt"`
	// Narration 该镜头的旁白文字。
	Narration string `json:"narration"`
	// DurationSec 期望时长（秒）。
	DurationSec int `json:"duration"`
	// Camera 运镜提示，如「远景缓缓推进」。
	Camera string `json:"camera"`
}
