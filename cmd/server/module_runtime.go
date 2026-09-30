package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/modulepki"
	"github.com/AnixOps/anix-control/v4/internal/moduleruntime"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/AnixOps/anix-control/v4/pkg/moduletls"
)

// moduleCAMaintenanceInterval is how often the kernel promotes a rotated
// module CA and prunes expired certificate records.
const moduleCAMaintenanceInterval = time.Hour

// startModuleRuntime starts the mTLS module listener when the module runtime
// is enabled. Remote installations bind through binder; nil rejects every
// Bind until the remote runtime manager provides one.
func (rt *serverRuntime) startModuleRuntime(cfg *config.Config, binder packagebridge.Binder) error {
	settings := cfg.ModuleRuntime
	if !settings.Enabled {
		return nil
	}
	cluster := settings.ClusterOrDefault()
	var (
		source    moduletls.Source
		authority *modulepki.Authority
		err       error
	)
	switch settings.PKIOrDefault() {
	case config.ModulePKIExternal:
		source, err = (&modulepki.ExternalSource{
			TrustBundleFile: settings.TrustBundleFile, CertFile: settings.CertFile, KeyFile: settings.KeyFile, Cluster: cluster,
		}).Source()
	default:
		authority, err = modulepki.FromConfig(settings, database.Get())
		if err == nil {
			source, err = authority.KernelTLS(context.Background(), time.Minute)
		}
	}
	if err != nil {
		return fmt.Errorf("module runtime TLS: %w", err)
	}
	bridge, err := packagebridge.NewModuleBridge(binder, packagebridge.ModuleBridgeOptions{Cluster: cluster})
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", settings.ListenOrDefault())
	if err != nil {
		return fmt.Errorf("module listener: %w", err)
	}
	server := &moduleruntime.Listener{TLS: source, Cluster: cluster, PKI: authority, Bridge: bridge}
	rt.workers.Go("module listener", func(ctx context.Context) {
		if err := server.Serve(ctx, listener); err != nil && !errors.Is(err, net.ErrClosed) {
			rt.fatal.Report(fmt.Errorf("module listener: %w", err))
		}
	})
	rt.workers.Go("module bridge session sweeper", bridge.Run)
	if authority != nil {
		rt.workers.Go("module CA maintenance", func(ctx context.Context) {
			maintainModuleCA(ctx, authority)
		})
	}
	log.Printf("Module runtime listening on %s (cluster %s, %s PKI)", listener.Addr(), cluster, settings.PKIOrDefault())
	return nil
}

func maintainModuleCA(ctx context.Context, authority *modulepki.Authority) {
	ticker := time.NewTicker(moduleCAMaintenanceInterval)
	defer ticker.Stop()
	for {
		if err := authority.Maintain(ctx); err != nil && ctx.Err() == nil {
			log.Printf("Module CA maintenance: %v", err)
		}
		if err := authority.PruneCertificates(ctx, time.Now().Add(-authority.Lifetime())); err != nil && ctx.Err() == nil {
			log.Printf("Module certificate pruning: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
