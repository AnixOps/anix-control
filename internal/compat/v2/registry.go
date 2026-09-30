// Package v2 resolves legacy API routes only from verified, installed package
// artifacts. It deliberately has no dependency on the design-time catalog.
package v2

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	pathpkg "path"
	"strings"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"gorm.io/gorm"
)

var (
	ErrRouteNotDeclared   = errors.New("v2 package route is not declared")
	ErrPackageUnavailable = errors.New("v2 package route is unavailable")
)

type Envelope string

const (
	EnvelopeData      Envelope = "data"
	EnvelopePanel     Envelope = "panel"
	EnvelopeRaw       Envelope = "raw"
	EnvelopeWebSocket Envelope = "websocket"
)

type Transport string

const (
	TransportHTTP      Transport = "http"
	TransportWebSocket Transport = "websocket"
)

// Route is a request-time route authorization bound to one verified package
// release and one running installation generation.
type Route struct {
	Method       string
	LegacyPath   string
	PackageID    string
	Version      string
	PackageRoute string
	Envelope     Envelope
	Generation   uint64
	Transport    Transport
}

func (r Route) Validate() error {
	_, err := normalizeRoute(r)
	return err
}

func normalizeRoute(route Route) (Route, error) {
	method, err := normalizeMethod(route.Method)
	if err != nil {
		return Route{}, err
	}
	legacyPath, err := normalizeLegacyPath(route.LegacyPath, true)
	if err != nil {
		return Route{}, err
	}
	if !safeToken(route.PackageID) || !safeToken(route.Version) {
		return Route{}, errors.New("package route identity is invalid")
	}
	if route.Generation == 0 {
		return Route{}, errors.New("package route generation is required")
	}
	if !safeRouteID(route.PackageRoute) {
		return Route{}, errors.New("package route id is invalid")
	}
	transport := route.Transport
	if transport == "" {
		transport = TransportHTTP
	}
	switch transport {
	case TransportHTTP:
		if route.Envelope != EnvelopeData && route.Envelope != EnvelopePanel && route.Envelope != EnvelopeRaw {
			return Route{}, errors.New("HTTP package route envelope is invalid")
		}
	case TransportWebSocket:
		if route.Envelope != EnvelopeWebSocket {
			return Route{}, errors.New("WebSocket package route envelope is invalid")
		}
	default:
		return Route{}, errors.New("package route transport is invalid")
	}
	route.Method = method
	route.LegacyPath = legacyPath
	route.Transport = transport
	return route, nil
}

// RouteSource performs the trusted, request-time route lookup.
type RouteSource interface {
	ResolveV2Route(context.Context, string, string) (Route, error)
}

type Registry struct {
	source RouteSource
}

func NewRegistry(source RouteSource) *Registry {
	return &Registry{source: source}
}

func (r *Registry) ResolveContext(ctx context.Context, method, requestPath string) (Route, error) {
	method, err := normalizeMethod(method)
	if err != nil {
		return Route{}, ErrRouteNotDeclared
	}
	requestPath, err = normalizeLegacyPath(requestPath, false)
	if err != nil {
		return Route{}, ErrRouteNotDeclared
	}
	if r == nil {
		return Route{}, ErrPackageUnavailable
	}
	var route Route
	if r.source != nil {
		route, err = r.source.ResolveV2Route(ctx, method, requestPath)
	} else {
		return Route{}, ErrPackageUnavailable
	}
	if err != nil {
		return Route{}, err
	}
	route, err = normalizeRoute(route)
	if err != nil {
		return Route{}, fmt.Errorf("%w: %v", ErrPackageUnavailable, err)
	}
	if route.Method != method || !routeMatches(route.LegacyPath, requestPath) {
		return Route{}, ErrRouteNotDeclared
	}
	return route, nil
}

// NewVerifiedRouteSource creates the production resolver with a private
// verified-route cache. An invalid trust root is retained as a fail-closed
// resolver error rather than falling back to any source-derived catalog entry.
func NewVerifiedRouteSource(db *gorm.DB, encodedPublicKey string) RouteSource {
	return NewVerifiedRouteSourceWithCache(db, encodedPublicKey, NewVerifiedRouteCache(0))
}

