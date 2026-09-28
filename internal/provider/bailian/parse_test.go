package bailian

import "testing"

func TestParseChatContent(t *testing.T) {
	raw := []byte(`{
	  "choices": [
		{"message": {"role": "assistant", "content": "你好"}}
	  ],
	  "id": "x"
	}`)
	got, err := parseChatContent(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got != "你好" {
		t.Fatalf("content = %q", got)
	}
}

func TestParseChatContentErrorEnvelope(t *testing.T) {
	raw := []byte(`{"error": {"code": 400, "message": "bad request"}}`)
	if _, err := parseChatContent(raw); err == nil {
		t.Fatal("错误信封应返回 error")
	}
}

// quiet 形态：bl 直接打印模型正文（无 choices 信封）。
func TestParseChatContentQuietBare(t *testing.T) {
	raw := []byte(`{"reply":"ok","drafts":[]}`)
	got, err := parseChatContent(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got != `{"reply":"ok","drafts":[]}` {
		t.Fatalf("bare content = %q", got)
	}
}

// --stream --quiet 形态：{"content": ...} 包装（无 choices 信封）。
func TestParseChatContentStreamWrapper(t *testing.T) {
	raw := []byte(`{"content":"{\"title\":\"x\"}"}`)
	got, err := parseChatContent(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got != `{"title":"x"}` {
		t.Fatalf("stream wrapper content = %q", got)
	}
}

// bl file upload 实际输出为 key-value 多行文本，需能取到 `url:` 后的地址。
func TestExtractUploadedURLKeyValue(t *testing.T) {
	out := []byte(`url: oss://dashscope-instant/abc/2026-09-27/def/sample-1790485099218630000.wav
model: cosyvoice-v3-flash
expires_in: 48 hours
note: "When using this URL in API calls, add header: X-DashScope-OssResourceResolve: enable"
`)
	want := "oss://dashscope-instant/abc/2026-09-27/def/sample-1790485099218630000.wav"
	if got := extractUploadedURL(out); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestExtractUploadedURLPlainAndJSON(t *testing.T) {
	if got := extractUploadedURL([]byte("oss://a/b.wav\n")); got != "oss://a/b.wav" {
		t.Fatalf("纯 URL 行解析失败: %q", got)
	}
	if got := extractUploadedURL([]byte(`{"data":{"url":"https://x/y.wav"}}`)); got != "https://x/y.wav" {
		t.Fatalf("JSON 解析失败: %q", got)
	}
}

func TestDecodeModelJSONWithFence(t *testing.T) {
	content := "```json\n{\"title\": \"t\", \"content\": \"c\"}\n```"
	out, err := decodeModelJSON[storyResponse](content)
	if err != nil {
		t.Fatal(err)
	}
	if out.Title != "t" || out.Content != "c" {
		t.Fatalf("围栏 JSON 解析失败: %+v", out)
	}
}

func TestDecodeModelJSONWithPrefix(t *testing.T) {
	content := `好的，结果如下：
{"scenes": [{"id": 1, "visual_prompt": "v", "narration": "n", "duration": 5}]}`
	out, err := decodeModelJSON[storyboardResponse](content)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Scenes) != 1 {
		t.Fatalf("容错截取 JSON 失败: %+v", out)
	}
}
