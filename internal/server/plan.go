package server

import (
	"errors"
	"net/http"

	"github.com/sjzsdu/story/internal/app"
	"github.com/sjzsdu/story/internal/domain"
)

// planChatRequest 对话请求体。
type planChatRequest struct {
	Message string `json:"message"`
}

// planApplyRequest 采纳请求体：客户端可编辑草案后提交。
type planApplyRequest struct {
	Drafts []domain.EpisodeDraft `json:"drafts"`
}

func (s *Server) registerPlanRoutes() {
	s.mux.HandleFunc("GET /api/series/{id}/plan", s.getPlan)
	s.mux.HandleFunc("POST /api/series/{id}/plan/chat", s.planChat)
	s.mux.HandleFunc("POST /api/series/{id}/plan/apply", s.planApply)
	s.mux.HandleFunc("DELETE /api/series/{id}/plan", s.resetPlan)
}

// getPlan 返回系列当前的策划会话（无会话时返回空消息/空草案）。
func (s *Server) getPlan(w http.ResponseWriter, r *http.Request) {
	ps, err := s.app.GetSeriesPlan(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, app.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "系列不存在")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ps)
}

// planChat 同步进行一轮 AI 策划对话（文本调用通常数秒到一分钟）。
func (s *Server) planChat(w http.ResponseWriter, r *http.Request) {
	var req planChatRequest
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "请求体解析失败: "+err.Error())
		return
	}
	ps, err := s.app.ChatSeriesPlan(r.Context(), r.PathValue("id"), req.Message)
	if err != nil {
		if errors.Is(err, app.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "系列不存在")
			return
		}
		writeErr(w, http.StatusBadGateway, "AI 策划失败: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ps)
}

// planApply 把草案批量创建为集（不触发生产）。
func (s *Server) planApply(w http.ResponseWriter, r *http.Request) {
	var req planApplyRequest
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "请求体解析失败: "+err.Error())
		return
	}
	eps, err := s.app.ApplyEpisodePlan(r.Context(), r.PathValue("id"), req.Drafts)
	if err != nil {
		if errors.Is(err, app.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "系列不存在")
			return
		}
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"episodes": eps})
}

// resetPlan 清空策划会话。
func (s *Server) resetPlan(w http.ResponseWriter, r *http.Request) {
	if err := s.app.ResetSeriesPlan(r.Context(), r.PathValue("id")); err != nil {
		if errors.Is(err, app.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "系列不存在")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
