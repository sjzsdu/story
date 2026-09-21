package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/sjzsdu/story/internal/domain"
	"github.com/sjzsdu/story/internal/engine"
	"github.com/sjzsdu/story/internal/port"
	sqlitestore "github.com/sjzsdu/story/internal/store/sqlite"
	"github.com/sjzsdu/story/testutil/mock"
)

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"鬼谷子":     "guiguzi",
		"三国":      "sanguo",
		"Tang 盛唐": "tang-shengtang",
		"":        "series-",
	}
	for in, prefix := range cases {
		got := Slugify(in)
		if in == "" {
			// 空名称走时间戳兜底，只检查前缀。
			if len(got) < len("series-") || got[:7] != "series-" {
				t.Fatalf("空名称 slug 异常: %s", got)
			}
			continue
		}
		if got != prefix {
			t.Errorf("Slugify(%q) = %q, 期望 %q", in, got, prefix)
		}
	}
}

// TestResolveVoiceBuilder 覆盖造声供应商分发：空值回退 bailian，显式未知供应商必须报错。
func TestResolveVoiceBuilder(t *testing.T) {
	bl := &mock.VoiceBuild{}
	a := &App{voiceBuilders: map[string]port.VoiceBuilder{
		domain.VoiceProviderBailian: bl,
	}}

	// 空值 → 默认 bailian。
	p, b, err := a.resolveVoiceBuilder("")
	if err != nil || p != domain.VoiceProviderBailian || b != port.VoiceBuilder(bl) {
		t.Fatalf("空 provider 应回退 bailian，得到 (%q, %v, %v)", p, b, err)
	}

	// 大小写/空白容错。
	if _, _, err := a.resolveVoiceBuilder("  Bailian "); err != nil {
		t.Fatalf("大小写/空白应归一：%v", err)
	}

	// 显式未知供应商 → 报错（不得静默回退 bailian）。
	if _, _, err := a.resolveVoiceBuilder("iflytek"); err == nil {
		t.Fatal("未知供应商应报错，而非静默回退 bailian")
	}
}

// TestSaveVoiceSample 覆盖参考音频保存与归一化（§16 复刻）：
// 时长下限拦截、归一化失败传播、未装配报错、恶意文件名不逃出目标目录。
func TestSaveVoiceSample(t *testing.T) {
	t.Run("时长合法返回归一化路径", func(t *testing.T) {
		norm := &mock.AudioNorm{Duration: 12.5}
		a := &App{audioNormalizer: norm}
		got, err := a.SaveVoiceSample(context.Background(), strings.NewReader("fake-webm"), "recording.webm")
		if err != nil {
			t.Fatalf("保存参考音频失败: %v", err)
		}
		if got.DurationSec != 12.5 {
			t.Errorf("时长 = %v, 期望 12.5", got.DurationSec)
		}
		if filepath.Ext(got.Path) != ".wav" {
			t.Errorf("归一化产物应为 wav，得到 %s", got.Path)
		}
		if _, err := os.Stat(got.Path); err != nil {
			t.Errorf("归一化产物不存在: %v", err)
		}
		// 原始件应落在同一个 story-voice-samples 目录下，且保留原始录音的扩展名。
		src := norm.LastSrc()
		if filepath.Dir(src) != filepath.Dir(got.Path) {
			t.Errorf("原始件目录 %s 与产物目录 %s 不一致", filepath.Dir(src), filepath.Dir(got.Path))
		}
		if filepath.Ext(src) != ".webm" {
			t.Errorf("原始件应保留 .webm 扩展名，得到 %s", src)
		}
	})

	t.Run("时长过短被拦截", func(t *testing.T) {
		norm := &mock.AudioNorm{Duration: 1.2}
		a := &App{audioNormalizer: norm}
		if _, err := a.SaveVoiceSample(context.Background(), strings.NewReader("x"), "a.wav"); err == nil {
			t.Fatal("时长低于下限应报错")
		}
	})

	t.Run("归一化失败向上传播", func(t *testing.T) {
		norm := &mock.AudioNorm{Err: errors.New("ffmpeg 挂了")}
		a := &App{audioNormalizer: norm}
		if _, err := a.SaveVoiceSample(context.Background(), strings.NewReader("x"), "a.wav"); err == nil {
			t.Fatal("归一化失败应报错")
		}
	})

	t.Run("未装配归一化能力报错", func(t *testing.T) {
		a := &App{}
		if _, err := a.SaveVoiceSample(context.Background(), strings.NewReader("x"), "a.wav"); err == nil {
			t.Fatal("audioNormalizer 为空应报错")
		}
	})

	t.Run("恶意文件名不逃出目标目录", func(t *testing.T) {
		norm := &mock.AudioNorm{Duration: 9}
		a := &App{audioNormalizer: norm}
		got, err := a.SaveVoiceSample(context.Background(), strings.NewReader("x"), "../../etc/passwd")
		if err != nil {
			t.Fatalf("保存失败: %v", err)
		}
		wantDir := filepath.Join(os.TempDir(), "story-voice-samples")
		if filepath.Dir(got.Path) != wantDir {
			t.Errorf("产物目录 = %s, 期望 %s", filepath.Dir(got.Path), wantDir)
		}
		if src := norm.LastSrc(); filepath.Dir(src) != wantDir {
			t.Errorf("原始件目录 = %s, 期望 %s（路径穿越必须被截断）", filepath.Dir(src), wantDir)
		}
	})
}

