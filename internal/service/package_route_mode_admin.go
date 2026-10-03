package service

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	configtables "github.com/AnixOps/anix-control/v4/config"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/AnixOps/anix-control/v4/internal/shadowsamples"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Errors of the route-mode administration.
var (
	// ErrRouteModeRejected: the change breaks a route-mode rule; the
	// message names the route and the rule.
	ErrRouteModeRejected = errors.New("route mode change rejected")
	// ErrRouteModeConfirmationRequired: a switch to native needs an
	// explicit confirmation and a reason.
	ErrRouteModeConfirmationRequired = errors.New("switching to native needs confirmation and a reason")
	// ErrRouteModePackageNotInstalled: the package has no Control
	// installation.
	ErrRouteModePackageNotInstalled = errors.New("the package has no Control installation")
)

// RouteModeCLIActor is the actor the command line records in the audit log
// and the revision history.
const RouteModeCLIActor = "system/cli"

// Package extraction modes (config/package-extraction.json).
const (
	RouteCatalogBridged       = "bridged"
	RouteCatalogKernelOwned   = "kernel-owned"
	RouteCatalogNativeFlagged = "native-flagged"
	RouteCatalogNative        = "native"
)

// Reasons a route is locked to its current mode in the administration.
const (
	RouteModeLockKernelOwned    = "kernel_owned"
	RouteModeLockIdentityGroupA = "identity_group_a"
	RouteModeLockWebSocket      = "websocket"
	RouteModeLockNotDeclared    = "not_declared"
	RouteModeLockNativeOnly     = "native_only"
)

const maxRouteModeReason = 1000

// errNoCompatibilityRoutes: the installed release is not a v2 package, so it
// serves no /api/v2 routes and has no route modes.
var errNoCompatibilityRoutes = errors.New("the installed release is not a v2 package and has no route modes")

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

