// Package mock 提供 port 接口的内存 mock，实现用于 engine 单元测试。
package mock

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/sjzsdu/story/internal/domain"
	"github.com/sjzsdu/story/internal/port"
)

// ---- Repository ----

// Repo 内存仓储。
type Repo struct {
	mu       sync.Mutex
	Series   map[string]*domain.Series
	Episodes map[string]*domain.Episode
	Plans    map[string]*domain.PlanSession
	Voices   map[string]*domain.Voice
}

// NewRepo 创建空内存仓储。
func NewRepo() *Repo {
	return &Repo{
		Series:   map[string]*domain.Series{},
		Episodes: map[string]*domain.Episode{},
		Plans:    map[string]*domain.PlanSession{},
		Voices:   map[string]*domain.Voice{},
	}
}

func (r *Repo) CreateSeries(_ context.Context, s *domain.Series) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.Series[s.ID]; ok {
		return fmt.Errorf("系列已存在: %s", s.ID)
	}
	r.Series[s.ID] = s
	return nil
}

func (r *Repo) GetSeries(_ context.Context, id string) (*domain.Series, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.Series[id]
	if !ok {
		return nil, fmt.Errorf("系列不存在: %s", id)
	}
	return s, nil
}

func (r *Repo) ListSeries(_ context.Context) ([]*domain.Series, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*domain.Series, 0, len(r.Series))
	for _, s := range r.Series {
		out = append(out, s)
	}
	return out, nil
}

func (r *Repo) UpdateSeries(_ context.Context, s *domain.Series) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Series[s.ID] = s
	return nil
}

func (r *Repo) DeleteSeries(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.Series[id]; !ok {
		return fmt.Errorf("系列不存在: %s", id)
	}
	delete(r.Series, id)
	// 级联删除该系列下的集与策划会话
	for eid, ep := range r.Episodes {
		if ep.SeriesID == id {
			delete(r.Episodes, eid)
		}
	}
	delete(r.Plans, id)
	return nil
}

func (r *Repo) CreateEpisode(_ context.Context, ep *domain.Episode) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.Episodes[ep.ID]; ok {
		return fmt.Errorf("集已存在: %s", ep.ID)
	}
	r.Episodes[ep.ID] = ep
	return nil
}

func (r *Repo) GetEpisode(_ context.Context, id string) (*domain.Episode, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ep, ok := r.Episodes[id]
	if !ok {
		return nil, fmt.Errorf("集不存在: %s", id)
	}
	return ep, nil
}

func (r *Repo) ListEpisodes(_ context.Context, seriesID string) ([]*domain.Episode, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*domain.Episode
	for _, ep := range r.Episodes {
		if ep.SeriesID == seriesID {
			out = append(out, ep)
		}
	}
	return out, nil
}

func (r *Repo) SaveEpisode(_ context.Context, ep *domain.Episode) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Episodes[ep.ID] = ep
	return nil
}

func (r *Repo) DeleteEpisode(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.Episodes[id]; !ok {
		return fmt.Errorf("集不存在: %s", id)
	}
	delete(r.Episodes, id)
	return nil
}

func (r *Repo) NextEpisodeNumber(_ context.Context, seriesID string) (int, error) {
	n := 0
	for _, ep := range r.Episodes {
		if ep.SeriesID == seriesID && ep.Number > n {
			n = ep.Number
		}
	}
	return n + 1, nil
}

func (r *Repo) Close() error { return nil }

// ---- 系列分集策划会话 ----

func (r *Repo) GetPlanSession(_ context.Context, seriesID string) (*domain.PlanSession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ps, ok := r.Plans[seriesID]
	if !ok {
		return nil, fmt.Errorf("%w: 策划会话 %s", port.ErrNotFound, seriesID)
	}
	return ps, nil
}

func (r *Repo) SavePlanSession(_ context.Context, ps *domain.PlanSession) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *ps
	r.Plans[ps.SeriesID] = &cp
	return nil
}

