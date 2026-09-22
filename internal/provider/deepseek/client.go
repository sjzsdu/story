// Package deepseek 实现基于 DeepSeek API 的 port 接口。
// DeepSeek API 兼容 OpenAI 格式，通过 HTTP 直连（无 CLI 依赖）。
package deepseek

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	defaultBaseURL = "https://api.deepseek.com"
	defaultModel   = "deepseek-chat"
	timeoutSec     = 300
)

// Client 封装对 DeepSeek API 的调用。
type Client struct {
	// APIKey DeepSeek API Key。
	APIKey string
	// BaseURL API 地址，空则用默认值。
	BaseURL string
	// Model 文本模型，空则用默认值。
	Model string
	// HTTPClient 可选的自定义客户端。
	HTTPClient *http.Client
}

// NewClient 创建客户端。
func NewClient(apiKey, baseURL, model string) *Client {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	if model == "" {
		model = defaultModel
	}
	return &Client{
		APIKey:  apiKey,
		BaseURL: strings.TrimRight(baseURL, "/"),
		Model:   model,
	}
}

// ---- OpenAI 兼容请求/响应结构 ----

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Stream      bool          `json:"stream"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	} `json:"error,omitempty"`
}

// chat 发起一次 chat completion 请求，返回模型输出文本。
func (c *Client) chat(ctx context.Context, systemPrompt, userPrompt string, temperature float64, maxTokens int) (string, error) {
	reqBody := chatRequest{
		Model: c.Model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: temperature,
		MaxTokens:   maxTokens,
		Stream:      false,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	url := c.BaseURL + "/v1/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)

	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: time.Duration(timeoutSec) * time.Second}
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("deepseek api: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("deepseek api %d: %s", resp.StatusCode, truncate(string(respBody), 500))
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if chatResp.Error != nil {
		return "", fmt.Errorf("deepseek error %d: %s", chatResp.Error.Code, chatResp.Error.Message)
	}
	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("deepseek: empty choices")
	}

	return chatResp.Choices[0].Message.Content, nil
}

// chatMulti 发起多轮对话请求。
func (c *Client) chatMulti(ctx context.Context, systemPrompt string, messages []chatMessage, temperature float64, maxTokens int) (string, error) {
	allMessages := make([]chatMessage, 0, len(messages)+1)
	allMessages = append(allMessages, chatMessage{Role: "system", Content: systemPrompt})
	allMessages = append(allMessages, messages...)

	reqBody := chatRequest{
		Model:       c.Model,
		Messages:    allMessages,
		Temperature: temperature,
		MaxTokens:   maxTokens,
		Stream:      false,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	url := c.BaseURL + "/v1/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)

	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: time.Duration(timeoutSec) * time.Second}
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("deepseek api: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("deepseek api %d: %s", resp.StatusCode, truncate(string(respBody), 500))
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if chatResp.Error != nil {
		return "", fmt.Errorf("deepseek error %d: %s", chatResp.Error.Code, chatResp.Error.Message)
	}
	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("deepseek: empty choices")
	}

	return chatResp.Choices[0].Message.Content, nil
}

// extractJSON 从模型输出中提取 JSON 内容（去除 markdown 代码围栏等）。
func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	// 去除 ```json ... ``` 包围
	if strings.HasPrefix(s, "```") {
		lines := strings.Split(s, "\n")
		start, end := 0, len(lines)
		for i, l := range lines {
			trim := strings.TrimSpace(l)
			if strings.HasPrefix(trim, "```") && start == 0 {
				start = i + 1
			} else if trim == "```" && i > start {
				end = i
				break
			}
		}
		if start > 0 && end > start {
			s = strings.Join(lines[start:end], "\n")
		}
	}
	return strings.TrimSpace(s)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
