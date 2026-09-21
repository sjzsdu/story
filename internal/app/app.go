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
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/mozillazg/go-pinyin"

	"github.com/sjzsdu/story/internal/config"
	"github.com/sjzsdu/story/internal/domain"
	"github.com/sjzsdu/story/internal/engine"
	"github.com/sjzsdu/story/internal/port"
	bailianprov "github.com/sjzsdu/story/internal/provider/bailian"
	ffmpegprov "github.com/sjzsdu/story/internal/provider/ffmpeg"
	sqlitestore "github.com/sjzsdu/story/internal/store/sqlite"
	"github.com/sjzsdu/story/internal/subtitle"
	"github.com/sjzsdu/story/internal/templates"
)

// App 应用容器，CLI 命令通过它访问全部能力。
type App struct {
	Cfg    config.Config
	Repo   port.Repository
	Engine *engine.Engine

	// voiceLister 系统音色列表查询（浏览音色库）；由装配的 TTS provider 提供。
	voiceLister port.VoiceLister
	// voiceBuilders 造声能力（声音设计/声音复刻）注册表，key = 供应商标识
	// （domain.VoiceProviderXxx）。造声与供应商强绑定（模型/音色体系各异），
	// 故按供应商分发而非持有单一实例；接第二家 TTS 时在此加一项即可。
	voiceBuilders map[string]port.VoiceBuilder
	// audioNormalizer 参考音频归一化（§16 声音复刻：浏览器录音/上传件统一转
	// 16kHz 单声道 wav 再提交）；由 ffmpeg provider 实现。
	audioNormalizer port.AudioNormalizer
}

// Bootstrap 装配整个应用（打开数据库、构造 provider 与 engine）。
func Bootstrap(ctx context.Context, cfg config.Config) (*App, error) {
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

	bl := bailianprov.NewClient(cfg.BLBin, cfg.TextModel, cfg.VideoModel, cfg.TTSModel, cfg.ImageModel,
		cfg.BailianAPIKey, cfg.BailianBaseURL)
	composer := ffmpegprov.New(cfg.FFMPEGBin, cfg.FFProbeBin())
	if cfg.SubtitleFont != "" {
		r, err := subtitle.NewRenderer(cfg.SubtitleFont)
		if err != nil {
			return nil, err
		}
		composer.WithSubtitles(r)
	}

	eng := engine.New(
		store, bl, bl, bl, bl, composer, bl, bl,
		cfg.ProjectsDir(),
		cfg.MaxConcurrency, cfg.MaxRetries,
		cfg.TTSVoice, cfg.TTSInstruction,
	)
	app := &App{
		Cfg:    cfg,
		Repo:   store,
		Engine: eng,
		// 造声能力注册表：当前只有百炼；新增供应商时在此登记其 VoiceBuilder。
		voiceLister: bl,
		voiceBuilders: map[string]port.VoiceBuilder{
			domain.VoiceProviderBailian: bl,
		},
		// 参考音频归一化复用 ffmpeg composer（§16 复刻：录音/上传件统一转码）。
		audioNormalizer: composer,
	}
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
	VoiceProfile    string
	Voice           string
	TTSInstruction  string
	Concurrency     int
	Retries         int
	TargetPlatforms []string
}

