package server

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/sjzsdu/story/internal/app"
	"github.com/sjzsdu/story/internal/domain"
)

func (s *Server) registerSeriesExtraRoutes() {
	s.mux.HandleFunc("GET /api/series/{id}/events", s.handleSeriesSSE)
	s.mux.HandleFunc("PUT /api/series/{id}/characters", s.updateCharacters)
	// 创作控制参数：叙事/受众/篇幅/运镜/画风 + 系列级自定义指令。
	s.mux.HandleFunc("PUT /api/series/{id}/creative", s.updateCreative)
	s.mux.HandleFunc("POST /api/series/{id}/keyframes", s.generateKeyframes)
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
