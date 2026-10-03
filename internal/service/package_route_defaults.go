package service

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	configtables "github.com/AnixOps/anix-control/v4/config"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"golang.org/x/mod/semver"
	"gorm.io/gorm"
)

// Package route default policies (package_routes.default_mode).
const (
	// PackageRouteDefaultRehearsed: the routes of
	// config/package-route-defaults.json run natively when no mode is stored
	// for them and the installed release is at least the rehearsed one.
	PackageRouteDefaultRehearsed = "rehearsed"
	// PackageRouteDefaultLegacy: the kill switch; no route defaults to
	// native.
	PackageRouteDefaultLegacy = "legacy"
)

// Where a route's mode comes from (RouteModeEntry.Source).
const (
	// RouteModeSourceStored: the installation's configuration stores the
	// mode (anix-control routes set/rollback, the identity cutover).
	RouteModeSourceStored = "stored"
	// RouteModeSourceDefault: no stored mode; the route is in the rehearsed
	// default set and runs natively.
	RouteModeSourceDefault = "default"
	// RouteModeSourceKillSwitch: no stored mode; the route is in the
	// default set, but package_routes.default_mode is legacy.
	RouteModeSourceKillSwitch = "kill-switch"
	// RouteModeSourcePackageTooOld: no stored mode; the route is in the
	// default set, but the installed release is older than the rehearsed
	// one.
	RouteModeSourcePackageTooOld = "package-too-old"
	// RouteModeSourceIdentityAuthority: no stored mode; an identity group A
	// route, native because identity is authoritative.
	RouteModeSourceIdentityAuthority = "identity-authority"
	// RouteModeSourceUnset: no stored mode and no default: legacy.
	RouteModeSourceUnset = "unset"
)

var packageRouteDefaultPolicy atomic.Value

// SetPackageRouteDefaultPolicy sets the default policy
// (package_routes.default_mode) for this process. Anything but legacy means
// rehearsed, the default.
func SetPackageRouteDefaultPolicy(policy string) {
	if strings.ToLower(strings.TrimSpace(policy)) == PackageRouteDefaultLegacy {
		packageRouteDefaultPolicy.Store(PackageRouteDefaultLegacy)
		return
	}
	packageRouteDefaultPolicy.Store(PackageRouteDefaultRehearsed)
}

// PackageRouteDefaultPolicy returns the process's default policy, rehearsed
// unless SetPackageRouteDefaultPolicy chose legacy.
func PackageRouteDefaultPolicy() string {
	if policy, ok := packageRouteDefaultPolicy.Load().(string); ok && policy != "" {
		return policy
	}
	return PackageRouteDefaultRehearsed
}

// PackageRouteDefaultPackage is one package of
// config/package-route-defaults.json.
type PackageRouteDefaultPackage struct {
	PackageID string `json:"package_id"`
	// Batch is the rehearsal batch that signed the package off.
	Batch int `json:"batch"`
	// MinVersion is the oldest release the default applies to: the
	// rehearsed one. Older releases (the signed 4.0 packages) stay legacy.
	MinVersion string   `json:"min_version"`
	Routes     []string `json:"routes"`
}

// PackageRouteDefaults is config/package-route-defaults.json.
type PackageRouteDefaults struct {
	Format      string                       `json:"format"`
	DefaultMode string                       `json:"default_mode"`
	Packages    []PackageRouteDefaultPackage `json:"packages"`
	byPackage   map[string]PackageRouteDefaultPackage
	routes      map[string]string
}

// Package returns a package's defaults.
func (d *PackageRouteDefaults) Package(packageID string) (PackageRouteDefaultPackage, bool) {
	if d == nil {
		return PackageRouteDefaultPackage{}, false
	}
	pkg, ok := d.byPackage[packageID]
	return pkg, ok
}

// RouteCount is the number of routes in the default set.
func (d *PackageRouteDefaults) RouteCount() int {
	if d == nil {
		return 0
	}
	return len(d.routes)
}

