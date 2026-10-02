// Package edition decides which features the configured product edition
// (app.edition) serves. The commercial packages and routes are listed once,
// in config/editions.json; the community edition answers their /api/v2
// routes exactly as it answers a route that is not declared, and its
// release leaves the packages out (packages/shared/build_package.py).
package edition

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	configtables "github.com/AnixOps/anix-control/v4/config"
	"github.com/AnixOps/anix-control/v4/internal/config"
)

const editionsFormat = "anixops.editions/v1"

// Table is config/editions.json.
type Table struct {
	Format             string   `json:"format"`
	Default            string   `json:"default"`
	CommercialPackages []string `json:"commercial_packages"`
	CommercialRoutes   []string `json:"commercial_routes"`
}

type extractionRoute struct {
	Method    string `json:"method"`
	Path      string `json:"path"`
	PackageID string `json:"package_id"`
	RouteID   string `json:"route_id"`
}

type extractionMap struct {
	Routes []extractionRoute `json:"routes"`
}

// Route is one /api/v2 route of config/package-extraction.json.
type Route struct {
	PackageID string
	RouteID   string
}

// Policy answers what one edition serves.
type Policy struct {
	edition        string
	hiddenPackages map[string]bool
	hiddenRoutes   map[string]bool
	routes         map[string]Route // "METHOD path" -> route
}

var (
	loadOnce   sync.Once
	loadedErr  error
	tableValue Table
	routeIndex map[string]Route
)

func load() {
	loadOnce.Do(func() {
		tableValue, routeIndex, loadedErr = parse(configtables.Editions, configtables.PackageExtraction)
	})
}

func parse(editions, extraction []byte) (Table, map[string]Route, error) {
	var table Table
	if err := json.Unmarshal(editions, &table); err != nil {
		return Table{}, nil, fmt.Errorf("parse config/editions.json: %w", err)
	}
	if table.Format != editionsFormat {
		return Table{}, nil, fmt.Errorf("config/editions.json format %q is not %s", table.Format, editionsFormat)
	}
	if _, err := config.NormalizeEdition(table.Default); err != nil || strings.TrimSpace(table.Default) == "" {
		return Table{}, nil, fmt.Errorf("config/editions.json default %q is not an edition", table.Default)
	}
	var routes extractionMap
	if err := json.Unmarshal(extraction, &routes); err != nil {
		return Table{}, nil, fmt.Errorf("parse config/package-extraction.json: %w", err)
	}
	index := make(map[string]Route, len(routes.Routes))
	for _, route := range routes.Routes {
		index[routeKey(route.Method, route.Path)] = Route{PackageID: route.PackageID, RouteID: route.RouteID}
	}
	return table, index, nil
}

func routeKey(method, path string) string {
	return strings.ToUpper(method) + " " + path
}

// Commercial returns config/editions.json. It panics when the embedded table
// is invalid, which TestEmbeddedTablesAreValid catches.
func Commercial() Table {
	load()
	if loadedErr != nil {
		panic(loadedErr)
	}
	return Table{
		Format:             tableValue.Format,
		Default:            tableValue.Default,
		CommercialPackages: append([]string(nil), tableValue.CommercialPackages...),
		CommercialRoutes:   append([]string(nil), tableValue.CommercialRoutes...),
	}
}

// For returns the policy of the configured edition.
func For(cfg *config.Config) *Policy {
	name := config.EditionCommunity
	if cfg != nil {
		name = cfg.App.EditionOrDefault()
	}
	return New(name)
}

// New returns the policy of an edition name (community when not valid).
func New(name string) *Policy {
	normalized, err := config.NormalizeEdition(name)
	if err != nil {
		normalized = config.EditionCommunity
	}
	table := Commercial()
	policy := &Policy{edition: normalized, hiddenPackages: map[string]bool{}, hiddenRoutes: map[string]bool{}, routes: routeIndex}
	if normalized == config.EditionCommercial {
		return policy
	}
	for _, id := range table.CommercialPackages {
		policy.hiddenPackages[id] = true
	}
	for _, id := range table.CommercialRoutes {
		policy.hiddenRoutes[id] = true
	}
	return policy
}

// Name is the edition name.
func (p *Policy) Name() string { return p.edition }

// Commercial reports whether this is the commercial edition.
func (p *Policy) Commercial() bool { return p.edition == config.EditionCommercial }

// Hides reports whether the edition hides a package route.
func (p *Policy) Hides(packageID, routeID string) bool {
	return p.hiddenPackages[packageID] || p.hiddenRoutes[routeID]
}

// HidesPackage reports whether the edition hides a whole package.
func (p *Policy) HidesPackage(packageID string) bool { return p.hiddenPackages[packageID] }

// HidesRequest reports whether the edition hides the /api/v2 route that a
// request matched: method and the gin route pattern (gin.Context.FullPath).
func (p *Policy) HidesRequest(method, fullPath string) bool {
	if len(p.hiddenPackages) == 0 && len(p.hiddenRoutes) == 0 {
		return false
	}
	route, ok := p.routes[routeKey(method, fullPath)]
	return ok && p.Hides(route.PackageID, route.RouteID)
}

// HiddenPackages lists the packages the edition hides, sorted.
func (p *Policy) HiddenPackages() []string {
	out := make([]string, 0, len(p.hiddenPackages))
	for id := range p.hiddenPackages {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// HiddenRoutes lists the /api/v2 routes ("METHOD path") the edition hides,
// sorted.
func (p *Policy) HiddenRoutes() []string {
	var out []string
	for key, route := range p.routes {
		if p.Hides(route.PackageID, route.RouteID) {
			out = append(out, key)
		}
	}
	sort.Strings(out)
	return out
}
