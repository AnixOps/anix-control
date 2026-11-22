//go:build unix

package pluginhost

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/AnixOps/anix-control/sdk/modulesdk"
	"github.com/AnixOps/anix-control/v5/internal/packagebridge"
)

const (
	hostSocketEnvironment             = "ANIX_CONTROL_HOST_SOCKET"
	hostRuntimeDirectoryFDEnvironment = "ANIX_CONTROL_HOST_DIR_FD"
	hostPackageBridgeFDEnvironment    = "ANIX_CONTROL_PACKAGE_BRIDGE_FD"
	hostMaxResponseBytesEnvironment   = "ANIX_CONTROL_HOST_MAX_RESPONSE_BYTES"
	childPackageBridgeFD              = 4
)

const maxUnixSocketPathBytes = 100

// defaultHostStopGrace bounds how long a stopping host may take to exit after
// SIGTERM before its process group is killed.
const defaultHostStopGrace = 2 * time.Second

const childRuntimeEntrypoint = childRuntimeDirectoryPath + "/entrypoint"

// hostStartOptions carries the supervisor settings a host process inherits.
// The watchdog reuses the exact options of the original start.
type hostStartOptions struct {
	startupTimeout time.Duration
	bridgeFactory  packagebridge.SessionFactory
	stopGrace      time.Duration
	logf           func(string, ...any)
	// maxResponseBytes is the configured response body limit, passed to the
	// host as ANIX_CONTROL_HOST_MAX_RESPONSE_BYTES and used to size the
	// kernel client's receive window. Restarts reuse it with the options.
	maxResponseBytes int64
	// packageEnvironment adds entries to one package's environment.
	packageEnvironment map[string][]string
	// inProcess is ManagerConfig.InProcess.
	inProcess func(ctx context.Context, packageID, version string) error
}

