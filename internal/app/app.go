// Package app 是唯一的依赖装配层：把具体 provider 注入 engine。
// 除 cmd/story 外，只有本包允许 import 具体实现。
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/mozillazg/go-pinyin"

	"github.com/sjzsdu/story/internal/capability"
	"github.com/sjzsdu/story/internal/config"
	"github.com/sjzsdu/story/internal/domain"
	"github.com/sjzsdu/story/internal/engine"
	"github.com/sjzsdu/story/internal/port"
	bailianprov "github.com/sjzsdu/story/internal/provider/bailian"
	deepseekprov "github.com/sjzsdu/story/internal/provider/deepseek"
	ffmpegprov "github.com/sjzsdu/story/internal/provider/ffmpeg"
	klingprov "github.com/sjzsdu/story/internal/provider/kling"
	minimaxprov "github.com/sjzsdu/story/internal/provider/minimax"
	zhipuprov "github.com/sjzsdu/story/internal/provider/zhipu"
	sqlitestore "github.com/sjzsdu/story/internal/store/sqlite"
	"github.com/sjzsdu/story/internal/subtitle"
	"github.com/sjzsdu/story/internal/templates"
)

// App 应用容器，CLI 命令通过它访问全部能力。
//
// 能力装配统一走 §21 能力注册表：engine 的 6 个 AI 能力槽由本容器持有（经 Engine 字段），
// 本容器另持 voice_build / voice_list / publish 三个槽；三者都用 capability.Slot 泛型槽，
// 沿「系列覆盖 → 系统默认」两级链路解析，设置热更新时整表原子换新（Reconfigure）。
type App struct {
	Repo   port.Repository
	Engine *engine.Engine

	// cfg 当前生效配置。加锁读写：PUT /api/settings 热更新时整体换新，
	// 读取一律经 Config()（返回副本），避免与热更新竞态。
	cfg   config.Config
	cfgMu sync.RWMutex
	// configPath story.yaml 路径（Bootstrap 记录）；空串＝不落盘（如单测）。
	configPath string

	// voiceListers 系统音色列表查询（浏览音色库）能力槽，key = 供应商标识
	// （domain.VoiceProviderXxx）；与 voiceBuilders 同构，按供应商分发。
	voiceListers *capability.Slot[port.VoiceLister]
	// voiceBuilders 造声能力（声音设计/声音复刻）槽，key = 供应商标识
	//（domain.VoiceProviderXxx）。造声与供应商强绑定（模型/音色体系各异），
	// 故按供应商分发而非持有单一实例；接第二家 TTS 时在此加一项即可。
	voiceBuilders *capability.Slot[port.VoiceBuilder]
	// publish 平台发布能力槽，key = string(domain.Platform)（§19）。
	// 接入新平台时在此加一项，engine/CLI 逻辑零改动。
	publish *capability.Slot[port.PlatformPublisher]
	// audioNormalizer 参考音频归一化（§16 声音复刻：浏览器录音/上传件统一转
	// 16kHz 单声道 wav 再提交）；由 ffmpeg provider 实现（不随热更新换，见 §21）。
	audioNormalizer port.AudioNormalizer
	// assetProber 素材时长探测（§23，ffmpeg ProbeDuration 注入）。
	// 可为 nil，且探测失败一律容错置 0——单测不依赖本机 ffprobe。
	assetProber func(ctx context.Context, path string) (float64, error)
}

// Config 返回当前生效配置的副本（加锁读，热更新后立即可见新值）。
func (a *App) Config() config.Config {
	a.cfgMu.RLock()
	defer a.cfgMu.RUnlock()
	return a.cfg
}

// setConfig 覆盖生效配置（调用方须已持锁或处于单写场景）。
func (a *App) setConfig(cfg config.Config) {
	a.cfgMu.Lock()
	a.cfg = cfg
	a.cfgMu.Unlock()
}

// ---- 能力实现构建（§21）----

// providerSet 一次构建出的全部能力实现（key ＝ 供应商/平台标识）。
// 构造 provider 客户端不会失败（凭据缺失要到真正调用时才报错），
// 因此 buildProviders 无 error 返回；「整体不生效」的失败只会发生在登记阶段。
type providerSet struct {
	text  map[string]port.StoryGenerator
	board map[string]port.StoryboardPlanner
	plan  map[string]port.SeriesPlanner
	tts   map[string]port.SpeechSynthesizer
	image map[string]port.ImageGenerator
	video map[string]port.VideoGenerator

	// §22：视觉理解 / 视频理解 / 音效三个新能力（当前无系列级覆盖，
	// 默认取自系统配置 *_provider，使用时可单次指定 override）。
	imageUnderstand map[string]port.ImageUnderstander
	videoUnderstand map[string]port.VideoUnderstander
	sfx             map[string]port.SoundEffectGenerator

	voiceBuilders map[string]port.VoiceBuilder
	voiceListers  map[string]port.VoiceLister
	publish       map[string]port.PlatformPublisher
}

// buildProviders 按配置构建全部能力实现的具名登记表。
//
// 注意每张表都**完整列出该能力全部可用供应商**（不是只建当前选中的那个）：
// 系列级覆盖可以指定任意一家，系统默认才由 defaultKeyFor 决定。
func buildProviders(cfg config.Config) providerSet {
	bl := bailianprov.NewClient(cfg.BLBin, cfg.TextModel, cfg.VideoModel, cfg.TTSModel, cfg.ImageModel,
		cfg.BailianAPIKey, cfg.BailianBaseURL)
	ds := deepseekprov.NewClient(cfg.DeepSeekAPIKey, cfg.DeepSeekBaseURL, cfg.DeepSeekModel)
	mm := minimaxprov.NewClient(cfg.MinimaxAPIKey, cfg.MinimaxBaseURL, cfg.MinimaxModel)
	zp := zhipuprov.NewClient(cfg.ZhipuAPIKey, cfg.ZhipuBaseURL, "")
	kl := klingprov.NewClient(cfg.KlingAccessKey, cfg.KlingSecretKey, cfg.KlingBaseURL, "")

	return providerSet{
		// 故事/分镜：bailian + deepseek。
		text:  map[string]port.StoryGenerator{"bailian": bl, "deepseek": ds},
		board: map[string]port.StoryboardPlanner{"bailian": bl, "deepseek": ds},
		// 系列策划：deepseek 客户端暂不实现 SeriesPlanner（多轮对话 + 复杂 JSON
		// 结构仍走 bl），故 deepseek 槽位登记 bl 实例——语义为「选 deepseek 时策划
		// 会话沿用 bl」，与历史行为一致，同时让显式覆盖不会被判成未知供应商。
		plan: map[string]port.SeriesPlanner{"bailian": bl, "deepseek": bl},
		// 语音合成：bailian + minimax。
		tts: map[string]port.SpeechSynthesizer{"bailian": bl, "minimax": mm},
		// 图片：bailian + zhipu。
		image: map[string]port.ImageGenerator{"bailian": bl, "zhipu": zp},
		// 视频：bailian + kling。
		video: map[string]port.VideoGenerator{"bailian": bl, "kling": kl},

		// §22：图/视频理解走 bl vision describe（当前只有百炼）；
		// 音效 bl 无对应命令 → 空表（等首个 provider 落地再登记）。
		imageUnderstand: map[string]port.ImageUnderstander{"bailian": bl},
		videoUnderstand: map[string]port.VideoUnderstander{"bailian": bl},
		sfx:             map[string]port.SoundEffectGenerator{},

		// 造声/系统音色：当前只有百炼；新增供应商时在此各登记一项。
		voiceBuilders: map[string]port.VoiceBuilder{domain.VoiceProviderBailian: bl},
		voiceListers:  map[string]port.VoiceLister{domain.VoiceProviderBailian: bl},
		// §19：平台发布能力。
		publish: buildPublishProviders(cfg),
	}
}

