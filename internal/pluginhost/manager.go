// Package pluginhost supervises local Control package processes. Package
// requests enter only after the kernel has completed admission and routing.
package pluginhost

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"time"
)

var (
	ErrHostUnavailable       = errors.New("plugin host unavailable")
	ErrHostNotFound          = errors.New("plugin host not found")
	ErrHostIncompatible      = errors.New("plugin host incompatible")
	ErrGenerationUnavailable = errors.New("plugin host generation unavailable")
	defaultManager           struct {
		sync.RWMutex
		manager Manager
	}
)

type DispatchInput struct {
	PackageID      string
	Version        string
	Generation     uint64
	RequestID      string
	IdempotencyKey string
	RouteID        string
	Method         string
	Body           []byte
	PrincipalJSON  []byte
	Deadline       time.Time
}

type DispatchOutput struct {
	StatusCode  uint32
	Body        []byte
	Headers     []Header
	OperationID string
	FailureCode string
}

type Header struct {
	Name  string
	Value string
}

type Manager interface {
	Start(context.Context, ArtifactRef, uint64) error
	Dispatch(context.Context, DispatchInput) (DispatchOutput, error)
	Health(context.Context, string, string, uint64) (HostHealth, error)
	Drain(context.Context, string, string, uint64, time.Time) error
	Stop(context.Context, string, string, uint64) error
	Rollback(context.Context, string, string, uint64) error
}

var _ Manager = (*Supervisor)(nil)

// SetDefaultManager installs the process-wide supervisor used by HTTP route
// handlers. A nil value deliberately leaves route dispatch fail-closed.
func SetDefaultManager(manager Manager) {
	defaultManager.Lock()
	defer defaultManager.Unlock()
	defaultManager.manager = manager
}

func DefaultManager() Manager {
	defaultManager.RLock()
	defer defaultManager.RUnlock()
	return defaultManager.manager
}

type HostHealth struct {
	Healthy     bool
	LeaseID     string
	DetailsJSON string
}

type DrainResult struct {
	Drained  bool
	InFlight uint64
}

type ManagerConfig struct {
	RuntimeDir     string
	StartupTimeout time.Duration
}

type Supervisor struct {
	mu          sync.RWMutex
	hosts       map[string]*hostProcess
	runtimeRoot *runtimeRoot
	// runtimeDir is diagnostic compatibility state. Runtime filesystem work is
	// always relative to runtimeRoot's pinned descriptor.
	runtimeDir     string
	startupTimeout time.Duration
	closed         bool
}

type hostProcess struct {
	packageID  string
	version    string
	generation uint64
	ref        ArtifactRef
	runtimeDir *hostRuntimeDir
	socketPath string
	command    *exec.Cmd
	waitDone   chan struct{}
	waitErr    error
	client     *hostClient
	leaseID    string
}

func NewManager(config ManagerConfig) (*Supervisor, error) {
	runtimeRoot, err := newRuntimeRoot(config.RuntimeDir)
	if err != nil {
		return nil, err
	}
	if config.StartupTimeout <= 0 {
		config.StartupTimeout = 5 * time.Second
	}
	return &Supervisor{
		hosts: make(map[string]*hostProcess), runtimeRoot: runtimeRoot, runtimeDir: runtimeRoot.configuredPath, startupTimeout: config.StartupTimeout,
	}, nil
}

func (m *Supervisor) Start(ctx context.Context, ref ArtifactRef, generation uint64) error {
	if m == nil {
		return ErrHostUnavailable
	}
	if generation == 0 {
		return ErrGenerationUnavailable
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%w: %v", ErrHostUnavailable, err)
	}
	if err := verifyArtifactRef(ref); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.runtimeRoot == nil {
		return ErrHostUnavailable
	}
	key := hostKey(ref.PackageID, ref.Version)
	if existing := m.hosts[key]; existing != nil {
		if existing.generation > generation {
			return ErrGenerationUnavailable
		}
		if existing.generation == generation {
			if existing.ref != ref {
				return fmt.Errorf("%w: active host artifact does not match", ErrHostIncompatible)
			}
			if !hostExited(existing) {
				return nil
			}
			if err := existing.stop(ctx); err != nil {
				return err
			}
			delete(m.hosts, key)
		} else {
			if err := existing.stop(ctx); err != nil {
				return err
			}
			delete(m.hosts, key)
		}
	}
	host, err := startHostProcess(ctx, m.runtimeRoot, ref, generation, m.startupTimeout)
	if err != nil {
		return err
	}
	m.hosts[key] = host
	return nil
}

