package service

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/packagebridge"
	"github.com/AnixOps/anix-control/v5/internal/packagestore"
	"gorm.io/gorm"
)

// RemovedRouteModesConfigKey is the configuration key that held a Control
// package's stored route modes until they were removed (single-binary
// design, section 4.3; the 0004_drop_route_modes migration deleted the
// stored maps). The kernel refuses a configuration document that sets it,
// so a client from before does not believe it switched a route.
const RemovedRouteModesConfigKey = "routes"

// rejectRemovedRouteModes refuses a Control package configuration that
// carries the removed route-mode map.
func rejectRemovedRouteModes(canonical string) error {
	trimmed := bytes.TrimSpace([]byte(canonical))
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &document); err != nil {
		return fmt.Errorf("plugin configuration is not a JSON object: %w", err)
	}
	if _, ok := document[RemovedRouteModesConfigKey]; ok {
		return fmt.Errorf("configuration key %q was removed: route modes come only from the rehearsed list (config/package-route-defaults.json)", RemovedRouteModesConfigKey)
	}
	return nil
}

// PackageHostOperations serves the session-scoped package bridge RPCs from
// the kernel database.
type PackageHostOperations struct {
	DB *gorm.DB
	// Storage provisions package storage for LeaseStorage; nil disables it.
	Storage *packagestore.Store
	// FallbackPublicKey verifies releases recorded without a trust root.
	FallbackPublicKey ed25519.PublicKey
}

var _ packagebridge.HostOperations = PackageHostOperations{}

// PackageConfig returns the calling host's route modes: the rehearsed list
// and identity group A (ResolveEffectivePackageRouteModes). The host must be
// the enabled control installation at its desired version and current
// lifecycle generation; any other host is fenced.
func (o PackageHostOperations) PackageConfig(ctx context.Context, host packagebridge.HostIdentity) (packagebridge.PackageConfig, error) {
	installation, err := o.currentInstallation(ctx, host)
	if err != nil {
		return packagebridge.PackageConfig{}, err
	}
	configuration, err := GetPluginConfiguration(o.DB.WithContext(ctx), installation.ID)
	if err != nil {
		return packagebridge.PackageConfig{}, err
	}
	modes, err := ResolveEffectivePackageRouteModes(o.DB.WithContext(ctx), host.PackageID, installation.DesiredVersion)
	if err != nil {
		return packagebridge.PackageConfig{}, err
	}
	active := make(map[string]string, len(modes))
	for route, mode := range modes {
		if mode == packagebridge.RouteModeShadow || mode == packagebridge.RouteModeNative {
			active[route] = mode
		}
	}
	return packagebridge.PackageConfig{Revision: configuration.Revision, ConfigHash: configuration.ConfigHash, RouteModes: active}, nil
}

// currentInstallation returns the host's Control installation if the host is
// its enabled desired version at the current lifecycle generation, and
// ErrHostFenced otherwise.
func (o PackageHostOperations) currentInstallation(ctx context.Context, host packagebridge.HostIdentity) (model.PluginInstallation, error) {
	var installation model.PluginInstallation
	if o.DB == nil {
		return installation, errors.New("database is not initialized")
	}
	if err := o.DB.WithContext(ctx).First(&installation, "plugin_id = ? AND target = ?", host.PackageID, "control").Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return installation, packagebridge.ErrHostFenced
		}
		return installation, err
	}
	if !installation.Enabled || installation.DesiredVersion != host.Version ||
		installation.LifecycleGeneration <= 0 || uint64(installation.LifecycleGeneration) != host.Generation {
		return installation, packagebridge.ErrHostFenced
	}
	return installation, nil
}