// defaultKeyFor 从配置取值解析某能力的默认 key：
// 取值非空且已登记 → 用它；空值/未登记 → 回退 "bailian"（系统内置默认）；
// 表里连 bailian 都没有 → 取升序第一个；空表 → 空串（该能力尚无实现）。
//
// 这里刻意**不**把未知配置值直接透传给 ReplaceAll：配置文件里残留的旧值
// （如已下线的供应商名）不应让整个应用起不来，兜底到内置默认最稳。
func defaultKeyFor[T any](want string, impls map[string]T) string {
	k := strings.ToLower(strings.TrimSpace(want))
	if k != "" {
		if _, ok := impls[k]; ok {
			return k
		}
	}
	if _, ok := impls["bailian"]; ok {
		return "bailian"
	}
	keys := make([]string, 0, len(impls))
	for key := range impls {
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return ""
	}
	// 稳定起见取最小 key。
	min := keys[0]
	for _, key := range keys[1:] {
		if key < min {
			min = key
		}
	}
	return min
}

// applyProviders 把一组能力实现整表登记进各能力槽并设好默认
// （engine 6 槽 + app 3 槽）。任一槽登记失败即返回错误（整组配置不生效）。
func (a *App) applyProviders(p providerSet, cfg config.Config) error {
	txtKey := defaultKeyFor(cfg.TextProvider, p.text)
	if err := a.Engine.Text.ReplaceAll(p.text, txtKey); err != nil {
		return err
	}
	// 分镜与系列策划共用 text_provider 作默认取值（board/plan 无独立配置项）。
	if err := a.Engine.Board.ReplaceAll(p.board, defaultKeyFor(cfg.TextProvider, p.board)); err != nil {
		return err
	}
	if err := a.Engine.Plan.ReplaceAll(p.plan, defaultKeyFor(cfg.TextProvider, p.plan)); err != nil {
		return err
	}
	if err := a.Engine.TTS.ReplaceAll(p.tts, defaultKeyFor(cfg.TTSProvider, p.tts)); err != nil {
		return err
	}
	if err := a.Engine.Image.ReplaceAll(p.image, defaultKeyFor(cfg.ImageProvider, p.image)); err != nil {
		return err
	}
	if err := a.Engine.Video.ReplaceAll(p.video, defaultKeyFor(cfg.VideoProvider, p.video)); err != nil {
		return err
	}
	// §22：三个新能力仅系统默认（无系列覆盖），空 override 调用时报错而非静默。
	if err := a.Engine.ImageUnderstand.ReplaceAll(p.imageUnderstand,
		defaultKeyFor(cfg.ImageUnderstandProvider, p.imageUnderstand)); err != nil {
		return err
	}
	if err := a.Engine.VideoUnderstand.ReplaceAll(p.videoUnderstand,
		defaultKeyFor(cfg.VideoUnderstandProvider, p.videoUnderstand)); err != nil {
		return err
	}
	if err := a.Engine.SFX.ReplaceAll(p.sfx, defaultKeyFor(cfg.SFXProvider, p.sfx)); err != nil {
		return err
	}
	if err := a.voiceBuilders.ReplaceAll(p.voiceBuilders,
		defaultKeyFor(domain.VoiceProviderBailian, p.voiceBuilders)); err != nil {
		return err
	}
	if err := a.voiceListers.ReplaceAll(p.voiceListers,
		defaultKeyFor(domain.VoiceProviderBailian, p.voiceListers)); err != nil {
		return err
	}
	// 发布槽只做枚举，不设默认（发布必须显式指定平台）。
	if err := a.publish.ReplaceAll(p.publish, ""); err != nil {
		return err
	}
	return nil
}

// Reconfigure 用新配置热更新：重建全部能力实现并整表换新各能力槽，
// 随后写入生效配置（顺序：先换槽、再换 cfg，槽登记失败则 cfg 保持旧值）。
//
// 生效范围与需重启项见 AGENTS.md §21；本方法只负责能力槽与 cfg，
// 不动 composer/ffmpeg/字幕字体、engine 默认语音回退与 projectsDir。
func (a *App) Reconfigure(cfg config.Config) error {
	if err := a.applyProviders(buildProviders(cfg), cfg); err != nil {
		return err
	}
	a.setConfig(cfg)
	return nil
}

// SaveConfig 把当前生效配置写回 story.yaml（configPath 为空则跳过，如单测）。
func (a *App) SaveConfig() error {
	if a.configPath == "" {
		return nil
	}
	return config.Save(a.configPath, a.Config())
}

// CapabilityInfo 返回能力目录 + 各槽运行时登记结果（GET /api/capabilities、`story capabilities`）。
func (a *App) CapabilityInfo() capability.Info {
	return capability.Info{
		Capabilities: []capability.CapabilityInfo{
			buildCapInfo(capability.Text, a.Engine.Text),
			buildCapInfo(capability.Board, a.Engine.Board),
			buildCapInfo(capability.Plan, a.Engine.Plan),
			buildCapInfo(capability.TTS, a.Engine.TTS),
			buildCapInfo(capability.Image, a.Engine.Image),
			buildCapInfo(capability.Video, a.Engine.Video),
			buildCapInfo(capability.ImageUnderstand, a.Engine.ImageUnderstand),
			buildCapInfo(capability.VideoUnderstand, a.Engine.VideoUnderstand),
			buildCapInfo(capability.SFX, a.Engine.SFX),
			buildCapInfo(capability.VoiceBuild, a.voiceBuilders),
			buildCapInfo(capability.VoiceList, a.voiceListers),
			buildCapInfo(capability.Publish, a.publish),
		},
		Providers: capability.KnownProviders(),
	}
}

// DescribeImage 图像理解入口（§22）：补上配置的默认理解模型（cfg.VisionModel，
// 空则由 provider 省略 --model 走 bl 默认），再透传 engine（override 单次指定实现）。
func (a *App) DescribeImage(ctx context.Context, req port.DescribeImageRequest, override string) (port.DescribeResult, error) {
	if req.Model == "" {
		req.Model = a.Config().VisionModel
	}
	return a.Engine.DescribeImage(ctx, req, override)
}

// DescribeVideo 视频理解入口（§22），模型透传语义同 DescribeImage。
func (a *App) DescribeVideo(ctx context.Context, req port.DescribeVideoRequest, override string) (port.DescribeResult, error) {
	if req.Model == "" {
		req.Model = a.Config().VisionModel
	}
	return a.Engine.DescribeVideo(ctx, req, override)
}

