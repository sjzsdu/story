package server

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sjzsdu/story/internal/app"
	"github.com/sjzsdu/story/internal/config"
	"github.com/sjzsdu/story/internal/domain"
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

	// 未知动作（pick 已随版本树移除）应 400。
	res, err = http.Post(ts.URL+"/api/episodes/guiguzi-e01/actions", "application/json",
		strings.NewReader(`{"action":"pick"}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("未知动作应 400，得到 %d", res.StatusCode)
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

// TestCancelAction 覆盖新端点：空闲的集没有可停止的任务，返回 canceled=false；
// 集不存在则 404。全程不触发任何模型调用。
func TestCancelAction(t *testing.T) {
	ts, _, _ := newTestServer(t)

	res, err := http.Post(ts.URL+"/api/series", "application/json",
		strings.NewReader(`{"name":"鬼谷子","dynasty":"战国"}`))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	res, err = http.Post(ts.URL+"/api/series/guiguzi/episodes", "application/json",
		strings.NewReader(`{"title":"入秦"}`))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()

	res, err = http.Post(ts.URL+"/api/episodes/guiguzi-e01/cancel", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("空闲集停止应 200，得到 %d", res.StatusCode)
	}
	var got map[string]any
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got["canceled"] != false {
		t.Fatalf("空闲集 canceled = %v，期望 false", got["canceled"])
	}

	res, err = http.Post(ts.URL+"/api/episodes/missing/cancel", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("对不存在的集停止应 404，得到 %d", res.StatusCode)
	}
}

// TestActivateAndDeleteNodeHTTP 覆盖 §17 版本树的两个同步端点：
// 切换活跃节点、删除节点（含后代与媒体目录）。手工种树，不触发任何模型调用。
func TestActivateAndDeleteNodeHTTP(t *testing.T) {
	ts, a, _ := newTestServer(t)
	ctx := context.Background()
	if _, err := a.CreateSeries(ctx, app.CreateSeriesInput{Name: "鬼谷子"}); err != nil {
		t.Fatal(err)
	}
	ep, err := a.CreateEpisode(ctx, "guiguzi", "入秦", "")
	if err != nil {
		t.Fatal(err)
	}

	// 手工种一棵两节点的树：story → storyboard（不调用模型）。
	storyDir := filepath.Join(ep.WorkDir, "versions", "story-1")
	boardDir := filepath.Join(ep.WorkDir, "versions", "storyboard-1")
	if err := os.MkdirAll(boardDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(boardDir, "storyboard.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	ep.Nodes = []*domain.VersionNode{
		{ID: "story-1", Stage: domain.StageStory, Status: domain.NodeDone, Dir: storyDir},
		{ID: "storyboard-1", Stage: domain.StageStoryboard, ParentID: "story-1", Status: domain.NodeDone, Dir: boardDir},
	}
	ep.ActiveNodeID = "storyboard-1"
	if err := a.Repo.SaveEpisode(ctx, ep); err != nil {
		t.Fatal(err)
	}

	// 切到 story 节点。
	res, err := http.Post(ts.URL+"/api/episodes/guiguzi-e01/nodes/story-1/activate", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("切换活跃节点应 200，得到 %d", res.StatusCode)
	}
	var got domain.Episode
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.ActiveNodeID != "story-1" {
		t.Fatalf("活跃节点 = %s，期望 story-1", got.ActiveNodeID)
	}

	// 不存在的节点应 400。
	res, err = http.Post(ts.URL+"/api/episodes/guiguzi-e01/nodes/nope/activate", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("切换不存在的节点应 400，得到 %d", res.StatusCode)
	}

	// 先激活分镜节点，再删除它：活跃指针应回退到父节点，目录被删。
	if res, err = http.Post(ts.URL+"/api/episodes/guiguzi-e01/nodes/storyboard-1/activate", "application/json", nil); err != nil {
		t.Fatal(err)
	} else {
		res.Body.Close()
	}
	req, err := http.NewRequest(http.MethodDelete, ts.URL+"/api/episodes/guiguzi-e01/nodes/storyboard-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("删除节点应 200，得到 %d", res.StatusCode)
	}
	after, err := a.GetEpisode(ctx, "guiguzi-e01")
	if err != nil {
		t.Fatal(err)
	}
	if after.NodeByID("storyboard-1") != nil {
		t.Fatal("节点应已删除")
	}
	if after.ActiveNodeID != "story-1" {
		t.Fatalf("活跃指针应回退到父节点，实际 %s", after.ActiveNodeID)
	}
	if _, err := os.Stat(boardDir); !os.IsNotExist(err) {
		t.Fatalf("媒体目录应被删除: %v", err)
	}

	// 删除不存在的节点应 400。
	req, err = http.NewRequest(http.MethodDelete, ts.URL+"/api/episodes/guiguzi-e01/nodes/nope", nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("删除不存在的节点应 400，得到 %d", res.StatusCode)
	}
}

// TestBuildActionWithFromAndReroll 覆盖动作映射：from 指向不存在的节点时
// 在触碰任何 provider 之前就报错（零费用），且已移除的 pick 不再是合法动作。
func TestBuildActionWithFromAndReroll(t *testing.T) {
	_, a, _ := newTestServer(t)
	ctx := context.Background()
	if _, err := a.CreateSeries(ctx, app.CreateSeriesInput{Name: "鬼谷子"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.CreateEpisode(ctx, "guiguzi", "入秦", ""); err != nil {
		t.Fatal(err)
	}
	s := &Server{app: a}

	fn, err := s.buildAction("guiguzi-e01", actionReq{Action: "produce", From: "node-not-exist"})
	if err != nil {
		t.Fatalf("动作映射不应失败: %v", err)
	}
	if err := fn(ctx); err == nil || !strings.Contains(err.Error(), "版本节点 node-not-exist 不存在") {
		t.Fatalf("from 指向不存在的节点应报错，实际: %v", err)
	}

	if _, err := s.buildAction("guiguzi-e01", actionReq{Action: "pick"}); err == nil {
		t.Fatal("pick 已随版本树移除，应报未知动作")
	}
	if _, err := s.buildAction("guiguzi-e01", actionReq{Action: "export"}); err == nil {
		t.Fatal("export 缺 ratio 应报错")
	}
}

// TestUploadVoiceSampleBadRequest 覆盖参考音频上传端点的请求校验分支。
// 两个用例都在读文件前返回，不触碰 ffmpeg/供应商，无费用。
func TestUploadVoiceSampleBadRequest(t *testing.T) {
	ts, _, _ := newTestServer(t)

	t.Run("缺 file 字段", func(t *testing.T) {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		if err := mw.WriteField("other", "x"); err != nil {
			t.Fatal(err)
		}
		if err := mw.Close(); err != nil {
			t.Fatal(err)
		}
		res, err := http.Post(ts.URL+"/api/voices/audio", mw.FormDataContentType(), &buf)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("缺 file 字段应 400，得到 %d", res.StatusCode)
		}
	})

	t.Run("非 multipart 请求", func(t *testing.T) {
		res, err := http.Post(ts.URL+"/api/voices/audio", "application/json", strings.NewReader("{}"))
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("非 multipart 请求应 400，得到 %d", res.StatusCode)
		}
	})
}
