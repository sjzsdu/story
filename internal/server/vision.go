package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/sjzsdu/story/internal/port"
)

// ---- 视觉理解端点（§22）----

// describeReq POST /api/vision/describe 请求体。
// kind 取 "image"（看图问答）或 "video"（视频理解）。
type describeReq struct {
	Kind     string `json:"kind"`
	Image    string `json:"image"`
	Video    string `json:"video"`
	Prompt   string `json:"prompt"`
	Model    string `json:"model"`
	Provider string `json:"provider"`
}

// describeImageOrVideo 图像/视频理解（§22）。
//
// 安全取舍：本端点会让服务端去读一个「路径或 URL」。URL 原样透传给 bl；
// 本地路径则**必须位于项目数据目录（cfg.DataDir）内**——否则任意 HTTP 客户端
// 都能借这个端点读服务器上的任意文件（路径穿越读文件）。serveMedia 的
// 集内白名单防护是同一思路的更严版本；这里放宽到整个 data 目录，方便对
// 产物截图/成片做理解，同时把系统文件挡在外面。CLI（story vision）不受此
// 限制（本机用户本来就能读自己的文件）。
func (s *Server) describeImageOrVideo(w http.ResponseWriter, r *http.Request) {
	var in describeReq
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, 400, "请求格式错误: "+err.Error())
		return
	}
	cfg := s.app.Config()
	check := func(raw string) error { return s.inDataDir(cfg.DataDir, raw) }

	switch in.Kind {
	case "image":
		if strings.TrimSpace(in.Image) == "" {
			writeErr(w, 400, "缺少 image（本地路径或 URL）")
			return
		}
		if err := check(in.Image); err != nil {
			writeErr(w, 403, err.Error())
			return
		}
		res, err := s.app.DescribeImage(r.Context(), port.DescribeImageRequest{
			Image: in.Image, Prompt: in.Prompt, Model: in.Model,
		}, in.Provider)
		if err != nil {
			writeErr(w, 502, err.Error())
			return
		}
		writeJSON(w, 200, map[string]string{"text": res.Text})
	case "video":
		if strings.TrimSpace(in.Video) == "" {
			writeErr(w, 400, "缺少 video（本地路径或 URL）")
			return
		}
		if err := check(in.Video); err != nil {
			writeErr(w, 403, err.Error())
			return
		}
		if in.Image != "" {
			if err := check(in.Image); err != nil {
				writeErr(w, 403, err.Error())
				return
			}
		}
		res, err := s.app.DescribeVideo(r.Context(), port.DescribeVideoRequest{
			Video: in.Video, Image: in.Image, Prompt: in.Prompt, Model: in.Model,
		}, in.Provider)
		if err != nil {
			writeErr(w, 502, err.Error())
			return
		}
		writeJSON(w, 200, map[string]string{"text": res.Text})
	default:
		writeErr(w, 400, `kind 必须是 "image" 或 "video"`)
	}
}

// inDataDir 校验本地路径落在 dataDir 内（防路径穿越）；URL（http/https）放行。
func (s *Server) inDataDir(dataDir, raw string) error {
	if isHTTPURL(raw) {
		return nil
	}
	root, err := filepath.Abs(dataDir)
	if err != nil {
		return err
	}
	abs, err := filepath.Abs(raw)
	if err != nil {
		return err
	}
	if abs != root && !strings.HasPrefix(abs, root+string(filepath.Separator)) {
		return fmt.Errorf("路径不在项目数据目录内，已拒绝（仅允许 data 目录内的本地文件或 http/https URL）: %s", abs)
	}
	if _, err := os.Stat(abs); err != nil {
		// 文件不存在也交由 provider 报更准确的错误，这里只要求「范围合法」。
		return nil
	}
	return nil
}

func isHTTPURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}