func hostExited(host *hostProcess) bool {
	if host == nil || host.waitDone == nil {
		return false
	}
	select {
	case <-host.waitDone:
		return true
	default:
		return false
	}
}

func (m *Supervisor) Dispatch(ctx context.Context, input DispatchInput) (DispatchOutput, error) {
	if m == nil {
		return DispatchOutput{}, ErrHostUnavailable
	}
	host, err := m.hostForGeneration(input.PackageID, input.Version, input.Generation)
	if err != nil {
		return DispatchOutput{}, err
	}
	return host.client.Dispatch(ctx, input)
}

func (m *Supervisor) Health(ctx context.Context, packageID, version string, generation uint64) (HostHealth, error) {
	host, err := m.hostForGeneration(packageID, version, generation)
	if err != nil {
		return HostHealth{}, err
	}
	health, err := host.client.Health(ctx, generation)
	if err != nil {
		return HostHealth{}, err
	}
	if !health.Healthy || health.LeaseID == "" || health.LeaseID != host.leaseID {
		return HostHealth{}, fmt.Errorf("%w: health lease does not match active generation", ErrHostIncompatible)
	}
	return health, nil
}

func (m *Supervisor) Drain(ctx context.Context, packageID, version string, generation uint64, deadline time.Time) error {
	if m == nil {
		return ErrHostUnavailable
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	host, err := m.hostForNewerLifecycleGeneration(packageID, version, generation)
	if err != nil {
		return err
	}
	result, err := host.client.Drain(ctx, generation, deadline)
	if err != nil {
		return err
	}
	if !result.Drained || result.InFlight != 0 {
		return fmt.Errorf("%w: host did not drain", ErrHostUnavailable)
	}
	return nil
}

func (m *Supervisor) Stop(ctx context.Context, packageID, version string, generation uint64) error {
	if m == nil {
		return ErrHostUnavailable
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	host := m.hosts[hostKey(packageID, version)]
	if host == nil {
		return fmt.Errorf("%w: %w", ErrHostUnavailable, ErrHostNotFound)
	}
	if host.version != version || generation == 0 || generation <= host.generation {
		return ErrGenerationUnavailable
	}
	delete(m.hosts, hostKey(packageID, version))
	return host.stop(ctx)
}

func (m *Supervisor) Rollback(ctx context.Context, packageID, version string, generation uint64) error {
	return m.Stop(ctx, packageID, version, generation)
}

func (m *Supervisor) Shutdown(ctx context.Context) error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	hosts := make([]*hostProcess, 0, len(m.hosts))
	for _, host := range m.hosts {
		hosts = append(hosts, host)
	}
	m.hosts = make(map[string]*hostProcess)
	runtimeRoot := m.runtimeRoot
	m.runtimeRoot = nil
	m.mu.Unlock()

	var shutdownErr error
	for _, host := range hosts {
		shutdownErr = errors.Join(shutdownErr, host.stop(ctx))
	}
	if runtimeRoot != nil {
		shutdownErr = errors.Join(shutdownErr, runtimeRoot.Close())
	}
	return shutdownErr
}

// Close retires every host and releases the pinned runtime-root descriptor.
func (m *Supervisor) Close() error {
	return m.Shutdown(context.Background())
}

func (m *Supervisor) hostForGeneration(packageID, version string, generation uint64) (*hostProcess, error) {
	if m == nil {
		return nil, ErrHostUnavailable
	}
	m.mu.RLock()
	host := m.hosts[hostKey(packageID, version)]
	m.mu.RUnlock()
	if host == nil {
		return nil, ErrHostUnavailable
	}
	if host.version != version || generation == 0 || host.generation != generation {
		return nil, ErrGenerationUnavailable
	}
	if host.client == nil {
		return nil, ErrHostUnavailable
	}
	return host, nil
}

// hostForNewerLifecycleGeneration authorizes retirement only from a later
// durable transition. Request dispatch always uses hostForGeneration instead.
// The caller holds m.mu for the duration of the lifecycle RPC.
func (m *Supervisor) hostForNewerLifecycleGeneration(packageID, version string, generation uint64) (*hostProcess, error) {
	host := m.hosts[hostKey(packageID, version)]
	if host == nil {
		return nil, fmt.Errorf("%w: %w", ErrHostUnavailable, ErrHostNotFound)
	}
	if host.version != version || generation == 0 || generation <= host.generation {
		return nil, ErrGenerationUnavailable
	}
	if host.client == nil {
		return nil, ErrHostUnavailable
	}
	return host, nil
}

func hostKey(packageID, _ string) string {
	return packageID
}
