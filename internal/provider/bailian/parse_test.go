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

func TestDecodeModelJSONWithFence(t *testing.T) {
	content := "```json\n{\"candidates\": [{\"index\": 1, \"title\": \"t\"}]}\n```"
	out, err := decodeModelJSON[candidatesResponse](content)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Candidates) != 1 || out.Candidates[0].Title != "t" {
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
