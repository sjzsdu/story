package server

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/sjzsdu/story/internal/app"
	"github.com/sjzsdu/story/internal/domain"
)

// 素材/资产（§23 统一资源管理）四端点：
//   - GET    /api/assets           列表（?kind=bgm|image_ref，空=全部）
//   - POST   /api/assets           multipart 上传（file/kind/name/description）
//   - DELETE /api/assets/{id}      删除（引用中 409，库外路径拒绝）
//   - GET    /api/assets/{id}/file 试听/回放（白名单 + Range）

// maxAssetBytes 素材上传体积上限：50MB（与 app 侧约定一致）。
const maxAssetBytes = 50 << 20

// listAssets 列出素材（kind 为空列出全部）。
func (s *Server) listAssets(w http.ResponseWriter, r *http.Request) {
	as, err := s.app.ListAssets(r.Context(), r.URL.Query().Get("kind"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if as == nil {
		as = []*domain.Asset{}
	}
	writeJSON(w, http.StatusOK, as)
}

// uploadAsset 接收上传的素材文件并入库。
// 请求：multipart/form-data，字段 file（必填）+ kind（默认 bgm）+ name + description。
func (s *Server) uploadAsset(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxAssetBytes)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "too large") {
			status = http.StatusRequestEntityTooLarge
		}
		writeErr(w, status, "解析素材上传: "+err.Error())
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll() // 临时件已由 app 复制进素材库
	}
	f, hdr, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "缺少 file 字段（multipart/form-data 上传素材）")
		return
	}
	defer f.Close()

	kind := r.FormValue("kind")
	if strings.TrimSpace(kind) == "" {
		kind = domain.AssetKindBGM
	}
	as, err := s.app.UploadAsset(r.Context(), f, hdr.Filename, kind,
		r.FormValue("name"), r.FormValue("description"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, as)
}

// deleteAsset 删除素材：被系列引用 → 409；不存在 → 404。
func (s *Server) deleteAsset(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.app.DeleteAsset(r.Context(), id); err != nil {
		switch {
		case errors.Is(err, app.ErrAssetInUse):
			writeErr(w, http.StatusConflict, err.Error())
		case errors.Is(err, app.ErrNotFound):
			writeErr(w, http.StatusNotFound, err.Error())
		default:
			writeErr(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "id": id})
}

// serveAssetFile 提供素材文件回放（试听）。只允许访问素材库目录 data/assets/ 之内
// 的文件（越权式与 serveMedia 同款），并设 Accept-Ranges 支持 <audio> 拖进度。
func (s *Server) serveAssetFile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	as, err := s.app.GetAsset(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "素材不存在: "+id)
		return
	}
	base := s.app.Config().AssetsDir()
	baseAbs, err := filepath.Abs(base)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	abs := as.Path
	if !filepath.IsAbs(abs) {
		if abs, err = filepath.Abs(abs); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	rel, err := filepath.Rel(baseAbs, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		writeErr(w, http.StatusForbidden, "禁止访问素材库之外的文件")
		return
	}
	if _, err := os.Stat(abs); err != nil {
		writeErr(w, http.StatusNotFound, fmt.Sprintf("素材文件不存在: %s；请重新上传该曲目", abs))
		return
	}
	w.Header().Set("Accept-Ranges", "bytes")
	http.ServeFile(w, r, abs)
}
