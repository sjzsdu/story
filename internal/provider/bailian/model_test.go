package bailian

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/sjzsdu/story/internal/domain"
	"github.com/sjzsdu/story/internal/port"
)

// ---- 系列级模型覆盖（§24 第二步）：bailian 六路 --model 参数 ----

// fakeBLEffects 造一个「既记录参数又落副作用」的假 bl（零真实调用，§10 成本红线）：
//   - 参数按 NUL 分隔逐个写进 args.txt——prompt 含换行，按行分隔会把一个参数串成多个；
//   - --download / --out 后面的路径写入非空内容（视频 / 语音的落盘验收）；
//   - --out-dir + --out-prefix 在目录里造一个 <prefix>* 文件（图片生成的落盘约定）；
//   - stdout 打印指定文本（text chat 的返回）。
//
// 与 vision_test.go 的 fakeBL 分工：那个只回显 stdout，不落盘副作用。
func fakeBLEffects(t *testing.T, stdout string) (*Client, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("假 bl 脚本用 sh 编写，Windows 跳过")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "bl")
	argsFile := filepath.Join(dir, "args.txt")

	script := "#!/bin/sh\n" +
		"printf '%s\\0' \"$@\" > " + shellQuote(argsFile) + "\n" +
		"out=''; prefix=''; prev=''\n" +
		"for a in \"$@\"; do\n" +
		"  case \"$prev\" in\n" +
		"    --download)   printf 'x' > \"$a\" ;;\n" +
		"    --out)        printf 'x' > \"$a\" ;;\n" +
		"    --out-dir)    out=\"$a\" ;;\n" +
		"    --out-prefix) prefix=\"$a\" ;;\n" +
		"  esac\n" +
		"  case \"$a\" in\n" +
		"    --download|--out|--out-dir|--out-prefix) prev=\"$a\" ;;\n" +
		"    *) prev='' ;;\n" +
		"  esac\n" +
		"done\n" +
		"if [ -n \"$out\" ]; then printf 'x' > \"${out}/${prefix}0.png\"; fi\n" +
		"printf '%s\\n' " + shellQuote(stdout) + "\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return NewClient(bin, "", "", "", "", "", ""), argsFile
}

// readArgList 读回假 bl 记录的参数列表（NUL 分隔，prompt 含换行也安全）。
func readArgList(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) == 0 {
		return nil
	}
	return strings.Split(strings.TrimSuffix(string(b), "\x00"), "\x00")
}

// argValueAfter 取某个 flag 的值（不存在时返回空串）。
func argValueAfter(args []string, flag string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag {
			return args[i+1]
		}
	}
	return ""
}

// hasArg 判断参数列表里是否出现过某个 flag。
func hasArg(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}

// chatStdout 把模型正文包成 OpenAI choices 信封（parseChatContent 形态 1）。
// 不能直接打裸 story JSON：storyResponse 自带顶层 content 键，会被形态 3 的
// {"content": ...} 包装分支提前截走正文，导致解析出的只有 content 一段。
func chatStdout(t *testing.T, inner string) string {
	t.Helper()
	b, err := json.Marshal(inner)
	if err != nil {
		t.Fatal(err)
	}
	return `{"choices":[{"message":{"content":` + string(b) + `}}]}`
}

const storyFixture = `{"title":"捭阖之术","dynasty":"战国","source":"《鬼谷子》",` +
	`"summary":"一句话梗概","content":"正文一段。"}`

const storyboardFixture = `{"refs":{"characters":[{"name":"张仪","description":"清瘦"}],` +
	`"scenes":[{"name":"大殿","description":"破败"}]},` +
	`"scenes":[` +
	`{"id":1,"visual_prompt":"画面一","narration":"旁白一","duration":8,"camera":"推近"},` +
	`{"id":2,"visual_prompt":"画面二","narration":"旁白二","duration":7,"camera":"拉远"},` +
	`{"id":3,"visual_prompt":"画面三","narration":"旁白三","duration":9,"camera":"左移"},` +
	`{"id":4,"visual_prompt":"画面四","narration":"旁白四","duration":6,"camera":"定格"}]}`

// assertModel 校验 --model 的最终取值：want 为空＝必须整项省略。
func assertModel(t *testing.T, args []string, want string) {
	t.Helper()
	got := argValueAfter(args, "--model")
	if want == "" {
		if hasArg(args, "--model") {
			t.Fatalf("未配置模型时不应下发 --model，实际参数: %v", args)
		}
		return
	}
	if got != want {
		t.Fatalf("--model = %q，期望 %q（参数: %v）", got, want, args)
	}
}

// ---- 故事 ----

