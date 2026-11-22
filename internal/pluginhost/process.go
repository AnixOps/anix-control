//go:build unix

package pluginhost

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/sdk/modulesdk"
	"github.com/AnixOps/anix-control/v5/internal/packagebridge"
)

const (
	hostSocketEnvironment           = "ANIX_CONTROL_HOST_SOCKET"
	hostMaxResponseBytesEnvironment = "ANIX_CONTROL_HOST_MAX_RESPONSE_BYTES"
)

const maxUnixSocketPathBytes = 100

// hostStartOptions carries the supervisor settings a host start uses. The
// watchdog reuses the exact options of the original start.
type hostStartOptions struct {
	startupTimeout time.Duration
	bridgeFactory  packagebridge.SessionFactory
	logf           func(string, ...any)
	// maxResponseBytes is the configured response body limit, passed to the
	// host as ANIX_CONTROL_HOST_MAX_RESPONSE_BYTES and used to size the
	// kernel client's receive window. Restarts reuse it with the options.
	maxResponseBytes int64
	// packageEnvironment adds entries to one package's settings.
	packageEnvironment map[string][]string
	// inProcess is ManagerConfig.InProcess.
	inProcess func(ctx context.Context, packageID, version string) error
}

func startHostProcess(ctx context.Context, runtimeRoot *runtimeRoot, ref ArtifactRef, generation uint64, options hostStartOptions) (*hostProcess, error) {
	if options.inProcess == nil {
		return nil, fmt.Errorf("%w: no package hosts are linked into this binary", ErrHostUnavailable)
	}
	directory, err := runtimeRoot.createHostRuntimeDir()
	if err != nil {
		return nil, err
	}
	cleanup := func() { _ = directory.remove() }
	var bridge *packagebridge.Session
	var bridgeEnd *os.File
	if options.bridgeFactory != nil {
		bridge, bridgeEnd, err = options.bridgeFactory.NewSession(packagebridge.HostIdentity{
			PackageID: ref.PackageID, Version: ref.Version, Generation: generation,
		})
		if err != nil {
			cleanup()
			return nil, fmt.Errorf("%w: create package bridge: %v", ErrHostUnavailable, err)
		}
		removeDirectory := cleanup
		cleanup = func() {
			_ = bridgeEnd.Close()
			_ = bridge.Close()
			removeDirectory()
		}
	}
	socketPath := directory.path("host.sock")
	if len([]byte(socketPath)) > maxUnixSocketPathBytes {
		cleanup()
		return nil, fmt.Errorf("%w: host socket path exceeds Unix socket limit", ErrHostIncompatible)
	}
	// Rehash every referenced file before the start.
	if err := verifyArtifactRef(ref); err != nil {
		cleanup()
		return nil, err
	}
	host := startInProcessHost(ref, generation, options, directory, socketPath, bridgeEnd)
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
func startInProcessHost(ref ArtifactRef, generation uint64, options hostStartOptions, directory *hostRuntimeDir, socketPath string, bridgeEnd *os.File) *hostProcess {
	environment := map[string]string{
		hostSocketEnvironment:           socketPath,
		hostMaxResponseBytesEnvironment: strconv.FormatInt(normalizeMaxResponseBytes(options.maxResponseBytes), 10),
	}
	for _, entry := range options.packageEnvironment[ref.PackageID] {
		if name, value, ok := strings.Cut(entry, "="); ok {
			environment[name] = value
		}
	}
	prefix := fmt.Sprintf("[pkg:%s v:%s gen:%d]", ref.PackageID, ref.Version, generation)
	runCtx, cancel := context.WithCancel(context.Background())
	runCtx = modulesdk.WithInProcess(runCtx, modulesdk.InProcess{
		Getenv: func(name string) string { return environment[name] }, Bridge: bridgeEnd,
		Logf: func(format string, args ...any) {
			options.logf("%s "+format, append([]any{prefix}, args...)...)
		},
	})
	host := &hostProcess{
		packageID: ref.PackageID, version: ref.Version, generation: generation, ref: ref,
		runtimeDir: directory, socketPath: socketPath, cancel: cancel, waitDone: make(chan struct{}),
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

// awaitHostStart waits until a started host answers Health, and stops it
// when it does not within startupTimeout.
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

func waitForHost(ctx context.Context, host *hostProcess) (*hostClient, HostHealth, error) {
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := ensurePrivateSocket(host.socketPath); err == nil {
			dialCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
			client, dialErr := dialHostClientWithLimit(dialCtx, host.socketPath, host.maxResponseBytes)
			cancel()
			if dialErr == nil {
				healthCtx, cancelHealth := context.WithTimeout(ctx, 100*time.Millisecond)
				health, healthErr := client.Health(healthCtx, host.generation)
				cancelHealth()
				if healthErr == nil && health.Healthy && health.LeaseID != "" {
					return client, health, nil
				}
				_ = client.Close()
				if healthErr != nil && errors.Is(healthErr, ErrHostIncompatible) {
					return nil, HostHealth{}, healthErr
				}
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

func ensurePrivateSocket(socketPath string) error {
	info, err := os.Lstat(socketPath)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("%w: host endpoint is not a Unix socket", ErrHostIncompatible)
	}
	if err := os.Chmod(socketPath, 0o600); err != nil {
		return fmt.Errorf("%w: secure host socket: %v", ErrHostUnavailable, err)
	}
	info, err = os.Lstat(socketPath)
	if err != nil || info.Mode().Perm() != 0o600 {
		return fmt.Errorf("%w: host socket is not private", ErrHostIncompatible)
	}
	return nil
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
	if h.client != nil {
		_ = h.client.Close()
	}
	if h.cancel != nil {
		// modulesdk stops the host's server, hard after its grace period.
		h.cancel()
	}
	waited := h.waitDone == nil
	if h.waitDone != nil {
		select {
		case <-h.waitDone:
			waited = true
		case <-ctx.Done():
			stopErr = errors.Join(stopErr, fmt.Errorf("%w: wait for host shutdown: %v", ErrHostUnavailable, ctx.Err()))
		}
	}
	if waited && h.runtimeDir != nil {
		if err := h.runtimeDir.remove(); err != nil {
			stopErr = errors.Join(stopErr, fmt.Errorf("%w: remove host runtime directory: %v", ErrHostUnavailable, err))
		}
		h.runtimeDir = nil
	}
	return stopErr
}