// ShadowSampleRoute looks a route up in the package extraction map for the
// shadow sample collector: the package that owns it and its path template.
func ShadowSampleRoute(routeID string) (shadowsamples.RouteInfo, bool) {
	catalog, err := PackageRouteCatalog()
	if err != nil {
		return shadowsamples.RouteInfo{}, false
	}
	entry, ok := catalog[routeID]
	if !ok || entry.PackageID == "" {
		return shadowsamples.RouteInfo{}, false
	}
	return shadowsamples.RouteInfo{PackageID: entry.PackageID, Path: entry.Path}, true
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

// RouteModeActor is who changes route modes, as the audit log and the
// revision history record it. UserID 0 is the command line.
type RouteModeActor struct {
	UserID    uint
	Name      string
	IP        string
	UserAgent string
}

// RouteHostObservation is what a package host reports about one route in
// its health details: the mode it applies and its native and shadow
// counters since the host started.
type RouteHostObservation struct {
	Mode           string `json:"mode"`
	Effective      string `json:"effective"`
	NativeTotal    uint64 `json:"native_total"`
	NativeErrors   uint64 `json:"native_errors"`
	ShadowTotal    uint64 `json:"shadow_total"`
	ShadowMismatch uint64 `json:"shadow_mismatch"`
	ShadowErrors   uint64 `json:"shadow_errors"`
	ShadowSkipped  uint64 `json:"shadow_skipped"`
	// MismatchRate is ShadowMismatch / ShadowTotal (0 without shadow
	// runs).
	MismatchRate float64 `json:"mismatch_rate"`
	// LastMismatchAt is when the host last saw a mismatch.
	LastMismatchAt *time.Time `json:"last_mismatch_at,omitempty"`
}

// WithRate returns the observation with MismatchRate computed.
func (o RouteHostObservation) WithRate() RouteHostObservation {
	o.MismatchRate = 0
	if o.ShadowTotal > 0 {
		o.MismatchRate = float64(o.ShadowMismatch) / float64(o.ShadowTotal)
	}
	return o
}

// RouteModeEntry is one compatibility route of a package and its modes.
type RouteModeEntry struct {
	RouteID   string `json:"route_id"`
	Method    string `json:"method"`
	Path      string `json:"path"`
	Transport string `json:"transport"`
	// Catalog is the route's package extraction mode ("" when the
	// extraction map does not list it).
	Catalog string `json:"catalog"`
	// Configured is the stored mode or, when the map does not name the
	// route, the default policy's (native for a route of the rehearsed
	// default set, legacy otherwise); Effective is what the kernel hands
	// the host (ResolveEffectivePackageRouteModes), which differs only for
	// identity group A. Stored is the raw stored mode ("" when none).
	Configured string `json:"configured"`
	Effective  string `json:"effective"`
	Stored     string `json:"stored,omitempty"`
	// Source is where the effective mode comes from: stored, default,
	// kill-switch, package-too-old, identity-authority or unset.
	Source       string   `json:"source"`
	AllowedModes []string `json:"allowed_modes"`
	Locked       string   `json:"locked"`
	LockedReason string   `json:"locked_reason"`
	// Host is the running host's report, when there is one.
	Host *RouteHostObservation `json:"host,omitempty"`
	// MismatchSamples summarizes the stored shadow mismatch samples of the
	// route (GET /api/v4/kernel/route-modes/mismatches), when there are
	// any.
	MismatchSamples *shadowsamples.Summary `json:"mismatch_samples,omitempty"`
}

// PackageRouteModes is one package's routes and their modes.
type PackageRouteModes struct {
	PackageID      string `json:"package_id"`
	InstallationID uint   `json:"installation_id"`
	Version        string `json:"version"`
	Enabled        bool   `json:"enabled"`
	ConfigRevision int64  `json:"config_revision"`
	Error          string `json:"error,omitempty"`
	// Defaults is how the rehearsed default set applies to the package.
	Defaults PackageRouteDefaultState `json:"defaults"`
	Routes   []RouteModeEntry         `json:"routes"`
}

// RouteModeChangeRequest asks for routes of a package to switch to Mode.
// No Routes means every route of the package that may switch to Mode.
type RouteModeChangeRequest struct {
	PackageID string
	Routes    []string
	Mode      string
	Reason    string
	// Confirm acknowledges a switch to native.
	Confirm bool
}

// RouteModeChange is one route's switch.
type RouteModeChange struct {
	RouteID string `json:"route_id"`
	From    string `json:"from"`
	To      string `json:"to"`
}

// RouteModeSkip is a route a whole-package change left as it is.
type RouteModeSkip struct {
	RouteID string `json:"route_id"`
	Reason  string `json:"reason"`
}

// RouteModeResult describes a committed change. A request that changes
// nothing has no GroupID and writes neither revisions nor an audit entry.
type RouteModeResult struct {
	GroupID        string            `json:"group_id,omitempty"`
	PackageID      string            `json:"package_id"`
	Action         string            `json:"action"`
	Mode           string            `json:"mode"`
	ConfigRevision int64             `json:"config_revision"`
	Changes        []RouteModeChange `json:"changes"`
	Skipped        []RouteModeSkip   `json:"skipped"`
}

// RouteModeAdmin lists and switches the route modes of v2 Control packages.
// Switches go through SetPackageRouteModesTx, the configuration path the
// identity cutover uses, so every rule of the configuration check holds
// and the package hosts pick the change up at their next configuration
// poll. Each switch records its revisions and an audit entry in the same
// transaction.
type RouteModeAdmin struct {
	DB        *gorm.DB
	PublicKey ed25519.PublicKey
	// Catalog defaults to PackageRouteCatalog.
	Catalog map[string]RouteCatalogEntry
	// Now defaults to time.Now.
	Now func() time.Time
}

func (a *RouteModeAdmin) now() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return time.Now()
}

func (a *RouteModeAdmin) catalog() (map[string]RouteCatalogEntry, error) {
	if a.Catalog != nil {
		return a.Catalog, nil
	}
	return PackageRouteCatalog()
}