// CreateSeries 创建一个新系列（ID 由名称生成拼音 slug，冲突时追加序号）。
// 必须指定 VoiceID；未指定时按平迁规则现场建/取一个（兼容旧 CLI/Web 请求）。
func (a *App) CreateSeries(ctx context.Context, in CreateSeriesInput) (*domain.Series, error) {
	id := Slugify(in.Name)
	id, err := a.uniqueSeriesID(ctx, id)
	if err != nil {
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
	se := &domain.Series{
		ID:          id,
		Name:        in.Name,
		Dynasty:     in.Dynasty,
		Description: in.Description,
		VoiceID:     voiceID,
		Config: domain.SeriesConfig{
			Dynasty:         in.Dynasty,
			Ratio:           firstNonEmpty(in.Ratio, a.Cfg.DefaultRatio),
			Resolution:      firstNonEmpty(in.Resolution, a.Cfg.DefaultResolution),
			VisualMode:      domain.NormalizeVisualMode(in.VisualMode),
			VoiceProfile:    in.VoiceProfile,
			TTSVoice:        firstNonEmpty(in.Voice, a.Cfg.TTSVoice),
			TTSInstruction:  firstNonEmpty(in.TTSInstruction, a.Cfg.TTSInstruction),
			TargetPlatforms: in.TargetPlatforms,
			MaxConcurrency:  orDefault(in.Concurrency, a.Cfg.MaxConcurrency),
			MaxRetries:      orDefault(in.Retries, a.Cfg.MaxRetries),
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := a.Repo.CreateSeries(ctx, se); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(a.Cfg.ProjectsDir(), id), 0o755); err != nil {
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
	voice := firstNonEmpty(in.Voice, a.Cfg.TTSVoice)
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
		Instruction: firstNonEmpty(in.TTSInstruction, a.Cfg.TTSInstruction),
		IsBuiltin:   false,
	}
	if err := a.Repo.CreateVoice(ctx, v); err != nil {
		return "", err
	}
	return vid, nil
}

// CreateEpisode 在系列下创建一集，并初始化工作目录（版本产物落在其 versions/ 子目录）。
func (a *App) CreateEpisode(ctx context.Context, seriesID, title, topic string) (*domain.Episode, error) {
	series, err := a.Repo.GetSeries(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	number, err := a.Repo.NextEpisodeNumber(ctx, seriesID)
	if err != nil {
		return nil, err
	}
	id := fmt.Sprintf("%s-e%02d", series.ID, number)
	workDir := filepath.Join(a.Cfg.ProjectsDir(), series.ID, id)
	if err := os.MkdirAll(filepath.Join(workDir, engine.VersionsDirName), 0o755); err != nil {
		return nil, err
	}

	ep := domain.NewEpisode(id, series.ID, number, title, topic, workDir)
	if err := a.Repo.CreateEpisode(ctx, ep); err != nil {
		return nil, err
	}
	return ep, nil
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
	_ = os.RemoveAll(filepath.Join(a.Cfg.ProjectsDir(), id))
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

	targetModel := firstNonEmpty(in.TargetModel, a.Cfg.TTSModel, domain.VoiceBuildModelDefault)
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
	if b, ok := a.voiceBuilders[p]; ok && b != nil {
		return p, b, nil
	}
	return "", nil, fmt.Errorf("造声能力暂不支持供应商 %q（当前支持：%s）", provider, strings.Join(a.voiceBuilderProviders(), "/"))
}

// voiceBuilderProviders 已登记造声能力的供应商标识（排序后，用于错误提示）。
func (a *App) voiceBuilderProviders() []string {
	out := make([]string, 0, len(a.voiceBuilders))
	for p, b := range a.voiceBuilders {
		if b != nil {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
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
// 只取元数据、不合成语音，不产生费用。provider 目前只支持 bailian；
// model 为空时用配置的 TTS 模型，再空由 provider 用其默认模型。
func (a *App) ListSystemVoices(ctx context.Context, provider, model string) ([]domain.SystemVoice, error) {
	pv := domain.NormalizeVoiceProvider(provider)
	if pv != domain.VoiceProviderBailian {
		return nil, fmt.Errorf("暂不支持 TTS 供应商 %q", provider)
	}
	if a.voiceLister == nil {
		return nil, fmt.Errorf("未配置系统音色列表查询能力（VoiceLister）")
	}
	if strings.TrimSpace(model) == "" {
		model = a.Cfg.TTSModel
	}
	vs, err := a.voiceLister.ListSystemVoices(ctx, model)
	if err != nil {
		return nil, err
	}
	return vs, nil
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
		ep, err := a.CreateEpisode(ctx, seriesID, title, strings.TrimSpace(d.Topic))
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
