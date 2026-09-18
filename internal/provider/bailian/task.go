package bailian

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sjzsdu/story/internal/port"
)

// taskEnvelope 兼容 bl video task get 的返回（字段名做了多版本兼容）。
type taskEnvelope struct {
	TaskID     string `json:"task_id"`
	TaskStatus string `json:"task_status"`
	Output     struct {
		VideoURL string `json:"video_url"`
	} `json:"output"`
	// 备选字段
	Status   string `json:"status"`
	VideoURL string `json:"video_url"`
}

// GetVideoTask 实现 port.TaskPoller。
func (c *Client) GetVideoTask(ctx context.Context, taskID string) (port.VideoTask, error) {
	raw, err := c.runJSON(ctx, "video", "task", "get", "--task-id", taskID)
	if err != nil {
		return port.VideoTask{}, err
	}
	var env taskEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return port.VideoTask{}, fmt.Errorf("解析任务状态: %w", err)
	}

	status := strings.ToUpper(firstNonEmptyStr(env.TaskStatus, env.Status))
	switch status {
	case "SUCCEEDED", "SUCCESS", "DONE", "COMPLETED":
		status = port.TaskSucceeded
	case "FAILED", "ERROR", "CANCELED", "CANCELLED":
		status = port.TaskFailed
	case "RUNNING":
		status = port.TaskRunning
	case "PENDING", "QUEUED":
		status = port.TaskPending
	default:
		status = port.TaskUnknown
	}

	return port.VideoTask{
		TaskID:   firstNonEmptyStr(env.TaskID, taskID),
		Status:   status,
		VideoURL: firstNonEmptyStr(env.Output.VideoURL, env.VideoURL),
	}, nil
}

// WaitForVideo 轮询任务直到成功，然后下载到 outPath。
func (c *Client) WaitForVideo(ctx context.Context, taskID, outPath string) error {
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		task, err := c.GetVideoTask(ctx, taskID)
		if err == nil {
			switch task.Status {
			case port.TaskSucceeded:
				_, err := c.run(ctx, "video", "download", "--task-id", taskID, "--out", outPath)
				return err
			case port.TaskFailed:
				return fmt.Errorf("视频任务 %s 失败", taskID)
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func firstNonEmptyStr(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