type compatibilityRoute struct {
	Method       string `json:"method"`
	LegacyPath   string `json:"legacy_path"`
	PackageRoute string `json:"package_route"`
	Transport    string `json:"transport"`
}

// packageRouteState is a package's compatibility routes and stored modes,
// read in one transaction.
type packageRouteState struct {
	installation  model.PluginInstallation
	configuration *model.PluginConfiguration
	routes        []compatibilityRoute
	stored        map[string]string
	effective     map[string]string
	sources       map[string]string
	authoritative bool
}

func (a *RouteModeAdmin) loadState(tx *gorm.DB, installation model.PluginInstallation) (*packageRouteState, error) {
	state := &packageRouteState{installation: installation}
	var release model.PluginRelease
	if err := tx.First(&release, "plugin_id = ? AND version = ?", installation.PluginID, installation.DesiredVersion).Error; err != nil {
		return state, fmt.Errorf("the desired release %s %s is not registered", installation.PluginID, installation.DesiredVersion)
	}
	if release.APIVersion != pluginManifestAPIVersionV2 {
		return state, errNoCompatibilityRoutes
	}
	raw, err := LoadVerifiedPluginCompatibilityRoutes(tx, release, a.PublicKey)
	if err != nil {
		return state, err
	}
	var declaration struct {
		Routes []compatibilityRoute `json:"routes"`
	}
	if err := json.Unmarshal(raw, &declaration); err != nil {
		return state, fmt.Errorf("decode compatibility routes: %w", err)
	}
	state.routes = declaration.Routes
	sort.Slice(state.routes, func(i, j int) bool { return state.routes[i].PackageRoute < state.routes[j].PackageRoute })
	configuration, err := GetPluginConfiguration(tx, installation.ID)
	if err != nil {
		return state, err
	}
	state.configuration = configuration
	_, stored, _, err := splitPackageRouteModes(configuration.ConfigJSON)
	if err != nil {
		return state, err
	}
	if stored == nil {
		stored = map[string]string{}
	}
	state.stored = stored
	if state.effective, state.sources, err = ResolveEffectivePackageRouteModes(tx, installation.PluginID, installation.DesiredVersion, stored); err != nil {
		return state, err
	}
	if installation.PluginID == IdentityPlatformPackageID {
		authority, err := IdentityAuthorityState(tx)
		if err != nil {
			return state, err
		}
		state.authoritative = IdentityAuthoritative(authority)
	}
	return state, nil
}

// configured is the route's stored mode or, without one, the default
// policy's: native for a defaulted route, legacy otherwise. Only identity
// group A's authority-driven mode is left out (see effectiveMode).
func (s *packageRouteState) configured(route string) string {
	if mode, ok := s.stored[route]; ok && mode != "" {
		return mode
	}
	if s.sources[route] == RouteModeSourceDefault {
		return packagebridge.RouteModeNative
	}
	return packagebridge.RouteModeLegacy
}

// effectiveMode is the mode the kernel hands the host for route.
func (s *packageRouteState) effectiveMode(route string) string {
	if mode := s.effective[route]; mode != "" {
		return mode
	}
	return packagebridge.RouteModeLegacy
}

// source is where route's effective mode comes from.
func (s *packageRouteState) source(route string) string {
	if source := s.sources[route]; source != "" {
		return source
	}
	return RouteModeSourceUnset
}