// Includes reports whether route of packageID is in the default set.
func (d *PackageRouteDefaults) Includes(packageID, route string) bool {
	return d != nil && d.routes[route] == packageID
}

const packageRouteDefaultsFormat = "anixops.package-route-defaults/v1"

func parsePackageRouteDefaults(raw []byte) (*PackageRouteDefaults, error) {
	defaults := &PackageRouteDefaults{}
	if err := json.Unmarshal(raw, defaults); err != nil {
		return nil, fmt.Errorf("parse config/package-route-defaults.json: %w", err)
	}
	if defaults.Format != packageRouteDefaultsFormat {
		return nil, fmt.Errorf("config/package-route-defaults.json: format %q, want %q", defaults.Format, packageRouteDefaultsFormat)
	}
	if defaults.DefaultMode != packagebridge.RouteModeNative {
		return nil, fmt.Errorf("config/package-route-defaults.json: default_mode %q, want native", defaults.DefaultMode)
	}
	defaults.byPackage = make(map[string]PackageRouteDefaultPackage, len(defaults.Packages))
	defaults.routes = map[string]string{}
	for _, pkg := range defaults.Packages {
		if pkg.PackageID == "" || len(pkg.Routes) == 0 {
			return nil, fmt.Errorf("config/package-route-defaults.json: a package needs an id and routes")
		}
		if _, dup := defaults.byPackage[pkg.PackageID]; dup {
			return nil, fmt.Errorf("config/package-route-defaults.json: package %s is listed twice", pkg.PackageID)
		}
		if !semver.IsValid("v" + pkg.MinVersion) {
			return nil, fmt.Errorf("config/package-route-defaults.json: %s min_version %q is not a semantic version", pkg.PackageID, pkg.MinVersion)
		}
		defaults.byPackage[pkg.PackageID] = pkg
		for _, route := range pkg.Routes {
			if _, dup := defaults.routes[route]; dup {
				return nil, fmt.Errorf("config/package-route-defaults.json: route %s is listed twice", route)
			}
			defaults.routes[route] = pkg.PackageID
		}
	}
	return defaults, nil
}

var (
	packageRouteDefaultsOnce  sync.Once
	packageRouteDefaultsValue *PackageRouteDefaults
	packageRouteDefaultsErr   error
)

// LoadPackageRouteDefaults returns the embedded default set.
func LoadPackageRouteDefaults() (*PackageRouteDefaults, error) {
	packageRouteDefaultsOnce.Do(func() {
		packageRouteDefaultsValue, packageRouteDefaultsErr = parsePackageRouteDefaults(configtables.PackageRouteDefaults)
	})
	return packageRouteDefaultsValue, packageRouteDefaultsErr
}

// PackageReleaseAtLeast reports whether version is a semantic version at
// least minimum ("4.1.0" is above "4.1.0-rc.5"). A version that does not
// parse is not.
func PackageReleaseAtLeast(version, minimum string) bool {
	v, m := "v"+strings.TrimPrefix(strings.TrimSpace(version), "v"), "v"+strings.TrimPrefix(minimum, "v")
	return semver.IsValid(v) && semver.IsValid(m) && semver.Compare(v, m) >= 0
}

// PackageRouteDefaultState is how the default policy applies to one
// installed package.
type PackageRouteDefaultState struct {
	// Policy is package_routes.default_mode.
	Policy string `json:"policy"`
	// Routes is the number of the package's routes in the default set.
	Routes int `json:"routes"`
	// MinVersion is the release the default needs ("" when the package has
	// no default routes).
	MinVersion string `json:"min_version,omitempty"`
	// Source is what the package's unstored default routes get: default,
	// kill-switch or package-too-old ("" without default routes).
	Source string `json:"source,omitempty"`
	// Note explains a default that does not apply.
	Note string `json:"note,omitempty"`
}

