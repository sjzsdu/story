package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/sjzsdu/story/internal/app"
	"github.com/sjzsdu/story/internal/domain"
)

func (s *Server) registerSeriesExtraRoutes() {
	s.mux.HandleFunc("GET /api/series/{id}/events", s.handleSeriesSSE)
	// 系列基础信息与规格（名称/朝代/简介/画幅/分辨率/并发重试/Provider 覆盖）。
	s.mux.HandleFunc("PUT /api/series/{id}", s.updateSeries)
	s.mux.HandleFunc("PUT /api/series/{id}/characters", s.updateCharacters)
	// 创作控制参数：叙事/受众/篇幅/运镜/画风 + 系列级自定义指令。
	s.mux.HandleFunc("PUT /api/series/{id}/creative", s.updateCreative)
	s.mux.HandleFunc("POST /api/series/{id}/keyframes", s.generateKeyframes)
	// 系列级发布汇总（该系列全部集的发布任务）。
	s.mux.HandleFunc("GET /api/series/{id}/publish", s.listSeriesPublishJobs)
	s.mux.HandleFunc("GET /api/series/{id}/media", s.serveSeriesMedia)
}

// seriesOrError 校验系列存在。
func (s *Server) seriesOrError(w http.ResponseWriter, r *http.Request) (*domain.Series, bool) {
	se, err := s.app.GetSeries(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, app.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "系列不存在")
		} else {
			writeErr(w, http.StatusInternalServerError, err.Error())
		}
		return nil, false
	}
	return se, true
}

// seriesSnapshot 系列状态快照（SSE 推送载荷）。
type seriesSnapshot struct {
	Series   *domain.Series    `json:"series"`
	Episodes []*domain.Episode `json:"episodes"`
}

