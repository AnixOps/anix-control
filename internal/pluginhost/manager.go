//go:build unix

// Package pluginhost supervises local Control package processes. Package
// requests enter only after the kernel has completed admission and routing.
package pluginhost

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
)

var (
	ErrHostUnavailable       = errors.New("plugin host unavailable")
	ErrHostNotFound          = errors.New("plugin host not found")
	ErrHostIncompatible      = errors.New("plugin host incompatible")
	ErrPackageFailed         = errors.New("plugin package failed")
	ErrGenerationUnavailable = errors.New("plugin host generation unavailable")
	defaultManager           struct {
		sync.RWMutex
		manager Manager
	}
)

type DispatchInput struct {
	PackageID        string
	Version          string
	Generation       uint64
	RequestID        string
	IdempotencyKey   string
	RouteID          string
	Method           string
	Body             []byte
	PrincipalJSON    []byte
	Metadata         RequestMetadata
	BridgeCapability []byte
	Deadline         time.Time
}

// RequestMetadata preserves the request address independently from the
// package route identifier. Query values remain multi-valued because v2
// clients may repeat a key.
type RequestMetadata struct {
	Path                             string              `json:"path,omitempty"`
	Query                            map[string][]string `json:"query,omitempty"`
	Headers                          map[string][]string `json:"headers,omitempty"`
	PathParams                       map[string]string   `json:"path_params,omitempty"`
	ClientIP                         string              `json:"client_ip,omitempty"`
	UserAgent                        string              `json:"user_agent,omitempty"`
	NodeID                           uint                `json:"node_id,omitempty"`
	TrustedAgentWebSocketAuth        bool                `json:"trusted_agent_websocket_auth,omitempty"`
	TrustedAgentWebSocketForwardNode bool                `json:"trusted_agent_websocket_forward_node,omitempty"`
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

// MigrationManager is intentionally separate from Manager so existing route
// dispatch consumers do not gain a migration capability by accident.
type MigrationManager interface {
	Migrate(context.Context, MigrationInput, MigrationCheckpointRecorder) (MigrationOutput, error)
}

// WebSocketManager is separate from unary route dispatch so callers must
// consciously opt into the bidirectional package-host transport.
type WebSocketManager interface {
	OpenWebSocket(context.Context, WebSocketInput) (*WebSocketRelay, error)
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

type hostRPCClient interface {
	Dispatch(context.Context, DispatchInput) (DispatchOutput, error)
	Migrate(context.Context, MigrationInput) (MigrationOutput, error)
	Health(context.Context, uint64) (HostHealth, error)
	Drain(context.Context, uint64, time.Time) (DrainResult, error)
	OpenWebSocket(context.Context, WebSocketInput) (webSocketTransport, error)
	Close() error
}

type DrainResult struct {
	Drained  bool
	InFlight uint64
}

type ManagerConfig struct {
	RuntimeDir     string
	StartupTimeout time.Duration
	BridgeFactory  packagebridge.SessionFactory
}

type Supervisor struct {
	// lifecycleMu serializes lifecycle transitions (Start, Drain, Stop,
	// Shutdown, and watchdog restarts). It is always taken before mu, and it
	// lets a watchdog restart run without holding mu so request dispatch to
	// other packages is not blocked while a crashed host restarts.
	lifecycleMu sync.Mutex
	// mu guards hosts, closed, runtimeRoot, and the supervision fields of every
	// hostProcess.
	mu          sync.RWMutex
	hosts       map[string]*hostProcess
	runtimeRoot *runtimeRoot
	// runtimeDir is diagnostic compatibility state. Runtime filesystem work is
	// always relative to runtimeRoot's pinned descriptor.
	runtimeDir     string
	startupTimeout time.Duration
	bridgeFactory  packagebridge.SessionFactory
	closed         bool

	// Watchdog and stop timings. NewManager sets the production defaults;
	// tests shorten them before the first Start.
	stopGrace        time.Duration
	restartBaseDelay time.Duration
	restartMaxDelay  time.Duration
	restartWindow    time.Duration
	maxRestarts      int
	logf             func(string, ...any)
	// restartCtx bounds watchdog restarts; Shutdown cancels it.
	restartCtx    context.Context
	cancelRestart context.CancelFunc
	recoverySeq   uint64 // guarded by mu

	statsMu sync.Mutex
	stats   map[string]*HostStats
}

type hostProcess struct {
	packageID         string
	version           string
	generation        uint64
	ref               ArtifactRef
	runtimeDir        *hostRuntimeDir
	socketPath        string
	command           *exec.Cmd
	waitDone          chan struct{}
	waitErr           error
	client            hostRPCClient
	leaseID           string
	relayMu           sync.Mutex
	relays            map[*WebSocketRelay]struct{}
	draining          bool
	relayHealthCancel context.CancelFunc
	bridge            *packagebridge.Session
	stopGrace         time.Duration
	// retiring is set once a lifecycle operation (Drain, Stop, replacement,
	// Shutdown) owns this process, so its exit is expected and never restarted.
	retiring atomic.Bool

	// Supervision state, guarded by Supervisor.mu.
	supervision    hostSupervisionState
	restartHistory []time.Time
	recoveryTimer  *time.Timer
	recoveryToken  uint64
}

func NewManager(config ManagerConfig) (*Supervisor, error) {
	runtimeRoot, err := newRuntimeRoot(config.RuntimeDir)
	if err != nil {
		return nil, err
	}
	if config.StartupTimeout <= 0 {
		config.StartupTimeout = 5 * time.Second
	}
	restartCtx, cancelRestart := context.WithCancel(context.Background())
	return &Supervisor{
		hosts: make(map[string]*hostProcess), runtimeRoot: runtimeRoot, runtimeDir: runtimeRoot.configuredPath,
		startupTimeout: config.StartupTimeout, bridgeFactory: config.BridgeFactory,
		stopGrace: defaultHostStopGrace, restartBaseDelay: defaultRestartBaseDelay, restartMaxDelay: defaultRestartMaxDelay,
		restartWindow: defaultRestartWindow, maxRestarts: defaultMaxRestarts, logf: log.Printf,
		restartCtx: restartCtx, cancelRestart: cancelRestart, stats: make(map[string]*HostStats),
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
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()
	// Start keeps mu for the whole start, as before: a request for the new
	// generation waits for the host instead of failing while it boots.
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
			if !sameArtifactRelease(existing.ref, ref) {
				return fmt.Errorf("%w: active host artifact does not match", ErrHostIncompatible)
			}
			if !hostExited(existing) {
				// An explicit lifecycle operation resets the restart budget.
				existing.restartHistory = nil
				return nil
			}
		}
		// Replace an older generation, or an exited, restarting, or failed
		// host at the same generation. Either way a pending watchdog restart
		// is cancelled and the restart budget starts over.
		m.cancelRecoveryLocked(existing)
		existing.retiring.Store(true)
		if err := existing.stop(ctx); err != nil {
			return err
		}
		delete(m.hosts, key)
		m.recordStateLocked(existing, HostStateStopped)
	}
	host, err := startHostProcess(ctx, m.runtimeRoot, ref, generation, m.hostStartOptions())
	if err != nil {
		return err
	}
	m.installHostLocked(key, host)
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
	capability, err := host.mintDispatchCapability(input)
	if err != nil {
		return DispatchOutput{}, err
	}
	if len(capability) > 0 {
		input.BridgeCapability = capability
		defer host.bridge.Revoke(capability)
	}
	return host.client.Dispatch(ctx, input)
}

func (h *hostProcess) mintDispatchCapability(input DispatchInput) ([]byte, error) {
	if h == nil || h.bridge == nil {
		return nil, nil
	}
	metadata, err := marshalRequestMetadata(input.Metadata)
	if err != nil {
		return nil, err
	}
	capability, err := h.bridge.Mint(packagebridge.Request{
		RequestID: input.RequestID, RouteID: input.RouteID, Method: input.Method,
		Body: input.Body, PrincipalJSON: input.PrincipalJSON, MetadataJSON: metadata, Deadline: input.Deadline,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: mint package bridge capability: %v", ErrHostUnavailable, err)
	}
	return capability, nil
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
	// lifecycleMu keeps Start, Stop, and watchdog restarts out while the
	// drain RPC runs, so mu is held only for the lookup.
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()
	m.mu.Lock()
	host, err := m.hostForNewerLifecycleGeneration(packageID, version, generation)
	if err != nil {
		m.mu.Unlock()
		return err
	}
	// A drained host is being retired: it must not be restarted if it exits.
	host.retiring.Store(true)
	m.cancelRecoveryLocked(host)
	exited := hostExited(host)
	if exited {
		host.supervision = hostSupervisionExited
		m.recordStateLocked(host, HostStateExited)
	}
	m.mu.Unlock()
	host.beginWebSocketDrain()
	if exited {
		// An exited host has nothing in flight; the following Stop cleans it up.
		return nil
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
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()
	m.mu.Lock()
	host := m.hosts[hostKey(packageID, version)]
	if host == nil {
		m.mu.Unlock()
		return fmt.Errorf("%w: %w", ErrHostUnavailable, ErrHostNotFound)
	}
	if host.version != version || generation == 0 || generation <= host.generation {
		m.mu.Unlock()
		return ErrGenerationUnavailable
	}
	m.cancelRecoveryLocked(host)
	host.retiring.Store(true)
	delete(m.hosts, hostKey(packageID, version))
	m.recordStateLocked(host, HostStateStopped)
	m.mu.Unlock()
	// The host is already unreachable for dispatch, so the SIGTERM grace
	// period runs without blocking requests to other packages.
	return host.stop(ctx)
}

func (m *Supervisor) Rollback(ctx context.Context, packageID, version string, generation uint64) error {
	return m.Stop(ctx, packageID, version, generation)
}

func (m *Supervisor) Shutdown(ctx context.Context) error {
	if m == nil {
		return nil
	}
	// Abort a watchdog restart that is still booting a host before waiting
	// for the lifecycle lock it holds.
	if m.cancelRestart != nil {
		m.cancelRestart()
	}
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	hosts := make([]*hostProcess, 0, len(m.hosts))
	for _, host := range m.hosts {
		m.cancelRecoveryLocked(host)
		host.retiring.Store(true)
		m.recordStateLocked(host, HostStateStopped)
		hosts = append(hosts, host)
	}
	m.hosts = make(map[string]*hostProcess)
	runtimeRoot := m.runtimeRoot
	m.runtimeRoot = nil
	m.mu.Unlock()

	// Stop hosts concurrently so SIGTERM grace periods overlap.
	var (
		shutdownErr error
		errMu       sync.Mutex
		stopped     sync.WaitGroup
	)
	for _, host := range hosts {
		stopped.Add(1)
		go func() {
			defer stopped.Done()
			if err := host.stop(ctx); err != nil {
				errMu.Lock()
				shutdownErr = errors.Join(shutdownErr, err)
				errMu.Unlock()
			}
		}()
	}
	stopped.Wait()
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
	var exitedState hostSupervisionState
	exited := host != nil && hostExited(host)
	if exited {
		exitedState = host.supervision
	}
	m.mu.RUnlock()
	if host == nil {
		return nil, ErrHostUnavailable
	}
	if host.version != version || generation == 0 || host.generation != generation {
		return nil, ErrGenerationUnavailable
	}
	if exited {
		// Fail fast instead of dialing the socket of a dead process.
		return nil, fmt.Errorf("%w: package %s generation %d host process %s", ErrHostUnavailable, packageID, generation, exitedState.describe())
	}
	if host.client == nil {
		return nil, ErrHostUnavailable
	}
	if host.isWebSocketDraining() {
		return nil, ErrHostUnavailable
	}
	return host, nil
}

// hostForNewerLifecycleGeneration authorizes retirement only from a later
// durable transition. Request dispatch always uses hostForGeneration instead.
// The caller holds m.mu for the lookup and lifecycleMu for the lifecycle RPC.
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
