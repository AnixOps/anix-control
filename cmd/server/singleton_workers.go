package main

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v5/internal/agentstreams"
	"github.com/AnixOps/anix-control/v5/internal/agentupgrade"
	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/database"
	grpcserver "github.com/AnixOps/anix-control/v5/internal/grpc"
	"github.com/AnixOps/anix-control/v5/internal/kernelforward"
	"github.com/AnixOps/anix-control/v5/internal/kernelnodeops"
	"github.com/AnixOps/anix-control/v5/internal/realitytarget"
	"github.com/AnixOps/anix-control/v5/internal/retention"
	"github.com/AnixOps/anix-control/v5/internal/service"
)

const (
	// singletonWorkerLease names the lease that selects the one Control
	// process running the singleton workers.
	singletonWorkerLease = "control.singleton-workers"
	// singletonWorkerLeaseTTL bounds how long the workers stay unowned after
	// the leader dies without releasing the lease.
	singletonWorkerLeaseTTL = 30 * time.Second
)

// localAgentSession reports whether this process holds node's Agent
// Control session.
func localAgentSession(node agentcontrol.AgentNode) bool {
	streams := grpcserver.GetAgentStreams()
	if streams == nil {
		return false
	}
	_, ok := streams.Session(node)
	return ok
}

// runSingletonWorkers runs the workers that must not run in two processes at
// once and returns after all of them stopped.
func runSingletonWorkers(ctx context.Context) {
	var wg sync.WaitGroup
	run := func(fn func(context.Context)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fn(ctx)
		}()
	}

	run(func(ctx context.Context) {
		worker := service.NewNodeMonthlyResetWorker(database.Get())
		if err := worker.RunOnce(time.Now()); err != nil {
			log.Printf("Initial node monthly reset run failed: %v", err)
		}
		worker.Start(ctx)
	})
	// The KernelNodeOps dispatcher recovers the operations a stopped
	// process left started, so it runs in the lease holder only.
	run(kernelnodeops.EngineFor(database.Get()).Run)
	// The forwarding state's timed work: expired routes are paused,
	// request ids are forgotten after a week.
	run(kernelforward.New(database.Get()).Run)
	// Entry HA through DNS (forward-sdk.md section 7.4, L2): keeps each
	// bound route's DNS records on its healthy entry nodes.
	run((&kernelforward.EntryHA{Service: kernelforward.New(database.Get()), LocalSession: localAgentSession}).Run)
	// Staged Agent upgrades (forward-sdk.md section 9, O4): the campaign's
	// batches are offered on, and closed by, this process's Agent streams.
	run((&agentupgrade.Worker{
		Service: &agentupgrade.Service{DB: database.Get()},
		Streams: func() agentstreams.Streams { return grpcserver.GetAgentStreams() },
	}).Run)
	// Automatic Reality target selection (internal/realitytarget, phase 3):
	// rechecks the nodes that have it on through their Agents here and
	// replaces a target that failed twice in a row.
	run((&realitytarget.AutoWorker{
		Service: &realitytarget.Service{DB: database.Get(), Prober: kernelnodeops.RealityProbes{}},
		Ready: func(node agentcontrol.AgentNode) bool {
			return kernelnodeops.RealityProbes{}.Support(node).Capable
		},
		Update: service.NewNodeServiceFor(database.Get()).UpdateProtocol,
	}).Run)
	// Certificate-expiry and phase alerts (alerts.*): scans the certificate
	// stores and the phased processes read-only and sends the administrators
	// one digest of what needs them. One process only, or they would all
	// notify.
	run(kernelAlertMonitor(database.Get(), config.Get()).Run)
	// Retention of the traffic, node and audit logs: deletes expired rows
	// in batches every hour and compacts the SQLite file (internal/retention).
	run(retention.NewWorker(database.Get()).Run)
	// Backups (internal/backup): the daily bundle at the panel's time
	// between 03:00 and 05:00, its verification and the retention; at its
	// start it verifies the pre-migration bundle of this start.
	run(backupWorker(config.Get()).Run)
	wg.Wait()
}
