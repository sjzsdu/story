// Package server 提供 Web UI 的 REST API、SSE 事件流与静态资源服务。
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/sjzsdu/story/internal/app"
	"github.com/sjzsdu/story/internal/domain"
)

// Server HTTP 服务，复用 CLI 同一个 app 容器。
type Server struct {
	app     *app.App
	rootCtx context.Context // 后台任务使用，不随单个 HTTP 请求取消
	broker  *broker
	assets  fs.FS // 嵌入的前端构建产物（可为 nil）
	mux     *http.ServeMux
}

// New 创建服务器；assets 为 go:embed 的前端 dist（可为 nil，仅提供 API）。
func New(rootCtx context.Context, application *app.App, assets fs.FS) *Server {
	s := &Server{
		app:     application,
		rootCtx: rootCtx,
		broker:  newBroker(),
		assets:  assets,
		mux:     http.NewServeMux(),
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/series", s.listSeries)
	s.mux.HandleFunc("POST /api/series", s.createSeries)
	s.mux.HandleFunc("GET /api/series/{id}", s.getSeries)
	s.mux.HandleFunc("DELETE /api/series/{id}", s.deleteSeries)
	s.mux.HandleFunc("POST /api/series/{id}/episodes", s.createEpisode)
	s.mux.HandleFunc("GET /api/series/{id}/episodes", s.listEpisodes)
	s.registerPlanRoutes()
	s.registerSeriesExtraRoutes()
	s.mux.HandleFunc("GET /api/episodes/{id}", s.getEpisode)
	s.mux.HandleFunc("DELETE /api/episodes/{id}", s.deleteEpisode)
	s.mux.HandleFunc("POST /api/episodes/{id}/actions", s.runActionHTTP)
	s.mux.HandleFunc("POST /api/episodes/{id}/refs", s.generateEpisodeRefs)
	s.mux.HandleFunc("GET /api/episodes/{id}/events", s.handleSSE)
	s.mux.HandleFunc("GET /api/episodes/{id}/media", s.serveMedia)
	s.mux.HandleFunc("GET /api/voices", s.listVoices)
	s.mux.HandleFunc("POST /api/voices", s.createVoice)
	s.mux.HandleFunc("GET /api/voices/{id}", s.getVoice)
	s.mux.HandleFunc("PUT /api/voices/{id}", s.updateVoice)
	s.mux.HandleFunc("DELETE /api/voices/{id}", s.deleteVoice)
	s.mux.HandleFunc("POST /api/voices/preview", s.previewVoice)
	s.mux.HandleFunc("GET /api/voices/preview", s.servePreview)
	// §16：series.voice_id 创建后锁定，不再允许单独更新；旧端点返回 409 提示编辑声音条目本身。
	s.mux.HandleFunc("PUT /api/series/{id}/voice", s.voiceProfileLocked)
}

// Handler 返回带 SPA 回退的总 handler。
func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			s.mux.ServeHTTP(w, r)
			return
		}
		s.serveSPA(w, r)
	})
}

// ListenAndServe 启动 HTTP 服务。
func (s *Server) ListenAndServe(addr string) error {
	srv := &http.Server{Addr: addr, Handler: s.Handler()}
	fmt.Printf("Web UI: http://%s\n", addr)
	return srv.ListenAndServe()
}

// ---------- 通用辅助 ----------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func decodeBody(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func (s *Server) episodeOrError(w http.ResponseWriter, r *http.Request) (*domain.Episode, bool) {
	ep, err := s.app.GetEpisode(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, app.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "集不存在")
		} else {
			writeErr(w, http.StatusInternalServerError, err.Error())
		}
		return nil, false
	}
	return ep, true
}

// ---------- 系列 ----------

