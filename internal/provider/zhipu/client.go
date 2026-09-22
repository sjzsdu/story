// Package zhipu 实现基于智谱 CogView API 的图片生成 provider。
// 智谱 CogView-4 支持中文 prompt，国风/工笔风格效果好。
package zhipu

import (
	"bytes"
	"context"
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
	defaultBaseURL = "https://open.bigmodel.cn/api/paas/v4"
	defaultModel   = "cogview-4"
	timeoutSec     = 120
)

// Client 封装对智谱 CogView API 的调用。
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

// ---- 智谱 CogView 请求/响应 ----

type imageRequest struct {
	Model          string `json:"model"`
	Prompt         string `json:"prompt"`
	Size           string `json:"size,omitempty"`
	N              int    `json:"n,omitempty"`
	ResponseFormat string `json:"response_format,omitempty"`
}

type imageResponse struct {
	Data []struct {
		URL     string `json:"url"`
		B64JSON string `json:"b64_json"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	} `json:"error,omitempty"`
}

// GenerateImage 实现 port.ImageGenerator。
// 智谱 CogView API：POST {base_url}/images/generations，返回 URL 或 base64。
func (c *Client) GenerateImage(ctx context.Context, req port.ImageRequest) (port.ImageResult, error) {
	if err := os.MkdirAll(filepath.Dir(req.OutPath), 0o755); err != nil {
		return port.ImageResult{}, err
	}

	imgReq := imageRequest{
		Model:          c.Model,
		Prompt:         req.Prompt,
		Size:           req.Size,
		N:              1,
		ResponseFormat: "url",
	}

	body, err := json.Marshal(imgReq)
	if err != nil {
		return port.ImageResult{}, fmt.Errorf("marshal request: %w", err)
	}

	url := c.BaseURL + "/images/generations"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return port.ImageResult{}, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)

	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: time.Duration(timeoutSec) * time.Second}
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return port.ImageResult{}, fmt.Errorf("zhipu image: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return port.ImageResult{}, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return port.ImageResult{}, fmt.Errorf("zhipu image %d: %s", resp.StatusCode, truncate(string(respBody), 500))
	}

	var imgResp imageResponse
	if err := json.Unmarshal(respBody, &imgResp); err != nil {
		return port.ImageResult{}, fmt.Errorf("decode response: %w", err)
	}
	if imgResp.Error != nil {
		return port.ImageResult{}, fmt.Errorf("zhipu error %d: %s", imgResp.Error.Code, imgResp.Error.Message)
	}
	if len(imgResp.Data) == 0 {
		return port.ImageResult{}, fmt.Errorf("zhipu: no image returned")
	}

	// 下载图片到本地文件。
	imageURL := imgResp.Data[0].URL
	if imageURL == "" && imgResp.Data[0].B64JSON != "" {
		// base64 模式：解码写入
		data, err := base64.StdEncoding.DecodeString(imgResp.Data[0].B64JSON)
		if err != nil {
			return port.ImageResult{}, fmt.Errorf("decode base64: %w", err)
		}
		if err := os.WriteFile(req.OutPath, data, 0o644); err != nil {
			return port.ImageResult{}, err
		}
		return port.ImageResult{OutPath: req.OutPath}, nil
	}

	if err := downloadFile(ctx, client, imageURL, req.OutPath); err != nil {
		return port.ImageResult{}, err
	}

	return port.ImageResult{OutPath: req.OutPath}, nil
}

func downloadFile(ctx context.Context, client *http.Client, url, outPath string) error {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("download image: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download image %d", resp.StatusCode)
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
