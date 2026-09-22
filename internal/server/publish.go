package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/sjzsdu/story/internal/app"
	"github.com/sjzsdu/story/internal/domain"
)

// ---- 发布任务端点 ----

type publishReq struct {
	Platforms   []string `json:"platforms"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Tags        []string `json:"tags,omitempty"`
	CoverPath   string   `json:"cover_path"`
	Category    string   `json:"category"`
	ScheduledAt string   `json:"scheduled_at,omitempty"` // RFC3339
}

func (s *Server) publishEpisode(w http.ResponseWriter, r *http.Request) {
	epID := r.PathValue("id")
	var in publishReq
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, 400, "请求格式错误: "+err.Error())
		return
	}
	if len(in.Platforms) == 0 {
		writeErr(w, 400, "platforms 不能为空")
		return
	}
	var scheduledAt *time.Time
	if in.ScheduledAt != "" {
		t, err := time.Parse(time.RFC3339, in.ScheduledAt)
		if err != nil {
			writeErr(w, 400, "scheduled_at 格式错误，需 RFC3339: "+err.Error())
			return
		}
		scheduledAt = &t
	}
	jobs, err := s.app.Publish(r.Context(), epID, app.PublishInput{
		Platforms:   in.Platforms,
		Title:       in.Title,
		Description: in.Description,
		Tags:        in.Tags,
		CoverPath:   in.CoverPath,
		Category:    in.Category,
		ScheduledAt: scheduledAt,
	})
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, jobs)
}

func (s *Server) listPublishJobs(w http.ResponseWriter, r *http.Request) {
	epID := r.PathValue("id")
	jobs, err := s.app.ListPublishJobs(r.Context(), epID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if jobs == nil {
		jobs = []*domain.PublishJob{}
	}
	writeJSON(w, 200, jobs)
}

func (s *Server) cancelPublishJob(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("jobId")
	if err := s.app.CancelPublish(r.Context(), jobID); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"status": "canceled", "id": jobID})
}

func (s *Server) deletePublishJob(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("jobId")
	if err := s.app.DeletePublished(r.Context(), jobID); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"status": "deleted", "id": jobID})
}

// ---- 平台账号管理端点 ----

// platformInfo 支持的平台信息。
type platformInfo struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	HasLogin    bool   `json:"has_login"`
	HasUpload   bool   `json:"has_upload"`
	HasNote     bool   `json:"has_note"`
	HasSchedule bool   `json:"has_schedule"`
}

var supportedPlatforms = []platformInfo{
	{Key: "douyin", Name: "抖音", HasLogin: true, HasUpload: true, HasNote: true, HasSchedule: true},
	{Key: "kuaishou", Name: "快手", HasLogin: true, HasUpload: true, HasNote: true, HasSchedule: true},
	{Key: "bilibili", Name: "B站", HasLogin: true, HasUpload: true, HasNote: false, HasSchedule: true},
	{Key: "xiaohongshu", Name: "小红书", HasLogin: true, HasUpload: true, HasNote: true, HasSchedule: true},
	{Key: "tencent", Name: "视频号", HasLogin: true, HasUpload: true, HasNote: false, HasSchedule: true},
	{Key: "baijiahao", Name: "百家号", HasLogin: true, HasUpload: true, HasNote: false, HasSchedule: false},
	{Key: "weibo", Name: "微博", HasLogin: true, HasUpload: true, HasNote: false, HasSchedule: false},
	{Key: "hupu", Name: "虎扑", HasLogin: true, HasUpload: true, HasNote: false, HasSchedule: false},
	{Key: "youtube", Name: "YouTube", HasLogin: true, HasUpload: true, HasNote: false, HasSchedule: false},
	{Key: "tiktok", Name: "TikTok", HasLogin: true, HasUpload: true, HasNote: false, HasSchedule: false},
}

func (s *Server) listPlatforms(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, supportedPlatforms)
}

type createAccountReq struct {
	AccountName string `json:"account_name"`
	AccountID   string `json:"account_id,omitempty"`
	Extra       string `json:"extra,omitempty"`
}

func (s *Server) createPlatformAccount(w http.ResponseWriter, r *http.Request) {
	platform := r.PathValue("platform")
	var in createAccountReq
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, 400, "请求格式错误: "+err.Error())
		return
	}
	if in.AccountName == "" {
		writeErr(w, 400, "account_name 不能为空")
		return
	}
	acct := &domain.PlatformAccount{
		ID:          platform + "-" + in.AccountName,
		Platform:    domain.Platform(platform),
		AccountName: in.AccountName,
		AccountID:   in.AccountID,
		Extra:       in.Extra,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := s.app.CreatePlatformAccount(r.Context(), acct); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 201, acct)
}

func (s *Server) listPlatformAccounts(w http.ResponseWriter, r *http.Request) {
	platform := r.PathValue("platform")
	accounts, err := s.app.ListPlatformAccounts(r.Context(), domain.Platform(platform))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if accounts == nil {
		accounts = []*domain.PlatformAccount{}
	}
	writeJSON(w, 200, accounts)
}

func (s *Server) deletePlatformAccount(w http.ResponseWriter, r *http.Request) {
	platform := r.PathValue("platform")
	accountID := r.PathValue("accountId")
	_ = platform // 未来可做平台白名单校验
	if err := s.app.DeletePlatformAccount(r.Context(), accountID); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"status": "deleted", "id": accountID})
}

type checkLoginReq struct {
	Platform string `json:"platform"`
	Account  string `json:"account"`
}

func (s *Server) checkPlatformLogin(w http.ResponseWriter, r *http.Request) {
	var in checkLoginReq
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, 400, "请求格式错误: "+err.Error())
		return
	}
	if in.Platform == "" || in.Account == "" {
		writeErr(w, 400, "platform 和 account 不能为空")
		return
	}
	valid, err := s.app.SauCheck(r.Context(), in.Platform, in.Account)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"valid": valid, "platform": in.Platform, "account": in.Account})
}

// registerPublishRoutes 注册发布相关路由。
func (s *Server) registerPublishRoutes() {
	// 发布任务
	s.mux.HandleFunc("POST /api/episodes/{id}/publish", s.publishEpisode)
	s.mux.HandleFunc("GET /api/episodes/{id}/publish", s.listPublishJobs)
	s.mux.HandleFunc("POST /api/episodes/{id}/publish/{jobId}/cancel", s.cancelPublishJob)
	s.mux.HandleFunc("DELETE /api/episodes/{id}/publish/{jobId}", s.deletePublishJob)
	// 平台与账号
	s.mux.HandleFunc("GET /api/platforms", s.listPlatforms)
	s.mux.HandleFunc("GET /api/platforms/{platform}/accounts", s.listPlatformAccounts)
	s.mux.HandleFunc("POST /api/platforms/{platform}/accounts", s.createPlatformAccount)
	s.mux.HandleFunc("DELETE /api/platforms/{platform}/accounts/{accountId}", s.deletePlatformAccount)
	s.mux.HandleFunc("POST /api/platforms/check", s.checkPlatformLogin)
}
