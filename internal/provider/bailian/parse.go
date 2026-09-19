package bailian

import (
	"encoding/json"
	"fmt"
	"strings"
)

// chatEnvelope bl text chat --output json 的 OpenAI 兼容信封。
type chatEnvelope struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// contentWrapper `--stream --quiet` 时直接打印 `{"content": ...}` 包装（无 choices）。
type contentWrapper struct {
	Content string `json:"content"`
}

// parseChatContent 从 bl 文本返回中取出模型正文。
//
// bl 版本差异，共三种形态都兼容：
//  1. 不带 --quiet：OpenAI 兼容信封（choices[0].message.content）；
//  2. --quiet 非 stream：直接打印模型正文；
//  3. --stream --quiet：打印 `{"content": ...}` 包装（见 runJSON 注释）。
func parseChatContent(raw []byte) (string, error) {
	var env chatEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return "", fmt.Errorf("解析 bl 返回 JSON: %w", err)
	}
	if env.Error != nil {
		return "", fmt.Errorf("bl 返回错误(code=%d): %s", env.Error.Code, env.Error.Message)
	}
	if len(env.Choices) > 0 {
		return env.Choices[0].Message.Content, nil
	}
	// --stream 包装形态：{"content": "..."}。
	var wrapper contentWrapper
	if err := json.Unmarshal(raw, &wrapper); err == nil && wrapper.Content != "" {
		return wrapper.Content, nil
	}
	// 无 choices：quiet 直出形态，整段 JSON 文本即模型正文（如 {"reply":...}）。
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return "", fmt.Errorf("bl 返回为空: %s", truncate(raw))
	}
	return trimmed, nil
}

// stripFence 去除模型可能包裹的 ```json ... ``` 围栏与前后空白。
func stripFence(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		// 去掉首行（```json 或 ```）
		if idx := strings.IndexByte(s, '\n'); idx >= 0 {
			s = s[idx+1:]
		}
		s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	}
	return strings.TrimSpace(s)
}

// decodeModelJSON 解析模型正文到目标结构，容忍代码围栏与外层包裹。
func decodeModelJSON[T any](content string) (*T, error) {
	content = stripFence(content)
	var out T
	if err := json.Unmarshal([]byte(content), &out); err == nil {
		return &out, nil
	}
	// 容错：截取第一个 { 到最后一个 } 之间的内容。
	start := strings.IndexByte(content, '{')
	end := strings.LastIndexByte(content, '}')
	if start >= 0 && end > start {
		if err := json.Unmarshal([]byte(content[start:end+1]), &out); err == nil {
			return &out, nil
		}
	}
	return nil, fmt.Errorf("解析模型 JSON 失败: %s", truncateString(content, 500))
}

func truncate(b []byte) string {
	return truncateString(string(b), 300)
}

func truncateString(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
