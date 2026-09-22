// Package minimax 实现基于 MiniMax API 的 TTS provider。
// MiniMax T2A（Text-to-Audio）API 中文语音自然度业界领先。
package minimax

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sjzsdu/story/internal/port"
)

const (
	defaultBaseURL = "https://api.minimax.chat"
	defaultModel   = "speech-02-hd"
	timeoutSec     = 120
)

// Client 封装对 MiniMax TTS API 的调用。
type Client struct {
	APIKey     string
	BaseURL    string
	Model      string
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

// ---- OpenAI 兼容 TTS 请求 ----

type ttsRequest struct {
	Model          string  `json:"model"`
	Input          string  `json:"input"`
	Voice          string  `json:"voice"`
	ResponseFormat string  `json:"response_format,omitempty"`
	Speed          float64 `json:"speed,omitempty"`
}

// Synthesize 实现 port.SpeechSynthesizer。
// MiniMax TTS API：POST {base_url}/v1/t2a_v2，返回音频二进制流。
func (c *Client) Synthesize(ctx context.Context, req port.SpeechRequest) (port.SpeechResult, error) {
	if err := os.MkdirAll(filepath.Dir(req.OutPath), 0o755); err != nil {
		return port.SpeechResult{}, err
	}

	speed := req.Rate
	if speed <= 0 {
		speed = 1.0
	}

	model := c.Model
	if req.Model != "" {
		model = req.Model
	}

	ttsReq := ttsRequest{
		Model:          model,
		Input:          req.Text,
		Voice:          req.Voice,
		ResponseFormat: formatOrDefault(req.Format),
		Speed:          speed,
	}

	body, err := json.Marshal(ttsReq)
	if err != nil {
		return port.SpeechResult{}, fmt.Errorf("marshal request: %w", err)
	}

	url := c.BaseURL + "/v1/t2a_v2"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return port.SpeechResult{}, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)

	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: time.Duration(timeoutSec) * time.Second}
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return port.SpeechResult{}, fmt.Errorf("minimax tts: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return port.SpeechResult{}, fmt.Errorf("minimax tts %d: %s", resp.StatusCode, truncate(string(respBody), 500))
	}

	// 成功：响应体为音频二进制，直接写入文件。
	f, err := os.Create(req.OutPath)
	if err != nil {
		return port.SpeechResult{}, fmt.Errorf("create output: %w", err)
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		return port.SpeechResult{}, fmt.Errorf("write audio: %w", err)
	}

	return port.SpeechResult{OutPath: req.OutPath}, nil
}

func formatOrDefault(f string) string {
	if f == "" {
		return "mp3"
	}
	return f
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