// GenerateSoundEffect 音效生成入口（§22）：补 cfg.SFXModel（预留字段）后透传。
// 当前无 provider 注册，调用会返回能力槽的「未配置默认实现」错误（三处降级之一）。
func (a *App) GenerateSoundEffect(ctx context.Context, req port.SoundEffectRequest, override string) (port.SoundEffectResult, error) {
	if req.Model == "" {
		req.Model = a.Config().SFXModel
	}
	return a.Engine.GenerateSoundEffect(ctx, req, override)
}

// slotEnum 能力槽的枚举视图（capability.Slot 泛型实例天然满足）。
type slotEnum interface {
	Keys() []string
	Default() string
}

// buildCapInfo 把静态目录项与运行时槽状态拼成下发视图。
func buildCapInfo[T any](capKey string, s *capability.Slot[T]) capability.CapabilityInfo {
	var c capability.Capability
	for _, item := range capability.Catalog() {
		if item.Key == capKey {
			c = item
			break
		}
	}
	var e slotEnum = s
	def := e.Default()
	// 槽里默认 key 仍是装配期占位 "default" 时对前端隐藏（不具信息量）。
	if def == capability.DefaultKey {
		def = ""
	}
	out := capability.CapabilityInfo{
		Key: c.Key, Label: c.Label, Help: c.Help,
		ConfigField: c.ConfigField, SeriesField: c.SeriesField,
		Default:   def,
		Providers: make([]capability.Provider, 0, len(e.Keys())),
	}
	for _, k := range e.Keys() {
		out.Providers = append(out.Providers, capability.ProviderInfo(k))
	}
	return out
}

// Bootstrap 装配整个应用（打开数据库、构造 provider 与 engine）。
//
// configPath 是 story.yaml 的路径，记录下来供 PUT /api/settings 落盘（热更新）；
// 空串＝不落盘（单测用）。落盘能力见 Reconfigure/SaveConfig。
func Bootstrap(ctx context.Context, cfg config.Config, configPath string) (*App, error) {
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建数据目录: %w", err)
	}
	store, err := sqlitestore.Open(ctx, cfg.DBPath())
	if err != nil {
		return nil, err
	}

	// §16：seed 内置声音条目（幂等）+ 旧 series 平迁 voice_id（仅迁移无 voice_id 的行）。
	if err := sqlitestore.SeedBuiltinVoices(ctx, store, templates.VoicePresets); err != nil {
		return nil, fmt.Errorf("seed 内置声音: %w", err)
	}
	if err := sqlitestore.MigrateSeriesVoiceIDs(ctx, store, cfg.TTSVoice, cfg.TTSInstruction); err != nil {
		return nil, fmt.Errorf("迁移系列 voice_id: %w", err)
	}
	// 2026-09-20：名人化名 ID（wangliqun 等）修正为音色真实 ID（幂等）。
	if err := sqlitestore.RemapLegacyBuiltinVoices(ctx, store); err != nil {
		return nil, fmt.Errorf("重映射旧内置声音: %w", err)
	}

	// ffmpeg 合成器（composer）不在能力槽内：全项目只有一种实现，
	// 且字幕字体渲染器在构造时绑定，热更新不换（§21 需重启项）。
	composer := ffmpegprov.New(cfg.FFMPEGBin, cfg.FFProbeBin())
	if cfg.SubtitleFont != "" {
		r, err := subtitle.NewRenderer(cfg.SubtitleFont)
		if err != nil {
			return nil, err
		}
		composer.WithSubtitles(r)
	}

	// 全部能力实现按配置一次性构建（每张表含该能力所有可用供应商）。
	set := buildProviders(cfg)

	eng := engine.New(
		store,
		set.text[defaultKeyFor(cfg.TextProvider, set.text)],
		set.board[defaultKeyFor(cfg.TextProvider, set.board)],
		set.video[defaultKeyFor(cfg.VideoProvider, set.video)],
		set.tts[defaultKeyFor(cfg.TTSProvider, set.tts)],
		composer,
		set.plan[defaultKeyFor(cfg.TextProvider, set.plan)],
		set.image[defaultKeyFor(cfg.ImageProvider, set.image)],
		cfg.ProjectsDir(),
		cfg.MaxConcurrency, cfg.MaxRetries,
		cfg.TTSVoice, cfg.TTSInstruction,
	)

	app := &App{
		Repo:   store,
		Engine: eng,
		// 造声/系统音色/平台发布三个能力槽（engine 6 槽由 engine.New 建好）。
		voiceBuilders:   capability.NewSlot[port.VoiceBuilder]("造声"),
		voiceListers:    capability.NewSlot[port.VoiceLister]("音色库列举"),
		publish:         capability.NewSlot[port.PlatformPublisher]("平台发布"),
		audioNormalizer: composer, // 参考音频归一化复用 ffmpeg composer（§16）
		assetProber:     composer.ProbeDuration, // §23 素材时长探测（容错，见 asset.go）
		configPath:      configPath,
	}
	// 整表登记 + 设默认（engine 6 槽一并升级为具名 key）。
	if err := app.applyProviders(set, cfg); err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("登记能力实现: %w", err)
	}
	app.setConfig(cfg)

	if err := bootstrapApp(ctx, app, store); err != nil {
		return nil, err
	}
	return app, nil
}

// bootstrapApp 装配完成后的收尾：§17 旧集一次性迁移到版本树（幂等）。
func bootstrapApp(ctx context.Context, a *App, store *sqlitestore.Store) error {
	if err := MigrateEpisodesToVersionTree(ctx, a, store); err != nil {
		return fmt.Errorf("迁移旧集到版本树: %w", err)
	}
	return nil
}

// Close 释放资源。
func (a *App) Close() error {
	return a.Repo.Close()
}

// CreateSeriesInput 创建系列的参数。
type CreateSeriesInput struct {
	Name        string
	Dynasty     string
	Description string
	Ratio       string
	Resolution  string
	VisualMode  string
	// VoiceID 选定的声音条目 ID（顶层 Voice 实体，必填）。
	// 创建后锁定不可改（store.UpdateSeries SQL 不含 voice_id 列）。
	VoiceID string
	// VoiceProfile/TTSInstruction 兼容旧请求体：未传 VoiceID 时按平迁规则现场建/取一个。
	VoiceProfile   string
	Voice          string
	TTSInstruction string
	Concurrency    int
	Retries        int
	// VideoStyle 全片画风（templates.VisualStyles 的 key，空＝默认画风）。
	VideoStyle string
	// Creative 创作控制参数（叙事/受众/篇幅/运镜/自定义指令）。
	// 零值＝内置默认，产出与历史行为完全一致。
	Creative domain.CreativeStyle
	// ---- 成片 BGM 背景音乐（§20）----
	// BGMPath 曲目路径：相对路径相对系列目录 data/projects/<series-id>/，空＝无 BGM。
	BGMPath string
	// BGMVolume 0..1 相对音量；0＝未设置（用默认 0.18）。
	BGMVolume float64
	// ---- 系列级 Provider 覆盖（空＝用系统默认） ----
	TextProvider  string
	TTSProvider   string
	ImageProvider string
	VideoProvider string
	// ---- 系列级模型覆盖（第二步统一资源管理；空＝跟随系统默认） ----
	// 绝不回填系统默认模型值：存进 config_json 会让存量/默认系列字节不一致、
	// 派生键全变（§17/§18 铁律）。空串即「跟随系统」。
	TextModel  string
	TTSModel   string
	ImageModel string
	VideoModel string
}

