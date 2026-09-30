package main

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/authn"
	compatv2 "github.com/AnixOps/anix-control/v4/internal/compat/v2"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/identitycutover"
	"github.com/AnixOps/anix-control/v4/internal/identityimport"
	"github.com/AnixOps/anix-control/v4/internal/identitykeys"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
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
	identityimport.SetDefault(nil, database.Get())
	identitycutover.SetDefault(nil)
	state, err := service.IdentityAuthorityState(database.Get())
	if err != nil {
		return fmt.Errorf("read the identity authority: %w", err)
	}
	if state == model.IdentityAuthorityFinalized {
		// Only identity issues tokens once the legacy credentials are gone.
		authn.RefuseLegacyTokens()
	}
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
	connect := func() (grpc.ClientConnInterface, error) { return hosts.PackageConn("identity-platform") }
	rt.workers.Go("identity token key refresher", func(ctx context.Context) {
		keys.Run(ctx, identityKeyRefresh, connect, report)
	})
	importer := &identityimport.Importer{DB: database.Get(), Connect: connect}
	runner := identityimport.NewRunner(importer, log.Printf)
	identityimport.SetDefault(runner, database.Get())
	rt.workers.Go("identity account importer", runner.Serve)
	identitycutover.SetDefault(&identitycutover.Service{
		DB: database.Get(), Importer: importer, Hosts: hosts, Freeze: compatv2.DefaultRouteFreeze(),
		PublicKey: func() (ed25519.PublicKey, error) {
			cfg := config.Get()
			if cfg == nil || cfg.Plugins.OfficialPublicKey == "" {
				return nil, service.ErrPluginTrustRootRequired
			}
			return service.ParseOfficialPluginPublicKey(cfg.Plugins.OfficialPublicKey)
		},
		RefreshKeys: func(ctx context.Context) error {
			conn, err := connect()
			if err != nil {
				return err
			}
			if err := keys.Refresh(ctx, conn); err != nil {
				return err
			}
			if len(keys.KeySet()) == 0 {
				return errors.New("identity publishes no token keys (is identity.kek set?)")
			}
			return nil
		},
		OnFinalized: authn.RefuseLegacyTokens,
	})
	return nil
}
