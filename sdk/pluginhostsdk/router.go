package pluginhostsdk

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AnixOps/anix-control/sdk/packagebridgesdk"
	"github.com/AnixOps/anix-control/sdk/shadowsample"
	"github.com/AnixOps/anix-control/sdk/v2compat"
)

// Route modes, as configured in the installation's "routes" document.
const (
	RouteModeLegacy = "legacy"
	RouteModeShadow = "shadow"
	RouteModeNative = "native"
)

const (
	defaultConfigPollInterval = 5 * time.Second
	defaultShadowTimeout      = 5 * time.Second
	defaultShadowConcurrency  = 4
	// maxShadowSamples bounds the shadow mismatch samples a router keeps
	// for its Health details, and shadowSampleMaxAge how long it reports
	// one: long enough for several kernel health polls to collect it.
	maxShadowSamples   = 32
	shadowSampleMaxAge = 10 * time.Minute
)

// IndexMigrationPrefix starts the id of a migration run that applies a
// package's whole migration index.
const IndexMigrationPrefix = "index."

// Principal is the kernel-authenticated caller of a v2 route.
type Principal struct {
	ActorID   uint   `json:"actor_id"`
	Admin     bool   `json:"admin"`
	PackageID string `json:"package_id"`
}

// NativeRequest is what a package-native route implementation receives.
type NativeRequest struct {
	RouteID string
	Method  string
	// Body is the request body. For the routes the kernel lists in
	// config/node-secret-fields.json, every node secret in it is a sealed
	// handle (v2compat.SealedHandlePrefix) that only the kernel resolves.
	Body      []byte
	Principal Principal
	Metadata  RequestMetadata
	// Binding is the request binding a KernelNodeOps call passes as
	// RequestBinding.bridge_capability: the kernel resolves the request's
	// sealed handles, and mints the handles its answer shows, only for it.
	// It is nil in a shadow run, which must not act, and whose handles are
	// never resolved.
	Binding []byte
}

// NativeResponse is a native route's HTTP-level answer; the kernel gateway
// applies the route's declared envelope rules to it, as for legacy answers.
type NativeResponse struct {
	StatusCode uint32
	Body       []byte
	Headers    []Header
}

// NativeHandler implements one v2 route inside the package host.
type NativeHandler func(context.Context, NativeRequest) (NativeResponse, error)

// PanelJSON is a NativeResponse carrying a panel envelope, byte-identical to
// the kernel's legacy panel responses.
func PanelJSON(panel v2compat.Panel) (NativeResponse, error) {
	body, err := json.Marshal(panel)
	if err != nil {
		return NativeResponse{}, err
	}
	return NativeResponse{StatusCode: http.StatusOK, Body: body, Headers: []Header{{Name: "Content-Type", Value: "application/json; charset=utf-8"}}}, nil
}

// ErrNativeRoutePanicked reports a recovered panic in a native route. A panic
// in a background shadow run would otherwise terminate the host process.
var ErrNativeRoutePanicked = errors.New("native route panicked")

// ErrNativeUnavailable is what a native handler returns when it cannot
// answer this request as the legacy handler would, for example because the
// kernel did not send request metadata the handler needs (an older kernel
// sends no request scheme and host). The router then answers from the
// legacy handler in native mode, and skips the comparison in shadow mode.
var ErrNativeUnavailable = errors.New("native route cannot serve this request")

// RouterBridge is the part of the package bridge client the router uses.
type RouterBridge interface {
	Invoke(ctx context.Context, capability []byte, operation string, payload []byte) (packagebridgesdk.Response, error)
	GetPackageConfig(ctx context.Context) (packagebridgesdk.PackageConfig, error)
}

// RouterWebSocketBridge is implemented by bridges that relay WebSocket routes.
type RouterWebSocketBridge interface {
	OpenWebSocket(ctx context.Context, capability []byte, operation string) (packagebridgesdk.WebSocketStream, error)
}

