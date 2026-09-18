package engine

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunnerConcurrencyLimit(t *testing.T) {
	r := NewRunner(3, 0)
	r.Backoff = time.Millisecond

	var inFlight, maxInFlight int32
	tasks := make([]Task, 9)
	for i := range tasks {
		tasks[i] = Task{
			Index: i,
			Name:  "t",
			Fn: func(context.Context) error {
				cur := atomic.AddInt32(&inFlight, 1)
				for {
					old := atomic.LoadInt32(&maxInFlight)
					if cur <= old || atomic.CompareAndSwapInt32(&maxInFlight, old, cur) {
						break
					}
				}
				time.Sleep(20 * time.Millisecond)
				atomic.AddInt32(&inFlight, -1)
				return nil
			},
		}
	}

	results := r.RunBatch(context.Background(), tasks)
	for _, res := range results {
		if res.Err != nil {
			t.Fatalf("任务意外失败: %v", res.Err)
		}
	}
	if max := atomic.LoadInt32(&maxInFlight); max != 3 {
		t.Fatalf("最大并发 = %d, 期望 3", max)
	}
}

func TestRunnerRetryUntilSuccess(t *testing.T) {
	r := NewRunner(1, 2)
	r.Backoff = time.Millisecond

	var calls int32
	tasks := []Task{{
		Index: 0,
		Name:  "flaky",
		Fn: func(context.Context) error {
			n := atomic.AddInt32(&calls, 1)
			if n < 3 {
				return errSentinel
			}
			return nil
		},
	}}

	results := r.RunBatch(context.Background(), tasks)
	if results[0].Err != nil {
		t.Fatalf("第三次应成功, got %v", results[0].Err)
	}
	if results[0].Attempts != 3 {
		t.Fatalf("尝试次数 = %d, 期望 3", results[0].Attempts)
	}
}

func TestRunnerRetriesExhausted(t *testing.T) {
	r := NewRunner(1, 1)
	r.Backoff = time.Millisecond

	var calls int32
	tasks := []Task{{
		Index: 0,
		Name:  "always-fail",
		Fn: func(context.Context) error {
			atomic.AddInt32(&calls, 1)
			return errSentinel
		},
	}}

	results := r.RunBatch(context.Background(), tasks)
	if results[0].Err == nil {
		t.Fatal("期望失败，却成功")
	}
	if results[0].Attempts != 2 {
		t.Fatalf("尝试次数 = %d, 期望 2（首次+1 次重试）", results[0].Attempts)
	}
}

type sentinelErr string

func (e sentinelErr) Error() string { return string(e) }

const errSentinel = sentinelErr("sentinel")