// routeModeRules returns the modes a route may switch to through the
// administration, and why it is locked when it may not switch at all.
func routeModeRules(packageID string, route compatibilityRoute, catalog map[string]RouteCatalogEntry, authoritative bool) (allowed []string, locked, reason string) {
	entry, listed := catalog[route.PackageRoute]
	switch {
	case packageID == IdentityPlatformPackageID && slices.Contains(IdentityGroupARoutes, route.PackageRoute):
		return []string{}, RouteModeLockIdentityGroupA, "identity group A switches together with the identity authority: use the identity cutover or rollback (POST /api/v4/kernel/identity/cutover, /rollback)"
	case listed && entry.Mode == RouteCatalogKernelOwned:
		return []string{}, RouteModeLockKernelOwned, "kernel-owned: the route stays in the kernel by design"
	case route.Transport == "websocket":
		return []string{packagebridge.RouteModeLegacy}, RouteModeLockWebSocket, "WebSocket routes stay in legacy mode"
	case !listed || entry.PackageID != packageID:
		return []string{packagebridge.RouteModeLegacy}, RouteModeLockNotDeclared, "the package extraction map does not list the route as native-flagged"
	case entry.Mode == RouteCatalogBridged:
		return []string{packagebridge.RouteModeLegacy}, "", "bridged: the package has no native implementation yet"
	case entry.Mode == RouteCatalogNative:
		return []string{packagebridge.RouteModeNative}, RouteModeLockNativeOnly, "native: the legacy handler is gone"
	case entry.Mode != RouteCatalogNativeFlagged:
		return []string{packagebridge.RouteModeLegacy}, RouteModeLockNotDeclared, "the package extraction map does not list the route as native-flagged"
	}
	if packageID == IdentityPlatformPackageID && !authoritative && slices.Contains(IdentityAccountReadRoutes, route.PackageRoute) {
		return []string{packagebridge.RouteModeLegacy}, "", "reads identity's accounts: leaves legacy mode only once identity is authoritative"
	}
	allowed = []string{packagebridge.RouteModeLegacy}
	if route.Method == http.MethodGet {
		allowed = append(allowed, packagebridge.RouteModeShadow)
	}
	return append(allowed, packagebridge.RouteModeNative), "", ""
}

// List returns the route modes of every Control installation, or of
// packageID's. A package whose compatibility routes cannot be verified is
// listed with Error. hosts, when not nil, adds each host's report by
// package and route.
func (a *RouteModeAdmin) List(ctx context.Context, packageID string, hosts map[string]map[string]RouteHostObservation) ([]PackageRouteModes, error) {
	if a.DB == nil {
		return nil, errors.New("database is not initialized")
	}
	catalog, err := a.catalog()
	if err != nil {
		return nil, err
	}
	db := a.DB.WithContext(ctx)
	query := db.Where("target = ?", "control").Order("plugin_id")
	if packageID != "" {
		query = query.Where("plugin_id = ?", packageID)
	}
	var installations []model.PluginInstallation
	if err := query.Find(&installations).Error; err != nil {
		return nil, err
	}
	if packageID != "" && len(installations) == 0 {
		return nil, ErrRouteModePackageNotInstalled
	}
	summaries, err := shadowsamples.Summaries(db, packageID)
	if err != nil {
		return nil, err
	}
	packages := make([]PackageRouteModes, 0, len(installations))
	for _, installation := range installations {
		entry := PackageRouteModes{
			PackageID: installation.PluginID, InstallationID: installation.ID, Version: installation.DesiredVersion,
			Enabled: installation.Enabled, ConfigRevision: installation.ConfigRevision, Routes: []RouteModeEntry{},
		}
		if defaults, err := LoadPackageRouteDefaults(); err == nil {
			entry.Defaults = packageRouteDefaultState(defaults, installation.PluginID, installation.DesiredVersion)
		} else {
			entry.Defaults = PackageRouteDefaultState{Policy: PackageRouteDefaultPolicy(), Note: err.Error()}
		}
		state, err := a.loadState(db, installation)
		if errors.Is(err, errNoCompatibilityRoutes) && packageID == "" {
			continue
		}
		if err != nil {
			entry.Error = err.Error()
			packages = append(packages, entry)
			continue
		}
		entry.ConfigRevision = state.configuration.Revision
		for _, route := range state.routes {
			allowed, locked, reason := routeModeRules(installation.PluginID, route, catalog, state.authoritative)
			row := RouteModeEntry{
				RouteID: route.PackageRoute, Method: route.Method, Path: route.LegacyPath, Transport: route.Transport,
				Catalog: catalog[route.PackageRoute].Mode, Configured: state.configured(route.PackageRoute),
				Effective: state.effectiveMode(route.PackageRoute), Stored: state.stored[route.PackageRoute],
				Source: state.source(route.PackageRoute), AllowedModes: allowed, Locked: locked, LockedReason: reason,
			}
			if observation, ok := hosts[installation.PluginID][route.PackageRoute]; ok {
				observation := observation.WithRate()
				row.Host = &observation
			}
			if summary, ok := summaries[installation.PluginID][route.PackageRoute]; ok {
				summary := summary
				row.MismatchSamples = &summary
			}
			entry.Routes = append(entry.Routes, row)
		}
		packages = append(packages, entry)
	}
	return packages, nil
}

