package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/authn"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/database"
)

// tokenRevocationRefresh is how often revocations written by other processes
// reach this one; revocations this process makes apply at once.
const tokenRevocationRefresh = 5 * time.Second

// startTokenRevocations loads the revocation tables before any server accepts
// a token, and keeps them fresh.
func (rt *serverRuntime) startTokenRevocations(cfg *config.Config) error {
	// A user revocation matters for as long as a token issued just before it
	// can live.
	lifetime := time.Duration(cfg.JWT.Expire)*time.Second + time.Minute
	store := authn.NewStore(database.Get(), lifetime)
	if err := store.Reload(context.Background()); err != nil {
		return fmt.Errorf("load token revocations: %w", err)
	}
	authn.SetDefaultStore(store)
	rt.workers.Go("token revocation refresher", func(ctx context.Context) {
		store.Run(ctx, tokenRevocationRefresh, func(err error) {
			log.Printf("Token revocation refresh failed: %v", err)
		})
	})
	return nil
}
