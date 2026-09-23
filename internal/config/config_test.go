package config

import "testing"

// TestApplyEnvNewVisionAndSFXFields §22：三个新能力的供应商与视觉理解模型
// 的环境变量覆盖生效，且未设置时保持原值（空 env 不清字段）。
func TestApplyEnvNewVisionAndSFXFields(t *testing.T) {
	cfg := Default()
	cfg.ImageUnderstandProvider = "yaml-image-uv"
	t.Setenv("STORY_IMAGE_UNDERSTAND_PROVIDER", "bailian")
	t.Setenv("STORY_VIDEO_UNDERSTAND_PROVIDER", "bailian")
	t.Setenv("STORY_SFX_PROVIDER", "")
	t.Setenv("STORY_VISION_MODEL", "qwen3-vl-plus")
	applyEnv(&cfg)

	if cfg.ImageUnderstandProvider != "bailian" {
		t.Fatalf("ImageUnderstandProvider = %q，env 应覆盖 yaml 值", cfg.ImageUnderstandProvider)
	}
	if cfg.VideoUnderstandProvider != "bailian" {
		t.Fatalf("VideoUnderstandProvider = %q，期望 bailian", cfg.VideoUnderstandProvider)
	}
	// 空 env 不覆盖（与其余 STORY_*_*_PROVIDER 同语义）。
	if cfg.SFXProvider != "" {
		t.Fatalf("SFXProvider = %q，空 env 不应写入", cfg.SFXProvider)
	}
	if cfg.VisionModel != "qwen3-vl-plus" {
		t.Fatalf("VisionModel = %q，期望 qwen3-vl-plus", cfg.VisionModel)
	}
}

// TestApplyEnvAbsentKeepsDefaults 未设置 §22 相关 env 时字段保持 yaml/默认值。
func TestApplyEnvAbsentKeepsDefaults(t *testing.T) {
	cfg := Default()
	cfg.SFXModel = "reserved-model"
	applyEnv(&cfg)
	if cfg.ImageUnderstandProvider != "" || cfg.VideoUnderstandProvider != "" ||
		cfg.SFXProvider != "" || cfg.VisionModel != "" {
		t.Fatalf("无 env 时 §22 字段应保持空: %+v", cfg)
	}
	if cfg.SFXModel != "reserved-model" {
		t.Fatalf("SFXModel 被意外改动: %q", cfg.SFXModel)
	}
}