func (s *Server) loadSeriesSnapshot(ctx context.Context, seriesID string) (any, error) {
	se, err := s.app.GetSeries(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	eps, err := s.app.ListEpisodes(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	if eps == nil {
		eps = []*domain.Episode{}
	}
	return seriesSnapshot{Series: se, Episodes: eps}, nil
}

// handleSeriesSSE 系列级事件流：每秒推送系列与集列表快照（含人物定妆照路径进度）。
func (s *Server) handleSeriesSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	id := r.PathValue("id")
	if _, ok := s.seriesOrError(w, r); !ok {
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	if snap, err := s.loadSeriesSnapshot(r.Context(), id); err == nil {
		writeSSE(w, evSnapshot, snap)
	}
	if j := s.broker.currentJob(id); j != nil {
		writeSSE(w, evJob, j)
	}
	flusher.Flush()

	ch, unsub := s.broker.subscribe(r.Context(), id)
	defer unsub()
	for {
		select {
		case <-r.Context().Done():
			return
		case frame, ok := <-ch:
			if !ok {
				return
			}
			if _, err := w.Write(frame); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// updateCharactersRequest 人工编辑人物设定集。
type updateCharactersRequest struct {
	Characters []domain.CharacterSetting `json:"characters"`
}

// updateCharacters 覆盖系列人物设定集（不影响策划会话）。
func (s *Server) updateCharacters(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.seriesOrError(w, r); !ok {
		return
	}
	var req updateCharactersRequest
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "请求体解析失败: "+err.Error())
		return
	}
	if err := s.app.UpdateSeriesCharacters(r.Context(), r.PathValue("id"), req.Characters); err != nil {
		if errors.Is(err, app.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "系列不存在")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	se, _ := s.app.GetSeries(r.Context(), r.PathValue("id"))
	writeJSON(w, http.StatusOK, se)
}

// updateSeriesRequest 系列元数据与规格的补丁请求体：nil 的字段保持原值。
// voice_id 与画面模式（visual_mode）创建后锁定，不在此列。
type updateSeriesRequest struct {
	Name           *string `json:"name"`
	Dynasty        *string `json:"dynasty"`
	Description    *string `json:"description"`
	Ratio          *string `json:"ratio"`
	Resolution     *string `json:"resolution"`
	MaxConcurrency *int    `json:"max_concurrency"`
	MaxRetries     *int    `json:"max_retries"`
	TextProvider   *string `json:"text_provider"`
	TTSProvider    *string `json:"tts_provider"`
	ImageProvider  *string `json:"image_provider"`
	VideoProvider  *string `json:"video_provider"`
	// 成片 BGM（§20）：path 为曲目路径（相对系列目录），volume 为 0..1 音量（0＝默认 0.18）。
	BGMPath   *string  `json:"bgm_path"`
	BGMVolume *float64 `json:"bgm_volume"`
}

// updateSeries 修改系列基础信息与规格。声音与画面模式仍锁定：本接口不写这两项。
func (s *Server) updateSeries(w http.ResponseWriter, r *http.Request) {
	se, ok := s.seriesOrError(w, r)
	if !ok {
		return
	}
	var req updateSeriesRequest
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "请求体解析失败: "+err.Error())
		return
	}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			writeErr(w, http.StatusBadRequest, "name 不能为空")
			return
		}
		se.Name = name
	}
	if req.Dynasty != nil {
		// 系列级朝代与 config.dynasty 是同一语义的两处存储，保持同步。
		se.Dynasty = strings.TrimSpace(*req.Dynasty)
		se.Config.Dynasty = se.Dynasty
	}
	if req.Description != nil {
		se.Description = strings.TrimSpace(*req.Description)
	}
	if req.Ratio != nil {
		ratio := strings.TrimSpace(*req.Ratio)
		if ratio == "" {
			writeErr(w, http.StatusBadRequest, "ratio 不能为空")
			return
		}
		se.Config.Ratio = ratio
	}
	if req.Resolution != nil {
		resolution := strings.TrimSpace(*req.Resolution)
		if resolution == "" {
			writeErr(w, http.StatusBadRequest, "resolution 不能为空")
			return
		}
		se.Config.Resolution = resolution
	}
	if req.MaxConcurrency != nil {
		if *req.MaxConcurrency < 1 {
			writeErr(w, http.StatusBadRequest, "max_concurrency 至少为 1")
			return
		}
		se.Config.MaxConcurrency = *req.MaxConcurrency
	}
	if req.MaxRetries != nil {
		if *req.MaxRetries < 0 {
			writeErr(w, http.StatusBadRequest, "max_retries 不能为负数")
			return
		}
		se.Config.MaxRetries = *req.MaxRetries
	}
	// Provider 覆盖：空串＝回到系统默认。
	if req.TextProvider != nil {
		se.Config.TextProvider = strings.TrimSpace(*req.TextProvider)
	}
	if req.TTSProvider != nil {
		se.Config.TTSProvider = strings.TrimSpace(*req.TTSProvider)
	}
	if req.ImageProvider != nil {
		se.Config.ImageProvider = strings.TrimSpace(*req.ImageProvider)
	}
	if req.VideoProvider != nil {
		se.Config.VideoProvider = strings.TrimSpace(*req.VideoProvider)
	}
	// 成片 BGM（§20）：nil＝保持原值；空串路径＝清除，音量 0＝回到默认 0.18。
	if req.BGMVolume != nil && (*req.BGMVolume < 0 || *req.BGMVolume > 1) {
		writeErr(w, http.StatusBadRequest,
			fmt.Sprintf("bgm_volume 必须在 0 到 1 之间（含 0 与 1，0＝默认 0.18），收到 %g", *req.BGMVolume))
		return
	}
	if req.BGMPath != nil {
		se.Config.BGMPath = strings.TrimSpace(*req.BGMPath)
	}
	if req.BGMVolume != nil {
		se.Config.BGMVolume = *req.BGMVolume
	}
	if err := s.app.UpdateSeries(r.Context(), se); err != nil {
		if errors.Is(err, app.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "系列不存在")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, se)
}

