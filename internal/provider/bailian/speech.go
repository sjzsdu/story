package bailian

import (
	"context"
	"os"
	"path/filepath"

	"github.com/sjzsdu/story/internal/port"
)

// Synthesize 实现 port.SpeechSynthesizer。
func (c *Client) Synthesize(ctx context.Context, req port.SpeechRequest) (port.SpeechResult, error) {
	if err := os.MkdirAll(filepath.Dir(req.OutPath), 0o755); err != nil {
		return port.SpeechResult{}, err
	}
	format := req.Format
	if format == "" {
		format = "mp3"
	}

	args := []string{
		"speech", "synthesize",
		"--text", req.Text,
		"--format", format,
		"--out", req.OutPath,
	}
	args = appendModel(args, c.TTSModel)
	if req.Voice != "" {
		args = append(args, "--voice", req.Voice)
	}
	if req.Instruction != "" {
		args = append(args, "--instruction", req.Instruction)
	}

	if _, err := c.run(ctx, args...); err != nil {
		return port.SpeechResult{}, err
	}
	fi, err := os.Stat(req.OutPath)
	if err != nil {
		return port.SpeechResult{}, err
	}
	if fi.Size() == 0 {
		return port.SpeechResult{}, os.ErrInvalid
	}
	return port.SpeechResult{OutPath: req.OutPath}, nil
}
