package main

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	appconfig "github.com/AnixOps/anix-control/v5/internal/config"
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
	cfg.Plugins.ControlPollInterval = "2s"

	intervals, err := parsePluginPollIntervals(cfg)
	if err != nil {
		t.Fatalf("parsePluginPollIntervals() error = %v", err)
	}
	// Agent plugin dispatch has no setting (off until M4): its worker would
	// poll at the built-in interval.
	want := pluginPollIntervals{control: 2 * time.Second, dispatch: 5 * time.Second}
	if intervals != want {
		t.Fatalf("intervals = %+v, want %+v", intervals, want)
	}
	cfg.Plugins.ControlPollInterval = ""
	if intervals, err = parsePluginPollIntervals(cfg); err != nil || intervals.control != 5*time.Second {
		t.Fatalf("parsePluginPollIntervals() = %+v, %v; want the 5s default", intervals, err)
	}

	for _, invalid := range []string{"soon", "0s", "-1s"} {
		t.Run(invalid, func(t *testing.T) {
			cfg := &appconfig.Config{}
			cfg.Plugins.ControlPollInterval = invalid
			if _, err := parsePluginPollIntervals(cfg); err == nil {
				t.Fatal("parsePluginPollIntervals() accepted an invalid control_poll_interval")
			}
		})
	}
}
