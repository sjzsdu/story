package engine

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Task 批处理中的一个工作单元。
type Task struct {
	Index int
	Name  string
	Fn    func(ctx context.Context) error
}

// TaskResult 工作单元的执行结果。
type TaskResult struct {
	Index    int
	Name     string
	Err      error
	Attempts int
}

// Runner 带并发上限与重试退避的批执行器。
// 业务代码统一通过 Runner 跑批量任务，不得自行实现并发/重试。
type Runner struct {
	// MaxConcurrency 最大并发数。
	MaxConcurrency int
	// MaxRetries 首次失败后的最大重试次数（总尝试次数 = MaxRetries+1）。
	MaxRetries int
	// Backoff 退避基数，实际等待 Backoff * 2^attempt。
	Backoff time.Duration
}

// NewRunner 创建执行器并对参数做下限保护。
func NewRunner(maxConcurrency, maxRetries int) *Runner {
	if maxConcurrency < 1 {
		maxConcurrency = 1
	}
	if maxRetries < 0 {
		maxRetries = 0
	}
	return &Runner{
		MaxConcurrency: maxConcurrency,
		MaxRetries:     maxRetries,
		Backoff:        2 * time.Second,
	}
}

// RunBatch 并发执行全部任务，返回与任务同序的结果。
func (r *Runner) RunBatch(ctx context.Context, tasks []Task) []TaskResult {
	results := make([]TaskResult, len(tasks))
	sem := make(chan struct{}, r.MaxConcurrency)
	var wg sync.WaitGroup

	for i := range tasks {
		t := tasks[i]
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				results[t.Index] = TaskResult{Index: t.Index, Name: t.Name, Err: ctx.Err()}
				return
			}
			defer func() { <-sem }()

			var err error
			attempt := 0
			for {
				attempt++
				err = t.Fn(ctx)
				if err == nil {
					results[t.Index] = TaskResult{Index: t.Index, Name: t.Name, Attempts: attempt}
					return
				}
				if attempt > r.MaxRetries || ctx.Err() != nil {
					break
				}
				wait := r.Backoff * time.Duration(1<<(attempt-1)) // 2s, 4s, 8s...
				select {
				case <-time.After(wait):
				case <-ctx.Done():
					results[t.Index] = TaskResult{Index: t.Index, Name: t.Name, Err: ctx.Err(), Attempts: attempt}
					return
				}
			}
			results[t.Index] = TaskResult{Index: t.Index, Name: t.Name, Err: err, Attempts: attempt}
		}()
	}
	wg.Wait()
	return results
}

// FailedError 汇总多个失败结果。
type FailedError struct {
	Items []TaskResult
}

func (e *FailedError) Error() string {
	if len(e.Items) == 0 {
		return ""
	}
	msg := fmt.Sprintf("%d 个任务失败:", len(e.Items))
	for _, it := range e.Items {
		msg += fmt.Sprintf("\n  - %s: %v", it.Name, it.Err)
	}
	return msg
}

// CollectFailures 过滤出失败结果。
func CollectFailures(results []TaskResult) []TaskResult {
	var failed []TaskResult
	for _, r := range results {
		if r.Err != nil {
			failed = append(failed, r)
		}
	}
	return failed
}