// NewVerifiedRouteSourceWithCache creates the production resolver backed by a
// caller-owned cache so the HTTP and WebSocket gateways built for one router
// share verification work. A nil cache disables caching: every resolution then
// re-verifies each installed release artifact.
func NewVerifiedRouteSourceWithCache(db *gorm.DB, encodedPublicKey string, cache *VerifiedRouteCache) RouteSource {
	publicKey, err := service.ParseOfficialPluginPublicKey(encodedPublicKey)
	source := &verifiedRouteSource{db: db, publicKey: publicKey, initErr: err, cache: cache}
	if err == nil {
		source.publicKeyFingerprint = service.PluginTrustRootFingerprint(publicKey)
	}
	return source
}

type verifiedRouteSource struct {
	db                   *gorm.DB
	publicKey            ed25519.PublicKey
	publicKeyFingerprint string
	initErr              error
	cache                *VerifiedRouteCache
}

// releaseIdentity is the immutable part of a release row that selects a
// verified route declaration. The manifest and signature are loaded only when
// the declaration is not already cached.
type releaseIdentity struct {
	ID                   uint
	PluginID             string
	Version              string
	ArtifactSHA256       string
	TrustRootFingerprint string
}

// resolutionSnapshot holds the per-request authorization state for every
// control installation. It is always read from the database; only the
// verified artifact-derived route declarations are cached.
type resolutionSnapshot struct {
	officialPlugins  map[string]struct{}
	releases         map[string]releaseIdentity
	activeTrustRoots map[string]struct{}
}

func (s *verifiedRouteSource) ResolveV2Route(ctx context.Context, method, requestPath string) (Route, error) {
	if s == nil || s.db == nil {
		return Route{}, ErrPackageUnavailable
	}
	if s.initErr != nil || len(s.publicKey) != ed25519.PublicKeySize {
		return Route{}, fmt.Errorf("%w: plugin trust root is not configured", ErrPackageUnavailable)
	}
	var installations []model.PluginInstallation
	if err := s.db.WithContext(ctx).Where("target = ?", "control").Order("plugin_id").Find(&installations).Error; err != nil {
		return Route{}, fmt.Errorf("%w: load package installations: %v", ErrPackageUnavailable, err)
	}
	if len(installations) == 0 {
		return Route{}, ErrRouteNotDeclared
	}
	snapshot, err := s.loadResolutionSnapshot(ctx, installations)
	if err != nil {
		return Route{}, fmt.Errorf("%w: load package releases: %v", ErrPackageUnavailable, err)
	}

	var matches []Route
	declaredUnavailable := false
	for _, installation := range installations {
		route, declared, err := s.resolveInstallation(ctx, snapshot, installation, method, requestPath)
		if err != nil {
			if installationActive(installation) {
				return Route{}, fmt.Errorf("%w: verify package %s: %v", ErrPackageUnavailable, installation.PluginID, err)
			}
			continue
		}
		if !declared {
			continue
		}
		if !installationActive(installation) {
			declaredUnavailable = true
			continue
		}
		matches = append(matches, route)
	}
	matches = preferMovedRouteOwners(matches)
	if len(matches) > 0 {
		best, ambiguous := mostSpecificRoute(matches)
		if ambiguous {
			return Route{}, fmt.Errorf("%w: multiple active packages declare %s %s", ErrPackageUnavailable, method, requestPath)
		}
		return best, nil
	}
	if declaredUnavailable {
		return Route{}, ErrPackageUnavailable
	}
	return Route{}, ErrRouteNotDeclared
}

