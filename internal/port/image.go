package port

import "context"

// ImageRequest 图片生成请求（用于角色定妆照等）。
type ImageRequest struct {
	// OutPath 图片落盘路径。
	OutPath string
	// Prompt 画面描述。
	Prompt string
	// NegativePrompt 负面提示词（可空）。
	NegativePrompt string
	// Size 尺寸，比例（3:4 / 1:1 / 16:9）或像素（W*H）。空用默认。
	Size string
	// Model 模型覆盖（系列级 image_model）：空＝用 provider 构造期的系统默认模型。
	Model string
}

// ImageResult 图片生成结果。
type ImageResult struct {
	OutPath string
}

// ImageGenerator 图片生成器（bl image generate 等实现）。
type ImageGenerator interface {
	GenerateImage(ctx context.Context, req ImageRequest) (ImageResult, error)
}