// CreateSeries 创建一个新系列（ID 由名称生成拼音 slug，冲突时追加序号）。
// 必须指定 VoiceID；未指定时按平迁规则现场建/取一个（兼容旧 CLI/Web 请求）。
func (a *App) CreateSeries(ctx context.Context, in CreateSeriesInput) (*domain.Series, error) {
	id := Slugify(in.Name)
	id, err := a.uniqueSeriesID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := validateBGMVolume(in.BGMVolume); err != nil {
		return nil, err
	}
	// §23：BGM 为 asset:<id> 引用时校验素材存在（创建即校验，避免带病引用）。
	if err := a.validateAssetRef(ctx, strings.TrimSpace(in.BGMPath)); err != nil {
		return nil, err
	}

	// §16：voice_id 必填；未传时按平迁规则现场建/取一个。
	voiceID := strings.TrimSpace(in.VoiceID)
	if voiceID == "" {
		vid, err := a.resolveCreateSeriesVoiceID(ctx, in)
		if err != nil {
			return nil, err
		}
		voiceID = vid
	} else if _, err := a.Repo.GetVoice(ctx, voiceID); err != nil {
		return nil, translateErr(fmt.Errorf("声音 %s 不存在: %w", voiceID, err))
	}

	now := time.Now()
	cfg := a.Config()
	se := &domain.Series{
		ID:          id,
		Name:        in.Name,
		Dynasty:     in.Dynasty,
		Description: in.Description,
		VoiceID:     voiceID,
		Config: domain.SeriesConfig{
			Dynasty:        in.Dynasty,
			Ratio:          firstNonEmpty(in.Ratio, cfg.DefaultRatio),
			Resolution:     firstNonEmpty(in.Resolution, cfg.DefaultResolution),
			VisualMode:     domain.NormalizeVisualMode(in.VisualMode),
			VoiceProfile:   in.VoiceProfile,
			TTSVoice:       firstNonEmpty(in.Voice, cfg.TTSVoice),
			TTSInstruction: firstNonEmpty(in.TTSInstruction, cfg.TTSInstruction),
			MaxConcurrency: orDefault(in.Concurrency, cfg.MaxConcurrency),
			MaxRetries:     orDefault(in.Retries, cfg.MaxRetries),
			VideoStyle:     in.VideoStyle,
			Creative:       in.Creative,
			// 成片 BGM（§20）：相对路径相对系列目录，音量 0＝默认 0.18。
			BGMPath:   strings.TrimSpace(in.BGMPath),
			BGMVolume: in.BGMVolume,
			// 系列级 Provider 覆盖：空＝用系统默认（engine resolveProviders 时回退）。
			TextProvider:  in.TextProvider,
			TTSProvider:   in.TTSProvider,
			ImageProvider: in.ImageProvider,
			VideoProvider: in.VideoProvider,
			// 系列级模型覆盖：原样存入，空串＝跟随系统（绝不回填 cfg.TextModel 等默认值）。
			TextModel:  in.TextModel,
			TTSModel:   in.TTSModel,
			ImageModel: in.ImageModel,
			VideoModel: in.VideoModel,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := a.Repo.CreateSeries(ctx, se); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(a.Config().ProjectsDir(), id), 0o755); err != nil {
		return nil, err
	}
	return se, nil
}

// resolveCreateSeriesVoiceID 旧请求体未传 VoiceID 时按平迁规则现场建/取一个。
// 1) VoiceProfile 非空 → 对应内置条目 ID（不存在则失败）。
// 2) TTSVoice 非空 → 按自定义参数建用户条目。
// 3) 全空 → 默认 longtian。
func (a *App) resolveCreateSeriesVoiceID(ctx context.Context, in CreateSeriesInput) (string, error) {
	if strings.TrimSpace(in.VoiceProfile) != "" {
		v, err := a.Repo.GetVoice(ctx, in.VoiceProfile)
		if err != nil {
			return "", translateErr(fmt.Errorf("声音 %s 不存在: %w", in.VoiceProfile, err))
		}
		return v.ID, nil
	}
	voice := firstNonEmpty(in.Voice, a.Config().TTSVoice)
	if voice == "" {
		// 全空 → 默认条目（Bootstrap 已 seed，必存在）。
		return domain.DefaultVoiceID, nil
	}
	// 自定义参数 → 新建用户条目。
	vid := fmt.Sprintf("custom-%d", time.Now().UnixNano())
	v := &domain.Voice{
		ID:          vid,
		Name:        "自定义 " + voice,
		Provider:    domain.VoiceProviderBailian,
		Voice:       voice,
		Instruction: firstNonEmpty(in.TTSInstruction, a.Config().TTSInstruction),
		IsBuiltin:   false,
	}
	if err := a.Repo.CreateVoice(ctx, v); err != nil {
		return "", err
	}
	return vid, nil
}

// CreateEpisode 在系列下创建一集，并初始化工作目录（版本产物落在其 versions/ 子目录）。
// instruction 为本集附加创作指令（自由文本，叠加在系列创作设置之上，可空）。
func (a *App) CreateEpisode(ctx context.Context, seriesID, title, topic, instruction string) (*domain.Episode, error) {
	series, err := a.Repo.GetSeries(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	number, err := a.Repo.NextEpisodeNumber(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	id := fmt.Sprintf("%s-e%02d", series.ID, number)
	workDir := filepath.Join(a.Config().ProjectsDir(), series.ID, id)
	if err := os.MkdirAll(filepath.Join(workDir, engine.VersionsDirName), 0o755); err != nil {
		return nil, err
	}

	ep := domain.NewEpisode(id, series.ID, number, title, topic, workDir)
	ep.Instruction = instruction
	if err := a.Repo.CreateEpisode(ctx, ep); err != nil {
		return nil, err
	}
	return ep, nil
}

// UpdateEpisodeMeta 修改集的标题/主题/附加指令，不触发任何生产动作，也不触碰版本树。
func (a *App) UpdateEpisodeMeta(ctx context.Context, id, title, topic, instruction string) error {
	return translateErr(a.Repo.UpdateEpisodeMeta(ctx, id, title, topic, instruction))
}

// ErrNotFound 业务侧「不存在」错误（屏蔽具体存储实现的哨兵错误）。
var ErrNotFound = errors.New("记录不存在")

func translateErr(err error) error {
	if errors.Is(err, sqlitestore.ErrNotFound) {
		return ErrNotFound
	}
	return err
}

// ---- 查询透传 ----

// GetSeries 查询系列。
func (a *App) GetSeries(ctx context.Context, id string) (*domain.Series, error) {
	s, err := a.Repo.GetSeries(ctx, id)
	return s, translateErr(err)
}

// ListSeries 列出系列。
func (a *App) ListSeries(ctx context.Context) ([]*domain.Series, error) {
	return a.Repo.ListSeries(ctx)
}

// GetEpisode 查询集。
func (a *App) GetEpisode(ctx context.Context, id string) (*domain.Episode, error) {
	ep, err := a.Repo.GetEpisode(ctx, id)
	return ep, translateErr(err)
}

// ListEpisodes 列出系列下的集。
func (a *App) ListEpisodes(ctx context.Context, seriesID string) ([]*domain.Episode, error) {
	return a.Repo.ListEpisodes(ctx, seriesID)
}

// DeleteSeries 删除系列及其持久化记录与媒体目录（目录下所有集的产物一并清除）。
func (a *App) DeleteSeries(ctx context.Context, id string) error {
	if err := translateErr(a.Repo.DeleteSeries(ctx, id)); err != nil {
		return err
	}
	_ = os.RemoveAll(filepath.Join(a.Config().ProjectsDir(), id))
	return nil
}

// DeleteEpisode 删除单集及其持久化记录与工作目录（所有片段/旁白/成片一并清除）。
func (a *App) DeleteEpisode(ctx context.Context, id string) error {
	ep, err := a.Repo.GetEpisode(ctx, id)
	if err != nil {
		return translateErr(err)
	}
	if err := translateErr(a.Repo.DeleteEpisode(ctx, id)); err != nil {
		return err
	}
	_ = os.RemoveAll(ep.WorkDir)
	return nil
}

// ---- 分集策划 ----

// GetSeriesPlan 读取系列的 AI 分集策划会话。
func (a *App) GetSeriesPlan(ctx context.Context, seriesID string) (*domain.PlanSession, error) {
	ps, err := a.Engine.GetSeriesPlan(ctx, seriesID)
	return ps, translateErr(err)
}

// ChatSeriesPlan 向策划会话追加一条用户消息，返回更新后的会话（含最新草案）。
func (a *App) ChatSeriesPlan(ctx context.Context, seriesID, message string) (*domain.PlanSession, error) {
	ps, err := a.Engine.ChatSeriesPlan(ctx, seriesID, message)
	return ps, translateErr(err)
}

// ResetSeriesPlan 清空策划会话，重新开始讨论。
func (a *App) ResetSeriesPlan(ctx context.Context, seriesID string) error {
	return translateErr(a.Engine.ResetSeriesPlan(ctx, seriesID))
}

// UpdateSeriesCharacters 覆盖系列的人物设定集（供策划采纳与人工编辑入口）。
func (a *App) UpdateSeriesCharacters(ctx context.Context, seriesID string, characters []domain.CharacterSetting) error {
	s, err := a.Repo.GetSeries(ctx, seriesID)
	if err != nil {
		return translateErr(err)
	}
	if characters == nil {
		characters = []domain.CharacterSetting{}
	}
	s.Characters = characters
	s.UpdatedAt = time.Now()
	return translateErr(a.Repo.UpdateSeries(ctx, s))
}

// ExpandCreative 展开「创作预设 + 逐项微调（{knobKey: value}）」为系列配置字段。
// 未知参数 key / 未知预设报错（HTTP 入口映射为 400）；空值一律等于内置默认。
// 参数名与合法值全部由 templates 注册表决定，本层不硬编码任何风格。
func ExpandCreative(preset string, knobs map[string]string) (domain.CreativeStyle, string, error) {
	cfg, err := templates.Expand(preset, knobs)
	if err != nil {
		return domain.CreativeStyle{}, "", err
	}
	return cfg.Creative, cfg.VideoStyle, nil
}

// UpdateSeriesCreative 更新系列的创作控制设置（不动 voice_id / visual_mode，二者创建后锁定）。
//   - preset 非空：先套用该预设，并记录溯源 key（默认预设＝清空溯源）。
//   - knobs 为 {knobKey: value}：逐项覆盖，未知 key 报错；显式空串＝清除该参数。
//     「自定义创作指令」也是其中一个 key（instruction，文本型），无需单独通道。
//   - 补丁语义：未出现在 knobs 里的参数保持原值。
func (a *App) UpdateSeriesCreative(ctx context.Context, seriesID, preset string, knobs map[string]string) (*domain.Series, error) {
	s, err := a.Repo.GetSeries(ctx, seriesID)
	if err != nil {
		return nil, translateErr(err)
	}
	if p := strings.TrimSpace(preset); p != "" {
		if err := templates.ApplyPreset(&s.Config, p); err != nil {
			return nil, err
		}
		// ApplyPreset 不落默认预设 key（保证零值＝现状），此处显式回填/清空溯源。
		if p == templates.DefaultPresetKey {
			s.Config.Creative.Preset = ""
		}
	}
	if err := templates.ApplyKnobs(&s.Config, knobs); err != nil {
		return nil, err
	}
	s.UpdatedAt = time.Now()
	if err := a.Repo.UpdateSeries(ctx, s); err != nil {
		return nil, translateErr(err)
	}
	return s, nil
}

// validateBGMVolume 校验成片 BGM 音量：必须落在 0..1（含 0 与 1）。
// 0 表示「未设置＝用默认 0.18」，是合法值。
func validateBGMVolume(v float64) error {
	if v < 0 || v > 1 {
		return fmt.Errorf("bgm_volume 必须在 0 到 1 之间（含 0 与 1，0＝默认 0.18），收到 %g", v)
	}
	return nil
}

// UpdateSeriesBGM 更新系列的成片背景音乐设置（§20，补丁语义）。
//   - path 非 nil：写入 BGM 路径（相对路径相对系列目录 data/projects/<series-id>/；空串＝清除）。
//   - volume 非 nil：写入 0..1 音量（0＝清除、回到默认 0.18），越界返回中文错误。
//   - 任一为 nil：该项保持原值。不动 voice_id / visual_mode，二者创建后锁定。
func (a *App) UpdateSeriesBGM(ctx context.Context, seriesID string, path *string, volume *float64) error {
	s, err := a.Repo.GetSeries(ctx, seriesID)
	if err != nil {
		return translateErr(err)
	}
	if path != nil {
		v := strings.TrimSpace(*path)
		// §23：asset:<id> 引用设值前快速校验素材存在（字面路径保持 §20 语义，交引擎验收）。
		if err := a.validateAssetRef(ctx, v); err != nil {
			return err
		}
		s.Config.BGMPath = v
	}
	if volume != nil {
		if err := validateBGMVolume(*volume); err != nil {
			return err
		}
		s.Config.BGMVolume = *volume
	}
	s.UpdatedAt = time.Now()
	return translateErr(a.Repo.UpdateSeries(ctx, s))
}

// GenerateSeriesKeyframes 为系列人物设定集批量生成定妆照（透传 engine）。
func (a *App) GenerateSeriesKeyframes(ctx context.Context, seriesID string, force bool) ([]domain.CharacterSetting, error) {
	cs, err := a.Engine.GenerateSeriesKeyframes(ctx, seriesID, force)
	return cs, translateErr(err)
}

// GenerateEpisodeRefs 为某集的视觉参考（人物/场景）批量生成参考图（透传 engine）。
func (a *App) GenerateEpisodeRefs(ctx context.Context, episodeID string, force bool) ([]domain.VisualRef, error) {
	refs, err := a.Engine.GenerateEpisodeRefs(ctx, episodeID, force)
	return refs, translateErr(err)
}

// ---- 声音（顶层实体，§16） ----

// ListVoices 列出全部声音条目（含内置 + 用户自定义）。
func (a *App) ListVoices(ctx context.Context) ([]*domain.Voice, error) {
	vs, err := a.Repo.ListVoices(ctx)
	if err != nil {
		return nil, err
	}
	if vs == nil {
		vs = []*domain.Voice{}
	}
	return vs, nil
}

// GetVoice 查询单个声音条目。
func (a *App) GetVoice(ctx context.Context, id string) (*domain.Voice, error) {
	v, err := a.Repo.GetVoice(ctx, id)
	return v, translateErr(err)
}

// CreateVoiceInput 创建声音条目的参数。
type CreateVoiceInput struct {
	Name string
	// Provider TTS 供应商（空值归一 bailian）；Voice 为该供应商体系内音色 ID。
	Provider string
	Voice    string
	// Model 驱动该音色的合成模型（造声音色必填；空则用全局 tts_model）。
	Model       string
	Instruction string
	Rate        float64
	Pitch       float64
	StyleNote   string
}

// CreateVoice 新建用户声音条目（ID = slug + nanos 后缀保证唯一）。
func (a *App) CreateVoice(ctx context.Context, in CreateVoiceInput) (*domain.Voice, error) {
	if strings.TrimSpace(in.Name) == "" {
		return nil, fmt.Errorf("name 不能为空")
	}
	if strings.TrimSpace(in.Voice) == "" {
		return nil, fmt.Errorf("voice 不能为空")
	}
	id, err := a.uniqueVoiceID(ctx, Slugify(in.Name))
	if err != nil {
		return nil, err
	}
	now := time.Now()
	v := &domain.Voice{
		ID:          id,
		Name:        in.Name,
		Provider:    domain.NormalizeVoiceProvider(in.Provider),
		Voice:       in.Voice,
		Model:       in.Model,
		Instruction: in.Instruction,
		Rate:        in.Rate,
		Pitch:       in.Pitch,
		StyleNote:   in.StyleNote,
		IsBuiltin:   false,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := a.Repo.CreateVoice(ctx, v); err != nil {
		return nil, err
	}
	return v, nil
}

// BuildVoiceInput 通过供应商造声能力（声音设计/声音复刻）创建声音条目的参数。
type BuildVoiceInput struct {
	// Kind 造声方式：domain.VoiceBuildDesign（文字描述设计）/ VoiceBuildClone（音频复刻）。
	Kind string
	// Provider TTS 供应商标识（留空默认 bailian）。造声与供应商强绑定，
	// 该值决定由哪个 VoiceBuilder 执行，并写入新建声音条目的 Provider。
	// 与 NormalizeVoiceProvider 不同：这里显式给出未知供应商会报错，而非静默回退。
	Provider string
	// Name 声音条目显示名（必填）。
	Name string
	// Prompt 声音描述文本（Kind=design 必填）。
	Prompt string
	// PreviewText 试听文本（Kind=design 必填）。
	PreviewText string
	// AudioPath 复刻音频本地路径（Kind=clone 用，与 AudioURL 二选一）。
	AudioPath string
	// AudioURL 复刻音频公网 URL / oss://（Kind=clone 用）。
	AudioURL string
	// TargetModel 驱动模型（留空用配置的 tts_model 再回退默认）。
	TargetModel string
	// LanguageHints 语种提示（默认 zh）。
	LanguageHints []string
	// MaxPromptAudioLength 复刻参考音频最大时长秒（3-30，0 用供应商默认）。
	MaxPromptAudioLength float64
	// EnablePreprocess 复刻音频预处理（降噪/增强）。
	EnablePreprocess bool
	// StyleNote 风格说明（留空按造声方式生成默认文案）。
	StyleNote string
}

// BuildVoiceResult 造声并落库的结果。
type BuildVoiceResult struct {
	// Voice 新建的声音条目（已持久化）。
	Voice *domain.Voice
	// PreviewAudioPath 试听音频路径（声音设计返回；可经 GET /api/voices/preview?path= 播放）。
	PreviewAudioPath string
}

// BuildVoice 调用供应商造声能力创建自定义音色，并自动落成一条声音条目（§16）。
// 造声按新建音色个数计费（成本红线：仅在用户明确要求时调用）。
// 造出的音色必须用造声时的驱动模型合成，故把返回的 target_model 写入 Voice.Model。
func (a *App) BuildVoice(ctx context.Context, in BuildVoiceInput) (*BuildVoiceResult, error) {
	if strings.TrimSpace(in.Name) == "" {
		return nil, fmt.Errorf("name 不能为空")
	}
	provider, builder, err := a.resolveVoiceBuilder(in.Provider)
	if err != nil {
		return nil, err
	}
	kind := strings.TrimSpace(in.Kind)
	switch kind {
	case domain.VoiceBuildDesign:
		if strings.TrimSpace(in.Prompt) == "" {
			return nil, fmt.Errorf("声音设计需要 --prompt 声音描述")
		}
		if strings.TrimSpace(in.PreviewText) == "" {
			return nil, fmt.Errorf("声音设计需要 --preview-text 试听文本")
		}
	case domain.VoiceBuildClone:
		if strings.TrimSpace(in.AudioPath) == "" && strings.TrimSpace(in.AudioURL) == "" {
			return nil, fmt.Errorf("声音复刻需要音频（本地路径或公网 URL）")
		}
	default:
		return nil, fmt.Errorf("未知造声方式 %q（支持 %s/%s）", in.Kind, domain.VoiceBuildDesign, domain.VoiceBuildClone)
	}

	targetModel := firstNonEmpty(in.TargetModel, a.Config().TTSModel, domain.VoiceBuildModelDefault)
	// 试听音频统一落在预览目录，便于经 /api/voices/preview 回放。
	previewPath := filepath.Join(os.TempDir(), "story-voice-preview", fmt.Sprintf("%s-%d.wav", kind, time.Now().UnixNano()))

	res, err := builder.BuildVoice(ctx, port.VoiceBuildRequest{
		Kind:                 kind,
		TargetModel:          targetModel,
		Prefix:               Slugify(in.Name),
		Prompt:               in.Prompt,
		PreviewText:          in.PreviewText,
		PreviewAudioPath:     previewPath,
		AudioURL:             in.AudioURL,
		AudioPath:            in.AudioPath,
		LanguageHints:        in.LanguageHints,
		MaxPromptAudioLength: in.MaxPromptAudioLength,
		EnablePreprocess:     in.EnablePreprocess,
	})
	if err != nil {
		return nil, err
	}

	styleNote := strings.TrimSpace(in.StyleNote)
	if styleNote == "" {
		if kind == domain.VoiceBuildDesign {
			styleNote = "声音设计：" + truncateRunes(in.Prompt, 40)
		} else {
			styleNote = "声音复刻（上传音频）"
		}
	}
	v, err := a.CreateVoice(ctx, CreateVoiceInput{
		Name:      in.Name,
		Provider:  provider,
		Voice:     res.VoiceID,
		Model:     firstNonEmpty(res.TargetModel, targetModel),
		StyleNote: styleNote,
	})
	if err != nil {
		return nil, err
	}
	return &BuildVoiceResult{Voice: v, PreviewAudioPath: res.PreviewAudioPath}, nil
}

// resolveVoiceBuilder 按供应商选出造声实现（§3 接口驱动：app 只认 port.VoiceBuilder）。
// 空值默认 bailian；显式给出未登记的供应商会报错——不与 NormalizeVoiceProvider
// 的「未知回退 bailian」同语义，避免把「iflytek」静默当成百炼去造声。
func (a *App) resolveVoiceBuilder(provider string) (string, port.VoiceBuilder, error) {
	p := strings.ToLower(strings.TrimSpace(provider))
	if p == "" {
		p = domain.VoiceProviderBailian
	}
	key, b, err := a.voiceBuilders.Resolve(p)
	if err != nil || b == nil {
		return "", nil, fmt.Errorf("造声能力暂不支持供应商 %q（当前支持：%s）",
			provider, strings.Join(a.voiceBuilderProviders(), "/"))
	}
	return key, b, nil
}

// voiceBuilderProviders 已登记造声能力的供应商标识（排序后，用于错误提示）。
func (a *App) voiceBuilderProviders() []string {
	return a.voiceBuilders.Keys()
}

// minSampleSeconds 复刻参考音频时长下限（供应商要求 3-30s）。
const minSampleSeconds = 3.0

// VoiceSample 保存并归一化后的参考音频。
type VoiceSample struct {
	// Path 归一化后的 wav 绝对路径（作为 §16 复刻的 audio_path 提交）。
	Path string `json:"path"`
	// DurationSec 归一化后时长（秒），供前端提示是否落在 3-30 秒内。
	DurationSec float64 `json:"duration_sec"`
}

// SaveVoiceSample 落盘一段上传/录音的参考音频，并归一化为 16kHz 单声道 wav（§16）。
//
// 为什么必须后端转码：浏览器录音产出 `audio/webm;codecs=opus`（Chrome/Firefox）
// 或 `audio/mp4`（Safari），供应商复刻接口不认这些容器；用户上传件也可能是
// 48kHz 立体声 m4a。统一转码后，录音与上传两条路径共用同一个提交格式。
//
// 原始件与转码件都留在系统临时目录 story-voice-samples/，不主动删除：
// 失败重试可复用同一段音频，也便于回听排查（仅存本机，随系统临时目录清理）。
func (a *App) SaveVoiceSample(ctx context.Context, r io.Reader, filename string) (*VoiceSample, error) {
	if a.audioNormalizer == nil {
		return nil, fmt.Errorf("未配置参考音频归一化能力（AudioNormalizer）")
	}
	dir := filepath.Join(os.TempDir(), "story-voice-samples")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}

	stamp := time.Now().UnixNano()
	raw := filepath.Join(dir, fmt.Sprintf("raw-%d%s", stamp, sanitizeAudioExt(filename)))
	f, err := os.Create(raw)
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		return nil, fmt.Errorf("写入参考音频: %w", err)
	}
	if err := f.Close(); err != nil {
		return nil, err
	}

	wav := filepath.Join(dir, fmt.Sprintf("sample-%d.wav", stamp))
	dur, err := a.audioNormalizer.NormalizeAudio(ctx, raw, wav)
	if err != nil {
		return nil, err
	}
	if dur < minSampleSeconds {
		return nil, fmt.Errorf("参考音频过短（%.1f 秒），请录制或上传 3-30 秒的清晰人声", dur)
	}
	return &VoiceSample{Path: wav, DurationSec: dur}, nil
}

// sanitizeAudioExt 从上传文件名取安全的扩展名（含点，如 ".wav"），不可信时返回 ".bin"。
// 先取 Base 截断目录成分，再只放行字母数字：用户的文件名绝不允许影响落盘路径。
// ffmpeg 靠内容嗅探识别容器，扩展名仅作排查便利。
func sanitizeAudioExt(filename string) string {
	ext := strings.ToLower(filepath.Ext(filepath.Base(filename)))
	if len(ext) < 2 || len(ext) > 6 {
		return ".bin"
	}
	for _, r := range ext[1:] {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			return ".bin"
		}
	}
	return ext
}

// UpdateVoice 更新声音条目（内置条目也可改；改后影响所有引用它的系列）。
func (a *App) UpdateVoice(ctx context.Context, v *domain.Voice) error {
	// 保持 IsBuiltin 不变：用户不能通过该接口把内置标记改成 false。
	existing, err := a.Repo.GetVoice(ctx, v.ID)
	if err != nil {
		return translateErr(err)
	}
	v.IsBuiltin = existing.IsBuiltin
	return translateErr(a.Repo.UpdateVoice(ctx, v))
}

// DeleteVoice 删除声音条目（内置不可删；被系列引用拒绝）。
func (a *App) DeleteVoice(ctx context.Context, id string) error {
	return translateErr(a.Repo.DeleteVoice(ctx, id))
}

// ListSystemVoices 列出某 TTS 供应商给定模型的系统音色（浏览音色库创建声音用）。
// 只取元数据、不合成语音，不产生费用。按供应商分发到已登记的 VoiceLister；
// 未登记该能力时明确报错（不与 NormalizeVoiceProvider 的「未知回退」同语义）。
// model 为空时用配置的 TTS 模型，再空由 provider 用其默认模型。
func (a *App) ListSystemVoices(ctx context.Context, provider, model string) ([]domain.SystemVoice, error) {
	p := domain.NormalizeVoiceProvider(provider)
	_, lister, err := a.voiceListers.Resolve(p)
	if err != nil || lister == nil {
		return nil, fmt.Errorf("暂不支持 TTS 供应商 %q 的系统音色列表（当前支持：%s）",
			provider, strings.Join(a.voiceListerProviders(), "/"))
	}
	if strings.TrimSpace(model) == "" {
		model = a.Config().TTSModel
	}
	return lister.ListSystemVoices(ctx, model)
}

// voiceListerProviders 已登记系统音色列表能力的供应商标识（排序后，用于错误提示）。
func (a *App) voiceListerProviders() []string {
	return a.voiceListers.Keys()
}

// VoiceProviderInfo TTS 供应商能力快照（声音库表单/造声下拉渲染用）。
// 前端据此动态渲染供应商选项，新增供应商时无需改前端。
type VoiceProviderInfo struct {
	// ID 供应商标识（domain.VoiceProviderXxx）。
	ID string `json:"id"`
	// CanBuild 是否具备造声能力（声音设计/声音复刻）。
	CanBuild bool `json:"can_build"`
	// CanList 是否具备系统音色列表能力（浏览音色库）。
	CanList bool `json:"can_list"`
}

// VoiceProviderInfos 返回系统已接入的 TTS 供应商及其能力（按注册顺序）。
func (a *App) VoiceProviderInfos() []VoiceProviderInfo {
	out := make([]VoiceProviderInfo, 0, len(domain.VoiceProviders))
	for _, id := range domain.VoiceProviders {
		out = append(out, VoiceProviderInfo{
			ID:       id,
			CanBuild: a.voiceBuilders.Has(id),
			CanList:  a.voiceListers.Has(id),
		})
	}
	return out
}

// GetSeriesVoice 取某系列引用的声音条目；找不到时回退默认预设（engine fallback 用）。
func (a *App) GetSeriesVoice(ctx context.Context, seriesID string) (*domain.Voice, error) {
	s, err := a.Repo.GetSeries(ctx, seriesID)
	if err != nil {
		return nil, translateErr(err)
	}
	if s.VoiceID != "" {
		v, err := a.Repo.GetVoice(ctx, s.VoiceID)
		if err == nil {
			return v, nil
		}
	}
	// 回退：按旧 VoiceProfile/TTSVoice 现场匹配，最终默认 longtian。
	key := s.Config.VoiceProfile
	if key == "" {
		key = domain.DefaultVoiceID
	}
	v, err := a.Repo.GetVoice(ctx, key)
	if err != nil {
		return nil, translateErr(err)
	}
	return v, nil
}

// uniqueVoiceID 生成不冲突的声音 ID（slug 冲突时追加序号）。
func (a *App) uniqueVoiceID(ctx context.Context, base string) (string, error) {
	if base == "" {
		base = fmt.Sprintf("voice-%x", time.Now().UnixNano()&0xffff)
	}
	id := base
	for i := 2; ; i++ {
		_, err := a.Repo.GetVoice(ctx, id)
		if err != nil {
			if strings.Contains(err.Error(), "不存在") {
				return id, nil
			}
			return "", err
		}
		id = fmt.Sprintf("%s-%d", base, i)
	}
}

// ListVoiceProfiles 返回内置预设语音画像列表（兼容旧 API，读取顶层 Voice 表）。
// 旧签名无 ctx；改为读表但保持调用方兼容（server 启动后表必有内置条目）。
func (a *App) ListVoiceProfiles(ctx context.Context) ([]*domain.Voice, error) {
	return a.ListVoices(ctx)
}

// MatchVoiceProfile 按预设 key 查找语音画像（供 server 试音 fallback 用）。
// 优先读表；表缺失时回退到 templates 内置预设。
func (a *App) MatchVoiceProfile(ctx context.Context, key string) domain.VoiceProfile {
	if key == "" {
		key = domain.DefaultVoiceID
	}
	if v, err := a.Repo.GetVoice(ctx, key); err == nil {
		return v.ToProfile()
	}
	return templates.MatchVoiceProfile(key)
}

// UpdateSeries 更新系列（配置/人物等），供系列级设置修改用。
// 注意：voice_id 不在此处更新（创建后锁定，store.UpdateSeries SQL 不含该列）。
func (a *App) UpdateSeries(ctx context.Context, series *domain.Series) error {
	// §23：本方法是 Web PUT /api/series/{id} 与 CLI series set 的整对象写回链路，
	// 与 CreateSeries / UpdateSeriesBGM 同款校验 asset 引用，避免悬空引用静默落库。
	if err := a.validateAssetRef(ctx, strings.TrimSpace(series.Config.BGMPath)); err != nil {
		return err
	}
	return a.Repo.UpdateSeries(ctx, series)
}

// PreviewVoice 用指定语音画像合成一段样音（供前端试听）。
// 调用一次 bl speech synthesize，按次计费（成本红线）。
func (a *App) PreviewVoice(ctx context.Context, profile domain.VoiceProfile, text string) (string, error) {
	if text == "" {
		text = "话说天下大势，分久必合，合久必分。"
	}
	dir := filepath.Join(os.TempDir(), "story-voice-preview")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	out := filepath.Join(dir, fmt.Sprintf("preview-%d.mp3", time.Now().UnixNano()))
	_, err := a.Engine.PreviewVoice(ctx, profile, text, out)
	if err != nil {
		return "", translateErr(err)
	}
	return out, nil
}

// ApplyEpisodePlan 把分集草案批量落为集（只创建集，不触发视频生产）。
// 已存在同标题集的草案会被跳过，因此全量草案与增量草案都可重复采纳；
// 每集梗概写入工作目录 brief.md，供后续故事生成参考。
// characters 非 nil 时同步覆盖系列人物设定集（策划会话产出或用户编辑后的版本）。
func (a *App) ApplyEpisodePlan(ctx context.Context, seriesID string, drafts []domain.EpisodeDraft, characters []domain.CharacterSetting) ([]*domain.Episode, error) {
	if err := engine.ValidateDrafts(drafts); err != nil {
		return nil, err
	}
	if characters != nil {
		if err := a.UpdateSeriesCharacters(ctx, seriesID, characters); err != nil {
			return nil, err
		}
	}
	existing, err := a.Repo.ListEpisodes(ctx, seriesID)
	if err != nil {
		return nil, translateErr(err)
	}
	existTitles := make(map[string]bool, len(existing))
	for _, ep := range existing {
		existTitles[strings.TrimSpace(ep.Title)] = true
	}

	created := make([]*domain.Episode, 0, len(drafts))
	for _, d := range drafts {
		title := strings.TrimSpace(d.Title)
		if title == "" || existTitles[title] {
			continue
		}
		ep, err := a.CreateEpisode(ctx, seriesID, title, strings.TrimSpace(d.Topic), "")
		if err != nil {
			return created, err
		}
		existTitles[title] = true
		if summary := strings.TrimSpace(d.Summary); summary != "" {
			brief := fmt.Sprintf("# 第%d集 %s\n\n- 主题：%s\n\n%s\n", ep.Number, ep.Title, ep.Topic, summary)
			if err := os.WriteFile(filepath.Join(ep.WorkDir, "brief.md"), []byte(brief), 0o644); err != nil {
				return created, err
			}
		}
		created = append(created, ep)
	}
	return created, nil
}

func (a *App) uniqueSeriesID(ctx context.Context, base string) (string, error) {
	id := base
	for i := 2; ; i++ {
		_, err := a.Repo.GetSeries(ctx, id)
		if err != nil {
			if strings.Contains(err.Error(), "不存在") {
				return id, nil
			}
			return "", err
		}
		id = fmt.Sprintf("%s-%d", base, i)
	}
}

// Slugify 把中文/混合名称转为拼音 slug，如「鬼谷子」→ guiguzi。
// 连续汉字拼成一个音节段，空格/标点是段间分隔符。
func Slugify(s string) string {
	pa := pinyin.NewArgs()
	pa.Style = pinyin.Normal

	var parts []string
	var ascii []rune
	var han strings.Builder
	flushASCII := func() {
		if len(ascii) > 0 {
			parts = append(parts, string(ascii))
			ascii = ascii[:0]
		}
	}
	flushHan := func() {
		if han.Len() > 0 {
			parts = append(parts, han.String())
			han.Reset()
		}
	}
	for _, r := range s {
		if py := pinyin.SinglePinyin(r, pa); len(py) > 0 {
			flushASCII()
			han.WriteString(py[0])
		} else if unicode.IsLetter(r) || unicode.IsDigit(r) {
			flushHan()
			ascii = append(ascii, []rune(strings.ToLower(string(r)))...)
		} else {
			flushASCII()
			flushHan()
		}
	}
	flushASCII()
	flushHan()

	slug := strings.Join(parts, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = fmt.Sprintf("series-%x", time.Now().UnixNano()&0xffff)
	}
	return slug
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// truncateRunes 按字符数截断字符串，超长时补省略号。
func truncateRunes(s string, max int) string {
	rs := []rune(strings.TrimSpace(s))
	if len(rs) <= max {
		return string(rs)
	}
	return string(rs[:max]) + "…"
}

func orDefault(v, def int) int {
	if v > 0 {
		return v
	}
	return def
}
