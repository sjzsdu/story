// Package config 负责加载 story.yaml 配置与环境变量覆盖。
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"gopkg.in/yaml.v3"
)

// Config 全局配置。
type Config struct {
	// DataDir 运行时数据根目录（数据库与 projects/ 媒体目录）。
	DataDir string `yaml:"data_dir"`
	// BLBin bl 可执行文件。
	BLBin string `yaml:"bl_bin"`
	// FFMPEGBin ffmpeg/ffprobe 可执行文件（ffprobe 同目录推导）。
	FFMPEGBin string `yaml:"ffmpeg_bin"`

	// 模型与音色（留空则使用 bl 自身默认值）。
	TextModel  string `yaml:"text_model"`
	VideoModel string `yaml:"video_model"`
	TTSModel   string `yaml:"tts_model"`
	TTSVoice   string `yaml:"tts_voice"`
	// ImageModel 图片模型（角色定妆照等，留空用 bl 默认）。
	ImageModel string `yaml:"image_model"`
	// TTSInstruction 默认旁白风格指令。
	TTSInstruction string `yaml:"tts_instruction"`

	// DefaultRatio 新系列默认画面比例。
	DefaultRatio string `yaml:"default_ratio"`
	// DefaultResolution 新系列默认分辨率。
	DefaultResolution string `yaml:"default_resolution"`

	// MaxConcurrency 默认单集并发镜头数。
	MaxConcurrency int `yaml:"max_concurrency"`
	// MaxRetries 默认单镜头重试次数。
	MaxRetries int `yaml:"max_retries"`

	// SubtitleFont 字幕字体路径（留空则自动探测系统中文字体）。
	SubtitleFont string `yaml:"subtitle_font"`
}

// Default 返回带默认值的配置。
func Default() Config {
	return Config{
		DataDir:           "data",
		BLBin:             "bl",
		FFMPEGBin:         "ffmpeg",
		TTSModel:          "cosyvoice-v3-flash",
		TTSVoice:          "longtian_v3", // 磁性理智男
		TTSInstruction:    "请用沉稳厚重、富有历史讲述感的语调，语速从容不迫，像学者在讲历史故事，不要播报腔。",
		DefaultRatio:      "9:16",
		DefaultResolution: "1080P",
		MaxConcurrency:    3,
		MaxRetries:        3,
	}
}

// Load 读取 yamlPath（不存在则返回默认配置），并应用环境变量覆盖。
func Load(yamlPath string) (Config, error) {
	cfg := Default()

	if b, err := os.ReadFile(yamlPath); err == nil {
		if err := yaml.Unmarshal(b, &cfg); err != nil {
			return cfg, fmt.Errorf("解析配置文件 %s: %w", yamlPath, err)
		}
	} else if !os.IsNotExist(err) {
		return cfg, fmt.Errorf("读取配置文件 %s: %w", yamlPath, err)
	}

	applyEnv(&cfg)
	return cfg, nil
}

func applyEnv(cfg *Config) {
	if v := os.Getenv("STORY_DATA_DIR"); v != "" {
		cfg.DataDir = v
	}
	if v := os.Getenv("STORY_BL_BIN"); v != "" {
		cfg.BLBin = v
	}
	if v := os.Getenv("STORY_FFMPEG_BIN"); v != "" {
		cfg.FFMPEGBin = v
	}
	if v := os.Getenv("STORY_TTS_VOICE"); v != "" {
		cfg.TTSVoice = v
	}
	if v := os.Getenv("STORY_MAX_CONCURRENCY"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.MaxConcurrency = n
		}
	}
}

// DBPath 返回 SQLite 数据库路径。
func (c Config) DBPath() string {
	return filepath.Join(c.DataDir, "story.db")
}

// ProjectsDir 返回媒体项目根目录。
func (c Config) ProjectsDir() string {
	return filepath.Join(c.DataDir, "projects")
}

// FFProbeBin 由 ffmpeg 路径推导 ffprobe 路径（同目录同名规则）。
func (c Config) FFProbeBin() string {
	dir := filepath.Dir(c.FFMPEGBin)
	if dir == "." {
		return "ffprobe"
	}
	return filepath.Join(dir, "ffprobe")
}
