package main

import (
	"context"
	"log"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/agentreports"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/kernelsettings"
	"github.com/AnixOps/anix-control/v4/internal/subscriber"
)

// subscriberChangePrune is how often the subscriber change log, the
// subscriber and settings request ledgers and the agent report batch record
// drop rows older than their retention.
const subscriberChangePrune = time.Hour

// startSubscriberChangePruner keeps the subscriber change log, the
// subscriber and settings request ledgers and the agent report batch record
// within their retention; a consumer with an older cursor lists again.
func (rt *serverRuntime) startSubscriberChangePruner() {
	rt.workers.Go("subscriber change log pruner", func(ctx context.Context) {
		for {
			db := database.Get().WithContext(ctx)
			if _, err := subscriber.PruneChanges(db, time.Now()); err != nil && ctx.Err() == nil {
				log.Printf("Subscriber change log prune failed: %v", err)
			}
			if _, err := subscriber.PruneRequests(db, time.Now()); err != nil && ctx.Err() == nil {
				log.Printf("Subscriber request ledger prune failed: %v", err)
			}
			if _, err := kernelsettings.PruneRequests(db, time.Now()); err != nil && ctx.Err() == nil {
				log.Printf("Settings request ledger prune failed: %v", err)
			}
			if _, err := agentreports.Prune(db, time.Now()); err != nil && ctx.Err() == nil {
				log.Printf("Agent report batch prune failed: %v", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(subscriberChangePrune):
			}
		}
	})
}
