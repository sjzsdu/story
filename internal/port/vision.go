package port

import "context"

// DescribeImageRequest 图像理解请求。
type DescribeImageRequest struct {
	// Image 图片本地路径或 URL（必填）。
	Image string
	// Prompt 提问/指令；空＝让模型自由描述。
	Prompt string
	// Model 理解模型；空则省略 --model（走 bl 默认 qwen3-vl-plus）。
	Model string
}

// DescribeVideoRequest 视频理解请求。
type DescribeVideoRequest struct {
	// Video 视频 URL 或本地路径（必填）。
	Video string
	// Image 可选的封面/关键帧图（路径或 URL），实现不要求时可留空。
	Image string
	// Prompt 提问/指令；空＝让模型自由描述。
	Prompt string
	// Model 理解模型；空则省略 --model（走 bl 默认）。
	Model string
}

// DescribeResult 理解结果（纯文本，不做 JSON 结构化）。
type DescribeResult struct {
	Text string
}

// ImageUnderstander 图像理解能力（§22）。
type ImageUnderstander interface {
	DescribeImage(ctx context.Context, req DescribeImageRequest) (DescribeResult, error)
}

// VideoUnderstander 视频理解能力（§22）。
// 与 ImageUnderstander 刻意拆成两个接口：同一条 bl vision describe 命令虽能同时
// 覆盖两者，但注册表按能力槽分槽登记，未来可能各自接入不同实现。
type VideoUnderstander interface {
	DescribeVideo(ctx context.Context, req DescribeVideoRequest) (DescribeResult, error)
}
