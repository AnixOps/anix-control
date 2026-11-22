package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/agentpki"
	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/modulepki"
	"gorm.io/gorm"
)

// moduleCAMaintenanceInterval is how often the kernel promotes a rotated
// module CA and prunes expired certificate records.
const moduleCAMaintenanceInterval = time.Hour

// startKernelCAMaintenance runs the built-in CA's maintenance, and the
// forward link CA's. The CA signs module and agent certificates, so it runs
// whenever the CA is configured (module_runtime.ca_kek), with or without the
// module listener.
func (rt *serverRuntime) startKernelCAMaintenance(cfg *config.Config, db *gorm.DB) error {
	authority, err := modulepki.FromConfig(cfg.ModuleRuntime, db)
	if errors.Is(err, modulepki.ErrBuiltinPKIDisabled) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("built-in CA: %w", err)
	}
	rt.workers.Go("module CA maintenance", func(ctx context.Context) {
		maintainModuleCA(ctx, authority)
	})
	link, err := agentpki.LinkAuthorityFromConfig(cfg, db)
	if err != nil {
		return fmt.Errorf("forward link CA: %w", err)
	}
	rt.workers.Go("forward link CA maintenance", func(ctx context.Context) {
		maintainLinkCA(ctx, link)
	})
	return nil
}

// maintainLinkCA promotes a next forward link CA once it has been trusted
// for one link certificate lifetime.
func maintainLinkCA(ctx context.Context, link *agentpki.LinkAuthority) {
	ticker := time.NewTicker(moduleCAMaintenanceInterval)
	defer ticker.Stop()
	for {
		if err := link.Maintain(ctx); err != nil && ctx.Err() == nil {
			log.Printf("Forward link CA maintenance: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func maintainModuleCA(ctx context.Context, authority *modulepki.Authority) {
	ticker := time.NewTicker(moduleCAMaintenanceInterval)
	defer ticker.Stop()
	for {
		if err := authority.Maintain(ctx); err != nil && ctx.Err() == nil {
			log.Printf("Module CA maintenance: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