// RouterConfig configures a Router.
type RouterConfig struct {
	PackageID string
	LeaseID   string
	Bridge    RouterBridge
	// Native maps route ids to package-native implementations. Routes without
	// one always run in legacy mode, whatever the configuration says.
	Native map[string]NativeHandler
	// AllowRoute, when set, rejects dispatch for route ids it returns false for.
	AllowRoute func(routeID string) bool
	// MigrationOperation maps a migration id to its bridge operation; nil uses
	// "migration.<package>.<id>". It returns false for unsupported migrations.
	MigrationOperation func(migrationID string) (string, bool)
	// IndexMigration runs the package's own migration index when the kernel
	// sends a migration id with IndexMigrationPrefix, which it does only for
	// packages that declare kernel.storage.v1. It needs no bridge capability;
	// packagestoresdk.IndexMigrator implements it.
	IndexMigration func(ctx context.Context, request MigrationRequest) (MigrationResponse, error)
	// Compare decides whether a shadow run matched; nil compares the status
	// code and v2compat-normalized bodies.
	Compare func(legacy, native NativeResponse) bool

	PollInterval      time.Duration
	ShadowTimeout     time.Duration
	ShadowConcurrency int
	Logf              func(format string, args ...any)
	Now               func() time.Time
}

// Router is a Package that applies per-route modes. Legacy routes pass through
// the package bridge to the kernel's legacy handler; native routes are
// answered by the package; shadow routes (GET only) answer from legacy and
// compare a background native run.
type Router struct {
	config   RouterConfig
	draining atomic.Bool
	shadow   chan struct{}

	mu            sync.RWMutex
	modes         map[string]string
	configStatus  string
	configRev     int64
	configHash    string
	configChecked time.Time
	routeStats    map[string]*routeStatistics
	// samples are the latest sanitized shadow mismatches, oldest first.
	samples []shadowsample.Sample
}

type routeStatistics struct {
	Native         uint64 `json:"native_total"`
	NativeErrors   uint64 `json:"native_errors"`
	Shadow         uint64 `json:"shadow_total"`
	ShadowMismatch uint64 `json:"shadow_mismatch"`
	ShadowErrors   uint64 `json:"shadow_errors"`
	ShadowSkipped  uint64 `json:"shadow_skipped"`
	LastMismatch   int64  `json:"last_mismatch_unix,omitempty"`
}

var (
	_ Package          = (*Router)(nil)
	_ WebSocketPackage = (*Router)(nil)
)

// NewRouter validates the configuration. Call Run to start polling the
// installation configuration; until the first successful poll every route is
// in legacy mode.
func NewRouter(config RouterConfig) (*Router, error) {
	if strings.TrimSpace(config.PackageID) == "" || config.Bridge == nil {
		return nil, errors.New("router needs a package id and a bridge")
	}
	if config.PollInterval <= 0 {
		config.PollInterval = defaultConfigPollInterval
	}
	if config.ShadowTimeout <= 0 {
		config.ShadowTimeout = defaultShadowTimeout
	}
	if config.ShadowConcurrency <= 0 {
		config.ShadowConcurrency = defaultShadowConcurrency
	}
	if config.Compare == nil {
		config.Compare = defaultShadowCompare
	}
	if config.Logf == nil {
		config.Logf = func(string, ...any) {}
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	return &Router{
		config: config, shadow: make(chan struct{}, config.ShadowConcurrency),
		modes: map[string]string{}, configStatus: "pending", routeStats: map[string]*routeStatistics{},
	}, nil
}

// Run polls the installation configuration until ctx ends.
func (r *Router) Run(ctx context.Context) {
	r.Refresh(ctx)
	ticker := time.NewTicker(r.config.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.Refresh(ctx)
		}
	}
}