// TestMigrateLegacyEpisode 覆盖 §17 旧集迁移：停在 pick 中间态的数据能取到被选故事、
// 旧布局产物搬进 versions/<节点键>/ 且登记路径改挂、迁移幂等。
func TestMigrateLegacyEpisode(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "story.db")
	store, err := sqlitestore.Open(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	workRoot := t.TempDir()
	now := time.Now()
	series := &domain.Series{
		ID: "guiguzi", Name: "鬼谷子", Dynasty: "战国",
		Config:    domain.SeriesConfig{Dynasty: "战国", Ratio: "9:16", Resolution: "1080P", TTSVoice: "longtian_v3"},
		CreatedAt: now, UpdatedAt: now,
	}
	if err := store.CreateSeries(ctx, series); err != nil {
		t.Fatal(err)
	}
	workDir := filepath.Join(workRoot, "guiguzi", "guiguzi-e01")
	for _, sub := range []string{"clips", "audio", "tmp", "output"} {
		if err := os.MkdirAll(filepath.Join(workDir, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	clipPath := filepath.Join(workDir, "clips", "scene-01.mp4")
	audioPath := filepath.Join(workDir, "audio", "scene-01.mp3")
	outPath := filepath.Join(workDir, "output", "guiguzi-e01-9x16.mp4")
	for _, p := range []string{filepath.Join(workDir, "story.md"), clipPath, audioPath, outPath} {
		if err := os.WriteFile(p, []byte("old"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ep := domain.NewEpisode("guiguzi-e01", "guiguzi", 1, "捭阖之术", "", workDir)
	if err := store.CreateEpisode(ctx, ep); err != nil {
		t.Fatal(err)
	}

	// 旧流水线状态：停在 generate 与 pick 之间（只有候选与选中序号，没有 Story）。
	legacy := fmt.Sprintf(`{
		"candidates":[{"index":1,"title":"捭阖初试","dynasty":"战国","source":"《鬼谷子》","content":"正文"}],
		"selected":1,
		"storyboard":{"scenes":[{"id":1,"visual_prompt":"v","narration":"n","duration_sec":5}]},
		"clips":[{"scene_id":1,"path":%q,"duration_sec":5}],
		"audios":[{"scene_id":1,"path":%q,"duration_sec":5}],
		"outputs":[%q]
	}`, clipPath, audioPath, outPath)
	seedLegacyState(t, dbPath, ep.ID, legacy)

	eng := engine.New(store, &mock.StoryGen{}, &mock.BoardPlanner{}, &mock.VideoGen{},
		&mock.SpeechGen{}, &mock.Composer{SceneDuration: 5}, &mock.SeriesPlanner{}, &mock.ImageGen{},
		workRoot, 2, 2, "longtian_v3", "沉稳")
	a := &App{Repo: store, Engine: eng}

	if err := MigrateEpisodesToVersionTree(ctx, a, store); err != nil {
		t.Fatalf("迁移: %v", err)
	}

	got, err := store.GetEpisode(ctx, ep.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Nodes) != 4 {
		t.Fatalf("节点数 = %d，期望 4", len(got.Nodes))
	}
	if path := got.ActivePath(); len(path) != 4 {
		t.Fatalf("活跃路径 = %d 段，期望 4", len(path))
	}
	for _, st := range domain.AllStages() {
		if got.ActiveNodeOfStage(st) == nil {
			t.Fatalf("活跃路径缺少「%s」节点", domain.StageLabel(st))
		}
	}
	// 停在 pick 的旧数据：故事取自被选中的候选。
	story := got.ActiveNodeOfStage(domain.StageStory)
	if story.Story == nil || story.Story.Title != "捭阖初试" {
		t.Fatalf("应取到被选候选作为故事: %+v", story.Story)
	}
	// 旧布局产物已搬进版本目录，旧位置不再存在。
	media := got.ActiveNodeOfStage(domain.StageMedia)
	if _, err := os.Stat(filepath.Join(media.Dir, "clips", "scene-01.mp4")); err != nil {
		t.Fatalf("片段应已搬进版本目录: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workDir, "clips")); !os.IsNotExist(err) {
		t.Fatal("旧 clips 目录应已搬走")
	}
	if len(media.Clips) != 1 || !strings.HasPrefix(media.Clips[0].Path, media.Dir) {
		t.Fatalf("登记的画面路径应改挂到版本目录: %+v", media.Clips)
	}
	final := got.ActiveNodeOfStage(domain.StageFinal)
	if len(final.Outputs) != 1 || !strings.HasPrefix(final.Outputs[0], final.Dir) {
		t.Fatalf("登记的成片路径应改挂到版本目录: %+v", final.Outputs)
	}

	// 幂等：重复迁移不新增节点、不重复搬动。
	if err := MigrateEpisodesToVersionTree(ctx, a, store); err != nil {
		t.Fatalf("重复迁移: %v", err)
	}
	again, err := store.GetEpisode(ctx, ep.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Nodes) != 4 {
		t.Fatalf("重复迁移不应新增节点: %d", len(again.Nodes))
	}
	if again.ActiveNodeID != got.ActiveNodeID {
		t.Fatalf("重复迁移不应改变活跃指针: %s → %s", got.ActiveNodeID, again.ActiveNodeID)
	}
}

// seedLegacyState 直接写旧 state_json（迁移前的历史快照，新代码不再写入该列）。
func seedLegacyState(t *testing.T, dbPath, id, raw string) {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`UPDATE episodes SET state_json = ? WHERE id = ?`, raw, id); err != nil {
		t.Fatal(err)
	}
}

// TestSanitizeAudioExt 覆盖扩展名白名单（不信任上传文件名）。
func TestSanitizeAudioExt(t *testing.T) {
	cases := map[string]string{
		"recording.webm":   ".webm",
		"RECORDING.WAV":    ".wav",
		"sample-v3.m4a":    ".m4a",
		"name.tar.gz":      ".gz",
		"../../etc/passwd": ".bin", // 无扩展名
		"evil.wav/../../x": ".bin", // Base 后无扩展名
		"weird.<script>":   ".bin", // 非法字符
		"toolongextension": ".bin", // 无扩展名
		"noext":            ".bin",
		"a.x":              ".x",   // 单字符扩展名合法，仅作命名提示
		"a.":               ".bin", // 只有点，无扩展名
	}
	for in, want := range cases {
		if got := sanitizeAudioExt(in); got != want {
			t.Errorf("sanitizeAudioExt(%q) = %q, 期望 %q", in, got, want)
		}
	}
}
