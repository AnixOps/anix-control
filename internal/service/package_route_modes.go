package service

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/AnixOps/anix-control/v4/internal/packagestore"
	"gorm.io/gorm"
)

// PackageRouteModesConfigKey is the kernel-reserved key of a Control package
// configuration document. It maps v2 route ids to legacy, shadow or native;
// the kernel validates it and removes it before the package's own
// config_schema is applied.
const PackageRouteModesConfigKey = "routes"

// splitPackageRouteModes separates the reserved route-mode map from a
// canonical configuration document. Documents that are not JSON objects, or
// have no routes key, are returned unchanged with present == false.
func splitPackageRouteModes(canonical string) (rest string, modes map[string]string, present bool, err error) {
	trimmed := bytes.TrimSpace([]byte(canonical))
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return canonical, nil, false, nil
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &document); err != nil {
		return "", nil, false, fmt.Errorf("plugin configuration is not a JSON object: %w", err)
	}
	raw, ok := document[PackageRouteModesConfigKey]
	if !ok {
		return canonical, nil, false, nil
	}
	if err := json.Unmarshal(raw, &modes); err != nil || modes == nil {
		return "", nil, true, fmt.Errorf("plugin configuration %q must map route ids to modes", PackageRouteModesConfigKey)
	}
	delete(document, PackageRouteModesConfigKey)
	encoded, err := json.Marshal(document)
	if err != nil {
		return "", nil, true, err
	}
	rest, err = CanonicalKernelOperationConfig(string(encoded))
	if err != nil {
		return "", nil, true, err
	}
	return rest, modes, true, nil
}

type packageCompatibilityRouteDeclaration struct {
	Routes []struct {
		Method       string `json:"method"`
		PackageRoute string `json:"package_route"`
		Transport    string `json:"transport"`
	} `json:"routes"`
}

// validatePackageRouteModes checks a route-mode map against the verified
// compatibility routes of the installation's release. Every route must belong
// to the package; shadow is allowed only for GET routes; WebSocket routes stay
// legacy. Whether the host has a native implementation is enforced by the host
// itself, which reports unsupported modes in its health details.
func validatePackageRouteModes(db *gorm.DB, release model.PluginRelease, publicKey ed25519.PublicKey, modes map[string]string) error {
	raw, err := LoadVerifiedPluginCompatibilityRoutes(db, release, publicKey)
	if err != nil {
		return fmt.Errorf("route modes need verified compatibility routes: %w", err)
	}
	var declaration packageCompatibilityRouteDeclaration
	if err := json.Unmarshal(raw, &declaration); err != nil {
		return fmt.Errorf("decode compatibility routes: %w", err)
	}
	type routeInfo struct{ method, transport string }
	routes := make(map[string]routeInfo, len(declaration.Routes))
	for _, route := range declaration.Routes {
		routes[route.PackageRoute] = routeInfo{method: route.Method, transport: route.Transport}
	}
	ids := make([]string, 0, len(modes))
	for id := range modes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		mode := modes[id]
		route, ok := routes[id]
		if !ok {
			return fmt.Errorf("route %q is not a compatibility route of %s", id, release.PluginID)
		}
		switch mode {
		case packagebridge.RouteModeLegacy:
		case packagebridge.RouteModeShadow:
			if route.method != http.MethodGet {
				return fmt.Errorf("route %q: shadow mode is only allowed for GET routes", id)
			}
			if route.transport == "websocket" {
				return fmt.Errorf("route %q: WebSocket routes stay in legacy mode", id)
			}
		case packagebridge.RouteModeNative:
			if route.transport == "websocket" {
				return fmt.Errorf("route %q: WebSocket routes stay in legacy mode", id)
			}
		default:
			return fmt.Errorf("route %q: mode %q must be legacy, shadow or native", id, mode)
		}
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

// PackageConfig returns the calling host's route modes. The host must be the
// enabled control installation at its desired version and current lifecycle
// generation; any other host is fenced.
func (o PackageHostOperations) PackageConfig(ctx context.Context, host packagebridge.HostIdentity) (packagebridge.PackageConfig, error) {
	installation, err := o.currentInstallation(ctx, host)
	if err != nil {
		return packagebridge.PackageConfig{}, err
	}
	configuration, err := GetPluginConfiguration(o.DB.WithContext(ctx), installation.ID)
	if err != nil {
		return packagebridge.PackageConfig{}, err
	}
	_, stored, _, err := splitPackageRouteModes(configuration.ConfigJSON)
	if err != nil {
		return packagebridge.PackageConfig{}, err
	}
	modes, err := ResolvePackageRouteModes(o.DB.WithContext(ctx), host.PackageID, stored)
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
