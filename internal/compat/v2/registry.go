// Package v2 resolves legacy API routes only from verified, installed package
// artifacts. It deliberately has no dependency on the design-time catalog.
package v2

import (
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

// Resolve is a boolean convenience API for router registration and callers
// that do not need an error class. Gateways use ResolveContext to preserve
// fail-closed status mapping.
func (r *Registry) Resolve(method, normalizedPath string) (Route, bool) {
	route, err := r.ResolveContext(context.Background(), method, normalizedPath)
	return route, err == nil
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

// NewVerifiedRouteSource creates the production resolver. An invalid trust
// root is retained as a fail-closed resolver error rather than falling back
// to any source-derived catalog entry.
func NewVerifiedRouteSource(db *gorm.DB, encodedPublicKey string) RouteSource {
	publicKey, err := service.ParseOfficialPluginPublicKey(encodedPublicKey)
	return &verifiedRouteSource{db: db, publicKey: publicKey, initErr: err}
}

type verifiedRouteSource struct {
	db        *gorm.DB
	publicKey ed25519.PublicKey
	initErr   error
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

	var matches []Route
	declaredUnavailable := false
	for _, installation := range installations {
		route, declared, err := s.resolveInstallation(ctx, installation, method, requestPath)
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
	if len(matches) > 1 {
		return Route{}, fmt.Errorf("%w: multiple active packages declare %s %s", ErrPackageUnavailable, method, requestPath)
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if declaredUnavailable {
		return Route{}, ErrPackageUnavailable
	}
	return Route{}, ErrRouteNotDeclared
}

func (s *verifiedRouteSource) resolveInstallation(ctx context.Context, installation model.PluginInstallation, method, requestPath string) (Route, bool, error) {
	version := strings.TrimSpace(installation.ObservedVersion)
	if version == "" {
		version = strings.TrimSpace(installation.DesiredVersion)
	}
	if !safeToken(installation.PluginID) || !safeToken(version) {
		return Route{}, false, errors.New("installation identity is invalid")
	}
	var plugin model.Plugin
	if err := s.db.WithContext(ctx).First(&plugin, "id = ? AND official = ? AND publisher = ?", installation.PluginID, true, "AnixOps").Error; err != nil {
		return Route{}, false, err
	}
	var release model.PluginRelease
	if err := s.db.WithContext(ctx).First(&release, "plugin_id = ? AND version = ?", installation.PluginID, version).Error; err != nil {
		return Route{}, false, err
	}
	declaration, err := service.LoadVerifiedPluginCompatibilityRoutes(s.db.WithContext(ctx), release, s.publicKey)
	if err != nil {
		return Route{}, false, err
	}
	routes, err := decodeRouteDeclaration(declaration, installation.PluginID, version, installation.LifecycleGeneration)
	if err != nil {
		return Route{}, false, err
	}
	for _, route := range routes {
		if route.Method == method && routeMatches(route.LegacyPath, requestPath) {
			return route, true, nil
		}
	}
	return Route{}, false, nil
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
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
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
	if declaration.APIVersion != "v2" || declaration.PackageID != packageID || generation <= 0 {
		return nil, errors.New("compatibility route declaration does not match its package")
	}
	seen := make(map[string]struct{}, len(declaration.Routes))
	routes := make([]Route, 0, len(declaration.Routes))
	for _, declared := range declaration.Routes {
		route, err := normalizeRoute(Route{
			Method: declared.Method, LegacyPath: declared.LegacyPath, PackageID: packageID,
			Version: version, PackageRoute: declared.PackageRoute, Envelope: declared.Envelope,
			Generation: uint64(generation), Transport: declared.Transport,
		})
		if err != nil {
			return nil, err
		}
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
