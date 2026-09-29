package main

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	appconfig "github.com/AnixOps/anix-control/v4/internal/config"
)

func TestBackgroundWorkersStopCancelsAndWaits(t *testing.T) {
	workers := newBackgroundWorkers(context.Background())
	var exited atomic.Int32
	for _, name := range []string{"a", "b", "b"} {
		workers.Go(name, func(ctx context.Context) {
			<-ctx.Done()
			time.Sleep(10 * time.Millisecond)
			exited.Add(1)
		})
	}

	stopped, running := workers.Stop(5 * time.Second)
	if !stopped || len(running) != 0 {
		t.Fatalf("Stop() = %v, %v; want true, none running", stopped, running)
	}
	if got := exited.Load(); got != 3 {
		t.Fatalf("exited workers = %d, want 3 (Stop must wait for every worker)", got)
	}
}

func TestBackgroundWorkersFollowParentContext(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	workers := newBackgroundWorkers(parent)
	done := make(chan struct{})
	workers.Go("worker", func(ctx context.Context) {
		<-ctx.Done()
		close(done)
	})

	cancelParent()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not observe parent cancellation")
	}
	if stopped, _ := workers.Stop(time.Second); !stopped {
		t.Fatal("Stop() reported running workers after they exited")
	}
}

func TestBackgroundWorkersStopRespectsTimeout(t *testing.T) {
	workers := newBackgroundWorkers(context.Background())
	release := make(chan struct{})
	defer close(release)
	workers.Go("quick", func(ctx context.Context) { <-ctx.Done() })
	workers.Go("stuck", func(context.Context) { <-release })

	start := time.Now()
	stopped, running := workers.Stop(50 * time.Millisecond)
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("Stop() took %s, want it bounded by the timeout", elapsed)
	}
	if stopped {
		t.Fatal("Stop() = true with a stuck worker")
	}
	if len(running) != 1 || running[0] != "stuck" {
		t.Fatalf("running = %v, want [stuck]", running)
	}
}

func TestFatalErrorsKeepsFirstErrorWithoutBlocking(t *testing.T) {
	fatal := newFatalErrors()
	first := errors.New("first")
	fatal.Report(first)
	fatal.Report(errors.New("second")) // must not block

	if err := <-fatal.ch; !errors.Is(err, first) {
		t.Fatalf("fatal error = %v, want %v", err, first)
	}
}

func TestParsePluginPollIntervals(t *testing.T) {
	cfg := &appconfig.Config{}
	cfg.GRPC.Enable = true
	cfg.Plugins.ControlExecutionEnabled = true
	cfg.Plugins.DispatchEnabled = true
	cfg.Plugins.DispatchPollInterval = "2s"
	cfg.Plugins.TopologyExecutionEnabled = true
	cfg.Plugins.TopologyPollInterval = "1m"

	intervals, err := parsePluginPollIntervals(cfg)
	if err != nil {
		t.Fatalf("parsePluginPollIntervals() error = %v", err)
	}
	want := pluginPollIntervals{control: 5 * time.Second, dispatch: 2 * time.Second, topology: time.Minute}
	if intervals != want {
		t.Fatalf("intervals = %+v, want %+v", intervals, want)
	}

	for name, mutate := range map[string]func(*appconfig.Config){
		"invalid control interval": func(c *appconfig.Config) { c.Plugins.ControlPollInterval = "soon" },
		"non-positive dispatch":    func(c *appconfig.Config) { c.Plugins.DispatchPollInterval = "0s" },
		"dispatch without grpc":    func(c *appconfig.Config) { c.GRPC.Enable = false },
		"topology without dispatch": func(c *appconfig.Config) {
			c.Plugins.DispatchEnabled = false
		},
	} {
		t.Run(name, func(t *testing.T) {
			invalid := *cfg
			mutate(&invalid)
			if _, err := parsePluginPollIntervals(&invalid); err == nil {
				t.Fatal("parsePluginPollIntervals() accepted invalid configuration")
			}
		})
	}
}
