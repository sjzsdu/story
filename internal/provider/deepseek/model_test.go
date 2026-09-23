package deepseek

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sjzsdu/story/internal/port"
)

// captureChat 启一个假 DeepSeek 服务：记录请求体 model 字段并返回合法的
// OpenAI 信封响应（content 由调用方给定）。返回假服务与 model 取值指针。
func captureChat(t *testing.T, content string) (*httptest.Server, *string) {
	t.Helper()
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
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":` + strconv_Quote(content) + `}}]}`))
	}))
	t.Cleanup(ts.Close)
	return ts, gotModel
}

// strconv_Quote 用 json 编码做字符串转义（避免手写引号拼接出错）。
func strconv_Quote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// TestChatModelOverride 验证系列级模型覆盖透传（§24 第二步）：
// req.Model 非空覆盖客户端系统默认，空则回落构造期默认——body.model 必须如实变化。
func TestChatModelOverride(t *testing.T) {
	for _, tc := range []struct {
		name, override, clientModel, want string
	}{
		{"请求级覆盖优先", "deepseek-reasoner", "deepseek-chat", "deepseek-reasoner"},
		{"空值回落系统默认", "", "deepseek-chat", "deepseek-chat"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ts, gotModel := captureChat(t, "ok")
			c := NewClient("test-key", ts.URL, tc.clientModel)
			content, err := c.chat(context.Background(), tc.override, "sys", "user", 0.9, 1024)
			if err != nil {
				t.Fatalf("chat: %v", err)
			}
			if content != "ok" {
				t.Fatalf("content = %q, want ok", content)
			}
			if *gotModel != tc.want {
				t.Fatalf("body model = %q, want %q", *gotModel, tc.want)
			}
		})
	}
}

// TestStoryRequestModelReachesChat 验证 port.StoryRequest.Model 一路透传到请求体
//（engine → provider 的接线，§24 第二步任务二第 8 条）。
func TestStoryRequestModelReachesChat(t *testing.T) {
	storyJSON := `{"title":"捭阖","dynasty":"秦","source":"《战国策》","summary":"s","content":"c"}`
	ts, gotModel := captureChat(t, storyJSON)

	c := NewClient("test-key", ts.URL, "deepseek-chat")
	got, err := c.GenerateCandidates(context.Background(), port.StoryRequest{
		SeriesName: "鬼谷子", Dynasty: "秦", Topic: "捭阖之术", Model: "deepseek-v3.2",
	})
	if err != nil {
		t.Fatalf("GenerateCandidates: %v", err)
	}
	if len(got) != 1 || got[0].Title != "捭阖" {
		t.Fatalf("故事解析异常: %+v", got)
	}
	if *gotModel != "deepseek-v3.2" {
		t.Fatalf("StoryRequest.Model 未透传: %q", *gotModel)
	}
}
