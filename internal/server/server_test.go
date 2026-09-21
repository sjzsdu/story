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
	ep, err := a.CreateEpisode(context.Background(), "guiguzi", "入秦", "", "")
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
	ep, err := a.CreateEpisode(ctx, "guiguzi", "入秦", "", "")
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
	if _, err := a.CreateEpisode(ctx, "guiguzi", "入秦", "", ""); err != nil {
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

// ---------- 创作控制参数（S6） ----------

// TestCreativeCatalogEndpoint 注册表快照可直接给前端渲染。
func TestCreativeCatalogEndpoint(t *testing.T) {
	ts, _, _ := newTestServer(t)
	res, err := http.Get(ts.URL + "/api/creative-catalog")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("catalog 状态码 = %d", res.StatusCode)
	}
	var cat struct {
		Knobs []struct {
			Key          string `json:"key"`
			Label        string `json:"label"`
			DefaultLabel string `json:"default_label"`
			Type         string `json:"type"`
			Options      []struct {
				Key   string `json:"key"`
				Label string `json:"label"`
			} `json:"options"`
		} `json:"knobs"`
		Presets []struct {
			Key    string            `json:"key"`
			Name   string            `json:"name"`
			Values map[string]string `json:"values"`
		} `json:"presets"`
		DefaultPreset string `json:"default_preset"`
	}
	if err := json.NewDecoder(res.Body).Decode(&cat); err != nil {
		t.Fatal(err)
	}
	if len(cat.Knobs) == 0 || len(cat.Presets) == 0 || cat.DefaultPreset == "" {
		t.Fatalf("catalog 内容不完整: %+v", cat)
	}
	// 前端靠这个 key 渲染「跟随默认」，必须存在且是合法的 knob key。
	seen := map[string]bool{}
	for _, k := range cat.Knobs {
		if k.Key == "" || k.Label == "" || k.DefaultLabel == "" {
			t.Fatalf("参数描述不完整: %+v", k)
		}
		seen[k.Key] = true
	}
	if !seen["video_style"] {
		t.Fatal("catalog 应含画风参数 video_style")
	}
	// 默认预设的 values 必须为空，前端才能把「一键套用默认」渲染成不改任何参数。
	for _, p := range cat.Presets {
		if p.Key == cat.DefaultPreset && len(p.Values) != 0 {
			t.Fatalf("默认预设不得带值: %+v", p)
		}
	}
}

// TestUpdateCreativeEndpoint 覆盖 PUT 生效、补丁语义与非法 key 的 400。
func TestUpdateCreativeEndpoint(t *testing.T) {
	ts, _, _ := newTestServer(t)

	// 建系列：预设 + 逐项微调。
	res, err := http.Post(ts.URL+"/api/series", "application/json", strings.NewReader(
		`{"name":"鬼谷子","preset":"suspense","creative":{"audience":"teen"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("创建系列状态码 = %d", res.StatusCode)
	}
	var se domain.Series
	json.NewDecoder(res.Body).Decode(&se)
	res.Body.Close()
	if se.Config.Creative.Preset != "suspense" || se.Config.Creative.Audience != "teen" {
		t.Fatalf("创建系列的创作设置未生效: %+v", se.Config.Creative)
	}
	// 预设列出但被逐项微调覆盖：预设里 narrative=suspense、length=short 仍在。
	if se.Config.Creative.Narrative != "suspense" || se.Config.Creative.Length != "short" {
		t.Fatalf("预设值未展开: %+v", se.Config.Creative)
	}

	put := func(payload string) *http.Response {
		req, err := http.NewRequest(http.MethodPut, ts.URL+"/api/series/guiguzi/creative", strings.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		r, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}

	// 补丁：只改画风，未提到的参数保持不动。
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/series/guiguzi/creative",
		strings.NewReader(`{"creative":{"video_style":"ink"}}`))
	req.Header.Set("Content-Type", "application/json")
	r, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var updated domain.Series
	json.NewDecoder(r.Body).Decode(&updated)
	r.Body.Close()
	if r.StatusCode != http.StatusOK {
		t.Fatalf("PUT creative 状态码 = %d", r.StatusCode)
	}
	if updated.Config.VideoStyle != "ink" {
		t.Fatalf("画风未生效: %q", updated.Config.VideoStyle)
	}
	if updated.Config.Creative.Audience != "teen" || updated.Config.Creative.Narrative != "suspense" {
		t.Fatalf("未提到的参数不应被改动: %+v", updated.Config.Creative)
	}

	// 显式空串＝清除该参数（回到内置默认）。注意用新变量解码：
	// creative 的字段是 omitempty，清空后不会出现在响应里。
	var cleared domain.Series
	r = put(`{"creative":{"audience":"","instruction":"本系列只用短句"}}`)
	json.NewDecoder(r.Body).Decode(&cleared)
	r.Body.Close()
	if r.StatusCode != http.StatusOK {
		t.Fatalf("PUT creative 状态码 = %d", r.StatusCode)
	}
	if cleared.Config.Creative.Audience != "" {
		t.Fatalf("空串应清除参数: %q", cleared.Config.Creative.Audience)
	}
	if cleared.Config.Creative.Instruction != "本系列只用短句" {
		t.Fatalf("系列级指令未生效: %q", cleared.Config.Creative.Instruction)
	}

	// 未知 knob key → 400 并列出支持的 key。
	r = put(`{"creative":{"nope":"x"}}`)
	r.Body.Close()
	if r.StatusCode != http.StatusBadRequest {
		t.Fatalf("未知创作参数应 400，得到 %d", r.StatusCode)
	}
	// 未知预设 → 400。
	r = put(`{"preset":"nope"}`)
	r.Body.Close()
	if r.StatusCode != http.StatusBadRequest {
		t.Fatalf("未知预设应 400，得到 %d", r.StatusCode)
	}
	// 不存在的系列 → 404。
	req, _ = http.NewRequest(http.MethodPut, ts.URL+"/api/series/missing/creative", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	r, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	if r.StatusCode != http.StatusNotFound {
		t.Fatalf("不存在的系列应 404，得到 %d", r.StatusCode)
	}
}

// TestCreateEpisodeWithInstruction 集级附加指令随创建写入并回读。
func TestCreateEpisodeWithInstruction(t *testing.T) {
	ts, _, _ := newTestServer(t)
	res, err := http.Post(ts.URL+"/api/series", "application/json", strings.NewReader(`{"name":"鬼谷子"}`))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()

	res, err = http.Post(ts.URL+"/api/series/guiguzi/episodes", "application/json",
		strings.NewReader(`{"title":"入秦","topic":"张仪","instruction":"本集只讲一个晚上。"}`))
	if err != nil {
		t.Fatal(err)
	}
	var ep domain.Episode
	json.NewDecoder(res.Body).Decode(&ep)
	res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("创建集状态码 = %d", res.StatusCode)
	}
	if ep.Instruction != "本集只讲一个晚上。" {
		t.Fatalf("集级附加指令未写入: %q", ep.Instruction)
	}
}
