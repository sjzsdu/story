package bailian

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

	"github.com/sjzsdu/story/internal/domain"
	"github.com/sjzsdu/story/internal/port"
)

// 造声（声音设计 / 声音复刻）相关常量。
//
// 背景：bl CLI 没有造声命令（只有 speech synthesize/recognize），因此本能力
// 直连百炼 HTTP 接口（架构例外，见 AGENTS.md §2/§16）。造声按新建音色个数计费。
const (
	// defaultBaseURL 百炼中国内地服务地址（造声接口）。
	defaultBaseURL = "https://dashscope.aliyuncs.com"
	// customizationPath CosyVoice 音色管理（复刻/设计）服务端点。
	customizationPath = "/api/v1/services/audio/tts/customization"
	// voiceEnrollmentModel 造声模型名；声音复刻与声音设计共用该值，
	// 由 input.action 区分（均为 create_voice）。
	voiceEnrollmentModel = "voice-enrollment"
	// voiceBuildTimeout 造声 HTTP 超时（造声为同步接口，通常数秒到数十秒）。
	voiceBuildTimeout = 3 * time.Minute
	// ossResolveHeader 请求体含 oss:// 临时文件 URL 时需携带，服务端据此解析。
	ossResolveHeader = "X-DashScope-OssResourceResolve"
	// defaultLanguageHint 默认语种提示。
	defaultLanguageHint = "zh"
)

