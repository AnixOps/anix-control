package main

import (
	"context"
	"log"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/subscriber"
)

// subscriberChangePrune is how often the subscriber change log drops rows
// older than its retention.
const subscriberChangePrune = time.Hour

// startSubscriberChangePruner keeps the subscriber change log within its
// retention; a consumer with an older cursor lists again.
func (rt *serverRuntime) startSubscriberChangePruner() {
	rt.workers.Go("subscriber change log pruner", func(ctx context.Context) {
		for {
			if _, err := subscriber.PruneChanges(database.Get().WithContext(ctx), time.Now()); err != nil && ctx.Err() == nil {
				log.Printf("Subscriber change log prune failed: %v", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(subscriberChangePrune):
			}
		}
	})
}
