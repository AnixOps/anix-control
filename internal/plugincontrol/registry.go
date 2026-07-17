// Package plugincontrol contains the Control-side runtime boundary for
// officially signed packages. The kernel owns admission and authorization;
// this package only executes an already admitted, version-bound request.
package plugincontrol

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"

	"gorm.io/gorm"
)

var (
	ErrExecutorNotFound               = errors.New("control plugin executor is not installed")
	ErrConfigurationValidatorNotFound = errors.New("control plugin configuration validator is not installed for this version")
	ErrRouteNotFound                  = errors.New("control plugin route is not implemented by the executor")
	ErrMethodNotAllowed               = errors.New("control plugin route does not allow this method")
	ErrInvalidPluginInput             = errors.New("control plugin request is invalid")
)

// RouteRequest deliberately contains only request data that a package API is
// allowed to consume. Authentication and authorization have already been
// completed by the kernel gateway; credentials are never forwarded to a
// package executor.
type RouteRequest struct {
	Method  string
	Path    string
	Query   url.Values
	Body    []byte
	ActorID uint
}

// RouteResponse is a controlled JSON response. Package code cannot select an
// arbitrary status outside the HTTP range or write directly to the Gin
// response writer.
type RouteResponse struct {
	Status int
	Data   any
}

// LifecycleRequest is the durable-operation input shared by Control package
// executors. Config is canonical JSON and OperationID is stable across retry.
type LifecycleRequest struct {
	OperationID string
	Kind        string
	Target      string
	Config      json.RawMessage
}

// Executor is the smallest Control package runtime contract. A package may
// expose no backend route, but lifecycle operations are always version-bound.
type Executor interface {
	PluginID() string
	Version() string
	HandleRoute(context.Context, RouteRequest) (RouteResponse, error)
	ExecuteLifecycle(context.Context, LifecycleRequest) (json.RawMessage, error)
}

// ConfigurationValidator is optional. Registries only invoke it for the exact
// executor version selected by the signed installation release.
type ConfigurationValidator interface {
	ValidateConfiguration(context.Context, json.RawMessage) error
}

type Registry struct {
	mu        sync.RWMutex
	executors map[string]Executor
}

var (
	defaultRegistryMu sync.Mutex
	defaultRegistryDB *gorm.DB
	defaultRegistry   *Registry
)

func NewRegistry(executors ...Executor) (*Registry, error) {
	r := &Registry{executors: make(map[string]Executor, len(executors))}
	for _, executor := range executors {
		if err := r.Register(executor); err != nil {
			return nil, err
		}
	}
	return r, nil
}

// DefaultRegistry returns the process-wide first-party executor set. The HTTP
// gateway and lifecycle worker must share executor instances because future
// Control packages may own an isolated subprocess or other runtime state.
func DefaultRegistry(db *gorm.DB) (*Registry, error) {
	if db == nil {
		return nil, errors.New("control plugin registry requires a database")
	}
	defaultRegistryMu.Lock()
	defer defaultRegistryMu.Unlock()
	if defaultRegistry != nil && defaultRegistryDB == db {
		return defaultRegistry, nil
	}
	registry, err := NewRegistry(NewMachineTelemetryExecutor(db), NewGostMeshExecutor(db))
	if err != nil {
		return nil, err
	}
	defaultRegistryDB, defaultRegistry = db, registry
	return defaultRegistry, nil
}

func (r *Registry) Register(executor Executor) error {
	if r == nil {
		return errors.New("control plugin registry is nil")
	}
	if executor == nil || strings.TrimSpace(executor.PluginID()) == "" || strings.TrimSpace(executor.Version()) == "" {
		return errors.New("control plugin executor id and version are required")
	}
	key := executorKey(executor.PluginID(), executor.Version())
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.executors[key]; exists {
		return errors.New("control plugin executor is already registered")
	}
	r.executors[key] = executor
	return nil
}

func (r *Registry) Lookup(pluginID, version string) (Executor, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	executor, ok := r.executors[executorKey(pluginID, version)]
	return executor, ok
}

func (r *Registry) ExecuteRoute(ctx context.Context, pluginID, version string, request RouteRequest) (RouteResponse, error) {
	executor, ok := r.Lookup(pluginID, version)
	if !ok {
		return RouteResponse{}, ErrExecutorNotFound
	}
	return executor.HandleRoute(ctx, request)
}

func (r *Registry) ExecuteLifecycle(ctx context.Context, pluginID, version string, request LifecycleRequest) (json.RawMessage, error) {
	executor, ok := r.Lookup(pluginID, version)
	if !ok {
		return nil, ErrExecutorNotFound
	}
	return executor.ExecuteLifecycle(ctx, request)
}

// ValidateConfiguration applies package-specific semantics after the kernel
// has verified the signed JSON Schema. Plugins without a registered validator
// remain schema-only. Once a plugin opts in, unknown versions fail closed so a
// validator for one release can never be reused for another release.
func (r *Registry) ValidateConfiguration(ctx context.Context, pluginID, version string, config json.RawMessage) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	executor, ok := r.Lookup(pluginID, version)
	if ok {
		validator, validates := executor.(ConfigurationValidator)
		if !validates {
			return nil
		}
		return validator.ValidateConfiguration(ctx, config)
	}
	if r.hasConfigurationValidator(pluginID) {
		return fmt.Errorf("%w: %s@%s", ErrConfigurationValidatorNotFound, pluginID, version)
	}
	return nil
}

func (r *Registry) hasConfigurationValidator(pluginID string) bool {
	if r == nil {
		return false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, executor := range r.executors {
		if executor.PluginID() == pluginID {
			if _, ok := executor.(ConfigurationValidator); ok {
				return true
			}
		}
	}
	return false
}

func executorKey(pluginID, version string) string { return pluginID + "@" + version }
