//go:build unix

package pluginhost

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/sdk/modulesdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v5/internal/packagebridge"
)

// hostStartOptions carries the supervisor settings a host start uses. The
// watchdog reuses the exact options of the original start.
type hostStartOptions struct {
	startupTimeout time.Duration
	bridgeFactory  packagebridge.SessionFactory
	logf           func(string, ...any)
	// maxResponseBytes is the configured response body limit, which the
	// kernel holds every package response to. Restarts reuse it.
	maxResponseBytes int64
	// packageEnvironment adds entries to one package's settings.
	packageEnvironment map[string][]string
	// inProcess is ManagerConfig.InProcess.
	inProcess func(ctx context.Context, packageID, version string) error
}

func startHostProcess(ctx context.Context, ref ArtifactRef, generation uint64, options hostStartOptions) (*hostProcess, error) {
	if options.inProcess == nil {
		return nil, fmt.Errorf("%w: no package hosts are linked into this binary", ErrHostUnavailable)
	}
	// Rehash every referenced file before the start.
	if err := verifyArtifactRef(ref); err != nil {
		return nil, err
	}
	var bridge *packagebridge.Session
	var bridgeEnd *os.File
	if options.bridgeFactory != nil {
		var err error
		bridge, bridgeEnd, err = options.bridgeFactory.NewSession(packagebridge.HostIdentity{
			PackageID: ref.PackageID, Version: ref.Version, Generation: generation,
		})
		if err != nil {
			return nil, fmt.Errorf("%w: create package bridge: %v", ErrHostUnavailable, err)
		}
	}
	host := startInProcessHost(ref, generation, options, bridgeEnd)
	// Assign only a real session: a typed nil would make the interface
	// non-nil and every Mint fail.
	if bridge != nil {
		host.bridge = bridge
	}
	return awaitHostStart(ctx, host, options.startupTimeout)
}

// startInProcessHost runs a host on a goroutine of Control. It gets its
// settings from the kernel, never from Control's environment, and its end of
// the bridge. A panic in its start ends the host, not Control.
func startInProcessHost(ref ArtifactRef, generation uint64, options hostStartOptions, bridgeEnd *os.File) *hostProcess {
	environment := map[string]string{}
	for _, entry := range options.packageEnvironment[ref.PackageID] {
		if name, value, ok := strings.Cut(entry, "="); ok {
			environment[name] = value
		}
	}
	prefix := fmt.Sprintf("[pkg:%s v:%s gen:%d]", ref.PackageID, ref.Version, generation)
	ready := make(chan pluginhostsdk.Package, 1)
	runCtx, cancel := context.WithCancel(context.Background())
	runCtx = modulesdk.WithInProcess(runCtx, modulesdk.InProcess{
		Getenv: func(name string) string { return environment[name] }, Bridge: bridgeEnd,
		Logf: func(format string, args ...any) {
			options.logf("%s "+format, append([]any{prefix}, args...)...)
		},
		Ready: func(packageImpl pluginhostsdk.Package) { ready <- packageImpl },
	})
	host := &hostProcess{
		packageID: ref.PackageID, version: ref.Version, generation: generation, ref: ref,
		cancel: cancel, waitDone: make(chan struct{}), ready: ready,
		maxResponseBytes: normalizeMaxResponseBytes(options.maxResponseBytes),
	}
	go func() {
		defer close(host.waitDone)
		defer func() {
			if recovered := recover(); recovered != nil {
				host.waitErr = fmt.Errorf("panic: %v", recovered)
				options.logf("%s panic: %v\n%s", prefix, recovered, debug.Stack())
			}
		}()
		host.waitErr = options.inProcess(runCtx, ref.PackageID, ref.Version)
		if host.waitErr != nil {
			options.logf("%s %v", prefix, host.waitErr)
		}
	}()
	return host
}

// awaitHostStart waits until a started host hands over its package and the
// package answers Health, and stops the host when that does not happen
// within startupTimeout.
func awaitHostStart(ctx context.Context, host *hostProcess, startupTimeout time.Duration) (*hostProcess, error) {
	startupCtx, cancel := context.WithTimeout(ctx, startupTimeout)
	defer cancel()
	client, health, err := waitForHost(startupCtx, host)
	if err != nil {
		_ = host.stop(context.Background())
		return nil, err
	}
	host.client = client
	host.leaseID = health.LeaseID
	return host, nil
}

func waitForHost(ctx context.Context, host *hostProcess) (*packageClient, HostHealth, error) {
	var client *packageClient
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		if client == nil {
			select {
			case packageImpl := <-host.ready:
				client = newPackageClient(packageImpl, host.maxResponseBytes)
			default:
			}
		}
		if client != nil {
			health, err := client.Health(ctx, host.generation)
			if err == nil && health.Healthy && health.LeaseID != "" {
				return client, health, nil
			}
			if err != nil && errors.Is(err, ErrHostIncompatible) {
				return nil, HostHealth{}, err
			}
		}
		select {
		case <-ctx.Done():
			return nil, HostHealth{}, fmt.Errorf("%w: host did not become healthy", ErrHostUnavailable)
		case <-host.waitDone:
			return nil, HostHealth{}, fmt.Errorf("%w: host exited before becoming healthy: %v", ErrHostUnavailable, host.waitErr)
		case <-ticker.C:
		}
	}
}

func (h *hostProcess) stop(ctx context.Context) error {
	if h == nil {
		return nil
	}
	h.beginWebSocketDrain()
	var stopErr error
	if h.bridge != nil {
		stopErr = errors.Join(stopErr, h.bridge.Close())
		h.bridge = nil
	}
	if h.cancel != nil {
		h.cancel()
	}
	if h.waitDone != nil {
		select {
		case <-h.waitDone:
		case <-ctx.Done():
			stopErr = errors.Join(stopErr, fmt.Errorf("%w: wait for host shutdown: %v", ErrHostUnavailable, ctx.Err()))
		}
	}
	return stopErr
}