// Refresh reads the configuration once. A failed read keeps the last
// successful modes; a kernel without session operations means all-legacy.
func (r *Router) Refresh(ctx context.Context) {
	pollCtx, cancel := context.WithTimeout(ctx, r.config.PollInterval)
	defer cancel()
	config, err := r.config.Bridge.GetPackageConfig(pollCtx)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.configChecked = r.config.Now()
	switch {
	case err == nil:
		r.modes = config.RouteModes
		if r.modes == nil {
			r.modes = map[string]string{}
		}
		r.configRev, r.configHash, r.configStatus = config.Revision, config.ConfigHash, "ok"
	case errors.Is(err, packagebridgesdk.ErrSessionOperationUnsupported):
		r.modes, r.configStatus = map[string]string{}, "unsupported"
	case errors.Is(err, packagebridgesdk.ErrHostFenced):
		r.configStatus = "fenced"
	default:
		if r.configStatus != "error" {
			r.config.Logf("package %s: reading route modes failed, keeping the last configuration: %v", r.config.PackageID, err)
		}
		r.configStatus = "error"
	}
}

// Mode returns the configured and the effective mode of a route. The effective
// mode is legacy whenever the package has no native implementation.
func (r *Router) Mode(routeID string) (configured, effective string) {
	r.mu.RLock()
	configured = r.modes[routeID]
	r.mu.RUnlock()
	if configured == "" {
		configured = RouteModeLegacy
	}
	if configured != RouteModeLegacy && r.config.Native[routeID] == nil {
		return configured, RouteModeLegacy
	}
	return configured, configured
}

func (r *Router) Dispatch(ctx context.Context, request DispatchRequest) (DispatchResponse, error) {
	if r == nil || r.draining.Load() {
		return DispatchResponse{}, errors.New("package is unavailable")
	}
	if strings.TrimSpace(request.RouteID) == "" {
		return DispatchResponse{}, errors.New("package route is required")
	}
	if r.config.AllowRoute != nil && !r.config.AllowRoute(request.RouteID) {
		return DispatchResponse{}, fmt.Errorf("package route %q is unsupported", request.RouteID)
	}
	_, mode := r.Mode(request.RouteID)
	switch mode {
	case RouteModeNative:
		response, err := r.runNative(ctx, request, request.BridgeCapability)
		if errors.Is(err, ErrNativeUnavailable) {
			return r.invokeLegacy(ctx, request)
		}
		r.count(request.RouteID, func(stats *routeStatistics) {
			stats.Native++
			if err != nil {
				stats.NativeErrors++
			}
		})
		if err != nil {
			return DispatchResponse{}, err
		}
		return DispatchResponse{StatusCode: response.StatusCode, ResponseBody: response.Body, Headers: response.Headers}, nil
	case RouteModeShadow:
		legacy, err := r.invokeLegacy(ctx, request)
		if err != nil {
			return DispatchResponse{}, err
		}
		if request.Method == http.MethodGet {
			r.startShadow(request, legacy)
		}
		return legacy, nil
	default:
		return r.invokeLegacy(ctx, request)
	}
}

func (r *Router) invokeLegacy(ctx context.Context, request DispatchRequest) (DispatchResponse, error) {
	if len(request.BridgeCapability) != 32 {
		return DispatchResponse{}, errors.New("package bridge capability is required")
	}
	response, err := r.config.Bridge.Invoke(ctx, request.BridgeCapability, request.RouteID, request.RequestBody)
	if err != nil {
		return DispatchResponse{}, err
	}
	headers := make([]Header, len(response.Headers))
	for index, header := range response.Headers {
		headers[index] = Header{Name: header.Name, Value: header.Value}
	}
	return DispatchResponse{StatusCode: response.StatusCode, ResponseBody: response.Body, Headers: headers}, nil
}

// runNative runs the route's native implementation. binding is the request
// binding the handler may pass to the kernel: the bridge capability in
// native mode, nil in a shadow run.
func (r *Router) runNative(ctx context.Context, request DispatchRequest, binding []byte) (response NativeResponse, err error) {
	handler := r.config.Native[request.RouteID]
	if handler == nil {
		return NativeResponse{}, fmt.Errorf("package route %q has no native implementation", request.RouteID)
	}
	var principal Principal
	if len(request.PrincipalJSON) > 0 {
		if err := json.Unmarshal(request.PrincipalJSON, &principal); err != nil {
			return NativeResponse{}, errors.New("package request principal is invalid")
		}
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%w: route %s: %v", ErrNativeRoutePanicked, request.RouteID, recovered)
		}
	}()
	response, err = handler(ctx, NativeRequest{
		RouteID: request.RouteID, Method: request.Method, Body: request.RequestBody, Principal: principal, Metadata: request.Metadata,
		Binding: append([]byte(nil), binding...),
	})
	if err == nil && response.StatusCode == 0 {
		response.StatusCode = http.StatusOK
	}
	return response, err
}