// loadResolutionSnapshot reads, in three small indexed queries, the official
// plugin rows, the selected release identities, and the currently active
// trust roots for all control installations. Trust-root retirement is
// therefore observed by the very next resolution even when the route
// declaration itself is served from the cache.
func (s *verifiedRouteSource) loadResolutionSnapshot(ctx context.Context, installations []model.PluginInstallation) (resolutionSnapshot, error) {
	snapshot := resolutionSnapshot{
		officialPlugins:  make(map[string]struct{}, len(installations)),
		releases:         make(map[string]releaseIdentity, len(installations)),
		activeTrustRoots: make(map[string]struct{}, 1),
	}
	pluginIDs := make([]string, 0, len(installations))
	versions := make([]string, 0, len(installations))
	for _, installation := range installations {
		pluginIDs = append(pluginIDs, installation.PluginID)
		if version := installationVersion(installation); version != "" {
			versions = append(versions, version)
		}
	}
	db := s.db.WithContext(ctx)
	var officialPlugins []string
	if err := db.Model(&model.Plugin{}).
		Where("id IN ? AND official = ? AND publisher = ?", pluginIDs, true, "AnixOps").
		Pluck("id", &officialPlugins).Error; err != nil {
		return resolutionSnapshot{}, err
	}
	for _, id := range officialPlugins {
		snapshot.officialPlugins[id] = struct{}{}
	}
	if len(versions) == 0 {
		return snapshot, nil
	}
	var releases []releaseIdentity
	if err := db.Model(&model.PluginRelease{}).
		Select("id", "plugin_id", "version", "artifact_sha256", "trust_root_fingerprint").
		Where("plugin_id IN ? AND version IN ?", pluginIDs, versions).
		Find(&releases).Error; err != nil {
		return resolutionSnapshot{}, err
	}
	fingerprints := make([]string, 0, 1)
	seenFingerprints := make(map[string]struct{}, 1)
	for _, release := range releases {
		snapshot.releases[releaseLookupKey(release.PluginID, release.Version)] = release
		fingerprint := strings.TrimSpace(release.TrustRootFingerprint)
		if fingerprint == "" {
			continue
		}
		if _, seen := seenFingerprints[fingerprint]; !seen {
			seenFingerprints[fingerprint] = struct{}{}
			fingerprints = append(fingerprints, fingerprint)
		}
	}
	if len(fingerprints) == 0 {
		return snapshot, nil
	}
	var activeRoots []string
	if err := db.Model(&model.PluginTrustRoot{}).
		Where("fingerprint IN ? AND active = ?", fingerprints, true).
		Pluck("fingerprint", &activeRoots).Error; err != nil {
		return resolutionSnapshot{}, err
	}
	for _, fingerprint := range activeRoots {
		snapshot.activeTrustRoots[fingerprint] = struct{}{}
	}
	return snapshot, nil
}

func (s *verifiedRouteSource) resolveInstallation(ctx context.Context, snapshot resolutionSnapshot, installation model.PluginInstallation, method, requestPath string) (Route, bool, error) {
	version := installationVersion(installation)
	if !safeToken(installation.PluginID) || !safeToken(version) {
		return Route{}, false, errors.New("installation identity is invalid")
	}
	if installation.LifecycleGeneration <= 0 {
		return Route{}, false, errors.New("installation generation is invalid")
	}
	if _, official := snapshot.officialPlugins[installation.PluginID]; !official {
		return Route{}, false, gorm.ErrRecordNotFound
	}
	release, ok := snapshot.releases[releaseLookupKey(installation.PluginID, version)]
	if !ok {
		return Route{}, false, gorm.ErrRecordNotFound
	}
	if fingerprint := strings.TrimSpace(release.TrustRootFingerprint); fingerprint != "" {
		if _, active := snapshot.activeTrustRoots[fingerprint]; !active {
			return Route{}, false, service.ErrPluginTrustRootRequired
		}
	}
	routes, err := s.verifiedRoutes(ctx, release)
	if err != nil {
		return Route{}, false, err
	}
	// Selection runs per request over the shared cached templates; the
	// returned copy is bound to the current installation generation.
	route, ok := selectMostSpecificRoute(routes, method, requestPath)
	if ok {
		route.Generation = uint64(installation.LifecycleGeneration)
	}
	return route, ok, nil
}

