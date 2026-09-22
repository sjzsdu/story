package server

import (
	"encoding/json"
	"net/http"

	"github.com/sjzsdu/story/internal/config"
)

// ---- 配置端点 ----

// configResponse 配置快照（前端渲染 Settings 页用）。
// 敏感字段（API Key）掩码返回。
type configResponse struct {
	DataDir              string `json:"data_dir"`
	BLBin                string `json:"bl_bin"`
	FFMPEGBin            string `json:"ffmpeg_bin"`
	TextModel            string `json:"text_model"`
	VideoModel           string `json:"video_model"`
	TTSModel             string `json:"tts_model"`
	TTSVoice             string `json:"tts_voice"`
	ImageModel           string `json:"image_model"`
	TTSInstruction       string `json:"tts_instruction"`
	TextProvider         string `json:"text_provider"`
	DeepSeekAPIKey       string `json:"deepseek_api_key,omitempty"` // 掩码
	DeepSeekBaseURL      string `json:"deepseek_base_url"`
	DeepSeekModel        string `json:"deepseek_model"`
	BailianAPIKey        string `json:"bailian_api_key,omitempty"` // 掩码
	BailianBaseURL       string `json:"bailian_base_url"`
	DefaultRatio         string `json:"default_ratio"`
	DefaultResolution    string `json:"default_resolution"`
	MaxConcurrency       int    `json:"max_concurrency"`
	MaxRetries           int    `json:"max_retries"`
	SubtitleFont         string `json:"subtitle_font"`
	SAUBin               string `json:"sau_bin"`
	PythonBin            string `json:"python_bin"`
	DefaultPublishAccount string `json:"default_publish_account"`
	BilibiliDefaultTid   int    `json:"bilibili_default_tid"`
}

func configFromInternal(cfg config.Config) configResponse {
	r := configResponse{
		DataDir:               cfg.DataDir,
		BLBin:                 cfg.BLBin,
		FFMPEGBin:             cfg.FFMPEGBin,
		TextModel:             cfg.TextModel,
		VideoModel:            cfg.VideoModel,
		TTSModel:              cfg.TTSModel,
		TTSVoice:              cfg.TTSVoice,
		ImageModel:            cfg.ImageModel,
		TTSInstruction:        cfg.TTSInstruction,
		TextProvider:          cfg.TextProvider,
		DeepSeekBaseURL:       cfg.DeepSeekBaseURL,
		DeepSeekModel:         cfg.DeepSeekModel,
		BailianBaseURL:        cfg.BailianBaseURL,
		DefaultRatio:          cfg.DefaultRatio,
		DefaultResolution:     cfg.DefaultResolution,
		MaxConcurrency:        cfg.MaxConcurrency,
		MaxRetries:            cfg.MaxRetries,
		SubtitleFont:          cfg.SubtitleFont,
		SAUBin:                cfg.SAUBin,
		PythonBin:             cfg.PythonBin,
		DefaultPublishAccount: cfg.DefaultPublishAccount,
		BilibiliDefaultTid:    cfg.BilibiliDefaultTid,
	}
	// 掩码 API Key
	if cfg.BailianAPIKey != "" {
		r.BailianAPIKey = maskKey(cfg.BailianAPIKey)
	}
	if cfg.DeepSeekAPIKey != "" {
		r.DeepSeekAPIKey = maskKey(cfg.DeepSeekAPIKey)
	}
	return r
}

func maskKey(k string) string {
	if len(k) <= 8 {
		return "****"
	}
	return k[:4] + "****" + k[len(k)-4:]
}

func (s *Server) getConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, configFromInternal(s.app.Cfg))
}

// updateConfigReq 前端提交的可编辑配置字段。
type updateConfigReq struct {
	TextModel             *string `json:"text_model"`
	VideoModel            *string `json:"video_model"`
	TTSModel              *string `json:"tts_model"`
	TTSVoice              *string `json:"tts_voice"`
	ImageModel            *string `json:"image_model"`
	TTSInstruction        *string `json:"tts_instruction"`
	TextProvider          *string `json:"text_provider"`
	DeepSeekAPIKey        *string `json:"deepseek_api_key"`
	DeepSeekBaseURL       *string `json:"deepseek_base_url"`
	DeepSeekModel         *string `json:"deepseek_model"`
	BailianAPIKey         *string `json:"bailian_api_key"`
	BailianBaseURL        *string `json:"bailian_base_url"`
	DefaultRatio          *string `json:"default_ratio"`
	DefaultResolution     *string `json:"default_resolution"`
	MaxConcurrency        *int    `json:"max_concurrency"`
	MaxRetries            *int    `json:"max_retries"`
	SubtitleFont          *string `json:"subtitle_font"`
	SAUBin                *string `json:"sau_bin"`
	PythonBin             *string `json:"python_bin"`
	DefaultPublishAccount *string `json:"default_publish_account"`
	BilibiliDefaultTid    *int    `json:"bilibili_default_tid"`
}

func (s *Server) updateConfig(w http.ResponseWriter, r *http.Request) {
	var in updateConfigReq
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, 400, "请求格式错误: "+err.Error())
		return
	}
	cfg := &s.app.Cfg
	if in.TextModel != nil {
		cfg.TextModel = *in.TextModel
	}
	if in.VideoModel != nil {
		cfg.VideoModel = *in.VideoModel
	}
	if in.TTSModel != nil {
		cfg.TTSModel = *in.TTSModel
	}
	if in.TTSVoice != nil {
		cfg.TTSVoice = *in.TTSVoice
	}
	if in.ImageModel != nil {
		cfg.ImageModel = *in.ImageModel
	}
	if in.TTSInstruction != nil {
		cfg.TTSInstruction = *in.TTSInstruction
	}
	if in.TextProvider != nil {
		cfg.TextProvider = *in.TextProvider
	}
	if in.DeepSeekAPIKey != nil && *in.DeepSeekAPIKey != "" && *in.DeepSeekAPIKey != "****" {
		cfg.DeepSeekAPIKey = *in.DeepSeekAPIKey
	}
	if in.DeepSeekBaseURL != nil {
		cfg.DeepSeekBaseURL = *in.DeepSeekBaseURL
	}
	if in.DeepSeekModel != nil {
		cfg.DeepSeekModel = *in.DeepSeekModel
	}
	if in.BailianAPIKey != nil && *in.BailianAPIKey != "" && *in.BailianAPIKey != "****" {
		cfg.BailianAPIKey = *in.BailianAPIKey
	}
	if in.BailianBaseURL != nil {
		cfg.BailianBaseURL = *in.BailianBaseURL
	}
	if in.DefaultRatio != nil {
		cfg.DefaultRatio = *in.DefaultRatio
	}
	if in.DefaultResolution != nil {
		cfg.DefaultResolution = *in.DefaultResolution
	}
	if in.MaxConcurrency != nil {
		cfg.MaxConcurrency = *in.MaxConcurrency
	}
	if in.MaxRetries != nil {
		cfg.MaxRetries = *in.MaxRetries
	}
	if in.SubtitleFont != nil {
		cfg.SubtitleFont = *in.SubtitleFont
	}
	if in.SAUBin != nil {
		cfg.SAUBin = *in.SAUBin
	}
	if in.PythonBin != nil {
		cfg.PythonBin = *in.PythonBin
	}
	if in.DefaultPublishAccount != nil {
		cfg.DefaultPublishAccount = *in.DefaultPublishAccount
	}
	if in.BilibiliDefaultTid != nil {
		cfg.BilibiliDefaultTid = *in.BilibiliDefaultTid
	}
	writeJSON(w, 200, configFromInternal(*cfg))
}