// listSeriesPublishJobs 系列级发布汇总：该系列下全部集的发布任务。
func (s *Server) listSeriesPublishJobs(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.seriesOrError(w, r); !ok {
		return
	}
	jobs, err := s.app.ListSeriesPublishJobs(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if jobs == nil {
		jobs = []*domain.PublishJob{}
	}
	writeJSON(w, http.StatusOK, jobs)
}

// updateCreativeRequest 创作设置请求体：预设 + 逐项微调。
type updateCreativeRequest struct {
	// Preset 预设 key；留空＝不套用预设，只做逐项微调。
	Preset string `json:"preset"`
	// Creative 形如 {knobKey: value}（含画风 video_style、自定义指令 instruction）。
	// 补丁语义：未出现的参数保持原值，显式空串＝清除该参数（回到内置默认）。
	Creative map[string]string `json:"creative"`
}

// updateCreative 覆盖系列的创作控制设置（voice_id / visual_mode 仍锁定，不在此列）。
// 未知参数 key / 未知预设返回 400。
func (s *Server) updateCreative(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.seriesOrError(w, r); !ok {
		return
	}
	var req updateCreativeRequest
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "请求体解析失败: "+err.Error())
		return
	}
	if err := validateCreative(req.Preset, req.Creative); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	se, err := s.app.UpdateSeriesCreative(r.Context(), r.PathValue("id"), req.Preset, req.Creative)
	if err != nil {
		if errors.Is(err, app.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "系列不存在")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, se)
}

// keyframesRequest 定妆照生成请求体。
type keyframesRequest struct {
	Force bool `json:"force"`
}

// generateKeyframes 后台批量生成系列人物定妆照（同一系列同时只允许一个任务）。
func (s *Server) generateKeyframes(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := s.seriesOrError(w, r); !ok {
		return
	}
	var req keyframesRequest
	if r.ContentLength > 0 {
		if err := decodeBody(r, &req); err != nil {
			writeErr(w, http.StatusBadRequest, "请求体解析失败: "+err.Error())
			return
		}
	}
	force := req.Force
	loadSnapshot := func() (any, error) {
		return s.loadSeriesSnapshot(s.rootCtx, id)
	}
	fn := func(ctx context.Context) error {
		_, err := s.app.GenerateSeriesKeyframes(ctx, id, force)
		return err
	}
	job, started := s.broker.runAction(s.rootCtx, id, "keyframes", loadSnapshot, fn)
	if !started {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error": "该系列已有任务在执行",
			"job":   job,
		})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job": job})
}

// serveSeriesMedia 只允许访问该系列项目目录（data/projects/<series-id>/，含 refs/）内的文件。
// 存量数据中的路径可能是绝对路径，也可能是相对 DataDir 的相对路径，统一归一到绝对路径后比对。
func (s *Server) serveSeriesMedia(w http.ResponseWriter, r *http.Request) {
	se, ok := s.seriesOrError(w, r)
	if !ok {
		return
	}
	base, err := filepath.Abs(filepath.Join(s.app.Cfg.ProjectsDir(), se.ID))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	raw := r.URL.Query().Get("path")
	if raw == "" {
		writeErr(w, http.StatusBadRequest, "缺少 path 参数")
		return
	}
	abs := raw
	if !filepath.IsAbs(abs) {
		if resolved, err := filepath.Abs(raw); err == nil {
			abs = resolved
		}
	}
	rel, err := filepath.Rel(base, abs)
	outside := err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel)
	if outside {
		if filepath.IsAbs(raw) {
			writeErr(w, http.StatusForbidden, "禁止访问系列目录之外的文件")
			return
		}
		abs = filepath.Join(base, filepath.Clean(raw))
		rel, err = filepath.Rel(base, abs)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			writeErr(w, http.StatusForbidden, "禁止访问系列目录之外的文件")
			return
		}
	}
	w.Header().Set("Accept-Ranges", "bytes")
	http.ServeFile(w, r, abs)
}
