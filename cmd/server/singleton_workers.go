package main

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/service"
)

const (
	// singletonWorkerLease names the lease that selects the one Control
	// process running the singleton workers.
	singletonWorkerLease = "control.singleton-workers"
	// singletonWorkerLeaseTTL bounds how long the workers stay unowned after
	// the leader dies without releasing the lease.
	singletonWorkerLeaseTTL = 30 * time.Second
)

// runSingletonWorkers runs the workers that must not run in two processes at
// once and returns after all of them stopped. The forward job executors
// requeue interrupted jobs when they start, which is safe because only the
// lease holder executes jobs.
func runSingletonWorkers(ctx context.Context, bridgeEnabled bool) {
	var wg sync.WaitGroup
	run := func(fn func(context.Context)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fn(ctx)
		}()
	}

	run(func(ctx context.Context) {
		service.NewPanelForwardRuntimeJobExecutor(database.Get()).Start(ctx)
	})
	if bridgeEnabled {
		run(func(ctx context.Context) {
			service.NewForwardAgentBridgeWorker(database.Get()).Start(ctx)
		})
	}
	run(func(ctx context.Context) {
		worker := service.NewForwardFlowResetWorker(database.Get())
		if err := worker.RunOnce(time.Now()); err != nil {
			log.Printf("Initial forward flow reset run failed: %v", err)
		}
		worker.Start(ctx)
	})
	run(func(ctx context.Context) {
		worker := service.NewNodeMonthlyResetWorker(database.Get())
		if err := worker.RunOnce(time.Now()); err != nil {
			log.Printf("Initial node monthly reset run failed: %v", err)
		}
		worker.Start(ctx)
	})
	run(func(ctx context.Context) {
		service.NewForwardGostStatsWorker(database.Get()).Start(ctx)
	})
	run(func(ctx context.Context) {
		service.NewForwardAnsibleStatsWorker(database.Get()).Start(ctx)
	})
	run(func(ctx context.Context) {
		service.NewForwardLatencyProber(database.Get()).Start(ctx)
	})
	wg.Wait()
}