func validRouteMode(mode string) bool {
	return mode == packagebridge.RouteModeLegacy || mode == packagebridge.RouteModeShadow || mode == packagebridge.RouteModeNative
}

// Set switches routes of a package to request.Mode. Named routes must all
// be allowed to switch, or nothing changes; without names every route that
// may switch to the mode does, and the others are reported as skipped.
// Switching to native needs request.Confirm and a reason.
func (a *RouteModeAdmin) Set(ctx context.Context, request RouteModeChangeRequest, actor RouteModeActor) (RouteModeResult, error) {
	request.Reason = strings.TrimSpace(request.Reason)
	if !validRouteMode(request.Mode) {
		return RouteModeResult{}, fmt.Errorf("%w: mode %q must be legacy, shadow or native", ErrRouteModeRejected, request.Mode)
	}
	if request.Mode == packagebridge.RouteModeNative && (!request.Confirm || request.Reason == "") {
		return RouteModeResult{}, ErrRouteModeConfirmationRequired
	}
	return a.apply(ctx, request, actor, model.RouteModeRevisionActionSet)
}

// Rollback returns every route of a package to legacy in one change, except
// identity group A, which only the identity rollback switches. It works on
// the effective modes: a route that runs natively only by default (no
// stored mode) gets an explicit legacy, so it stays legacy whatever the
// default policy becomes. It needs no confirmation; the reason is optional.
func (a *RouteModeAdmin) Rollback(ctx context.Context, packageID, reason string, actor RouteModeActor) (RouteModeResult, error) {
	request := RouteModeChangeRequest{PackageID: packageID, Mode: packagebridge.RouteModeLegacy, Reason: strings.TrimSpace(reason)}
	return a.apply(ctx, request, actor, model.RouteModeRevisionActionRollback)
}