// selectMostSpecificRoute returns the matching route with gin's precedence:
// a static segment beats a parameter segment, compared left to right. First
// match in declaration order would send GET /admin/nodes/stats to
// /admin/nodes/:id.
func selectMostSpecificRoute(routes []Route, method, requestPath string) (Route, bool) {
	var candidates []Route
	for _, route := range routes {
		if route.Method == method && routeMatches(route.LegacyPath, requestPath) {
			candidates = append(candidates, route)
		}
	}
	if len(candidates) == 0 {
		return Route{}, false
	}
	best, _ := mostSpecificRoute(candidates)
	return best, true
}

// mostSpecificRoute picks the most specific of routes that all match the same
// request path. ambiguous reports that another match is equally specific.
func mostSpecificRoute(routes []Route) (best Route, ambiguous bool) {
	best = routes[0]
	for _, route := range routes[1:] {
		switch {
		case moreSpecificPattern(route.LegacyPath, best.LegacyPath):
			best, ambiguous = route, false
		case !moreSpecificPattern(best.LegacyPath, route.LegacyPath):
			ambiguous = true
		}
	}
	return best, ambiguous
}

func moreSpecificPattern(a, b string) bool {
	aParts := strings.Split(strings.TrimPrefix(a, "/"), "/")
	bParts := strings.Split(strings.TrimPrefix(b, "/"), "/")
	for index := 0; index < len(aParts) && index < len(bParts); index++ {
		aParam := strings.HasPrefix(aParts[index], ":")
		bParam := strings.HasPrefix(bParts[index], ":")
		if aParam != bParam {
			return !aParam
		}
	}
	return false
}

// verifiedRoutes returns the route templates of one signed release. Templates
// carry the release version but no generation; the caller binds the current
// installation generation on every request.
func (s *verifiedRouteSource) verifiedRoutes(ctx context.Context, identity releaseIdentity) ([]Route, error) {
	key := verifiedRouteCacheKey{
		releaseID:            identity.ID,
		artifactSHA256:       strings.ToLower(identity.ArtifactSHA256),
		trustRootFingerprint: identity.TrustRootFingerprint,
		configuredKey:        s.publicKeyFingerprint,
	}
	return s.cache.load(ctx, key, func(ctx context.Context) ([]Route, error) {
		return s.verifyRelease(ctx, identity)
	})
}

func (s *verifiedRouteSource) verifyRelease(ctx context.Context, identity releaseIdentity) ([]Route, error) {
	var release model.PluginRelease
	if err := s.db.WithContext(ctx).First(&release, "id = ?", identity.ID).Error; err != nil {
		return nil, err
	}
	if release.PluginID != identity.PluginID || release.Version != identity.Version ||
		release.ArtifactSHA256 != identity.ArtifactSHA256 || release.TrustRootFingerprint != identity.TrustRootFingerprint {
		return nil, errors.New("package release changed during verification")
	}
	declaration, err := service.LoadVerifiedPluginCompatibilityRoutes(s.db.WithContext(ctx), release, s.publicKey)
	if err != nil {
		return nil, err
	}
	return decodeRouteTemplates(declaration, release.PluginID, release.Version)
}

func installationVersion(installation model.PluginInstallation) string {
	version := strings.TrimSpace(installation.ObservedVersion)
	if version == "" {
		version = strings.TrimSpace(installation.DesiredVersion)
	}
	return version
}

func releaseLookupKey(pluginID, version string) string {
	return pluginID + "\x00" + version
}

func installationActive(installation model.PluginInstallation) bool {
	return installation.Enabled &&
		(installation.State == "enabled" || installation.State == "healthy") &&
		installation.DesiredVersion != "" &&
		installation.ObservedVersion == installation.DesiredVersion &&
		installation.LifecycleGeneration > 0
}

type routeDeclaration struct {
	APIVersion string            `json:"api_version"`
	PackageID  string            `json:"package_id"`
	Routes     []declaredV2Route `json:"routes"`
}

type declaredV2Route struct {
	Method       string    `json:"method"`
	LegacyPath   string    `json:"legacy_path"`
	PackageRoute string    `json:"package_route"`
	Envelope     Envelope  `json:"envelope"`
	Transport    Transport `json:"transport"`
}

