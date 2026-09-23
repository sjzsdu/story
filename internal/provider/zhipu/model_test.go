package zhipu

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/sjzsdu/story/internal/port"
)

// TestImageModelOverride 验证系列级图片模型覆盖透传（§24 第二步）：
// req.Model 非空覆盖客户端系统默认，空则回落构造期默认——body.model 必须如实变化。
// 假服务返回 b64_json（base64 "img"），走解码落盘分支、不发起二次下载。
func TestImageModelOverride(t *testing.T) {
	for _, tc := range []struct {
		name, override, clientModel, want string
	}{
		{"请求级覆盖优先", "cogview-4-0110", "cogview-4", "cogview-4-0110"},
		{"空值回落系统默认", "", "cogview-4", "cogview-4"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gotModel := new(string)
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Model string `json:"model"`
				}
				b, _ := io.ReadAll(r.Body)
				if err := json.Unmarshal(b, &body); err != nil {
					t.Errorf("解析请求体: %v", err)
				}
				*gotModel = body.Model
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"data":[{"b64_json":"aW1n"}]}`))
			}))
			defer ts.Close()

			c := NewClient("test-key", ts.URL, tc.clientModel)
			out := filepath.Join(t.TempDir(), "panel.png")
			if _, err := c.GenerateImage(context.Background(), port.ImageRequest{
				OutPath: out, Prompt: "工笔人物", Model: tc.override,
			}); err != nil {
				t.Fatalf("GenerateImage: %v", err)
			}
			if *gotModel != tc.want {
				t.Fatalf("body model = %q, want %q", *gotModel, tc.want)
			}
			if fi, err := os.Stat(out); err != nil || fi.Size() == 0 {
				t.Fatalf("图片未落盘: %v", err)
			}
		})
	}
}