// plan decides the switches of request against state.
func plan(state *packageRouteState, catalog map[string]RouteCatalogEntry, request RouteModeChangeRequest, action string) (map[string]string, []RouteModeChange, []RouteModeSkip, error) {
	packageID := state.installation.PluginID
	modes := map[string]string{}
	var changes []RouteModeChange
	skipped := []RouteModeSkip{}
	// Changes compare against the effective mode: a defaulted native route
	// switched to legacy is a change (stored as an explicit legacy), one
	// already running natively by default switched to native is not.
	add := func(route, to string) {
		from := state.effectiveMode(route)
		if from == to {
			return
		}
		modes[route] = to
		changes = append(changes, RouteModeChange{RouteID: route, From: from, To: to})
	}
	byID := make(map[string]compatibilityRoute, len(state.routes))
	for _, route := range state.routes {
		byID[route.PackageRoute] = route
	}
	switch {
	case action == model.RouteModeRevisionActionRollback:
		// Every stored mode and every route running in shadow or native
		// mode (defaulted ones included) returns to legacy, routes the
		// release no longer declares included, except group A.
		named := make(map[string]bool, len(state.stored)+len(state.effective))
		for route := range state.stored {
			named[route] = true
		}
		for route, mode := range state.effective {
			if mode == packagebridge.RouteModeShadow || mode == packagebridge.RouteModeNative {
				named[route] = true
			}
		}
		routes := make([]string, 0, len(named))
		for route := range named {
			routes = append(routes, route)
		}
		sort.Strings(routes)
		for _, route := range routes {
			if packageID == IdentityPlatformPackageID && slices.Contains(IdentityGroupARoutes, route) {
				if state.effectiveMode(route) != packagebridge.RouteModeLegacy {
					skipped = append(skipped, RouteModeSkip{RouteID: route, Reason: "identity group A: use the identity rollback"})
				}
				continue
			}
			if declared, ok := byID[route]; ok {
				if entry, listed := catalog[declared.PackageRoute]; listed && entry.Mode == RouteCatalogNative {
					skipped = append(skipped, RouteModeSkip{RouteID: route, Reason: "native: the legacy handler is gone"})
					continue
				}
			}
			add(route, packagebridge.RouteModeLegacy)
		}
	case len(request.Routes) == 0:
		for _, route := range state.routes {
			allowed, _, reason := routeModeRules(packageID, route, catalog, state.authoritative)
			if slices.Contains(allowed, request.Mode) {
				add(route.PackageRoute, request.Mode)
				continue
			}
			if state.effectiveMode(route.PackageRoute) != request.Mode {
				if reason == "" {
					reason = fmt.Sprintf("%s mode is not allowed for %s %s", request.Mode, route.Method, route.PackageRoute)
				}
				skipped = append(skipped, RouteModeSkip{RouteID: route.PackageRoute, Reason: reason})
			}
		}
	default:
		seen := map[string]bool{}
		for _, id := range request.Routes {
			id = strings.TrimSpace(id)
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			route, ok := byID[id]
			if !ok {
				return nil, nil, nil, fmt.Errorf("%w: %q is not a route of %s", ErrRouteModeRejected, id, packageID)
			}
			allowed, locked, reason := routeModeRules(packageID, route, catalog, state.authoritative)
			if !slices.Contains(allowed, request.Mode) {
				switch {
				case locked == RouteModeLockIdentityGroupA:
					return nil, nil, nil, fmt.Errorf("%w: %s: %s", ErrRouteModeRejected, id, reason)
				case reason != "":
					return nil, nil, nil, fmt.Errorf("%w: %s cannot switch to %s: %s", ErrRouteModeRejected, id, request.Mode, reason)
				default:
					return nil, nil, nil, fmt.Errorf("%w: %s cannot switch to %s (only GET routes run in shadow mode)", ErrRouteModeRejected, id, request.Mode)
				}
			}
			add(id, request.Mode)
		}
		if len(seen) == 0 {
			return nil, nil, nil, fmt.Errorf("%w: no route named", ErrRouteModeRejected)
		}
	}
	return modes, changes, skipped, nil
}

