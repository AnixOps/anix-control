package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/AnixOps/anix-control/sdk/moduletls"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/identitybridge"
	"github.com/AnixOps/anix-control/v4/internal/modulepki"
	"github.com/AnixOps/anix-control/v4/internal/moduleruntime"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/AnixOps/anix-control/v4/internal/pluginhost"
	"github.com/AnixOps/anix-control/v4/internal/service"
)

// moduleCAMaintenanceInterval is how often the kernel promotes a rotated
// module CA and prunes expired certificate records.
const moduleCAMaintenanceInterval = time.Hour

// startModuleRuntime starts the mTLS module listener when the module runtime
// is enabled and turns on the remote runtime of hosts, which admits remote
// instances. Without hosts (package execution off) every Bind is rejected.
func (rt *serverRuntime) startModuleRuntime(cfg *config.Config, hosts *pluginhost.Supervisor) error {
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
	var binder packagebridge.Binder
	if hosts != nil {
		binder = hosts
	}
	bridge, err := packagebridge.NewModuleBridge(binder, packagebridge.ModuleBridgeOptions{Cluster: cluster})
	if err != nil {
		return err
	}
	if hosts != nil {
		if err := hosts.EnableRemote(pluginhost.RemoteConfig{
			Instances: bridge, TLS: source, Cluster: cluster, BindTimeout: settings.BindTimeoutOrDefault(),
			IsRemote: service.PluginRuntimeIsRemote(database.Get()),
		}); err != nil {
			return fmt.Errorf("module runtime: %w", err)
		}
	}
	listener, err := net.Listen("tcp", settings.ListenOrDefault())
	if err != nil {
		return fmt.Errorf("module listener: %w", err)
	}
	identity, err := identitybridge.NewKernelIdentity(cfg)
	if err != nil {
		return fmt.Errorf("module runtime kernel identity: %w", err)
	}
	orders, err := identitybridge.NewKernelOrder(cfg)
	if err != nil {
		return fmt.Errorf("module runtime kernel order: %w", err)
	}
	subscribers, err := identitybridge.NewKernelSubscriber(cfg)
	if err != nil {
		return err
	}
	kernelSettings, err := identitybridge.NewKernelSettings(cfg)
	if err != nil {
		return err
	}
	telemetry, err := identitybridge.NewKernelTelemetry(cfg)
	if err != nil {
		return fmt.Errorf("module runtime kernel telemetry: %w", err)
	}
	nodeOps, err := identitybridge.NewKernelNodeOps(cfg)
	if err != nil {
		return fmt.Errorf("module runtime kernel node operations: %w", err)
	}
	server := &moduleruntime.Listener{
		TLS: source, Cluster: cluster, PKI: authority, Bridge: bridge, KernelIdentity: identity, KernelSubscriber: subscribers,
		KernelSettings: kernelSettings, KernelTelemetry: telemetry, KernelNodeOps: nodeOps,
	}
	server.KernelOrder = orders
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
