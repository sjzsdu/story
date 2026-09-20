package server

import (
	"context"
	"net/http"
)

// episodeRefsRequest 本集视觉参考图生成请求体。
type episodeRefsRequest struct {
	Force bool `json:"force"`
}

// generateEpisodeRefs 后台批量生成本集视觉参考图（人物/场景）。
// 与该集流水线动作共用集槽位：同一集同时只允许一个后台任务。
func (s *Server) generateEpisodeRefs(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := s.episodeOrError(w, r); !ok {
		return
	}
	var req episodeRefsRequest
	if r.ContentLength > 0 {
		if err := decodeBody(r, &req); err != nil {
			writeErr(w, http.StatusBadRequest, "请求体解析失败: "+err.Error())
			return
		}
	}
	force := req.Force
	loadSnapshot := func() (any, error) {
		return s.app.GetEpisode(s.rootCtx, id)
	}
	fn := func(ctx context.Context) error {
		_, err := s.app.GenerateEpisodeRefs(ctx, id, force)
		return err
	}
	job, started := s.broker.runAction(s.rootCtx, id, "episode-refs", loadSnapshot, fn)
	if !started {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error": "该集已有任务在执行",
			"job":   job,
		})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job": job})
}