func startHostProcess(ctx context.Context, runtimeRoot *runtimeRoot, ref ArtifactRef, generation uint64, options hostStartOptions) (*hostProcess, error) {
	startupTimeout, bridgeFactory := options.startupTimeout, options.bridgeFactory
	directory, err := runtimeRoot.createHostRuntimeDir()
	if err != nil {
		return nil, err
	}
	cleanup := func() { _ = directory.remove() }
	var bridge *packagebridge.Session
	var bridgeChild *os.File
	if bridgeFactory != nil {
		bridge, bridgeChild, err = bridgeFactory.NewSession(packagebridge.HostIdentity{
			PackageID: ref.PackageID, Version: ref.Version, Generation: generation,
		})
		if err != nil {
			cleanup()
			return nil, fmt.Errorf("%w: create package bridge: %v", ErrHostUnavailable, err)
		}
		removeDirectory := cleanup
		cleanup = func() {
			if bridgeChild != nil {
				_ = bridgeChild.Close()
			}
			_ = bridge.Close()
			removeDirectory()
		}
	}
	socketPath := directory.path("host.sock")
	if len([]byte(socketPath)) > maxUnixSocketPathBytes || len([]byte(childRuntimePath("host.sock"))) > maxUnixSocketPathBytes {
		cleanup()
		return nil, fmt.Errorf("%w: host socket path exceeds Unix socket limit", ErrHostIncompatible)
	}
	// Rehash every referenced file before execution.
	if err := verifyArtifactRef(ref); err != nil {
		cleanup()
		return nil, err
	}
	if ref.Builtin && options.inProcess != nil {
		host := startInProcessHost(ref, generation, options, directory, socketPath, bridgeChild)
		bridgeChild = nil // the host owns it now
		if bridge != nil {
			host.bridge = bridge
		}
		return awaitHostStart(ctx, host, startupTimeout)
	}
	command, err := hostCommand(ref, directory)
	if err != nil {
		cleanup()
		return nil, err
	}
	// os/exec changes directory before it remaps ExtraFiles to FD 3, so this
	// parent-side /proc descriptor path preserves the pinned directory for cwd.
	command.Dir = directory.path(".")
	command.Env = append(hostEnvironment(childRuntimePath("host.sock"), childRuntimeDirectoryPath, bridgeChild != nil),
		hostMaxResponseBytesEnvironment+"="+strconv.FormatInt(normalizeMaxResponseBytes(options.maxResponseBytes), 10))
	if ref.Builtin {
		command.Env = append(command.Env, BuiltinHostVersionEnvironment+"="+ref.Version)
	}
	command.Env = append(command.Env, options.packageEnvironment[ref.PackageID]...)
	command.ExtraFiles = []*os.File{directory.directory}
	if bridgeChild != nil {
		command.ExtraFiles = append(command.ExtraFiles, bridgeChild)
	}
	command.SysProcAttr = hostSysProcAttr()
	output, err := newHostOutputPipes(command)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("%w: create package host output pipes: %v", ErrHostUnavailable, err)
	}
	if err := command.Start(); err != nil {
		output.abort()
		cleanup()
		return nil, fmt.Errorf("%w: start package host: %v", ErrHostUnavailable, err)
	}
	output.forward(hostOutputPrefix(ref, generation), options.logf)
	if bridgeChild != nil {
		if err := bridgeChild.Close(); err != nil {
			_ = terminateHostProcessGroup(command.Process.Pid)
			_ = command.Wait()
			_ = bridge.Close()
			cleanup()
			return nil, fmt.Errorf("%w: close child package bridge descriptor: %v", ErrHostUnavailable, err)
		}
		bridgeChild = nil
	}
	host := &hostProcess{
		packageID: ref.PackageID, version: ref.Version, generation: generation, ref: ref,
		runtimeDir: directory, socketPath: socketPath, command: command, waitDone: make(chan struct{}),
		stopGrace: options.stopGrace, maxResponseBytes: normalizeMaxResponseBytes(options.maxResponseBytes),
	}
	// Assign only a real session: a typed nil would make the interface
	// non-nil and every Mint fail.
	if bridge != nil {
		host.bridge = bridge
	}
	go func() {
		host.waitErr = command.Wait()
		close(host.waitDone)
	}()
	return awaitHostStart(ctx, host, startupTimeout)
}

