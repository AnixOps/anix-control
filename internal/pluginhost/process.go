package pluginhost

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

const (
	hostSocketEnvironment             = "ANIX_CONTROL_HOST_SOCKET"
	hostRuntimeDirectoryFDEnvironment = "ANIX_CONTROL_HOST_DIR_FD"
)

const maxUnixSocketPathBytes = 100

func startHostProcess(ctx context.Context, runtimeRoot *runtimeRoot, ref ArtifactRef, generation uint64, startupTimeout time.Duration) (*hostProcess, error) {
	directory, err := runtimeRoot.createHostRuntimeDir()
	if err != nil {
		return nil, err
	}
	cleanup := func() { _ = directory.remove() }
	socketPath := directory.path("host.sock")
	if len([]byte(socketPath)) > maxUnixSocketPathBytes || len([]byte(childRuntimePath("host.sock"))) > maxUnixSocketPathBytes {
		cleanup()
		return nil, fmt.Errorf("%w: host socket path exceeds Unix socket limit", ErrHostIncompatible)
	}
	// Rehash every referenced file and snapshot the entrypoint from its opened
	// descriptor before execution. The staged path cannot be replaced through
	// the caller-provided source pathname after this point.
	if err := verifyArtifactRef(ref); err != nil {
		cleanup()
		return nil, err
	}
	_, err = stageVerifiedEntrypoint(ref, directory)
	if err != nil {
		cleanup()
		return nil, err
	}
	command := exec.CommandContext(context.Background(), childRuntimePath("entrypoint"))
	// os/exec changes directory before it remaps ExtraFiles to FD 3, so this
	// parent-side /proc descriptor path preserves the pinned directory for cwd.
	command.Dir = directory.path(".")
	command.Env = hostEnvironment(childRuntimePath("host.sock"), childRuntimeDirectoryPath)
	command.ExtraFiles = []*os.File{directory.directory}
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := command.Start(); err != nil {
		cleanup()
		return nil, fmt.Errorf("%w: start package host: %v", ErrHostUnavailable, err)
	}
	host := &hostProcess{
		packageID: ref.PackageID, version: ref.Version, generation: generation, ref: ref,
		runtimeDir: directory, socketPath: socketPath, command: command, waitDone: make(chan struct{}),
	}
	go func() {
		host.waitErr = command.Wait()
		close(host.waitDone)
	}()

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

func hostEnvironment(socketPath, runtimeDir string) []string {
	return []string{
		hostSocketEnvironment + "=" + socketPath,
		hostRuntimeDirectoryFDEnvironment + "=" + fmt.Sprint(childRuntimeDirectoryFD),
		"HOME=" + runtimeDir,
		"TMPDIR=" + runtimeDir,
	}
}

func waitForHost(ctx context.Context, host *hostProcess) (*hostClient, HostHealth, error) {
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := ensurePrivateSocket(host.socketPath); err == nil {
			dialCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
			client, dialErr := dialHostClient(dialCtx, host.socketPath)
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
	var stopErr error
	if h.client != nil {
		_ = h.client.Close()
	}
	if h.command != nil && h.command.Process != nil {
		// The group can survive after its leader has exited. Always target the
		// original PGID so descendants cannot outlive retirement.
		if err := terminateHostProcessGroup(h.command.Process.Pid); err != nil && !errors.Is(err, os.ErrProcessDone) {
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

func terminateHostProcessGroup(pid int) error {
	if pid <= 0 {
		return nil
	}
	err := syscall.Kill(-pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}
