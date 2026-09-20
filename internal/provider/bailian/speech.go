package bailian

import (
	"context"
	"os"
	"path/filepath"
	"strconv"

	"github.com/sjzsdu/story/internal/port"
)

// Synthesize 实现 port.SpeechSynthesizer。
//
// 注意：部分音色（实测 longtian_v3 + cosyvoice-v3-flash）不支持 --instruction
// 风格指令，引擎报 428 InvalidParameter；此时自动去掉指令降级重试一次。
func (c *Client) Synthesize(ctx context.Context, req port.SpeechRequest) (port.SpeechResult, error) {
	res, err := c.synthesize(ctx, req)
	if err != nil && req.Instruction != "" {
		req.Instruction = ""
		return c.synthesize(ctx, req)
	}
	return res, err
}

func (c *Client) synthesize(ctx context.Context, req port.SpeechRequest) (port.SpeechResult, error) {
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
	if req.Rate > 0 {
		args = append(args, "--rate", strconv.FormatFloat(req.Rate, 'f', -1, 64))
	}
	if req.Pitch > 0 {
		args = append(args, "--pitch", strconv.FormatFloat(req.Pitch, 'f', -1, 64))
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