// startInProcessHost runs a built-in host on a goroutine of Control. It gets
// the settings a child process gets in its environment, less the runtime
// directory and locale (it shares Control's), and its end of the bridge
// directly. A panic in its start ends the host, not Control.
func startInProcessHost(ref ArtifactRef, generation uint64, options hostStartOptions, directory *hostRuntimeDir, socketPath string, bridgeChild *os.File) *hostProcess {
	environment := map[string]string{
		hostSocketEnvironment:           socketPath,
		hostMaxResponseBytesEnvironment: strconv.FormatInt(normalizeMaxResponseBytes(options.maxResponseBytes), 10),
		BuiltinHostVersionEnvironment:   ref.Version,
	}
	for _, entry := range options.packageEnvironment[ref.PackageID] {
		if name, value, ok := strings.Cut(entry, "="); ok {
			environment[name] = value
		}
	}
	prefix := "[" + hostOutputPrefix(ref, generation) + "]"
	runCtx, cancel := context.WithCancel(context.Background())
	runCtx = modulesdk.WithInProcess(runCtx, modulesdk.InProcess{
		Getenv: func(name string) string { return environment[name] }, Bridge: bridgeChild,
		Logf: func(format string, args ...any) {
			options.logf("%s "+format, append([]any{prefix}, args...)...)
		},
	})
	host := &hostProcess{
		packageID: ref.PackageID, version: ref.Version, generation: generation, ref: ref,
		runtimeDir: directory, socketPath: socketPath, cancel: cancel, waitDone: make(chan struct{}),
		stopGrace: options.stopGrace, maxResponseBytes: normalizeMaxResponseBytes(options.maxResponseBytes),
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

// hostCommand returns the process a host start executes. A built-in host is
// the running Control binary itself (SelfExecutable), checked before every
// start. Any other host is the materialized entrypoint, snapshotted from its
// opened descriptor into the private runtime directory after its digest was
// verified, so the caller-provided source pathname cannot be swapped.
func hostCommand(ref ArtifactRef, directory *hostRuntimeDir) (*exec.Cmd, error) {
	if ref.Builtin {
		self, err := Self()
		if err != nil {
			return nil, err
		}
		if err := self.Check(); err != nil {
			return nil, err
		}
		command := exec.CommandContext(context.Background(), self.Path, BuiltinHostCommand, ref.PackageID) // #nosec G204 -- the recorded running binary and a verified package id.
		command.Args[0] = builtinHostArgv0
		return command, nil
	}
	if _, err := stageVerifiedEntrypoint(ref, directory); err != nil {
		return nil, err
	}
	return exec.CommandContext(context.Background(), childRuntimeEntrypoint), nil // #nosec G204 -- fixed descriptor-relative path after digest-verified staging.
}

func hostEnvironment(socketPath, runtimeDir string, bridgeEnabled ...bool) []string {
	environment := []string{
		hostSocketEnvironment + "=" + socketPath,
		hostRuntimeDirectoryFDEnvironment + "=" + fmt.Sprint(childRuntimeDirectoryFD),
		"HOME=" + runtimeDir,
		"TMPDIR=" + runtimeDir,
	}
	environment = append(environment, hostLocaleEnvironment()...)
	if len(bridgeEnabled) > 0 && bridgeEnabled[0] {
		environment = append(environment, hostPackageBridgeFDEnvironment+"="+fmt.Sprint(childPackageBridgeFD))
	}
	return environment
}

// hostLocaleEnvironment passes the kernel's time zone and locale through so
// package output formats dates like the in-process handlers did. Only these
// two names are forwarded, and only when the kernel has them set.
func hostLocaleEnvironment() []string {
	var environment []string
	for _, name := range []string{"TZ", "LANG"} {
		if value, ok := os.LookupEnv(name); ok && value != "" {
			environment = append(environment, name+"="+value)
		}
	}
	return environment
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
	if h.command != nil && h.command.Process != nil {
		pid := h.command.Process.Pid
		if !hostExited(h) {
			// Ask the group to exit first so hosts can flush and close cleanly.
			if err := signalHostProcessGroup(pid, syscall.SIGTERM); err != nil {
				stopErr = errors.Join(stopErr, fmt.Errorf("%w: signal host process: %v", ErrHostUnavailable, err))
			}
			h.awaitExit(ctx, h.gracePeriod())
		}
		// The group can survive after its leader has exited. Always target the
		// original PGID so descendants cannot outlive retirement.
		if err := terminateHostProcessGroup(pid); err != nil && !errors.Is(err, os.ErrProcessDone) {
			stopErr = errors.Join(stopErr, fmt.Errorf("%w: stop host process: %v", ErrHostUnavailable, err))
		}
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

func (h *hostProcess) gracePeriod() time.Duration {
	if h.stopGrace > 0 {
		return h.stopGrace
	}
	return defaultHostStopGrace
}

// awaitExit waits until the host leader exits, the grace period ends, or ctx
// is done, whichever comes first.
func (h *hostProcess) awaitExit(ctx context.Context, grace time.Duration) {
	if h.waitDone == nil {
		return
	}
	timer := time.NewTimer(grace)
	defer timer.Stop()
	select {
	case <-h.waitDone:
	case <-timer.C:
	case <-ctx.Done():
	}
}

func terminateHostProcessGroup(pid int) error {
	return signalHostProcessGroup(pid, syscall.SIGKILL)
}

func signalHostProcessGroup(pid int, signal syscall.Signal) error {
	if pid <= 0 {
		return nil
	}
	err := syscall.Kill(-pid, signal)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}