// startShadow runs the native implementation in the background without
// delaying the legacy answer. When all shadow slots are busy the run is
// skipped and counted.
func (r *Router) startShadow(request DispatchRequest, legacy DispatchResponse) {
	select {
	case r.shadow <- struct{}{}:
	default:
		r.count(request.RouteID, func(stats *routeStatistics) { stats.ShadowSkipped++ })
		return
	}
	request.RequestBody = append([]byte(nil), request.RequestBody...)
	// The shadow run gets no request binding: the legacy answer stands, so
	// the kernel must not act or resolve a sealed handle for it.
	request.BridgeCapability = nil
	go func() {
		defer func() { <-r.shadow }()
		ctx, cancel := context.WithTimeout(context.Background(), r.config.ShadowTimeout)
		defer cancel()
		native, err := r.runNative(ctx, request, nil)
		if errors.Is(err, ErrNativeUnavailable) {
			r.count(request.RouteID, func(stats *routeStatistics) { stats.ShadowSkipped++ })
			return
		}
		matched := err == nil && r.config.Compare(NativeResponse{StatusCode: legacy.StatusCode, Body: legacy.ResponseBody, Headers: legacy.Headers}, native)
		now := r.config.Now().Unix()
		r.count(request.RouteID, func(stats *routeStatistics) {
			stats.Shadow++
			switch {
			case err != nil:
				stats.ShadowErrors++
			case !matched:
				stats.ShadowMismatch++
				stats.LastMismatch = now
			}
		})
		if err == nil && !matched {
			r.config.Logf("package %s: shadow mismatch on route %s", r.config.PackageID, request.RouteID)
			r.recordSample(request, legacy, native, now)
		}
	}()
}

// recordSample keeps a sanitized description of a shadow mismatch for the
// Health details, from which the kernel stores it. The request body is
// never kept; the diff holds only sanitized values (package shadowsample).
func (r *Router) recordSample(request DispatchRequest, legacy DispatchResponse, native NativeResponse, observedAt int64) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return
	}
	diff, truncated := shadowsample.Diff(legacy.ResponseBody, native.Body)
	sample := shadowsample.Sanitize(shadowsample.Sample{
		ID: hex.EncodeToString(id[:]), RouteID: request.RouteID, Method: request.Method,
		Path:         shadowsample.SanitizePath(request.Metadata.Path, request.Metadata.PathParams, request.Metadata.Query),
		LegacyStatus: legacy.StatusCode, NativeStatus: native.StatusCode,
		Diff: diff, DiffTruncated: truncated, RequestID: request.RequestID, ObservedAt: observedAt,
	})
	r.mu.Lock()
	defer r.mu.Unlock()
	r.samples = append(r.samples, sample)
	if len(r.samples) > maxShadowSamples {
		r.samples = append([]shadowsample.Sample(nil), r.samples[len(r.samples)-maxShadowSamples:]...)
	}
}

func defaultShadowCompare(legacy, native NativeResponse) bool {
	return legacy.StatusCode == native.StatusCode && v2compat.EqualForCompare(legacy.Body, native.Body)
}

func (r *Router) count(routeID string, update func(*routeStatistics)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	stats := r.routeStats[routeID]
	if stats == nil {
		stats = &routeStatistics{}
		r.routeStats[routeID] = stats
	}
	update(stats)
}

