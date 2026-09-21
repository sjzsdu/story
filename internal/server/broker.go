package server

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// eventKind 推送给浏览器的 SSE 事件类型。
type eventKind string

const (
	evSnapshot eventKind = "snapshot" // 集状态快照
	evJob      eventKind = "job"      // 后台任务生命周期
	evLog      eventKind = "log"      // 文本日志
)

// jobStatus 后台动作状态。
type jobStatus string

const (
	jobRunning  jobStatus = "running"
	jobDone     jobStatus = "done"
	jobFailed   jobStatus = "failed"
	jobCanceled jobStatus = "canceled" // 用户手动停止
)

// jobEvent 一个后台动作的状态通知。
type jobEvent struct {
	ID      string    `json:"id"`
	Action  string    `json:"action"`
	Status  jobStatus `json:"status"`
	Error   string    `json:"error,omitempty"`
	Started time.Time `json:"started_at"`
	Ended   time.Time `json:"ended_at,omitempty"`
	// canceled 内部标记（不出现在 JSON）：finishJob 据此把状态置为 canceled。
	canceled bool
}

type subscriber struct {
	ch   chan []byte
	ctx  context.Context
	once sync.Once
}

// broker 按 episodeID 维护 SSE 订阅者，并保证同一集同时只有一个动作在跑。
type broker struct {
	mu      sync.Mutex
	subs    map[string]map[*subscriber]struct{}
	running map[string]*jobEvent
	// cancels 各集在跑动作的取消函数（runAction 启动前登记，finishJob 清理）。
	cancels map[string]context.CancelFunc
}

func newBroker() *broker {
	return &broker{
		subs:    make(map[string]map[*subscriber]struct{}),
		running: make(map[string]*jobEvent),
		cancels: make(map[string]context.CancelFunc),
	}
}

// subscribe 订阅某一集的事件，返回接收通道与退订函数。
func (b *broker) subscribe(ctx context.Context, episodeID string) (<-chan []byte, func()) {
	s := &subscriber{ch: make(chan []byte, 16), ctx: ctx}
	b.mu.Lock()
	if b.subs[episodeID] == nil {
		b.subs[episodeID] = make(map[*subscriber]struct{})
	}
	b.subs[episodeID][s] = struct{}{}
	b.mu.Unlock()

	unsub := func() {
		b.mu.Lock()
		if set, ok := b.subs[episodeID]; ok {
			delete(set, s)
			if len(set) == 0 {
				delete(b.subs, episodeID)
			}
		}
		s.once.Do(func() { close(s.ch) })
		b.mu.Unlock()
	}
	return s.ch, unsub
}

// publish 向某一集的全部订阅者非阻塞广播一条 SSE 帧。
func (b *broker) publish(episodeID string, kind eventKind, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	frame := []byte(fmt.Sprintf("event: %s\ndata: %s\n\n", kind, data))

	b.mu.Lock()
	set := b.subs[episodeID]
	subs := make([]*subscriber, 0, len(set))
	for s := range set {
		subs = append(subs, s)
	}
	b.mu.Unlock()

	for _, s := range subs {
		select {
		case s.ch <- frame:
		case <-s.ctx.Done():
		default:
			// 慢客户端丢弃一帧（状态快照每秒会重发）。
		}
	}
}

// tryStartJob 尝试为某集抢占动作槽位，已有任务运行时返回 false。
func (b *broker) tryStartJob(episodeID, action string) (*jobEvent, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if cur := b.running[episodeID]; cur != nil && cur.Status == jobRunning {
		return cur, false
	}
	j := &jobEvent{ID: fmt.Sprintf("%s-%d", episodeID, time.Now().UnixNano()), Action: action, Status: jobRunning, Started: time.Now()}
	b.running[episodeID] = j
	return j, true
}

// cancelJob 请求停止某集正在执行的动作；返回 false 表示当前没有可停止的任务。
// 停止只取消上下文（进而 kill 正在跑的 bl/ffmpeg 子进程），已完成的产物全部保留。
func (b *broker) cancelJob(episodeID string) (*jobEvent, bool) {
	b.mu.Lock()
	j := b.running[episodeID]
	if j == nil || j.Status != jobRunning {
		b.mu.Unlock()
		return j, false
	}
	j.canceled = true
	cancel := b.cancels[episodeID]
	b.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return j, true
}

func (b *broker) finishJob(episodeID string, j *jobEvent, err error) {
	b.mu.Lock()
	j.Ended = time.Now()
	delete(b.cancels, episodeID)
	switch {
	case j.canceled:
		j.Status = jobCanceled
		j.Error = "已手动停止；已完成的产物全部保留，可再次执行续跑"
	case err != nil:
		j.Status = jobFailed
		j.Error = err.Error()
	default:
		j.Status = jobDone
	}
	b.running[episodeID] = j
	b.mu.Unlock()
}

// runAction 抢占槽位、后台执行 fn，并周期推送集状态快照。
// loadSnapshot 在每次推送前读取最新状态；fn 为具体动作。
func (b *broker) runAction(rootCtx context.Context, episodeID, action string,
	loadSnapshot func() (any, error), fn func(ctx context.Context) error,
) (*jobEvent, bool) {
	// 先建 ctx 并登记取消函数，再抢槽位：保证 cancelJob 一定能停到任务。
	ctx, cancel := context.WithCancel(rootCtx)
	j, ok := b.tryStartJob(episodeID, action)
	if !ok {
		cancel()
		return j, false
	}
	b.mu.Lock()
	b.cancels[episodeID] = cancel
	b.mu.Unlock()
	b.publish(episodeID, evJob, j)

	go func() {
		defer cancel()

		pushSnapshot := func() {
			snap, err := loadSnapshot()
			if err != nil {
				return
			}
			b.publish(episodeID, evSnapshot, snap)
		}
		pushSnapshot()

		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					pushSnapshot()
				}
			}
		}()

		err := fn(ctx)
		b.finishJob(episodeID, j, err)
		pushSnapshot()
		b.publish(episodeID, evJob, b.currentJob(episodeID))
	}()
	return j, true
}

func (b *broker) currentJob(episodeID string) *jobEvent {
	b.mu.Lock()
	defer b.mu.Unlock()
	if j := b.running[episodeID]; j != nil {
		cp := *j
		return &cp
	}
	return nil
}
