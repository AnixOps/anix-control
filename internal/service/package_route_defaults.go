package service

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	configtables "github.com/AnixOps/anix-control/v5/config"
	"github.com/AnixOps/anix-control/v5/internal/packagebridge"
	"golang.org/x/mod/semver"
	"gorm.io/gorm"
)

// Route modes of the v2 Control packages. Nothing stores or switches them
// any more (single-binary design, section 4.3): a route runs natively when
// it is in the rehearsed list (config/package-route-defaults.json) and the
// installed release is at least the one the list names, identity group A
// follows the identity authority, and every other route runs its legacy
// handler. N6 (M5) makes every route native and deletes the legacy handlers.

// Package extraction modes (config/package-extraction.json).
const (
	RouteCatalogBridged       = "bridged"
	RouteCatalogKernelOwned   = "kernel-owned"
	RouteCatalogNativeFlagged = "native-flagged"
	RouteCatalogNative        = "native"
)

// RouteCatalogEntry is one route of config/package-extraction.json.
type RouteCatalogEntry struct {
	Method    string `json:"method"`
	Path      string `json:"path"`
	PackageID string `json:"package_id"`
	RouteID   string `json:"route_id"`
	Mode      string `json:"mode"`
	Reason    string `json:"reason,omitempty"`
}

var (
	routeCatalogOnce  sync.Once
	routeCatalogValue map[string]RouteCatalogEntry
	routeCatalogErr   error
)

// PackageRouteCatalog returns the embedded package extraction map by route
// id: which routes have a native implementation (native-flagged), which
// only the legacy one (bridged) and which stay in the kernel (kernel-owned).
func PackageRouteCatalog() (map[string]RouteCatalogEntry, error) {
	routeCatalogOnce.Do(func() {
		routeCatalogValue, routeCatalogErr = parsePackageRouteCatalog(configtables.PackageExtraction)
	})
	return routeCatalogValue, routeCatalogErr
}

func parsePackageRouteCatalog(raw []byte) (map[string]RouteCatalogEntry, error) {
	var document struct {
		Routes []RouteCatalogEntry `json:"routes"`
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, fmt.Errorf("parse config/package-extraction.json: %w", err)
	}
	catalog := make(map[string]RouteCatalogEntry, len(document.Routes))
	for _, route := range document.Routes {
		catalog[route.RouteID] = route
	}
	return catalog, nil
}

// PackageRouteDefaultPackage is one package of
// config/package-route-defaults.json.
type PackageRouteDefaultPackage struct {
	PackageID string `json:"package_id"`
	// Batch is the rehearsal batch that signed the package off.
	Batch int `json:"batch"`
	// MinVersion is the oldest release whose listed routes run natively:
	// the rehearsed one. Older releases (the signed 4.0 packages) stay
	// legacy.
	MinVersion string   `json:"min_version"`
	Routes     []string `json:"routes"`
}

// PackageRouteDefaults is config/package-route-defaults.json, the rehearsed
// list.
type PackageRouteDefaults struct {
	Format      string                       `json:"format"`
	DefaultMode string                       `json:"default_mode"`
	Packages    []PackageRouteDefaultPackage `json:"packages"`
	byPackage   map[string]PackageRouteDefaultPackage
	routes      map[string]string
}

// Package returns a package's entry of the list.
func (d *PackageRouteDefaults) Package(packageID string) (PackageRouteDefaultPackage, bool) {
	if d == nil {
		return PackageRouteDefaultPackage{}, false
	}
	pkg, ok := d.byPackage[packageID]
	return pkg, ok
}

// RouteCount is the number of routes in the list.
func (d *PackageRouteDefaults) RouteCount() int {
	if d == nil {
		return 0
	}
	return len(d.routes)
}

// Includes reports whether route of packageID is in the list.
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

// LoadPackageRouteDefaults returns the embedded rehearsed list.
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

// ResolveEffectivePackageRouteModes returns the routes of packageID at the
// installed release version that do not run their legacy handler, with
// their mode (native). It is what the package bridge hands the package
// host:
//   - identity group A follows the identity authority
//     (ResolvePackageRouteModes);
//   - a route of the rehearsed list is native when the installed release is
//     at least the package's min_version and the extraction map still lists
//     the route as native-flagged for the package;
//   - every other route is legacy and is not named.
func ResolveEffectivePackageRouteModes(db *gorm.DB, packageID, version string) (map[string]string, error) {
	modes, err := ResolvePackageRouteModes(db, packageID)
	if err != nil {
		return nil, err
	}
	defaults, err := LoadPackageRouteDefaults()
	if err != nil {
		return nil, err
	}
	pkg, ok := defaults.Package(packageID)
	if !ok || !PackageReleaseAtLeast(version, pkg.MinVersion) {
		return modes, nil
	}
	catalog, err := PackageRouteCatalog()
	if err != nil {
		return nil, err
	}
	routes := append([]string(nil), pkg.Routes...)
	sort.Strings(routes)
	for _, route := range routes {
		if _, resolved := modes[route]; resolved {
			continue
		}
		if entry, listed := catalog[route]; listed && entry.PackageID == packageID && entry.Mode == RouteCatalogNativeFlagged {
			modes[route] = packagebridge.RouteModeNative
		}
	}
	return modes, nil
}

// PackageRouteDefaultsSummary describes the route modes for the start log:
// how many routes of how many packages run natively.
func PackageRouteDefaultsSummary() (string, error) {
	defaults, err := LoadPackageRouteDefaults()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("package route modes: the %d rehearsed routes of %d packages (config/package-route-defaults.json) run natively once the installed package is at least its rehearsed release; every other route runs its legacy handler",
		defaults.RouteCount(), len(defaults.Packages)), nil
}
