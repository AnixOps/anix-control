package main

import (
	"context"
	"log"
	"sort"
	"sync"
	"time"
)

// 关闭阶段各步骤的超时。HTTP 排空沿用 shutdownTimeout；后台 worker、gRPC 和
// 插件宿主各自拥有独立的超时，前一步耗尽时间不会让后一步直接拿到已过期的 ctx。
const (
	workerStopTimeout     = 15 * time.Second
	grpcStopTimeout       = 10 * time.Second
	pluginHostStopTimeout = 15 * time.Second
)

// backgroundWorkers 跟踪进程内的常驻后台 goroutine。所有 worker 共用一个从
// 根 context 派生的 ctx；Stop 取消该 ctx 并在限定时间内等待它们全部返回。
type backgroundWorkers struct {
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	mu      sync.Mutex
	running map[string]int
}

func newBackgroundWorkers(parent context.Context) *backgroundWorkers {
	ctx, cancel := context.WithCancel(parent)
	return &backgroundWorkers{ctx: ctx, cancel: cancel, running: make(map[string]int)}
}

// Go runs fn in a new goroutine. fn must return once ctx is cancelled.
func (w *backgroundWorkers) Go(name string, fn func(ctx context.Context)) {
	w.mu.Lock()
	w.running[name]++
	w.mu.Unlock()
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		defer func() {
			w.mu.Lock()
			if w.running[name]--; w.running[name] <= 0 {
				delete(w.running, name)
			}
			w.mu.Unlock()
		}()
		fn(w.ctx)
	}()
}

// Stop cancels every worker and waits up to timeout for them to return. It
// reports whether all workers stopped; on timeout the still-running worker
// names are returned so the caller can log them.
func (w *backgroundWorkers) Stop(timeout time.Duration) (bool, []string) {
	w.cancel()
	if waitTimeout(w.wg.Wait, timeout) {
		return true, nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	names := make([]string, 0, len(w.running))
	for name := range w.running {
		names = append(names, name)
	}
	sort.Strings(names)
	return false, names
}

// waitTimeout runs wait in a new goroutine and reports whether it returned
// within timeout. On timeout the goroutine is left running; this is only used
// on the way to process exit.
func waitTimeout(wait func(), timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		defer close(done)
		wait()
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
		return true
	case <-timer.C:
		return false
	}
}

// fatalErrors collects runtime failures that happen after startup (for
// example an HTTP listener that cannot bind). The first error triggers the
// same orderly shutdown as SIGTERM, after which the process exits non-zero.
type fatalErrors struct {
	ch chan error
}

func newFatalErrors() *fatalErrors {
	return &fatalErrors{ch: make(chan error, 1)}
}

// Report records err without blocking; only the first error is kept, later
// ones are logged.
func (f *fatalErrors) Report(err error) {
	select {
	case f.ch <- err:
	default:
		log.Printf("Additional fatal error during shutdown: %v", err)
	}
}