func (r *Repo) DeletePlanSession(_ context.Context, seriesID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.Plans, seriesID)
	return nil
}

// ---- 声音（顶层实体，§16 mock） ----

func (r *Repo) CreateVoice(_ context.Context, v *domain.Voice) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.Voices[v.ID]; ok {
		return fmt.Errorf("声音已存在: %s", v.ID)
	}
	r.Voices[v.ID] = v
	return nil
}

func (r *Repo) GetVoice(_ context.Context, id string) (*domain.Voice, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.Voices[id]
	if !ok {
		return nil, fmt.Errorf("%w: 声音 %s", port.ErrNotFound, id)
	}
	return v, nil
}

func (r *Repo) ListVoices(_ context.Context) ([]*domain.Voice, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*domain.Voice, 0, len(r.Voices))
	for _, v := range r.Voices {
		out = append(out, v)
	}
	return out, nil
}

func (r *Repo) UpdateVoice(_ context.Context, v *domain.Voice) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.Voices[v.ID]; !ok {
		return fmt.Errorf("%w: 声音 %s", port.ErrNotFound, v.ID)
	}
	r.Voices[v.ID] = v
	return nil
}

func (r *Repo) DeleteVoice(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.Voices[id]
	if !ok {
		return fmt.Errorf("%w: 声音 %s", port.ErrNotFound, id)
	}
	if v.IsBuiltin {
		return fmt.Errorf("内置声音不可删除: %s", id)
	}
	// 引用计数：扫一遍 Series
	count := 0
	for _, s := range r.Series {
		if s.VoiceID == id {
			count++
		}
	}
	if count > 0 {
		return fmt.Errorf("声音被 %d 个系列引用: %s", count, id)
	}
	delete(r.Voices, id)
	return nil
}

func (r *Repo) CountSeriesByVoiceID(_ context.Context, voiceID string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, s := range r.Series {
		if s.VoiceID == voiceID {
			n++
		}
	}
	return n, nil
}

// SeriesPlanner 分集策划 mock。
type SeriesPlanner struct {
	Result port.SeriesPlanResult
	Err    error
	Calls  int
}

// PlanEpisodes 实现 port.SeriesPlanner。
func (m *SeriesPlanner) PlanEpisodes(_ context.Context, _ port.SeriesPlanRequest) (port.SeriesPlanResult, error) {
	m.Calls++
	return m.Result, m.Err
}

// ---- Story / Storyboard ----

// StoryGen 候选故事 mock。
type StoryGen struct {
	Candidates []domain.StoryCandidate
	Err        error
	Calls      int
}

// GenerateCandidates 实现 port.StoryGenerator。
func (m *StoryGen) GenerateCandidates(_ context.Context, _ port.StoryRequest) ([]domain.StoryCandidate, error) {
	m.Calls++
	return m.Candidates, m.Err
}

// BoardPlanner 分镜 mock。
type BoardPlanner struct {
	Storyboard *domain.Storyboard
	Err        error
	Calls      int
}

// PlanStoryboard 实现 port.StoryboardPlanner。
func (m *BoardPlanner) PlanStoryboard(_ context.Context, _ port.StoryboardRequest) (*domain.Storyboard, error) {
	m.Calls++
	return m.Storyboard, m.Err
}

// ---- Image ----

// ImageGen 图片生成 mock：在 OutPath 写入假 png，记录请求。
type ImageGen struct {
	mu       sync.Mutex
	FailN    int // 前 N 次调用失败（验证重试）
	Calls    int
	Requests []port.ImageRequest
}

