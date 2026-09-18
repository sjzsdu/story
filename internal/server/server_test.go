package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sjzsdu/story/internal/app"
	"github.com/sjzsdu/story/internal/config"
)

func newTestServer(t *testing.T) (*httptest.Server, *app.App, string) {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Default()
	cfg.DataDir = dir

	a, err := app.Bootstrap(context.Background(), cfg)
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })

	srv := New(context.Background(), a, nil)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, a, dir
}

func TestSeriesAndEpisodeFlow(t *testing.T) {
	ts, _, _ := newTestServer(t)

	// 创建系列。
	body := `{"name":"鬼谷子","dynasty":"战国","ratio":"9:16","resolution":"720P"}`
	res, err := http.Post(ts.URL+"/api/series", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("创建系列状态码 = %d", res.StatusCode)
	}
	var se map[string]any
	json.NewDecoder(res.Body).Decode(&se)
	res.Body.Close()
	if se["id"] != "guiguzi" {
		t.Fatalf("系列 id = %v", se["id"])
	}

	// 列表。
	res, err = http.Get(ts.URL + "/api/series")
	if err != nil {
		t.Fatal(err)
	}
	var list []map[string]any
	json.NewDecoder(res.Body).Decode(&list)
	res.Body.Close()
	if len(list) != 1 {
		t.Fatalf("系列数 = %d", len(list))
	}

	// 创建集。
	res, err = http.Post(ts.URL+"/api/series/guiguzi/episodes", "application/json",
		strings.NewReader(`{"title":"入秦"}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("创建集状态码 = %d", res.StatusCode)
	}
	res.Body.Close()

	// 查询集，初始停在 generate/pending。
	res, err = http.Get(ts.URL + "/api/episodes/guiguzi-e01")
	if err != nil {
		t.Fatal(err)
	}
	var ep map[string]any
	json.NewDecoder(res.Body).Decode(&ep)
	res.Body.Close()
	if ep["id"] != "guiguzi-e01" {
		t.Fatalf("集 id = %v", ep["id"])
	}

	// 404。
	res, err = http.Get(ts.URL + "/api/episodes/nope")
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("不存在的集应 404，得到 %d", res.StatusCode)
	}
	res.Body.Close()

	// 非法动作体。
	res, err = http.Post(ts.URL+"/api/episodes/guiguzi-e01/actions", "application/json",
		strings.NewReader(`{"action":"pick"}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("pick 缺 index 应 400，得到 %d", res.StatusCode)
	}
	res.Body.Close()
}

func TestMediaTraversalForbidden(t *testing.T) {
	ts, a, dir := newTestServer(t)
	if _, err := a.CreateSeries(context.Background(), app.CreateSeriesInput{Name: "鬼谷子"}); err != nil {
		t.Fatal(err)
	}
	ep, err := a.CreateEpisode(context.Background(), "guiguzi", "入秦", "")
	if err != nil {
		t.Fatal(err)
	}

	outside := filepath.Join(dir, "story.db")
	url := ts.URL + "/api/episodes/" + ep.ID + "/media?path=" + outside
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("越权访问应 403，得到 %d", res.StatusCode)
	}
}

func TestSPAFallback(t *testing.T) {
	ts, _, _ := newTestServer(t)
	// 未嵌入前端时，未知路径返回 404 提示而非崩溃。
	res, err := http.Get(ts.URL + "/series/guiguzi")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	// assets 为 nil 时返回 404；集成真实嵌入产物时返回 index.html(200)。
	if res.StatusCode != http.StatusNotFound && res.StatusCode != http.StatusOK {
		t.Fatalf("SPA 路径状态码 = %d", res.StatusCode)
	}
}

func TestActionOnMissingEpisode(t *testing.T) {
	ts, _, _ := newTestServer(t)
	res, err := http.Post(ts.URL+"/api/episodes/missing/actions", "application/json",
		strings.NewReader(`{"action":"compose"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("对不存在的集触发动作应 404，得到 %d", res.StatusCode)
	}
}
