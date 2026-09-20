package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/sjzsdu/story/internal/domain"
)

// listVoices 返回预设语音画像列表。
func (s *Server) listVoices(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.app.ListVoiceProfiles())
}

// previewVoice 用指定语音画像合成一段样音并返回可播放的路径。
// 请求体: {"voice":"longtian_v3","rate":0.9,"pitch":1.0,"instruction":"","text":"可选自定义文本"}
// 也可用预设 key: {"profile":"wangliqun","text":"可选"}
func (s *Server) previewVoice(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Profile     string  `json:"profile"`
		Voice       string  `json:"voice"`
		Rate        float64 `json:"rate"`
		Pitch       float64 `json:"pitch"`
		Instruction string  `json:"instruction"`
		Text        string  `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	var p domain.VoiceProfile
	if req.Profile != "" {
		// 预设模式：从预设库取，可被请求参数覆盖
		p = s.app.MatchVoiceProfile(req.Profile)
	} else {
		// 自定义模式：直接用请求参数
		p = domain.VoiceProfile{
			Voice:       req.Voice,
			Rate:        req.Rate,
			Pitch:       req.Pitch,
			Instruction: req.Instruction,
		}
	}
	// 请求参数覆盖预设
	if req.Voice != "" {
		p.Voice = req.Voice
	}
	if req.Rate > 0 {
		p.Rate = req.Rate
	}
	if req.Pitch > 0 {
		p.Pitch = req.Pitch
	}
	if req.Instruction != "" {
		p.Instruction = req.Instruction
	}

	outPath, err := s.app.PreviewVoice(r.Context(), p, req.Text)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": outPath})
}

// updateVoiceProfile 更新系列的语音画像配置。
// 请求体: {"profile":"wangliqun"} 或 {"voice":"longze_v3","rate":0.85,"pitch":1.0,"instruction":""}
func (s *Server) updateVoiceProfile(w http.ResponseWriter, r *http.Request) {
	seriesID := r.PathValue("id")
	var req struct {
		Profile     string  `json:"profile"`
		Voice       string  `json:"voice"`
		Rate        float64 `json:"rate"`
		Pitch       float64 `json:"pitch"`
		Instruction string  `json:"instruction"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	series, err := s.app.GetSeries(r.Context(), seriesID)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	if req.Profile != "" {
		series.Config.VoiceProfile = req.Profile
	} else {
		// 自定义模式：清空预设 key，写完整参数
		series.Config.VoiceProfile = ""
		series.Config.TTSVoice = req.Voice
		series.Config.TTSRate = req.Rate
		series.Config.TTSPitch = req.Pitch
		series.Config.TTSInstruction = req.Instruction
	}
	if err := s.app.UpdateSeries(r.Context(), series); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, series.Config)
}

// servePreview 提供 GET 方式播放试音文件（路径来自 POST /api/voices/preview 返回的 path）。
func (s *Server) servePreview(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("path")
	if raw == "" {
		writeErr(w, http.StatusBadRequest, "缺少 path 参数")
		return
	}
	// 限制只能访问系统临时目录下的 story-voice-preview 子目录
	abs := raw
	if !filepath.IsAbs(abs) {
		abs, _ = filepath.Abs(raw)
	}
	if !strings.Contains(abs, "story-voice-preview") {
		writeErr(w, http.StatusForbidden, "禁止访问该路径")
		return
	}
	if _, err := os.Stat(abs); err != nil {
		writeErr(w, http.StatusNotFound, "文件不存在")
		return
	}
	http.ServeFile(w, r, abs)
}