func (s *Server) listSeries(w http.ResponseWriter, r *http.Request) {
	series, err := s.app.ListSeries(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if series == nil {
		series = []*domain.Series{}
	}
	writeJSON(w, http.StatusOK, series)
}

type createSeriesReq struct {
	Name        string `json:"name"`
	Dynasty     string `json:"dynasty"`
	Description string `json:"description"`
	Ratio       string `json:"ratio"`
	Resolution  string `json:"resolution"`
	VisualMode  string `json:"visual_mode"`
	// VoiceID 顶层声音条目 ID（§16，推荐路径）。
	VoiceID string `json:"voice_id"`
	// Voice/VoiceProfile/TTSInstruction 旧字段：未传 VoiceID 时按平迁规则现场建/取一个。
	Voice           string   `json:"voice"`
	VoiceProfile    string   `json:"voice_profile"`
	TTSInstruction  string   `json:"tts_instruction"`
	Concurrency     int      `json:"concurrency"`
	Retries         int      `json:"retries"`
	TargetPlatforms []string `json:"target_platforms"`
}

func (s *Server) createSeries(w http.ResponseWriter, r *http.Request) {
	var req createSeriesReq
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "请求体解析失败: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeErr(w, http.StatusBadRequest, "name 不能为空")
		return
	}
	se, err := s.app.CreateSeries(r.Context(), app.CreateSeriesInput{
		Name:            req.Name,
		Dynasty:         req.Dynasty,
		Description:     req.Description,
		Ratio:           req.Ratio,
		Resolution:      req.Resolution,
		VisualMode:      req.VisualMode,
		VoiceID:         req.VoiceID,
		Voice:           req.Voice,
		VoiceProfile:    req.VoiceProfile,
		TTSInstruction:  req.TTSInstruction,
		Concurrency:     req.Concurrency,
		Retries:         req.Retries,
		TargetPlatforms: req.TargetPlatforms,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, se)
}

func (s *Server) getSeries(w http.ResponseWriter, r *http.Request) {
	se, err := s.app.GetSeries(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, app.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "系列不存在")
		} else {
			writeErr(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	eps, err := s.app.ListEpisodes(r.Context(), se.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if eps == nil {
		eps = []*domain.Episode{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"series": se, "episodes": eps})
}

func (s *Server) deleteSeries(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.app.DeleteSeries(r.Context(), id); err != nil {
		if errors.Is(err, app.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "系列不存在")
		} else {
			writeErr(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "id": id})
}

// ---------- 集 ----------

type createEpisodeReq struct {
	Title string `json:"title"`
	Topic string `json:"topic"`
}

func (s *Server) createEpisode(w http.ResponseWriter, r *http.Request) {
	var req createEpisodeReq
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "请求体解析失败: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Title) == "" {
		writeErr(w, http.StatusBadRequest, "title 不能为空")
		return
	}
	ep, err := s.app.CreateEpisode(r.Context(), r.PathValue("id"), req.Title, req.Topic)
	if err != nil {
		if errors.Is(err, app.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "系列不存在")
		} else {
			writeErr(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusCreated, ep)
}

func (s *Server) listEpisodes(w http.ResponseWriter, r *http.Request) {
	eps, err := s.app.ListEpisodes(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if eps == nil {
		eps = []*domain.Episode{}
	}
	writeJSON(w, http.StatusOK, eps)
}

func (s *Server) getEpisode(w http.ResponseWriter, r *http.Request) {
	ep, ok := s.episodeOrError(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, ep)
}

func (s *Server) deleteEpisode(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.app.DeleteEpisode(r.Context(), id); err != nil {
		if errors.Is(err, app.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "集不存在")
		} else {
			writeErr(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "id": id})
}

// ---------- 动作触发 ----------

type actionReq struct {
	Action string `json:"action"` // candidates|pick|storyboard|produce|compose|run|export
	Index  int    `json:"index"`
	Ratio  string `json:"ratio"`
}

func (s *Server) runActionHTTP(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.episodeOrError(w, r); !ok {
		return
	}
	var req actionReq
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "请求体解析失败: "+err.Error())
		return
	}

	fn, err := s.buildAction(r.PathValue("id"), req)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	loadSnapshot := func() (any, error) {
		return s.app.GetEpisode(s.rootCtx, r.PathValue("id"))
	}
	job, started := s.broker.runAction(s.rootCtx, r.PathValue("id"), req.Action, loadSnapshot, fn)
	if !started {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error": "该集已有任务在执行",
			"job":   job,
		})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job": job})
}

// buildAction 把动作名映射为 engine 调用；run 为多步串联。
func (s *Server) buildAction(episodeID string, req actionReq) (func(ctx context.Context) error, error) {
	eng := s.app.Engine
	switch req.Action {
	case "candidates":
		return func(ctx context.Context) error { _, err := eng.GenerateCandidates(ctx, episodeID); return err }, nil
	case "pick":
		if req.Index < 1 {
			return nil, fmt.Errorf("pick 需要 index ≥ 1")
		}
		return func(ctx context.Context) error { return eng.Pick(ctx, episodeID, req.Index) }, nil
	case "storyboard":
		return func(ctx context.Context) error { _, err := eng.PlanStoryboard(ctx, episodeID); return err }, nil
	case "produce":
		return func(ctx context.Context) error { return eng.Produce(ctx, episodeID) }, nil
	case "compose":
		return func(ctx context.Context) error { _, err := eng.Compose(ctx, episodeID); return err }, nil
	case "export":
		if strings.TrimSpace(req.Ratio) == "" {
			return nil, fmt.Errorf("export 需要 ratio（16:9/9:16/1:1/3:4）")
		}
		return func(ctx context.Context) error { _, err := eng.Export(ctx, episodeID, req.Ratio); return err }, nil
	case "run":
		return s.buildRunAction(episodeID, req.Index), nil
	default:
		return nil, fmt.Errorf("未知动作: %s", req.Action)
	}
}

func (s *Server) buildRunAction(episodeID string, index int) func(ctx context.Context) error {
	eng := s.app.Engine
	// index 仅为兼容旧请求保留：新流程 generate 自动定稿，无需也不再使用候选序号。
	_ = index
	return func(ctx context.Context) error {
		ep, err := s.app.GetEpisode(ctx, episodeID)
		if err != nil {
			return err
		}
		if ep.State.Story == nil {
			// 含旧版停在 generate 后未 pick 的数据：直接生成定稿故事。
			if _, err := eng.GenerateCandidates(ctx, episodeID); err != nil {
				return err
			}
		}
		ep, err = s.app.GetEpisode(ctx, episodeID)
		if err != nil {
			return err
		}
		if ep.State.Storyboard == nil || ep.State.Steps[domain.StepStoryboard].Status != domain.StatusDone {
			if _, err := eng.PlanStoryboard(ctx, episodeID); err != nil {
				return err
			}
		}
		if err := eng.Produce(ctx, episodeID); err != nil {
			return err
		}
		_, err = eng.Compose(ctx, episodeID)
		return err
	}
}

// ---------- SSE ----------

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	if _, ok := s.episodeOrError(w, r); !ok {
		return
	}
	id := r.PathValue("id")

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// 连接即推送当前快照与任务状态。
	if ep, err := s.app.GetEpisode(r.Context(), id); err == nil {
		writeSSE(w, evSnapshot, ep)
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

func writeSSE(w http.ResponseWriter, kind eventKind, payload any) {
	data, _ := json.Marshal(payload)
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", kind, data)
}

// ---------- 媒体文件 ----------

// serveMedia 只允许访问该集 WorkDir 之内的文件（clips/audio/tmp/output）。
// 路径可能是绝对或相对（取决于服务端 DataDir 配置），统一归一到绝对路径后比对。
func (s *Server) serveMedia(w http.ResponseWriter, r *http.Request) {
	ep, ok := s.episodeOrError(w, r)
	if !ok {
		return
	}
	raw := r.URL.Query().Get("path")
	if raw == "" {
		writeErr(w, http.StatusBadRequest, "缺少 path 参数")
		return
	}
	base, err := filepath.Abs(ep.WorkDir)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	// engine 存的路径是相对项目根（CWD）的；前端也可能传相对 WorkDir 的路径。
	// 先按 CWD 解析为绝对，再校验是否落在 WorkDir 内；不满足时对相对路径回退按 WorkDir 解析。
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
			writeErr(w, http.StatusForbidden, "禁止访问工作目录之外的文件")
			return
		}
		abs = filepath.Join(base, filepath.Clean(raw))
		rel, err = filepath.Rel(base, abs)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			writeErr(w, http.StatusForbidden, "禁止访问工作目录之外的文件")
			return
		}
	}
	w.Header().Set("Accept-Ranges", "bytes")
	http.ServeFile(w, r, abs)
}

// ---------- SPA 静态资源 ----------

func (s *Server) serveSPA(w http.ResponseWriter, r *http.Request) {
	if s.assets == nil {
		http.Error(w, "前端未构建：请在 web/ 下执行 npm run build，或使用 npm run dev 开发模式", http.StatusNotFound)
		return
	}
	name := strings.TrimPrefix(filepath.Clean(r.URL.Path), "/")
	if name == "" || name == "." {
		name = "index.html"
	}
	if f, err := s.assets.Open(name); err == nil {
		_ = f.Close()
		http.FileServerFS(s.assets).ServeHTTP(w, r)
		return
	}
	// 其余路径（前端路由）回退 index.html。
	r2 := r.Clone(r.Context())
	r2.URL.Path = "/"
	http.FileServerFS(s.assets).ServeHTTP(w, r2)
}
