package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/authn"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/identitykeys"
	"google.golang.org/grpc"
)

// identityKeyRefresh is how often Control pulls the identity token keys.
// The identity module publishes a key 15 minutes before it signs.
const identityKeyRefresh = 5 * time.Minute

// startIdentityKeys loads the persisted identity token keys, so identity
// tokens verify from the first request, and keeps them fresh from the
// identity-platform host when Control runs package hosts.
func (rt *serverRuntime) startIdentityKeys() error {
	keys := identitykeys.New(database.Get())
	if err := keys.Reload(context.Background()); err != nil {
		return fmt.Errorf("load identity token keys: %w", err)
	}
	authn.SetDefaultIdentityKeys(keys)
	hosts := rt.controlPluginHosts
	if hosts == nil {
		return nil
	}
	last := ""
	report := func(err error) {
		// Identity may not be installed, or may have no KEK yet: say so once.
		if message := err.Error(); message != last {
			last = message
			log.Printf("Identity token keys not refreshed: %v", err)
		}
	}
	rt.workers.Go("identity token key refresher", func(ctx context.Context) {
		keys.Run(ctx, identityKeyRefresh, func() (grpc.ClientConnInterface, error) {
			return hosts.PackageConn("identity-platform")
		}, report)
	})
	return nil
}
