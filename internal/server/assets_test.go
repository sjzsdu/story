package server

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/sjzsdu/story/internal/domain"
)

// 素材/资产（§23）四端点测试：上传 / 列表 / 试听 / 删除 + BGM 引用设值校验。
// 全程 mock/临时库，不触碰 bl（成本红线）；上传的假 mp3 不做真实解码。

// uploadAssetMultipart 组装一次真实 multipart 上传请求并返回响应。
func uploadAssetMultipart(t *testing.T, baseURL, filename, content, kind, name string) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if kind != "" {
		if err := mw.WriteField("kind", kind); err != nil {
			t.Fatal(err)
		}
	}
	if name != "" {
		if err := mw.WriteField("name", name); err != nil {
			t.Fatal(err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	res, err := http.Post(baseURL+"/api/assets", mw.FormDataContentType(), &buf)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// putJSON 发 PUT 请求（assets_test 内自用，不依赖 server_test 的局部闭包）。
func putJSON(t *testing.T, url, payload string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPut, url, strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// errBody 读取 {"error": ...} 响应体文案。
func errBody(t *testing.T, res *http.Response) string {
	t.Helper()
	defer res.Body.Close()
	var body struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("解析错误响应: %v", err)
	}
	return body.Error
}

func TestAssetEndpoints(t *testing.T) {
	ts, _, _ := newTestServer(t)

	// ---- 上传：multipart 真实链路 → 201，落盘进素材库 ----
	res := uploadAssetMultipart(t, ts.URL, "theme.mp3", "fake-mp3-content", domain.AssetKindBGM, "主题曲")
	if res.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		t.Fatalf("上传状态码 = %d, body=%s", res.StatusCode, body)
	}
	var asset struct {
		ID       string  `json:"id"`
		Kind     string  `json:"kind"`
		Name     string  `json:"name"`
		Path     string  `json:"path"`
		Origin   string  `json:"origin"`
		Duration float64 `json:"duration_sec"`
	}
	if err := json.NewDecoder(res.Body).Decode(&asset); err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if asset.ID == "" || asset.Kind != "bgm" || asset.Origin != "upload" {
		t.Fatalf("上传返回异常: %+v", asset)
	}
	if !strings.Contains(asset.Path, "assets") {
		t.Fatalf("素材应落盘素材库目录: %s", asset.Path)
	}

	// 缺 file 字段 → 400。
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("kind", "bgm")
	_ = mw.Close()
	r, err := http.Post(ts.URL+"/api/assets", mw.FormDataContentType(), &buf)
	if err != nil {
		t.Fatal(err)
	}
	if r.StatusCode != http.StatusBadRequest {
		t.Fatalf("缺 file 应 400, 得 %d", r.StatusCode)
	}
	r.Body.Close()

	// 未知 kind 上传 → 400（不静默回退）。
	r = uploadAssetMultipart(t, ts.URL, "a.mp3", "x", "evil-kind", "")
	if r.StatusCode != http.StatusBadRequest {
		t.Fatalf("未知 kind 上传应 400, 得 %d", r.StatusCode)
	}
	r.Body.Close()

	// ---- 列表：kind 过滤与未知 kind ----
	r, err = http.Get(ts.URL + "/api/assets?kind=bgm")
	if err != nil {
		t.Fatal(err)
	}
	var list []struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	found := false
	for _, a := range list {
		if a.ID == asset.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("kind=bgm 列表应含刚上传的素材: %+v", list)
	}
	r, err = http.Get(ts.URL + "/api/assets?kind=nope")
	if err != nil {
		t.Fatal(err)
	}
	if r.StatusCode != http.StatusBadRequest {
		t.Fatalf("未知 kind 查询应 400, 得 %d", r.StatusCode)
	}
	r.Body.Close()

	// ---- 试听：200 + Accept-Ranges（<audio> 拖进度）----
	r, err = http.Get(ts.URL + "/api/assets/" + asset.ID + "/file")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(r.Body)
	r.Body.Close()
	if r.StatusCode != http.StatusOK || string(body) != "fake-mp3-content" {
		t.Fatalf("试听状态码 = %d, body=%q", r.StatusCode, body)
	}
	if r.Header.Get("Accept-Ranges") != "bytes" {
		t.Fatalf("试听应设 Accept-Ranges: bytes, got %q", r.Header.Get("Accept-Ranges"))
	}
	// 不存在的素材试听 → 404。
	r, err = http.Get(ts.URL + "/api/assets/nope/file")
	if err != nil {
		t.Fatal(err)
	}
	if r.StatusCode != http.StatusNotFound {
		t.Fatalf("素材不存在试听应 404, 得 %d", r.StatusCode)
	}
	r.Body.Close()

	// ---- BGM 引用设值校验：不存在的引用 → 400（创建与 PUT 对称）----
	r, err = http.Post(ts.URL+"/api/series", "application/json",
		strings.NewReader(`{"name":"乙","bgm_path":"asset:bgm-ghost"}`))
	if err != nil {
		t.Fatal(err)
	}
	if r.StatusCode != http.StatusBadRequest {
		t.Fatalf("创建时悬空引用应 400, 得 %d", r.StatusCode)
	}
	msg := errBody(t, r)
	if !strings.Contains(msg, "素材引用无效") || !strings.Contains(msg, "请先在素材库上传") {
		t.Fatalf("400 文案应含修复方式: %s", msg)
	}

	// ---- 引用中的素材删除 → 409；未引用删除 → 200；不存在 → 404 ----
	createBody, _ := json.Marshal(map[string]any{"name": "鬼谷子", "bgm_path": "asset:" + asset.ID})
	r, err = http.Post(ts.URL+"/api/series", "application/json", bytes.NewReader(createBody))
	if err != nil {
		t.Fatal(err)
	}
	if r.StatusCode != http.StatusCreated {
		t.Fatalf("带有效引用创建系列状态码 = %d, msg=%s", r.StatusCode, errBody(t, r))
	}
	var se struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&se)
	r.Body.Close()

	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/assets/"+asset.ID, nil)
	r, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if r.StatusCode != http.StatusConflict {
		t.Fatalf("引用中删除应 409, 得 %d", r.StatusCode)
	}
	msg = errBody(t, r)
	if !strings.Contains(msg, "素材被系列引用") || !strings.Contains(msg, "1 个系列引用") {
		t.Fatalf("409 文案应含引用原因: %s", msg)
	}

	// PUT 指向悬空引用同样 400，且系列配置不被改坏。
	r = putJSON(t, ts.URL+"/api/series/"+se.ID, `{"bgm_path":"asset:bgm-ghost"}`)
	if r.StatusCode != http.StatusBadRequest {
		t.Fatalf("PUT 悬空引用应 400, 得 %d", r.StatusCode)
	}
	r.Body.Close()
	// 字面路径回归：仍然放行（§20 历史语义）。
	r = putJSON(t, ts.URL+"/api/series/"+se.ID, `{"bgm_path":"bgm/theme.mp3"}`)
	if r.StatusCode != http.StatusOK {
		t.Fatalf("字面路径 PUT 应 200, 得 %d", r.StatusCode)
	}
	r.Body.Close()

	// 解除引用 → 可删（行与文件都清，这里至少验证 200）。
	r = putJSON(t, ts.URL+"/api/series/"+se.ID, `{"bgm_path":""}`)
	r.Body.Close()
	req, _ = http.NewRequest(http.MethodDelete, ts.URL+"/api/assets/"+asset.ID, nil)
	r, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if r.StatusCode != http.StatusOK {
		t.Fatalf("解除引用后删除应 200, 得 %d, msg=%s", r.StatusCode, errBody(t, r))
	}
	r.Body.Close()

	// 不存在的素材删除 → 404。
	req, _ = http.NewRequest(http.MethodDelete, ts.URL+"/api/assets/nope", nil)
	r, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if r.StatusCode != http.StatusNotFound {
		t.Fatalf("不存在素材删除应 404, 得 %d", r.StatusCode)
	}
	r.Body.Close()
}