// GenerateImage 实现 port.ImageGenerator。
func (m *ImageGen) GenerateImage(_ context.Context, req port.ImageRequest) (port.ImageResult, error) {
	m.mu.Lock()
	m.Calls++
	calls := m.Calls
	m.Requests = append(m.Requests, req)
	m.mu.Unlock()
	if calls <= m.FailN {
		return port.ImageResult{}, fmt.Errorf("模拟图片失败 #%d", calls)
	}
	if err := os.MkdirAll(filepath.Dir(req.OutPath), 0o755); err != nil {
		return port.ImageResult{}, err
	}
	if err := os.WriteFile(req.OutPath, []byte("fake-png"), 0o644); err != nil {
		return port.ImageResult{}, err
	}
	return port.ImageResult{OutPath: req.OutPath}, nil
}

// CallsCount 线程安全地读取调用次数。
func (m *ImageGen) CallsCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.Calls
}

// ---- Video / Speech ----

// VideoGen 视频生成 mock：在 OutPath 写入假文件；FailFirst 次调用失败以验证重试。
type VideoGen struct {
	mu        sync.Mutex
	FailFirst int
	Calls     int
	Requests  []port.ClipRequest
}

// GenerateClip 实现 port.VideoGenerator。
func (m *VideoGen) GenerateClip(_ context.Context, req port.ClipRequest) (port.ClipResult, error) {
	m.mu.Lock()
	m.Calls++
	m.Requests = append(m.Requests, req)
	calls := m.Calls
	fail := m.FailFirst
	m.mu.Unlock()

	if calls <= fail {
		return port.ClipResult{}, fmt.Errorf("模拟视频失败 #%d", calls)
	}
	if err := os.MkdirAll(filepath.Dir(req.OutPath), 0o755); err != nil {
		return port.ClipResult{}, err
	}
	if err := os.WriteFile(req.OutPath, []byte("fake-mp4"), 0o644); err != nil {
		return port.ClipResult{}, err
	}
	return port.ClipResult{OutPath: req.OutPath}, nil
}

// CallsCount 线程安全地读取调用次数。
func (m *VideoGen) CallsCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.Calls
}

// SpeechGen 语音合成 mock。
type SpeechGen struct {
	mu    sync.Mutex
	Calls int
}

// Synthesize 实现 port.SpeechSynthesizer。
func (m *SpeechGen) Synthesize(_ context.Context, req port.SpeechRequest) (port.SpeechResult, error) {
	m.mu.Lock()
	m.Calls++
	m.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(req.OutPath), 0o755); err != nil {
		return port.SpeechResult{}, err
	}
	if err := os.WriteFile(req.OutPath, []byte("fake-mp3"), 0o644); err != nil {
		return port.SpeechResult{}, err
	}
	return port.SpeechResult{OutPath: req.OutPath}, nil
}

// Composer 后处理 mock：对存在的文件返回固定时长，Compose 写一个假成片。
type Composer struct {
	mu            sync.Mutex
	SceneDuration float64
	ComposeCalls  int
	ExportCalls   int
	FailCompose   bool
	StillRequests []port.StillRequest
}

// RenderStill 实现 port.VideoComposer：记录请求并写一个假片段文件。
func (m *Composer) RenderStill(_ context.Context, req port.StillRequest) error {
	m.mu.Lock()
	m.StillRequests = append(m.StillRequests, req)
	m.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(req.OutPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(req.OutPath, []byte("fake-still-mp4"), 0o644)
}

// StillCalls 线程安全地读取静帧渲染调用次数。
func (m *Composer) StillCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.StillRequests)
}

// ProbeDuration 实现 port.VideoComposer。
func (m *Composer) ProbeDuration(_ context.Context, path string) (float64, error) {
	if _, err := os.Stat(path); err != nil {
		return 0, err
	}
	if m.SceneDuration <= 0 {
		return 5, nil
	}
	return m.SceneDuration, nil
}

