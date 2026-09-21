package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/sjzsdu/story/internal/app"
	"github.com/sjzsdu/story/internal/domain"
)

// listVoices 返回全部声音条目（内置 + 用户自定义）。
func (s *Server) listVoices(w http.ResponseWriter, r *http.Request) {
	vs, err := s.app.ListVoices(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, vs)
}

// getVoice 查询单个声音条目。
func (s *Server) getVoice(w http.ResponseWriter, r *http.Request) {
	v, err := s.app.GetVoice(r.Context(), r.PathValue("id"))
	if err != nil {
		if strings.Contains(err.Error(), "不存在") {
			writeErr(w, http.StatusNotFound, err.Error())
		} else {
			writeErr(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// createVoiceReqBody 新建声音请求体。
type createVoiceReqBody struct {
	Name        string  `json:"name"`
	Provider    string  `json:"provider"`
	Voice       string  `json:"voice"`
	Model       string  `json:"model"`
	Instruction string  `json:"instruction"`
	Rate        float64 `json:"rate"`
	Pitch       float64 `json:"pitch"`
	StyleNote   string  `json:"style_note"`
}

// createVoice 新建用户声音条目。
func (s *Server) createVoice(w http.ResponseWriter, r *http.Request) {
	var req createVoiceReqBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	v, err := s.app.CreateVoice(r.Context(), app.CreateVoiceInput{
		Name:        req.Name,
		Provider:    req.Provider,
		Voice:       req.Voice,
		Model:       req.Model,
		Instruction: req.Instruction,
		Rate:        req.Rate,
		Pitch:       req.Pitch,
		StyleNote:   req.StyleNote,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

// updateVoice 更新声音条目（内置条目也可改；改后影响所有引用它的系列）。
func (s *Server) updateVoice(w http.ResponseWriter, r *http.Request) {
	var req createVoiceReqBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	id := r.PathValue("id")
	existing, err := s.app.GetVoice(r.Context(), id)
	if err != nil {
		if strings.Contains(err.Error(), "不存在") {
			writeErr(w, http.StatusNotFound, err.Error())
		} else {
			writeErr(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	// 部分更新：请求体未传字段保留原值。
	if strings.TrimSpace(req.Name) != "" {
		existing.Name = req.Name
	}
	if strings.TrimSpace(req.Provider) != "" {
		existing.Provider = domain.NormalizeVoiceProvider(req.Provider)
	}
	if strings.TrimSpace(req.Voice) != "" {
		existing.Voice = req.Voice
	}
	if strings.TrimSpace(req.Model) != "" {
		existing.Model = req.Model
	}
	if req.Instruction != "" || strings.TrimSpace(req.Voice) != "" {
		// Instruction 允许清空（与 Voice 同字段组提交时按请求体覆盖）
		existing.Instruction = req.Instruction
	}
	if req.Rate > 0 {
		existing.Rate = req.Rate
	}
	if req.Pitch > 0 {
		existing.Pitch = req.Pitch
	}
	if req.StyleNote != "" {
		existing.StyleNote = req.StyleNote
	}
	if err := s.app.UpdateVoice(r.Context(), existing); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, existing)
}

// deleteVoice 删除声音条目（内置不可删；被系列引用拒绝）。
func (s *Server) deleteVoice(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.app.DeleteVoice(r.Context(), id); err != nil {
		msg := err.Error()
		switch {
		case strings.Contains(msg, "内置声音不可删除"):
			writeErr(w, http.StatusConflict, msg)
		case strings.Contains(msg, "被") && strings.Contains(msg, "系列引用"):
			writeErr(w, http.StatusConflict, msg)
		case strings.Contains(msg, "不存在"):
			writeErr(w, http.StatusNotFound, msg)
		default:
			writeErr(w, http.StatusInternalServerError, msg)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "id": id})
}

// voiceProfileLocked §16：series.voice_id 创建后锁定，旧 PUT 端点返回 409 提示编辑声音条目本身。
func (s *Server) voiceProfileLocked(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusConflict, map[string]string{
		"error": "声音创建后锁定不可改，请到「声音」页编辑声音条目本身（编辑后影响所有引用它的系列）",
	})
}

// previewVoice 用指定语音画像合成一段样音并返回可播放的路径。
// 三种入口：
//   - voice_id：按顶层 Voice 条目解析（推荐，§16）。
//   - profile：旧预设 key（回退兼容）。
//   - voice + rate + pitch + instruction：裸自定义参数。
//
// 请求体任选其一；voice_id 命中后其他参数作覆盖。
func (s *Server) previewVoice(w http.ResponseWriter, r *http.Request) {
	var req struct {
		VoiceID     string  `json:"voice_id"`
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
	switch {
	case req.VoiceID != "":
		v, err := s.app.GetVoice(r.Context(), req.VoiceID)
		if err != nil {
			writeErr(w, http.StatusNotFound, "声音条目不存在: "+req.VoiceID)
			return
		}
		p = v.ToProfile()
	case req.Profile != "":
		p = s.app.MatchVoiceProfile(r.Context(), req.Profile)
	default:
		p = domain.VoiceProfile{
			Voice:       req.Voice,
			Rate:        req.Rate,
			Pitch:       req.Pitch,
			Instruction: req.Instruction,
		}
	}
	// 请求参数覆盖命中条目（保留旧语义：profile/voice_id 之外的可单独覆盖）。
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

// buildVoiceReqBody 造声（声音设计/声音复刻）请求体。
type buildVoiceReqBody struct {
	// Provider TTS 供应商（留空默认 bailian）；决定由哪个造声实现执行。
	Provider             string   `json:"provider"`
	Name                 string   `json:"name"`
	Prompt               string   `json:"prompt"`
	PreviewText          string   `json:"preview_text"`
	AudioPath            string   `json:"audio_path"`
	AudioURL             string   `json:"audio_url"`
	TargetModel          string   `json:"target_model"`
	LanguageHints        []string `json:"language_hints"`
	MaxPromptAudioLength float64  `json:"max_prompt_audio_length"`
	EnablePreprocess     bool     `json:"enable_preprocess"`
	StyleNote            string   `json:"style_note"`
}

// designVoice 声音设计：用文字描述生成全新音色并落成声音条目（按新建音色个数计费）。
func (s *Server) designVoice(w http.ResponseWriter, r *http.Request) {
	var req buildVoiceReqBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.buildVoice(w, r, domain.VoiceBuildDesign, req)
}

// cloneVoice 声音复刻：用上传的音频克隆音色并落成声音条目（按新建音色个数计费）。
func (s *Server) cloneVoice(w http.ResponseWriter, r *http.Request) {
	var req buildVoiceReqBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.buildVoice(w, r, domain.VoiceBuildClone, req)
}

// buildVoice 设计/复刻共用实现：调 app.BuildVoice 造声并落库，返回声音条目与试听路径。
func (s *Server) buildVoice(w http.ResponseWriter, r *http.Request, kind string, req buildVoiceReqBody) {
	res, err := s.app.BuildVoice(r.Context(), app.BuildVoiceInput{
		Kind:                 kind,
		Provider:             req.Provider,
		Name:                 req.Name,
		Prompt:               req.Prompt,
		PreviewText:          req.PreviewText,
		AudioPath:            req.AudioPath,
		AudioURL:             req.AudioURL,
		TargetModel:          req.TargetModel,
		LanguageHints:        req.LanguageHints,
		MaxPromptAudioLength: req.MaxPromptAudioLength,
		EnablePreprocess:     req.EnablePreprocess,
		StyleNote:            req.StyleNote,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"voice":              res.Voice,
		"preview_audio_path": res.PreviewAudioPath,
	})
}

// maxVoiceSampleBytes 参考音频上传体积上限：25MB。
// 3-30 秒的音频（即使未压缩 wav）远小于此值，超出即视为异常请求。
const maxVoiceSampleBytes = 25 << 20

// uploadVoiceSample 接收浏览器上传或现场录制的参考音频，归一化后返回服务器路径。
//
// 请求：multipart/form-data，字段名 `file`。
// 响应：201 {path, duration_sec}（path 供随后 POST /api/voices/clone 的 audio_path 使用）。
//
// 为什么要先上传再克隆：浏览器拿不到用户本地文件的服务器路径，录音也只有 Blob；
// 且录音格式（webm/opus、mp4）供应商不认，必须服务端转码（见 app.SaveVoiceSample）。
// 两步式也保留了「先上传试听、再决定是否克隆」的余地（克隆才计费）。
func (s *Server) uploadVoiceSample(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxVoiceSampleBytes)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "too large") {
			status = http.StatusRequestEntityTooLarge
		}
		writeErr(w, status, "解析参考音频上传: "+err.Error())
		return
	}
	if r.MultipartForm != nil {
		// 清掉 ParseMultipartForm 落的临时件（内容已由 SaveVoiceSample 复制到自己的目录）。
		defer r.MultipartForm.RemoveAll()
	}

	f, hdr, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "缺少 file 字段（multipart/form-data 上传参考音频）")
		return
	}
	defer f.Close()

	sample, err := s.app.SaveVoiceSample(r.Context(), f, hdr.Filename)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, sample)
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

// listSystemVoices 列出某 TTS 供应商给定模型的系统音色（浏览音色库挑选）。
// 只取元数据、不合成语音，不产生费用。
func (s *Server) listSystemVoices(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	model := r.URL.Query().Get("model")
	vs, err := s.app.ListSystemVoices(r.Context(), provider, model)
	if err != nil {
		if strings.Contains(err.Error(), "暂不支持") {
			writeErr(w, http.StatusNotFound, err.Error())
		} else {
			writeErr(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, vs)
}
