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
	"time"

	"github.com/AnixOps/anix-control/v4/internal/pluginhost"
)

var (
	ErrRouteNotFound      = errors.New("control plugin route is not implemented by the executor")
	ErrMethodNotAllowed   = errors.New("control plugin route does not allow this method")
	ErrInvalidPluginInput = errors.New("control plugin request is invalid")
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
	Generation  uint64
	Config      json.RawMessage
}

// LifecycleDispatcher is the durable-operation execution boundary. The
// production server injects HostLifecycleDispatcher.
type LifecycleDispatcher interface {
	ExecuteLifecycle(context.Context, string, string, LifecycleRequest) (json.RawMessage, error)
}

// ArtifactRefResolver supplies the immutable materialized host files for an
// installed package. Task 4 owns resolving the v2 manifest entrypoint fields;
// without one, lifecycle execution deliberately fails closed.
type ArtifactRefResolver func(context.Context, string, string) (pluginhost.ArtifactRef, error)

// HostLifecycleDispatcher maps the existing durable lifecycle operations onto
// the narrow host manager API. It never falls back to an in-process executor.
type HostLifecycleDispatcher struct {
	hosts     pluginhost.Manager
	artifacts ArtifactRefResolver
	migrator  HostMigrator
}

// HostMigrator runs a package's migrations in its freshly started host.
// service.PackageHostMigrator implements it on the migration ledger.
type HostMigrator interface {
	MigrateStartedHost(ctx context.Context, ref pluginhost.ArtifactRef, generation uint64) error
}

func NewHostLifecycleDispatcher(hosts pluginhost.Manager, artifacts ArtifactRefResolver) *HostLifecycleDispatcher {
	return &HostLifecycleDispatcher{hosts: hosts, artifacts: artifacts}
}

// WithMigrator makes every host start run the package's migrations before
// the lifecycle operation succeeds.
func (d *HostLifecycleDispatcher) WithMigrator(migrator HostMigrator) *HostLifecycleDispatcher {
	d.migrator = migrator
	return d
}

// migrationStopTimeout bounds stopping a host whose migration failed.
const migrationStopTimeout = 30 * time.Second

var _ LifecycleDispatcher = (*HostLifecycleDispatcher)(nil)

func (d *HostLifecycleDispatcher) ExecuteLifecycle(ctx context.Context, pluginID, version string, request LifecycleRequest) (json.RawMessage, error) {
	if d == nil || d.hosts == nil {
		return nil, pluginhost.ErrHostUnavailable
	}
	if request.Generation == 0 {
		return nil, pluginhost.ErrGenerationUnavailable
	}
	switch request.Kind {
	case "plugin.install", "plugin.enable", "plugin.update", "plugin.rollback":
		return d.startResolvedHost(ctx, pluginID, version, request.Generation)
	case "plugin.disable":
		deadline, ok := ctx.Deadline()
		if !ok {
			deadline = time.Now().Add(30 * time.Second)
		}
		if err := d.hosts.Drain(ctx, pluginID, version, request.Generation, deadline); err != nil {
			if errors.Is(err, pluginhost.ErrHostNotFound) {
				return json.RawMessage(`{}`), nil
			}
			return nil, err
		}
		if err := d.hosts.Stop(ctx, pluginID, version, request.Generation); err != nil {
			if errors.Is(err, pluginhost.ErrHostNotFound) {
				return json.RawMessage(`{}`), nil
			}
			return nil, err
		}
		return json.RawMessage(`{}`), nil
	case "plugin.configure":
		// Control hosts pull their configuration (route modes) over the
		// package bridge; the new revision is already stored when this runs.
		return json.RawMessage(`{}`), nil
	case "plugin.health":
		health, err := d.hosts.Health(ctx, pluginID, version, request.Generation)
		if err != nil {
			return nil, err
		}
		if !health.Healthy {
			return nil, fmt.Errorf("%w: host health check failed", pluginhost.ErrHostUnavailable)
		}
		return json.RawMessage(`{"healthy":true}`), nil
	default:
		return nil, fmt.Errorf("%w: host protocol does not support lifecycle operation %q", pluginhost.ErrHostIncompatible, request.Kind)
	}
}

func (d *HostLifecycleDispatcher) startResolvedHost(ctx context.Context, pluginID, version string, generation uint64) (json.RawMessage, error) {
	if d.artifacts == nil {
		return nil, fmt.Errorf("%w: no verified artifact reference is available", pluginhost.ErrHostUnavailable)
	}
	ref, err := d.artifacts(ctx, pluginID, version)
	if err != nil {
		return nil, fmt.Errorf("%w: verified artifact reference is unavailable", pluginhost.ErrHostUnavailable)
	}
	if ref.PackageID != pluginID || ref.Version != version {
		return nil, fmt.Errorf("%w: resolved artifact identity does not match lifecycle request", pluginhost.ErrHostIncompatible)
	}
	if err := d.hosts.Start(ctx, ref, generation); err != nil {
		return nil, err
	}
	if d.migrator != nil {
		if err := d.migrator.MigrateStartedHost(ctx, ref, generation); err != nil {
			// A host must not serve a schema its migrations did not reach.
			stopContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), migrationStopTimeout)
			defer cancel()
			if stopErr := d.hosts.Stop(stopContext, pluginID, version, generation); stopErr != nil && !errors.Is(stopErr, pluginhost.ErrHostNotFound) {
				return nil, fmt.Errorf("package migration failed: %w (stopping the host also failed: %v)", err, stopErr)
			}
			return nil, fmt.Errorf("package migration failed: %w", err)
		}
	}
	return json.RawMessage(`{}`), nil
}