// Compose 实现 port.VideoComposer。
func (m *Composer) Compose(_ context.Context, req port.ComposeRequest) (port.ComposeResult, error) {
	m.ComposeCalls++
	if m.FailCompose {
		return port.ComposeResult{}, fmt.Errorf("模拟合成失败")
	}
	if err := os.MkdirAll(filepath.Dir(req.FinalPath), 0o755); err != nil {
		return port.ComposeResult{}, err
	}
	if err := os.WriteFile(req.FinalPath, []byte("fake-final-mp4"), 0o644); err != nil {
		return port.ComposeResult{}, err
	}
	dur := m.SceneDuration
	if dur <= 0 {
		dur = 5
	}
	return port.ComposeResult{FinalPath: req.FinalPath, DurationSec: dur * float64(len(req.Tracks)), Width: 1080, Height: 1920}, nil
}

// Export 实现 port.VideoComposer。
func (m *Composer) Export(_ context.Context, req port.ExportRequest) error {
	m.ExportCalls++
	if err := os.MkdirAll(filepath.Dir(req.DstPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(req.DstPath, []byte("fake-export"), 0o644)
}

// ---- 造声（声音设计 / 声音复刻，§16）----

// VoiceBuild 造声 mock：返回固定音色 ID，并按需写一个假试听音频。
type VoiceBuild struct {
	mu       sync.Mutex
	Calls    int
	Err      error
	VoiceID  string
	Target   string
	Requests []port.VoiceBuildRequest
}

// BuildVoice 实现 port.VoiceBuilder。
func (m *VoiceBuild) BuildVoice(_ context.Context, req port.VoiceBuildRequest) (port.VoiceBuildResult, error) {
	m.mu.Lock()
	m.Calls++
	m.Requests = append(m.Requests, req)
	voiceID, target, err := m.VoiceID, m.Target, m.Err
	m.mu.Unlock()
	if err != nil {
		return port.VoiceBuildResult{}, err
	}
	if voiceID == "" {
		voiceID = "mock-voice-id"
	}
	if target == "" {
		target = req.TargetModel
	}
	if req.PreviewAudioPath != "" {
		if err := os.MkdirAll(filepath.Dir(req.PreviewAudioPath), 0o755); err != nil {
			return port.VoiceBuildResult{}, err
		}
		if err := os.WriteFile(req.PreviewAudioPath, []byte("fake-wav"), 0o644); err != nil {
			return port.VoiceBuildResult{}, err
		}
	}
	return port.VoiceBuildResult{VoiceID: voiceID, TargetModel: target, PreviewAudioPath: req.PreviewAudioPath}, nil
}

// CallsCount 线程安全地读取调用次数。
func (m *VoiceBuild) CallsCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.Calls
}

// VoiceList 系统音色列表 mock。
type VoiceList struct {
	Voices []domain.SystemVoice
	Err    error
}

// ListSystemVoices 实现 port.VoiceLister。
func (m *VoiceList) ListSystemVoices(_ context.Context, _ string) ([]domain.SystemVoice, error) {
	return m.Voices, m.Err
}

// AudioNorm 参考音频归一化 mock：按需写一个假 wav 并返回配置的时长。
// 让 App.SaveVoiceSample 的时长校验逻辑可脱离真实 ffmpeg 单测。
type AudioNorm struct {
	mu       sync.Mutex
	Calls    int
	Duration float64
	Err      error
	Srcs     []string
	Dsts     []string
}

// NormalizeAudio 实现 port.AudioNormalizer。
func (m *AudioNorm) NormalizeAudio(_ context.Context, src, dst string) (float64, error) {
	m.mu.Lock()
	m.Calls++
	m.Srcs = append(m.Srcs, src)
	m.Dsts = append(m.Dsts, dst)
	dur, err := m.Duration, m.Err
	m.mu.Unlock()
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return 0, err
	}
	if err := os.WriteFile(dst, []byte("fake-normalized-wav"), 0o644); err != nil {
		return 0, err
	}
	return dur, nil
}

// CallsCount 线程安全地读取调用次数。
func (m *AudioNorm) CallsCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.Calls
}

// LastSrc 线程安全地读取最近一次归一化的源文件路径。
func (m *AudioNorm) LastSrc() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.Srcs) == 0 {
		return ""
	}
	return m.Srcs[len(m.Srcs)-1]
}