// OpenWebSocket relays WebSocket routes to the kernel; they always run in
// legacy mode.
func (r *Router) OpenWebSocket(ctx context.Context, open WebSocketOpen, stream WebSocketStream) error {
	if r == nil || r.draining.Load() || stream == nil {
		return errors.New("package is unavailable")
	}
	if strings.TrimSpace(open.RouteID) == "" || len(open.BridgeCapability) != 32 {
		return errors.New("package WebSocket bridge capability is required")
	}
	if r.config.AllowRoute != nil && !r.config.AllowRoute(open.RouteID) {
		return fmt.Errorf("package route %q is unsupported", open.RouteID)
	}
	bridge, ok := r.config.Bridge.(RouterWebSocketBridge)
	if !ok {
		return errors.New("package WebSocket bridge is unavailable")
	}
	relayContext, cancel := context.WithCancel(ctx)
	defer cancel()
	bridgeStream, err := bridge.OpenWebSocket(relayContext, open.BridgeCapability, open.RouteID)
	if err != nil {
		return err
	}
	return relayWebSocketBridge(relayContext, stream, bridgeStream)
}

// Migrate forwards a migration to its kernel bridge operation.
func (r *Router) Migrate(ctx context.Context, request MigrationRequest) (MigrationResponse, error) {
	if r == nil || r.draining.Load() {
		return MigrationResponse{}, errors.New("package is unavailable")
	}
	if strings.HasPrefix(request.MigrationID, IndexMigrationPrefix) {
		if r.config.IndexMigration == nil {
			return MigrationResponse{}, errors.New("package has no migration index runner")
		}
		return r.config.IndexMigration(ctx, request)
	}
	if strings.TrimSpace(request.MigrationID) == "" || len(request.BridgeCapability) != 32 {
		return MigrationResponse{}, errors.New("package migration bridge capability is required")
	}
	operation := "migration." + r.config.PackageID + "." + request.MigrationID
	if r.config.MigrationOperation != nil {
		var ok bool
		if operation, ok = r.config.MigrationOperation(request.MigrationID); !ok {
			return MigrationResponse{}, errors.New("package migration is unsupported")
		}
	}
	response, err := r.config.Bridge.Invoke(ctx, request.BridgeCapability, operation, nil)
	if err != nil {
		return MigrationResponse{}, err
	}
	var result struct {
		Checkpoint       string `json:"checkpoint"`
		ValidationDigest string `json:"validation_digest"`
		Complete         bool   `json:"complete"`
	}
	if response.StatusCode != http.StatusOK || json.Unmarshal(response.Body, &result) != nil || result.Checkpoint == "" || result.ValidationDigest == "" || !result.Complete {
		return MigrationResponse{}, errors.New("package migration bridge response is invalid")
	}
	return MigrationResponse{Checkpoint: result.Checkpoint, ValidationDigest: result.ValidationDigest, Complete: true}, nil
}

// routerHealthDetails is the Health details document. The kernel reads it for
// metrics and diagnostics.
type routerHealthDetails struct {
	Bridge string                       `json:"bridge"`
	Config routerConfigDetails          `json:"config"`
	Routes map[string]routeHealthDetail `json:"routes,omitempty"`
	// ShadowSamples are the latest sanitized shadow mismatches, at most
	// maxShadowSamples, none older than shadowSampleMaxAge. A kernel that
	// does not read them ignores the field.
	ShadowSamples []shadowsample.Sample `json:"shadow_samples,omitempty"`
}

type routerConfigDetails struct {
	Status    string `json:"status"`
	Revision  int64  `json:"revision,omitempty"`
	Hash      string `json:"hash,omitempty"`
	CheckedAt int64  `json:"checked_at_unix,omitempty"`
}

type routeHealthDetail struct {
	Mode        string `json:"mode"`
	Effective   string `json:"effective"`
	Unsupported bool   `json:"mode_unsupported,omitempty"`
	routeStatistics
}

func (r *Router) Health(context.Context) (HealthResponse, error) {
	if r == nil {
		return HealthResponse{Healthy: false}, nil
	}
	if r.draining.Load() {
		return HealthResponse{Healthy: false, LeaseID: r.config.LeaseID}, nil
	}
	details, err := json.Marshal(r.healthDetails())
	if err != nil {
		return HealthResponse{}, err
	}
	return HealthResponse{Healthy: true, LeaseID: r.config.LeaseID, DetailsJSON: string(details)}, nil
}

