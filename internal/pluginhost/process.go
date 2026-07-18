package pluginhost

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const hostSocketEnvironment = "ANIX_CONTROL_HOST_SOCKET"
const maxUnixSocketPathBytes = 100

func startHostProcess(ctx context.Context, runtimeRoot string, ref ArtifactRef, generation uint64, startupTimeout time.Duration) (*hostProcess, error) {
	directory, err := createHostRuntimeDir(runtimeRoot, ref.PackageID, ref.Version, generation)
	if err != nil {
		return nil, err
	}
	cleanup := func() { _ = os.RemoveAll(directory) }
	socketPath := filepath.Join(directory, "host.sock")
	command := exec.CommandContext(context.Background(), ref.EntrypointPath)
	command.Dir = directory
	command.Env = append(os.Environ(), hostSocketEnvironment+"="+socketPath)
	// Rehash immediately before exec as well as before replacing an active host.
	// The first verification protects the currently healthy generation; this
	// second verification narrows the check-to-exec window for the new process.
	if err := verifyArtifactRef(ref); err != nil {
		cleanup()
		return nil, err
	}
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

func createHostRuntimeDir(runtimeRoot, packageID, version string, generation uint64) (string, error) {
	if runtimeRoot == "" || !filepath.IsAbs(runtimeRoot) {
		return "", fmt.Errorf("%w: host runtime directory must be absolute", ErrHostIncompatible)
	}
	if err := os.MkdirAll(runtimeRoot, 0o700); err != nil {
		return "", fmt.Errorf("%w: create host runtime directory: %v", ErrHostUnavailable, err)
	}
	if err := os.Chmod(runtimeRoot, 0o700); err != nil {
		return "", fmt.Errorf("%w: secure host runtime directory: %v", ErrHostUnavailable, err)
	}
	rootInfo, err := os.Lstat(runtimeRoot)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 || rootInfo.Mode().Perm() != 0o700 {
		return "", fmt.Errorf("%w: host runtime directory is not private", ErrHostIncompatible)
	}
	directory, err := os.MkdirTemp(runtimeRoot, "host-")
	if err != nil {
		return "", fmt.Errorf("%w: create package runtime directory: %v", ErrHostUnavailable, err)
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		_ = os.RemoveAll(directory)
		return "", fmt.Errorf("%w: secure package runtime directory: %v", ErrHostUnavailable, err)
	}
	if len([]byte(filepath.Join(directory, "host.sock"))) > maxUnixSocketPathBytes {
		_ = os.RemoveAll(directory)
		return "", fmt.Errorf("%w: host socket path exceeds Unix socket limit", ErrHostIncompatible)
	}
	return directory, nil
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
	if h.client != nil {
		_ = h.client.Close()
	}
	if h.command != nil && h.command.Process != nil {
		if !hostExited(h) {
			if err := h.command.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
				return fmt.Errorf("%w: stop host process: %v", ErrHostUnavailable, err)
			}
		}
	}
	if h.waitDone != nil {
		select {
		case <-h.waitDone:
		case <-ctx.Done():
			return fmt.Errorf("%w: wait for host shutdown: %v", ErrHostUnavailable, ctx.Err())
		}
	}
	if err := os.RemoveAll(h.runtimeDir); err != nil {
		return fmt.Errorf("%w: remove host runtime directory: %v", ErrHostUnavailable, err)
	}
	return nil
}