func decodeRouteDeclaration(raw []byte, packageID, version string, generation int64) ([]Route, error) {
	if generation <= 0 {
		return nil, errors.New("compatibility route declaration does not match its package")
	}
	routes, err := decodeRouteTemplates(raw, packageID, version)
	if err != nil {
		return nil, err
	}
	for index := range routes {
		routes[index].Generation = uint64(generation)
	}
	return routes, nil
}

// decodeRouteTemplates validates a signed declaration independent of the
// installation generation, which changes without a new release.
func decodeRouteTemplates(raw []byte, packageID, version string) ([]Route, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var declaration routeDeclaration
	if err := decoder.Decode(&declaration); err != nil {
		return nil, fmt.Errorf("decode compatibility routes: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err == nil {
		return nil, errors.New("decode compatibility routes: multiple JSON values")
	} else if !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("decode compatibility routes: %w", err)
	}
	if declaration.APIVersion != "v2" || declaration.PackageID != packageID {
		return nil, errors.New("compatibility route declaration does not match its package")
	}
	seen := make(map[string]struct{}, len(declaration.Routes))
	routes := make([]Route, 0, len(declaration.Routes))
	for _, declared := range declaration.Routes {
		// Validate with a placeholder generation; the caller binds the real one.
		route, err := normalizeRoute(Route{
			Method: declared.Method, LegacyPath: declared.LegacyPath, PackageID: packageID,
			Version: version, PackageRoute: declared.PackageRoute, Envelope: declared.Envelope,
			Generation: 1, Transport: declared.Transport,
		})
		if err != nil {
			return nil, err
		}
		route.Generation = 0
		key := route.Method + "\x00" + route.LegacyPath
		if _, exists := seen[key]; exists {
			return nil, errors.New("compatibility route declaration contains a duplicate method/path")
		}
		seen[key] = struct{}{}
		routes = append(routes, route)
	}
	return routes, nil
}

func normalizeMethod(method string) (string, error) {
	method = strings.ToUpper(strings.TrimSpace(method))
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions:
		return method, nil
	default:
		return "", errors.New("v2 route method is invalid")
	}
}

func normalizeLegacyPath(value string, declaration bool) (string, error) {
	if value == "" || value != strings.TrimSpace(value) || !strings.HasPrefix(value, "/api/v2/") ||
		strings.ContainsAny(value, "\\\\%?#\r\n\t ") || pathpkg.Clean(value) != value || strings.Contains(value, "//") {
		return "", errors.New("v2 route path is invalid")
	}
	for _, segment := range strings.Split(strings.TrimPrefix(value, "/"), "/") {
		if strings.HasPrefix(segment, ":") {
			if !declaration || !safeToken(strings.TrimPrefix(segment, ":")) {
				return "", errors.New("v2 route path parameter is invalid")
			}
			continue
		}
		if !safePathSegment(segment) {
			return "", errors.New("v2 route path is invalid")
		}
	}
	return value, nil
}

func routeMatches(pattern, requestPath string) bool {
	patternParts := strings.Split(strings.TrimPrefix(pattern, "/"), "/")
	requestParts := strings.Split(strings.TrimPrefix(requestPath, "/"), "/")
	if len(patternParts) != len(requestParts) {
		return false
	}
	for index, patternPart := range patternParts {
		if strings.HasPrefix(patternPart, ":") {
			if requestParts[index] == "" {
				return false
			}
			continue
		}
		if patternPart != requestParts[index] {
			return false
		}
	}
	return true
}

func safeToken(value string) bool {
	if value == "" || len(value) > 180 || value != strings.TrimSpace(value) || strings.Contains(value, "..") {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || strings.ContainsRune("._+-", character) {
			continue
		}
		return false
	}
	return true
}

func safeRouteID(value string) bool { return safeToken(value) }

func safePathSegment(value string) bool {
	if value == "" || len(value) > 180 {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || strings.ContainsRune("._+-", character) {
			continue
		}
		return false
	}
	return true
}
