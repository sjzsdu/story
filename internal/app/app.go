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

	bl := bailianprov.NewClient(cfg.BLBin, cfg.TextModel, cfg.VideoModel, cfg.TTSModel)
	composer := ffmpegprov.New(cfg.FFMPEGBin, cfg.FFProbeBin())
	if cfg.SubtitleFont != "" {
		r, err := subtitle.NewRenderer(cfg.SubtitleFont)
		if err != nil {
			return nil, err
		}
		composer.WithSubtitles(r)
	}

	eng := engine.New(
		store, bl, bl, bl, bl, composer,
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
	Name            string
	Dynasty         string
	Description     string
	Ratio           string
	Resolution      string
	Voice           string
	TTSInstruction  string
	Concurrency     int
	Retries         int
	TargetPlatforms []string
}

// CreateSeries 创建一个新系列（ID 由名称生成拼音 slug，冲突时追加序号）。
func (a *App) CreateSeries(ctx context.Context, in CreateSeriesInput) (*domain.Series, error) {
	id := Slugify(in.Name)
	id, err := a.uniqueSeriesID(ctx, id)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	se := &domain.Series{
		ID:          id,
		Name:        in.Name,
		Dynasty:     in.Dynasty,
		Description: in.Description,
		Config: domain.SeriesConfig{
			Dynasty:         in.Dynasty,
			Ratio:           firstNonEmpty(in.Ratio, a.Cfg.DefaultRatio),
			Resolution:      firstNonEmpty(in.Resolution, a.Cfg.DefaultResolution),
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