// packageRouteDefaultState decides how the default set applies to
// packageID at version under the process policy.
func packageRouteDefaultState(defaults *PackageRouteDefaults, packageID, version string) PackageRouteDefaultState {
	state := PackageRouteDefaultState{Policy: PackageRouteDefaultPolicy()}
	pkg, ok := defaults.Package(packageID)
	if !ok {
		return state
	}
	state.Routes, state.MinVersion = len(pkg.Routes), pkg.MinVersion
	switch {
	case state.Policy == PackageRouteDefaultLegacy:
		state.Source = RouteModeSourceKillSwitch
		state.Note = "package_routes.default_mode is legacy: no route defaults to native"
	case !PackageReleaseAtLeast(version, pkg.MinVersion):
		state.Source = RouteModeSourcePackageTooOld
		state.Note = fmt.Sprintf("the installed release %s is older than %s, the rehearsed release: its routes default to legacy", version, pkg.MinVersion)
	default:
		state.Source = RouteModeSourceDefault
	}
	return state
}

// ResolveEffectivePackageRouteModes returns a package's effective route
// modes and where each comes from. It is what every reader that hands modes
// to a package host, or shows them, uses: the bridge's GetPackageConfig and
// the route-mode administration. In order:
//   - a stored mode wins, legacy included;
//   - identity group A follows the identity authority
//     (ResolvePackageRouteModes);
//   - a route of the default set (config/package-route-defaults.json) is
//     native when the policy is rehearsed, the installed release (version)
//     is at least the package's min_version and the extraction map still
//     lists the route as native-flagged for the package;
//   - anything else is legacy.
//
// The returned modes name every route that is not legacy plus the stored
// ones; sources name the stored, identity and default-set routes (any other
// route is RouteModeSourceUnset).
func ResolveEffectivePackageRouteModes(db *gorm.DB, packageID, version string, stored map[string]string) (map[string]string, map[string]string, error) {
	modes, err := ResolvePackageRouteModes(db, packageID, stored)
	if err != nil {
		return nil, nil, err
	}
	sources := make(map[string]string, len(modes))
	for route := range modes {
		if _, named := stored[route]; named {
			sources[route] = RouteModeSourceStored
		} else {
			sources[route] = RouteModeSourceIdentityAuthority
		}
	}
	defaults, err := LoadPackageRouteDefaults()
	if err != nil {
		return nil, nil, err
	}
	pkg, ok := defaults.Package(packageID)
	if !ok {
		return modes, sources, nil
	}
	catalog, err := PackageRouteCatalog()
	if err != nil {
		return nil, nil, err
	}
	state := packageRouteDefaultState(defaults, packageID, version)
	routes := append([]string(nil), pkg.Routes...)
	sort.Strings(routes)
	for _, route := range routes {
		if _, named := stored[route]; named {
			continue
		}
		if _, resolved := modes[route]; resolved {
			continue
		}
		if entry, listed := catalog[route]; !listed || entry.PackageID != packageID || entry.Mode != RouteCatalogNativeFlagged {
			continue
		}
		sources[route] = state.Source
		if state.Source == RouteModeSourceDefault {
			modes[route] = packagebridge.RouteModeNative
		}
	}
	return modes, sources, nil
}

// PackageRouteDefaultsSummary describes the process policy for the startup
// log: the policy, and how many routes of how many packages default to
// native under it (for releases at least their min_version).
func PackageRouteDefaultsSummary() (string, error) {
	defaults, err := LoadPackageRouteDefaults()
	if err != nil {
		return "", err
	}
	policy := PackageRouteDefaultPolicy()
	if policy == PackageRouteDefaultLegacy {
		return fmt.Sprintf("package route defaults: policy legacy (package_routes.default_mode): 0 routes default to native; every route without a stored mode runs its legacy handler (%d rehearsed routes of %d packages held back)",
			defaults.RouteCount(), len(defaults.Packages)), nil
	}
	return fmt.Sprintf("package route defaults: policy rehearsed (package_routes.default_mode): %d routes of %d packages default to native where no mode is stored and the installed package is at least its rehearsed release; set package_routes.default_mode=legacy to keep them legacy",
		defaults.RouteCount(), len(defaults.Packages)), nil
}