func (a *RouteModeAdmin) apply(ctx context.Context, request RouteModeChangeRequest, actor RouteModeActor, action string) (RouteModeResult, error) {
	result := RouteModeResult{PackageID: request.PackageID, Action: action, Mode: request.Mode, Changes: []RouteModeChange{}, Skipped: []RouteModeSkip{}}
	if a.DB == nil {
		return result, errors.New("database is not initialized")
	}
	if len(a.PublicKey) != ed25519.PublicKeySize {
		return result, ErrPluginTrustRootRequired
	}
	if len(request.Reason) > maxRouteModeReason {
		return result, fmt.Errorf("%w: the reason is longer than %d characters", ErrRouteModeRejected, maxRouteModeReason)
	}
	catalog, err := a.catalog()
	if err != nil {
		return result, err
	}
	err = WithAgentLifecycleTransaction(a.DB.WithContext(ctx), func(tx *gorm.DB) error {
		result.GroupID, result.ConfigRevision = "", 0
		result.Changes, result.Skipped = []RouteModeChange{}, []RouteModeSkip{}
		var installation model.PluginInstallation
		if err := tx.Where("plugin_id = ? AND target = ?", request.PackageID, "control").Take(&installation).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrRouteModePackageNotInstalled
			}
			return err
		}
		state, err := a.loadState(tx, installation)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrRouteModeRejected, err)
		}
		result.ConfigRevision = state.configuration.Revision
		modes, changes, skipped, err := plan(state, catalog, request, action)
		if err != nil {
			return err
		}
		result.Skipped = skipped
		if len(changes) == 0 {
			return nil
		}
		// A route of the default set keeps an explicit legacy, or the
		// default would take it back; a route the release does not declare
		// cannot be stored and is removed.
		defaults, err := LoadPackageRouteDefaults()
		if err != nil {
			return err
		}
		declared := make(map[string]bool, len(state.routes))
		for _, route := range state.routes {
			declared[route.PackageRoute] = true
		}
		pin := func(route string) bool { return declared[route] && defaults.Includes(installation.PluginID, route) }
		configuration, err := setPackageRouteModesTx(tx, a.PublicKey, installation.ID, modes, actor.UserID, pin)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrRouteModeRejected, err)
		}
		result.Changes, result.ConfigRevision, result.GroupID = changes, configuration.Revision, uuid.NewString()
		now := a.now().UTC()
		revisions := make([]model.RouteModeRevision, 0, len(changes))
		for _, change := range changes {
			revisions = append(revisions, model.RouteModeRevision{
				GroupID: result.GroupID, PackageID: installation.PluginID, RouteID: change.RouteID, Action: action,
				FromMode: change.From, ToMode: change.To, ActorUserID: actor.UserID, Actor: actor.Name,
				Reason: request.Reason, ConfigRevision: configuration.Revision, CreatedAt: now,
			})
		}
		if err := tx.Create(&revisions).Error; err != nil {
			return err
		}
		content, err := json.Marshal(map[string]any{
			"package_id": installation.PluginID, "group_id": result.GroupID, "mode": request.Mode,
			"changes": changes, "reason": request.Reason, "config_revision": configuration.Revision,
		})
		if err != nil {
			return err
		}
		var userID *uint
		if actor.UserID != 0 {
			id := actor.UserID
			userID = &id
		}
		targetID := installation.ID
		return NewOperationLogService(tx).Record(&OperationLogInput{
			UserID: userID, Username: actor.Name, Action: "route_mode_" + action, Module: "kernel",
			TargetType: "plugin_installation", TargetID: &targetID, Content: string(content),
			IP: actor.IP, UserAgent: actor.UserAgent, Status: 1,
		})
	})
	return result, err
}

// Revisions returns the latest route-mode revisions, newest first, of
// packageID or of every package.
func (a *RouteModeAdmin) Revisions(ctx context.Context, packageID string, limit int) ([]model.RouteModeRevision, error) {
	if a.DB == nil {
		return nil, errors.New("database is not initialized")
	}
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	query := a.DB.WithContext(ctx).Order("id DESC").Limit(limit)
	if packageID != "" {
		query = query.Where("package_id = ?", packageID)
	}
	revisions := []model.RouteModeRevision{}
	if err := query.Find(&revisions).Error; err != nil {
		return nil, err
	}
	return revisions, nil
}

// IsSuperAdmin reports whether userID may switch route modes. The kernel
// has no separate super-administrator role, so this is its strictest
// administrator check, read from the database rather than the token: an
// administrator (is_admin) who is not staff and not banned. A demoted or
// banned administrator loses the right at once, not when the token expires.
func IsSuperAdmin(db *gorm.DB, userID uint) (bool, error) {
	if db == nil || userID == 0 {
		return false, nil
	}
	var users []model.User
	if err := db.Select("id", "is_admin", "is_staff", "banned").Where("id = ?", userID).Limit(1).Find(&users).Error; err != nil {
		return false, err
	}
	if len(users) == 0 {
		return false, nil
	}
	user := users[0]
	return user.IsAdmin == 1 && user.IsStaff == 0 && user.Banned == 0, nil
}

// RouteModeActorName is the audit name of a signed-in administrator: the
// e-mail, or "user:<id>".
func RouteModeActorName(db *gorm.DB, userID uint) string {
	if name := AuditUsername(db, &userID); name != "" {
		return name
	}
	return "user:" + strconv.FormatUint(uint64(userID), 10)
}
