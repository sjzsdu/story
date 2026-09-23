package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sjzsdu/story/testutil/mock"
)

// TestVisionDescribeEndpoint §22 图像/视频理解端点：
// 数据目录内路径放行并把请求透传给槽实现；目录外本地路径 403（防路径穿越读文件）；
// kind/必填项非法 400；未知 provider 由槽报 502。
func TestVisionDescribeEndpoint(t *testing.T) {
	ts, a, dir := newTestServer(t)

	v := &mock.Visioner{Reply: "一位古装老者立于竹林"}
	if err := a.Engine.ImageUnderstand.Register("mock", v); err != nil {
		t.Fatal(err)
	}

	// 目录内图片 → 200，透传 prompt/model/provider。
	img := filepath.Join(dir, "shots", "a.png")
	if err := os.MkdirAll(filepath.Dir(img), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(img, []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	body := `{"kind":"image","image":"` + jsonPath(img) + `","prompt":"图里有谁","provider":"mock"}`
	res := postJSON(t, ts.URL+"/api/vision/describe", body)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("目录内路径状态码 = %d", res.StatusCode)
	}
	var out map[string]string
	json.NewDecoder(res.Body).Decode(&out)
	res.Body.Close()
	if out["text"] != "一位古装老者立于竹林" {
		t.Fatalf("返回文本 = %q", out["text"])
	}
	if v.ImageCalls != 1 || v.ImageReq.Prompt != "图里有谁" || v.ImageReq.Image != img {
		t.Fatalf("请求未透传: calls=%d req=%+v", v.ImageCalls, v.ImageReq)
	}

	// 目录外本地路径 → 403（安全取舍，见 vision.go 注释），且绝不触达实现。
	res = postJSON(t, ts.URL+"/api/vision/describe",
		`{"kind":"image","image":"/etc/hosts","provider":"mock"}`)
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("目录外路径状态码 = %d，期望 403", res.StatusCode)
	}
	res.Body.Close()
	if v.ImageCalls != 1 {
		t.Fatalf("403 不应触达实现，calls=%d", v.ImageCalls)
	}

	// URL 放行（不校验存在性，交给实现）。
	res = postJSON(t, ts.URL+"/api/vision/describe",
		`{"kind":"image","image":"https://example.com/a.png","provider":"mock"}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("URL 状态码 = %d", res.StatusCode)
	}
	res.Body.Close()

	// 必填项 / kind 校验 → 400。
	for _, bad := range []string{
		`{"kind":"image"}`,
		`{"kind":"video"}`,
		`{"kind":"audio","image":"x"}`,
	} {
		res = postJSON(t, ts.URL+"/api/vision/describe", bad)
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("非法请求 %s 状态码 = %d，期望 400", bad, res.StatusCode)
		}
		res.Body.Close()
	}

	// 未知 provider → 槽报「未登记实现」，端点归为 502（上游失败）。
	res = postJSON(t, ts.URL+"/api/vision/describe",
		`{"kind":"image","image":"https://example.com/a.png","provider":"openai"}`)
	if res.StatusCode != http.StatusBadGateway {
		t.Fatalf("未知 provider 状态码 = %d，期望 502", res.StatusCode)
	}
	res.Body.Close()
}

// TestVisionDescribeVideoEntry 视频理解分支：video 必填、Image 可选透传。
func TestVisionDescribeVideoEntry(t *testing.T) {
	ts, a, _ := newTestServer(t)
	v := &mock.Visioner{Reply: "视频摘要"}
	if err := a.Engine.VideoUnderstand.Register("mock", v); err != nil {
		t.Fatal(err)
	}
	res := postJSON(t, ts.URL+"/api/vision/describe",
		`{"kind":"video","video":"https://example.com/v.mp4","image":"https://example.com/kf.png","provider":"mock"}`)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("视频理解状态码 = %d", res.StatusCode)
	}
	res.Body.Close()
	if v.VideoCalls != 1 || v.VideoReq.Video != "https://example.com/v.mp4" ||
		v.VideoReq.Image != "https://example.com/kf.png" {
		t.Fatalf("视频请求未透传: %+v", v.VideoReq)
	}
}

// postJSON 发 JSON 并返回响应（body 留给调用方关闭）。
func postJSON(t *testing.T, url, body string) *http.Response {
	t.Helper()
	res, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// jsonPath 把路径转成 JSON 字符串字面量（处理反斜杠）。
func jsonPath(p string) string {
	b, _ := json.Marshal(p)
	return string(b[1 : len(b)-1])
}
