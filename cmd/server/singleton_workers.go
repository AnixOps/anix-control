package main

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/agentstreams"
	"github.com/AnixOps/anix-control/v4/internal/agentupgrade"
	"github.com/AnixOps/anix-control/v4/internal/database"
	grpcserver "github.com/AnixOps/anix-control/v4/internal/grpc"
	"github.com/AnixOps/anix-control/v4/internal/kernelforward"
	"github.com/AnixOps/anix-control/v4/internal/kernelnodeops"
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

	// Runtime job payloads written before NO-7 carry node tokens: they are
	// scrubbed before the executors serve a row (docs/UPGRADE.md).
	service.RunForwardRuntimeJobPayloadScrub(ctx, database.Get())
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
	// The KernelNodeOps dispatcher recovers the operations a stopped
	// process left started, so it runs in the lease holder only.
	run(kernelnodeops.EngineFor(database.Get()).Run)
	// The forwarding state's timed work: expired routes are paused,
	// request ids are forgotten after a week.
	run(kernelforward.New(database.Get()).Run)
	// Staged Agent upgrades (forward-sdk.md section 9, O4): the campaign's
	// batches are offered on, and closed by, this process's Agent streams.
	run((&agentupgrade.Worker{
		Service: &agentupgrade.Service{DB: database.Get()},
		Streams: func() agentstreams.Streams { return grpcserver.GetAgentStreams() },
	}).Run)
	wg.Wait()
}
