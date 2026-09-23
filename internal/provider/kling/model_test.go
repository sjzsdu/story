package kling

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sjzsdu/story/internal/port"
)

// TestGenerateClipModelOverride 验证系列级视频模型覆盖透传（§24 第二步）：
// req.Model 非空覆盖客户端系统默认 model_name，空则回落构造期默认。
// 假服务在提交阶段直接返回 code!=0 中止任务，不进入 5s 轮询——只为拿到请求体断言。
func TestGenerateClipModelOverride(t *testing.T) {
	for _, tc := range []struct {
		name, override, clientModel, want string
	}{
		{"请求级覆盖优先", "kling-v2-master", "kling-v1", "kling-v2-master"},
		{"空值回落系统默认", "", "kling-v1", "kling-v1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gotModel := new(string)
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					ModelName string `json:"model_name"`
				}
				b, _ := io.ReadAll(r.Body)
				if err := json.Unmarshal(b, &body); err != nil {
					t.Errorf("解析请求体: %v", err)
				}
				*gotModel = body.ModelName
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"code":1,"msg":"测试中止"}`))
			}))
			defer ts.Close()

			c := NewClient("ak", "sk", ts.URL, tc.clientModel)
			_, err := c.GenerateClip(context.Background(), port.ClipRequest{
				OutPath: filepath.Join(t.TempDir(), "scene.mp4"),
				Prompt:  "画面", Ratio: "9:16", Model: tc.override,
			})
			if err == nil || !strings.Contains(err.Error(), "kling error") {
				t.Fatalf("应在提交阶段被假服务中止: %v", err)
			}
			if *gotModel != tc.want {
				t.Fatalf("body model_name = %q, want %q", *gotModel, tc.want)
			}
		})
	}
}
