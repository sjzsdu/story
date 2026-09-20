// Package app 是唯一的依赖装配层：把具体 provider 注入 engine。
// 除 cmd/story 外，只有本包允许 import 具体实现。
package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

	bl := bailianprov.NewClient(cfg.BLBin, cfg.TextModel, cfg.VideoModel, cfg.TTSModel, cfg.ImageModel)
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
	return &App{Cfg: cfg, Repo: store, Engine: eng}, nil
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
// 3) 全空 → 默认 wangliqun。
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
		return domain.VoiceProfileWangliqun, nil
	}
	// 自定义参数 → 新建用户条目。
	vid := fmt.Sprintf("custom-%d", time.Now().UnixNano())
	v := &domain.Voice{
		ID:          vid,
		Name:        "自定义 " + voice,
		Voice:       voice,
		Instruction: firstNonEmpty(in.TTSInstruction, a.Cfg.TTSInstruction),
		IsBuiltin:   false,
	}
	if err := a.Repo.CreateVoice(ctx, v); err != nil {
		return "", err
	}
	return vid, nil
}

// CreateEpisode 在系列下创建一集，并初始化工作目录与流水线状态。
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
	for _, sub := range []string{"clips", "audio", "tmp", "output"} {
		if err := os.MkdirAll(filepath.Join(workDir, sub), 0o755); err != nil {
			return nil, err
		}
	}

	now := time.Now()
	ep := &domain.Episode{
		ID:        id,
		SeriesID:  series.ID,
		Number:    number,
		Title:     title,
		Topic:     topic,
		State:     *domain.NewPipelineState(),
		WorkDir:   workDir,
		CreatedAt: now,
		UpdatedAt: now,
	}
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
	Name        string
	Voice       string
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
		Voice:       in.Voice,
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
	// 回退：按旧 VoiceProfile/TTSVoice 现场匹配，最终默认 wangliqun。
	key := s.Config.VoiceProfile
	if key == "" {
		key = domain.VoiceProfileWangliqun
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
		key = domain.VoiceProfileWangliqun
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

func orDefault(v, def int) int {
	if v > 0 {
		return v
	}
	return def
}
