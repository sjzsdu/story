package bailian

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/sjzsdu/story/internal/domain"
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
	// 造声（设计/复刻）音色必须用造声时的 target_model 合成，否则引擎报错；
	// 请求带 Model 时优先，否则用配置的 TTS 模型。
	args = appendModel(args, firstNonEmptyStr(req.Model, c.TTSModel))
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

// ListSystemVoices 实现 port.VoiceLister：执行 `bl speech synthesize --list-voices`
// 并解析表格输出。只取音色元数据、不合成语音，不产生合成费用。
//
// 表格形如：
//
//	VOICE ID      NAME  DESCRIPTION  LANGUAGE
//	------------- ----- ------------ --------
//	longanyang    龙安洋  阳光大男孩      中文/英文
//
// 用空白切字段；表头/标题/分隔行跳过。字段多于 4 个时（描述含空格），
// 末字段为语言、第二字段为名、中间合并为描述。
func (c *Client) ListSystemVoices(ctx context.Context, model string) ([]domain.SystemVoice, error) {
	args := []string{"speech", "synthesize", "--list-voices"}
	args = appendModel(args, model)
	out, err := c.run(ctx, args...)
	if err != nil {
		return nil, err
	}

	var voices []domain.SystemVoice
	for _, line := range strings.Split(string(out), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" ||
			strings.HasPrefix(trimmed, "System voices") ||
			strings.HasPrefix(trimmed, "VOICE ID") ||
			isDashSeparator(trimmed) {
			continue
		}
		f := strings.Fields(trimmed)
		if len(f) < 4 {
			continue
		}
		sv := domain.SystemVoice{ID: f[0], Name: f[1]}
		if len(f) == 4 {
			sv.Description = f[2]
			sv.Language = f[3]
		} else {
			sv.Description = strings.Join(f[2:len(f)-1], " ")
			sv.Language = f[len(f)-1]
		}
		voices = append(voices, sv)
	}
	if len(voices) == 0 {
		return nil, os.ErrInvalid // 输出格式异常或模型无系统音色
	}
	return voices, nil
}

// isDashSeparator 判断是否为表格分隔行（仅由 '-' 和空格组成且含 '-'）。
func isDashSeparator(line string) bool {
	if !strings.Contains(line, "-") {
		return false
	}
	for _, r := range line {
		if r != '-' && r != ' ' {
			return false
		}
	}
	return true
}
