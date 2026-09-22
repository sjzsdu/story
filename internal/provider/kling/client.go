// Package kling 实现基于可灵 (Kling) API 的视频生成 provider。
// 可灵是快手出品的 AI 视频模型，中文场景理解好，支持参考图。
package kling

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
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
	defaultBaseURL = "https://api.klingai.com"
	defaultModel   = "kling-v1"
	pollInterval   = 5 * time.Second
	maxPollTime    = 10 * time.Minute
	timeoutSec     = 600
)

// Client 封装对可灵 API 的调用。
type Client struct {
	AccessKey  string
	SecretKey  string
	BaseURL    string
	Model      string
	HTTPClient *http.Client
}

// NewClient 创建客户端。
func NewClient(accessKey, secretKey, baseURL, model string) *Client {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	if model == "" {
		model = defaultModel
	}
	return &Client{
		AccessKey: accessKey,
		SecretKey: secretKey,
		BaseURL:   strings.TrimRight(baseURL, "/"),
		Model:     model,
	}
}

// ---- JWT 认证（标准库实现，无需第三方依赖） ----

func (c *Client) generateToken() (string, error) {
	now := time.Now()

	// Header: {"alg":"HS256","typ":"JWT"}
	header := base64URLEncode([]byte(`{"alg":"HS256","typ":"JWT"}`))

	// Payload
	claims := map[string]any{
		"iss": c.AccessKey,
		"exp": now.Add(time.Hour).Unix(),
		"nbf": now.Add(-time.Minute).Unix(),
		"iat": now.Unix(),
	}
	payloadBytes, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	payload := base64URLEncode(payloadBytes)

	// Signature
	signingInput := header + "." + payload
	mac := hmac.New(sha256.New, []byte(c.SecretKey))
	mac.Write([]byte(signingInput))
	signature := base64URLEncode(mac.Sum(nil))

	return signingInput + "." + signature, nil
}

func base64URLEncode(data []byte) string {
	return strings.TrimRight(base64.URLEncoding.EncodeToString(data), "=")
}

// ---- 可灵 API 结构 ----

type text2VideoRequest struct {
	ModelName       string     `json:"model_name"`
	Prompt          string     `json:"prompt"`
	NegativePrompt  string     `json:"negative_prompt,omitempty"`
	Duration        string     `json:"duration"`
	AspectRatio     string     `json:"aspect_ratio"`
	ReferenceImage  *refImage  `json:"reference_image,omitempty"`
}

type refImage struct {
	ImageURL string `json:"image_url"`
}

type taskResponse struct {
	Code  int    `json:"code"`
	Msg   string `json:"msg"`
	Data  struct {
		TaskID string `json:"task_id"`
	} `json:"data"`
}

type taskStatusResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		TaskID     string `json:"task_id"`
		TaskStatus string `json:"task_status"`
		TaskResult *struct {
			Videos []struct {
				URL string `json:"url"`
			} `json:"videos"`
		} `json:"task_result,omitempty"`
	} `json:"data"`
}

// GenerateClip 实现 port.VideoGenerator。
// 可灵 API：提交任务 → 轮询状态 → 下载视频。
func (c *Client) GenerateClip(ctx context.Context, req port.ClipRequest) (port.ClipResult, error) {
	if err := os.MkdirAll(filepath.Dir(req.OutPath), 0o755); err != nil {
		return port.ClipResult{}, err
	}

	token, err := c.generateToken()
	if err != nil {
		return port.ClipResult{}, fmt.Errorf("generate token: %w", err)
	}

	// 构建请求
	duration := "5"
	if req.DurationSec >= 10 {
		duration = "10"
	}

	v2Req := text2VideoRequest{
		ModelName:   c.Model,
		Prompt:      req.Prompt,
		Duration:    duration,
		AspectRatio: req.Ratio,
	}
	if req.ImagePath != "" {
		v2Req.ReferenceImage = &refImage{ImageURL: req.ImagePath}
	}

	body, err := json.Marshal(v2Req)
	if err != nil {
		return port.ClipResult{}, fmt.Errorf("marshal request: %w", err)
	}

	// 提交任务
	url := c.BaseURL + "/v1/videos/text2video"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return port.ClipResult{}, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+token)

	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: time.Duration(timeoutSec) * time.Second}
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return port.ClipResult{}, fmt.Errorf("kling submit: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return port.ClipResult{}, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return port.ClipResult{}, fmt.Errorf("kling submit %d: %s", resp.StatusCode, truncate(string(respBody), 500))
	}

	var taskResp taskResponse
	if err := json.Unmarshal(respBody, &taskResp); err != nil {
		return port.ClipResult{}, fmt.Errorf("decode response: %w", err)
	}
	if taskResp.Code != 0 {
		return port.ClipResult{}, fmt.Errorf("kling error %d: %s", taskResp.Code, taskResp.Msg)
	}

	taskID := taskResp.Data.TaskID

	// 轮询等待完成
	deadline := time.Now().Add(maxPollTime)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return port.ClipResult{}, ctx.Err()
		case <-time.After(pollInterval):
		}

		status, err := c.queryStatus(ctx, client, token, taskID)
		if err != nil {
			return port.ClipResult{}, err
		}

		switch status.Data.TaskStatus {
		case "succeed":
			if len(status.Data.TaskResult.Videos) == 0 {
				return port.ClipResult{}, fmt.Errorf("kling: task succeeded but no video")
			}
			videoURL := status.Data.TaskResult.Videos[0].URL
			if err := downloadFile(ctx, client, videoURL, req.OutPath); err != nil {
				return port.ClipResult{}, err
			}
			return port.ClipResult{OutPath: req.OutPath, TaskID: taskID}, nil

		case "failed":
			return port.ClipResult{}, fmt.Errorf("kling: task failed: %s", status.Msg)

		case "running", "pending":
			// 继续等待
		}
	}

	return port.ClipResult{}, fmt.Errorf("kling: task timed out after %v", maxPollTime)
}

func (c *Client) queryStatus(ctx context.Context, client *http.Client, token, taskID string) (*taskStatusResponse, error) {
	url := fmt.Sprintf("%s/v1/videos/text2video/%s", c.BaseURL, taskID)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("kling status: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("kling status %d: %s", resp.StatusCode, truncate(string(respBody), 500))
	}

	var status taskStatusResponse
	if err := json.Unmarshal(respBody, &status); err != nil {
		return nil, fmt.Errorf("decode status: %w", err)
	}
	return &status, nil
}

func downloadFile(ctx context.Context, client *http.Client, url, outPath string) error {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %d", resp.StatusCode)
	}
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