func TestStoryModelOverride(t *testing.T) {
	ctx := context.Background()

	t.Run("请求级覆盖优先", func(t *testing.T) {
		c, argsFile := fakeBLEffects(t, chatStdout(t, storyFixture))
		c.TextModel = "qwen3-max" // 系统默认也要被请求级覆盖压过
		if _, err := c.GenerateCandidates(ctx, port.StoryRequest{
			SeriesName: "鬼谷子", Dynasty: "战国", Topic: "捭阖之术", Model: "deepseek-v3.2",
		}); err != nil {
			t.Fatalf("GenerateCandidates: %v", err)
		}
		assertModel(t, readArgList(t, argsFile), "deepseek-v3.2")
	})

	t.Run("空值回落系统默认", func(t *testing.T) {
		c, argsFile := fakeBLEffects(t, chatStdout(t, storyFixture))
		c.TextModel = "qwen3-max"
		if _, err := c.GenerateCandidates(ctx, port.StoryRequest{SeriesName: "鬼谷子", Dynasty: "战国"}); err != nil {
			t.Fatalf("GenerateCandidates: %v", err)
		}
		assertModel(t, readArgList(t, argsFile), "qwen3-max")
	})

	t.Run("双空省略参数", func(t *testing.T) {
		c, argsFile := fakeBLEffects(t, chatStdout(t, storyFixture))
		if _, err := c.GenerateCandidates(ctx, port.StoryRequest{SeriesName: "鬼谷子", Dynasty: "战国"}); err != nil {
			t.Fatalf("GenerateCandidates: %v", err)
		}
		args := readArgList(t, argsFile)
		assertModel(t, args, "")
		if args[0] != "text" || args[1] != "chat" {
			t.Fatalf("子命令应为 text chat: %v", args)
		}
	})
}

// ---- 分镜 ----

func TestStoryboardModelOverride(t *testing.T) {
	ctx := context.Background()
	// 分镜请求的故事正文必须非空（user prompt 组装依赖）。
	story := domain.StoryCandidate{Index: 1, Title: "捭阖之术", Dynasty: "战国", Content: "正文一段。"}

	t.Run("请求级覆盖优先", func(t *testing.T) {
		c, argsFile := fakeBLEffects(t, chatStdout(t, storyboardFixture))
		c.TextModel = "qwen3-max"
		sb, err := c.PlanStoryboard(ctx, port.StoryboardRequest{
			Story: story, Dynasty: "战国", Ratio: "9:16", Resolution: "1080P",
			VideoStyle: "gongbi", Model: "deepseek-v3.2",
		})
		if err != nil {
			t.Fatalf("PlanStoryboard: %v", err)
		}
		if len(sb.Scenes) != 4 {
			t.Fatalf("镜头数 = %d，期望 4", len(sb.Scenes))
		}
		assertModel(t, readArgList(t, argsFile), "deepseek-v3.2")
	})

	t.Run("空值回落系统默认", func(t *testing.T) {
		c, argsFile := fakeBLEffects(t, chatStdout(t, storyboardFixture))
		c.TextModel = "qwen3-max"
		if _, err := c.PlanStoryboard(ctx, port.StoryboardRequest{
			Story: story, Dynasty: "战国", Ratio: "9:16", Resolution: "1080P",
		}); err != nil {
			t.Fatalf("PlanStoryboard: %v", err)
		}
		assertModel(t, readArgList(t, argsFile), "qwen3-max")
	})

	t.Run("双空省略参数", func(t *testing.T) {
		c, argsFile := fakeBLEffects(t, chatStdout(t, storyboardFixture))
		if _, err := c.PlanStoryboard(ctx, port.StoryboardRequest{
			Story: story, Dynasty: "战国", Ratio: "9:16", Resolution: "1080P",
		}); err != nil {
			t.Fatalf("PlanStoryboard: %v", err)
		}
		assertModel(t, readArgList(t, argsFile), "")
	})
}

// ---- 图片 ----

func TestImageModelOverride(t *testing.T) {
	ctx := context.Background()

	t.Run("请求级覆盖优先", func(t *testing.T) {
		c, argsFile := fakeBLEffects(t, "")
		c.ImageModel = "wanx2.1-t2i"
		if _, err := c.GenerateImage(ctx, port.ImageRequest{
			OutPath: filepath.Join(t.TempDir(), "panel.png"), Prompt: "工笔人物", Model: "cogview-4",
		}); err != nil {
			t.Fatalf("GenerateImage: %v", err)
		}
		assertModel(t, readArgList(t, argsFile), "cogview-4")
	})

	t.Run("空值回落系统默认", func(t *testing.T) {
		c, argsFile := fakeBLEffects(t, "")
		c.ImageModel = "wanx2.1-t2i"
		if _, err := c.GenerateImage(ctx, port.ImageRequest{
			OutPath: filepath.Join(t.TempDir(), "panel.png"), Prompt: "工笔人物",
		}); err != nil {
			t.Fatalf("GenerateImage: %v", err)
		}
		assertModel(t, readArgList(t, argsFile), "wanx2.1-t2i")
	})

	t.Run("双空省略参数", func(t *testing.T) {
		c, argsFile := fakeBLEffects(t, "")
		if _, err := c.GenerateImage(ctx, port.ImageRequest{
			OutPath: filepath.Join(t.TempDir(), "panel.png"), Prompt: "工笔人物",
		}); err != nil {
			t.Fatalf("GenerateImage: %v", err)
		}
		assertModel(t, readArgList(t, argsFile), "")
	})
}

// ---- 视频（generate 与 ref 两条路径）----