func (r *Router) healthDetails() routerHealthDetails {
	r.mu.RLock()
	details := routerHealthDetails{Bridge: "required", Config: routerConfigDetails{Status: r.configStatus, Revision: r.configRev, Hash: r.configHash}}
	if !r.configChecked.IsZero() {
		details.Config.CheckedAt = r.configChecked.Unix()
	}
	ids := make(map[string]struct{}, len(r.modes)+len(r.routeStats))
	for id := range r.modes {
		ids[id] = struct{}{}
	}
	for id := range r.routeStats {
		ids[id] = struct{}{}
	}
	stats := make(map[string]routeStatistics, len(r.routeStats))
	for id, entry := range r.routeStats {
		stats[id] = *entry
	}
	oldest := r.config.Now().Add(-shadowSampleMaxAge).Unix()
	for _, sample := range r.samples {
		if sample.ObservedAt >= oldest {
			details.ShadowSamples = append(details.ShadowSamples, sample)
		}
	}
	r.mu.RUnlock()
	if len(ids) == 0 {
		return details
	}
	names := make([]string, 0, len(ids))
	for id := range ids {
		names = append(names, id)
	}
	sort.Strings(names)
	details.Routes = make(map[string]routeHealthDetail, len(names))
	for _, id := range names {
		configured, effective := r.Mode(id)
		details.Routes[id] = routeHealthDetail{
			Mode: configured, Effective: effective, Unsupported: configured != effective, routeStatistics: stats[id],
		}
	}
	return details
}

func (r *Router) Drain(context.Context) (DrainResponse, error) {
	if r == nil {
		return DrainResponse{}, errors.New("package is unavailable")
	}
	r.draining.Store(true)
	return DrainResponse{Drained: true}, nil
}

// Resume serves again after a Drain.
func (r *Router) Resume(context.Context) error {
	if r == nil {
		return errors.New("package is unavailable")
	}
	r.draining.Store(false)
	return nil
}

// Draining reports whether the router refuses new requests.
func (r *Router) Draining() bool { return r != nil && r.draining.Load() }

func relayWebSocketBridge(ctx context.Context, host WebSocketStream, bridge packagebridgesdk.WebSocketStream) error {
	result := make(chan error, 2)
	go func() {
		for {
			frame, err := host.Recv()
			if err != nil {
				if errors.Is(err, io.EOF) {
					_ = bridge.CloseSend()
					result <- nil
					return
				}
				result <- err
				return
			}
			if frame.Close != nil && len(frame.Data) != 0 {
				result <- errors.New("package WebSocket frame cannot contain data and close")
				return
			}
			bridgeFrame := packagebridgesdk.WebSocketFrame{Data: append([]byte(nil), frame.Data...)}
			if frame.Close != nil {
				bridgeFrame.Close = &packagebridgesdk.WebSocketClose{Code: frame.Close.Code, Reason: frame.Close.Reason}
			}
			if err := bridge.Send(bridgeFrame); err != nil {
				result <- err
				return
			}
			if frame.Close != nil {
				_ = bridge.CloseSend()
				result <- nil
				return
			}
		}
	}()
	go func() {
		for {
			frame, err := bridge.Recv()
			if err != nil {
				if errors.Is(err, io.EOF) {
					result <- nil
					return
				}
				result <- err
				return
			}
			if frame.Close != nil && len(frame.Data) != 0 {
				result <- errors.New("bridge WebSocket frame cannot contain data and close")
				return
			}
			hostFrame := WebSocketFrame{Data: append([]byte(nil), frame.Data...)}
			if frame.Close != nil {
				hostFrame.Close = &WebSocketClose{Code: frame.Close.Code, Reason: frame.Close.Reason}
			}
			if err := host.Send(hostFrame); err != nil {
				result <- err
				return
			}
			if frame.Close != nil {
				result <- nil
				return
			}
		}
	}()
	select {
	case err := <-result:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
