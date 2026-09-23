package minimax

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

// TestSynthesizeModelOverride 验证系列级 TTS 模型覆盖透传（§24 第二步）：
// req.Model 非空覆盖客户端系统默认，空则回落构造期默认——body.model 必须如实变化。
// （engine 侧优先级 voice.Model > 系列 tts_model 见 engine/model_test.go。）
func TestSynthesizeModelOverride(t *testing.T) {
	for _, tc := range []struct {
		name, override, clientModel, want string
	}{
		{"请求级覆盖优先", "speech-02-hd", "speech-01-hd", "speech-02-hd"},
		{"空值回落系统默认", "", "speech-01-hd", "speech-01-hd"},
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
				w.Header().Set("Content-Type", "audio/mpeg")
				_, _ = w.Write([]byte("AUDIO"))
			}))
			defer ts.Close()

			c := NewClient("test-key", ts.URL, tc.clientModel)
			out := filepath.Join(t.TempDir(), "a.mp3")
			if _, err := c.Synthesize(context.Background(), port.SpeechRequest{
				OutPath: out, Text: "旁白", Voice: "female-shaonv", Model: tc.override,
			}); err != nil {
				t.Fatalf("Synthesize: %v", err)
			}
			if *gotModel != tc.want {
				t.Fatalf("body model = %q, want %q", *gotModel, tc.want)
			}
			data, err := os.ReadFile(out)
			if err != nil || string(data) != "AUDIO" {
				t.Fatalf("音频未正确落盘: %v %q", err, data)
			}
		})
	}
}