func TestVideoModelOverride(t *testing.T) {
	ctx := context.Background()

	t.Run("generate 请求级覆盖优先", func(t *testing.T) {
		c, argsFile := fakeBLEffects(t, "")
		c.VideoModel = "wan3.0-video"
		if _, err := c.GenerateClip(ctx, port.ClipRequest{
			OutPath: filepath.Join(t.TempDir(), "scene.mp4"), Prompt: "画面", Watermark: true,
			Model: "kling-v1",
		}); err != nil {
			t.Fatalf("GenerateClip: %v", err)
		}
		assertModel(t, readArgList(t, argsFile), "kling-v1")
	})

	t.Run("generate 空值回落系统默认", func(t *testing.T) {
		c, argsFile := fakeBLEffects(t, "")
		c.VideoModel = "wan3.0-video"
		if _, err := c.GenerateClip(ctx, port.ClipRequest{
			OutPath: filepath.Join(t.TempDir(), "scene.mp4"), Prompt: "画面", Watermark: true,
		}); err != nil {
			t.Fatalf("GenerateClip: %v", err)
		}
		assertModel(t, readArgList(t, argsFile), "wan3.0-video")
	})

	t.Run("generate 双空省略参数", func(t *testing.T) {
		c, argsFile := fakeBLEffects(t, "")
		if _, err := c.GenerateClip(ctx, port.ClipRequest{
			OutPath: filepath.Join(t.TempDir(), "scene.mp4"), Prompt: "画面", Watermark: true,
		}); err != nil {
			t.Fatalf("GenerateClip: %v", err)
		}
		assertModel(t, readArgList(t, argsFile), "")
	})

	// ref 路径与 generate 同款：系列 / 系统视频模型覆盖都要生效。
	for _, tc := range []struct {
		name      string
		reqModel  string
		cliModel  string
		wantModel string
	}{
		{"ref 请求级覆盖优先", "kling-v2", "wan3.0-video", "kling-v2"},
		{"ref 空值回落系统默认", "", "wan3.0-video", "wan3.0-video"},
		{"ref 双空省略参数", "", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, argsFile := fakeBLEffects(t, "")
			c.VideoModel = tc.cliModel
			if _, err := c.GenerateClip(ctx, port.ClipRequest{
				OutPath:   filepath.Join(t.TempDir(), "scene.mp4"),
				Prompt:    "Image 1 张仪的画面",
				RefImages: []string{"/tmp/ref.png"},
				Watermark: true,
				Model:     tc.reqModel,
			}); err != nil {
				t.Fatalf("GenerateClip ref: %v", err)
			}
			args := readArgList(t, argsFile)
			assertModel(t, args, tc.wantModel)
			if args[0] != "video" || args[1] != "ref" {
				t.Fatalf("子命令应为 video ref: %v", args)
			}
			if !hasArg(args, "--image") {
				t.Fatalf("ref 路径必须带参考图: %v", args)
			}
		})
	}
}

// ---- 语音 ----
// 注意：本用例不带 Instruction——带指令失败会触发自动降级重试（第二次调用），
// 会把 args.txt 覆盖成重试那一次的参数，断言就不可信了。
func TestSpeechModelOverride(t *testing.T) {
	ctx := context.Background()

	t.Run("请求级覆盖优先", func(t *testing.T) {
		c, argsFile := fakeBLEffects(t, "")
		c.TTSModel = "cosyvoice-v3-flash"
		if _, err := c.Synthesize(ctx, port.SpeechRequest{
			OutPath: filepath.Join(t.TempDir(), "a.mp3"), Text: "旁白", Voice: "longtian_v3",
			Model: "cosyvoice-v3.5-plus",
		}); err != nil {
			t.Fatalf("Synthesize: %v", err)
		}
		assertModel(t, readArgList(t, argsFile), "cosyvoice-v3.5-plus")
	})

	t.Run("空值回落系统默认", func(t *testing.T) {
		c, argsFile := fakeBLEffects(t, "")
		c.TTSModel = "cosyvoice-v3-flash"
		if _, err := c.Synthesize(ctx, port.SpeechRequest{
			OutPath: filepath.Join(t.TempDir(), "a.mp3"), Text: "旁白", Voice: "longtian_v3",
		}); err != nil {
			t.Fatalf("Synthesize: %v", err)
		}
		assertModel(t, readArgList(t, argsFile), "cosyvoice-v3-flash")
	})

	t.Run("双空省略参数", func(t *testing.T) {
		c, argsFile := fakeBLEffects(t, "")
		if _, err := c.Synthesize(ctx, port.SpeechRequest{
			OutPath: filepath.Join(t.TempDir(), "a.mp3"), Text: "旁白", Voice: "longtian_v3",
		}); err != nil {
			t.Fatalf("Synthesize: %v", err)
		}
		args := readArgList(t, argsFile)
		assertModel(t, args, "")
		if args[0] != "speech" || args[1] != "synthesize" {
			t.Fatalf("子命令应为 speech synthesize: %v", args)
		}
		if hasArg(args, "--instruction") {
			t.Fatalf("未带指令时不应出现 --instruction: %v", args)
		}
	})
}