// voiceBuildResponse 造声接口返回体（CosyVoice 形态）。
type voiceBuildResponse struct {
	Output struct {
		VoiceID      string `json:"voice_id"`
		TargetModel  string `json:"target_model"`
		PreviewAudio *struct {
			Data           string `json:"data"`
			SampleRate     int    `json:"sample_rate"`
			ResponseFormat string `json:"response_format"`
		} `json:"preview_audio"`
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"output"`
	RequestID string `json:"request_id"`
	Code      string `json:"code"`
	Message   string `json:"message"`
}

// BuildVoice 实现 port.VoiceBuilder：调用百炼音色管理接口创建自定义音色。
//
//   - Kind=domain.VoiceBuildDesign：voice_prompt（文字描述）→ 新音色 + 试听音频；
//   - Kind=domain.VoiceBuildClone：url（音频，本地路径自动经 bl file upload 上传）→ 新音色。
//
// 生成的音色必须用返回的 target_model 合成（调用方应写入 Voice.Model）。
func (c *Client) BuildVoice(ctx context.Context, req port.VoiceBuildRequest) (port.VoiceBuildResult, error) {
	kind := strings.TrimSpace(req.Kind)
	if kind != domain.VoiceBuildDesign && kind != domain.VoiceBuildClone {
		return port.VoiceBuildResult{}, fmt.Errorf("未知造声方式 %q（支持 %s/%s）",
			req.Kind, domain.VoiceBuildDesign, domain.VoiceBuildClone)
	}
	targetModel := firstNonEmptyStr(req.TargetModel, c.TTSModel, domain.VoiceBuildModelDefault)

	input := map[string]any{
		"action":       "create_voice",
		"target_model": targetModel,
		"prefix":       sanitizeVoicePrefix(req.Prefix),
		"language_hints": firstNonEmptySlice(
			normalizeHints(req.LanguageHints), []string{defaultLanguageHint}),
	}
	body := map[string]any{"model": voiceEnrollmentModel, "input": input}

	switch kind {
	case domain.VoiceBuildDesign:
		if strings.TrimSpace(req.Prompt) == "" {
			return port.VoiceBuildResult{}, fmt.Errorf("声音设计需要声音描述（voice_prompt）")
		}
		if strings.TrimSpace(req.PreviewText) == "" {
			return port.VoiceBuildResult{}, fmt.Errorf("声音设计需要试听文本（preview_text）")
		}
		input["voice_prompt"] = req.Prompt
		input["preview_text"] = req.PreviewText
		// 试听音频固定返回 wav（便于直接内嵌播放）。
		body["parameters"] = map[string]any{"sample_rate": 24000, "response_format": "wav"}
	case domain.VoiceBuildClone:
		url, err := c.resolveAudioURL(ctx, req, targetModel)
		if err != nil {
			return port.VoiceBuildResult{}, err
		}
		input["url"] = url
		if req.MaxPromptAudioLength > 0 {
			input["max_prompt_audio_length"] = req.MaxPromptAudioLength
		}
		if req.EnablePreprocess {
			input["enable_preprocess"] = true
		}
	}

	raw, err := c.postCustomization(ctx, body)
	if err != nil {
		return port.VoiceBuildResult{}, err
	}
	var resp voiceBuildResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return port.VoiceBuildResult{}, fmt.Errorf("解析造声响应: %w; %s", err, truncateStr(string(raw), 300))
	}
	if resp.Code != "" || resp.Output.Code != "" {
		return port.VoiceBuildResult{}, fmt.Errorf("造声失败: %s %s",
			firstNonEmptyStr(resp.Code, resp.Output.Code), firstNonEmptyStr(resp.Message, resp.Output.Message))
	}
	if resp.Output.VoiceID == "" {
		return port.VoiceBuildResult{}, fmt.Errorf("造声未返回 voice_id: %s", truncateStr(string(raw), 300))
	}

	out := port.VoiceBuildResult{
		VoiceID:     resp.Output.VoiceID,
		TargetModel: firstNonEmptyStr(resp.Output.TargetModel, targetModel),
		RequestID:   resp.RequestID,
	}
	if resp.Output.PreviewAudio != nil && resp.Output.PreviewAudio.Data != "" && req.PreviewAudioPath != "" {
		if err := writePreviewAudio(req.PreviewAudioPath, resp.Output.PreviewAudio.Data); err != nil {
			return out, fmt.Errorf("保存试听音频: %w", err)
		}
		out.PreviewAudioPath = req.PreviewAudioPath
	}
	return out, nil
}

// postCustomization 向百炼音色管理端点发起 POST，返回响应体字节。
func (c *Client) postCustomization(ctx context.Context, body map[string]any) ([]byte, error) {
	key, err := c.apiKey()
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	url := strings.TrimRight(firstNonEmptyStr(c.BaseURL, os.Getenv("DASHSCOPE_BASE_URL"), defaultBaseURL), "/") + customizationPath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(ossResolveHeader, "enable")

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("调用百炼造声接口: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("造声接口 HTTP %d: %s", resp.StatusCode, truncateStr(string(raw), 300))
	}
	return raw, nil
}

// resolveAudioURL 取复刻音频的可访问 URL：优先显式 URL，其次本地文件上传。
// 本地文件经 `bl file upload` 上传到百炼临时存储（48h），返回 oss:// 地址。
func (c *Client) resolveAudioURL(ctx context.Context, req port.VoiceBuildRequest, model string) (string, error) {
	if u := strings.TrimSpace(req.AudioURL); u != "" {
		return u, nil
	}
	path := strings.TrimSpace(req.AudioPath)
	if path == "" {
		return "", fmt.Errorf("声音复刻需要音频（本地路径或公网 URL）")
	}
	if fi, err := os.Stat(path); err != nil || fi.IsDir() {
		return "", fmt.Errorf("音频文件不可用: %s", path)
	}
	args := []string{"file", "upload", "--file", path}
	args = appendModel(args, model)
	out, err := c.run(ctx, args...)
	if err != nil {
		return "", fmt.Errorf("上传复刻音频: %w", err)
	}
	u := extractUploadedURL(out)
	if u == "" {
		return "", fmt.Errorf("上传复刻音频未取得地址: %s", truncateStr(strings.TrimSpace(string(out)), 300))
	}
	return u, nil
}

// extractUploadedURL 从 `bl file upload` 输出中取 URL：
// 兼容纯文本（默认输出 oss://...）与 JSON（{"url":...} / {"data":{"url":...}}）。
func extractUploadedURL(out []byte) string {
	text := strings.TrimSpace(string(out))
	if text == "" {
		return ""
	}
	if strings.HasPrefix(text, "{") {
		var m map[string]any
		if err := json.Unmarshal([]byte(text), &m); err == nil {
			if u := findURLValue(m); u != "" {
				return u
			}
		}
	}
	// 纯文本：取最后一行中形如 URL 的片段。
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if looksLikeURL(line) {
			return line
		}
	}
	return ""
}

// findURLValue 递归在 JSON 对象里找形如 URL 的字符串值（优先 url 字段）。
func findURLValue(m map[string]any) string {
	if v, ok := m["url"].(string); ok && looksLikeURL(v) {
		return v
	}
	for _, v := range m {
		switch t := v.(type) {
		case map[string]any:
			if u := findURLValue(t); u != "" {
				return u
			}
		case string:
			if looksLikeURL(t) {
				return t
			}
		}
	}
	return ""
}

func looksLikeURL(s string) bool {
	for _, p := range []string{"http://", "https://", "oss://", "data:"} {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// writePreviewAudio 把 base64 试听音频写入目标路径。
func writePreviewAudio(path, b64 string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// apiKey 解析百炼 API Key：显式配置 → 环境变量 → ~/.bailian/config.json。
func (c *Client) apiKey() (string, error) {
	if k := strings.TrimSpace(c.APIKey); k != "" {
		return k, nil
	}
	if k := strings.TrimSpace(os.Getenv("DASHSCOPE_API_KEY")); k != "" {
		return k, nil
	}
	if home, err := os.UserHomeDir(); err == nil {
		b, err := os.ReadFile(filepath.Join(home, ".bailian", "config.json"))
		if err == nil {
			var cfg struct {
				APIKey string `json:"api_key"`
			}
			if json.Unmarshal(b, &cfg) == nil && strings.TrimSpace(cfg.APIKey) != "" {
				return strings.TrimSpace(cfg.APIKey), nil
			}
		}
	}
	return "", fmt.Errorf("未找到百炼 API Key：请配置 bailian_api_key 或环境变量 DASHSCOPE_API_KEY（也可先 bl auth login）")
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: voiceBuildTimeout}
}

// sanitizeVoicePrefix 归一音色名前缀：仅保留字母数字、最多 10 位；空则用 story。
func sanitizeVoicePrefix(p string) string {
	var b strings.Builder
	for _, r := range p {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
		if b.Len() >= 10 {
			break
		}
	}
	if b.Len() == 0 {
		return "story"
	}
	return b.String()
}

func normalizeHints(hints []string) []string {
	var out []string
	for _, h := range hints {
		if h = strings.TrimSpace(h); h != "" {
			out = append(out, h)
		}
	}
	return out
}

func firstNonEmptySlice(vals ...[]string) []string {
	for _, v := range vals {
		if len(v) > 0 {
			return v
		}
	}
	return nil
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	rs := []rune(s)
	if len(rs) <= n {
		return s
	}
	return string(rs[:n]) + "..."
}
