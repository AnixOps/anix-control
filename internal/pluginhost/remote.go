//go:build unix

package pluginhost

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	pluginhostv1 "github.com/AnixOps/anix-control/sdk/api/pluginhost/v1"
	"github.com/AnixOps/anix-control/sdk/moduletls"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
)

// Remote runtime defaults.
const (
	DefaultRemoteBindTimeout = 2 * time.Minute
	// remoteHealthCache keeps WebSocket per-frame checks and dispatch
	// selection off the network.
	remoteHealthCache = 2 * time.Second
	// remoteEjection keeps an instance out of rotation after a failure.
	remoteEjection   = 10 * time.Second
	remoteProbeLimit = 2 * time.Second
	remoteBindPoll   = 100 * time.Millisecond
)

// RemoteInstances lists the module instances bound to a generation; the
// kernel's packagebridge.ModuleBridge implements it.
type RemoteInstances interface {
	Instances(packageID string, generation uint64) []packagebridge.BoundInstance
}

// RemoteConfig enables the remote runtime of a Supervisor.
type RemoteConfig struct {
	Instances RemoteInstances
	// TLS is the kernel's client identity for dialing modules.
	TLS     moduletls.Source
	Cluster string
	// IsRemote reports whether a package's installation runs remotely.
	IsRemote func(ctx context.Context, packageID string) (bool, error)
	// BindTimeout bounds how long Start waits for a healthy instance.
	BindTimeout time.Duration
}

// remoteState is the Supervisor's remote runtime.
type remoteState struct {
	config RemoteConfig

	mu sync.Mutex
	// pending holds a starting remote generation, visible to Bind before it
	// replaces the active host.
	pending map[string]*hostProcess
}

// EnableRemote turns on the remote runtime. It must be called before the
// first Start of a remote package.
func (m *Supervisor) EnableRemote(config RemoteConfig) error {
	if m == nil {
		return ErrHostUnavailable
	}
	if config.Instances == nil || config.TLS.Certificate == nil || config.TLS.Roots == nil || config.IsRemote == nil {
		return errors.New("remote runtime needs instances, kernel TLS and a runtime resolver")
	}
	if !moduletls.ValidCluster(config.Cluster) {
		return errors.New("remote runtime needs a valid cluster name")
	}
	if config.BindTimeout <= 0 {
		config.BindTimeout = DefaultRemoteBindTimeout
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.remote = &remoteState{config: config, pending: make(map[string]*hostProcess)}
	return nil
}

// BindInstance admits a remote instance to the generation it may serve: a
// starting generation of its version, or the active one. It implements
// packagebridge.Binder.
func (m *Supervisor) BindInstance(_ context.Context, binding packagebridge.InstanceBinding) (*packagebridge.GenerationSession, error) {
	if m == nil {
		return nil, packagebridge.ErrNotRemote
	}
	m.mu.RLock()
	remote := m.remote
	active := m.hosts[hostKey(binding.PackageID, binding.Version)]
	m.mu.RUnlock()
	if remote == nil {
		return nil, packagebridge.ErrNotRemote
	}
	remote.mu.Lock()
	pending := remote.pending[hostKey(binding.PackageID, binding.Version)]
	remote.mu.Unlock()
	for _, host := range []*hostProcess{pending, active} {
		if host == nil || !host.remote || host.version != binding.Version || host.retiring.Load() {
			continue
		}
		session, ok := host.bridge.(*packagebridge.GenerationSession)
		if !ok || session.Closed() {
			return nil, packagebridge.ErrHostFenced
		}
		return session, nil
	}
	return nil, packagebridge.ErrNotRemote
}

func (m *Supervisor) remoteRuntime(ctx context.Context, packageID string) (bool, error) {
	m.mu.RLock()
	remote := m.remote
	m.mu.RUnlock()
	if remote == nil {
		return false, nil
	}
	return remote.config.IsRemote(ctx, packageID)
}

// startRemote activates a generation served by module instances. The
// kernel starts no process: it creates the generation's bridge state, waits
// until an instance has bound and reports healthy, then routes to it and
// retires the previous host. For a new version the previous generation keeps
// serving while new instances start; for the same version it is fenced first
// so that its instances bind to the new generation.
func (m *Supervisor) startRemote(ctx context.Context, ref ArtifactRef, generation uint64) error {
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()
	key := hostKey(ref.PackageID, ref.Version)
	m.mu.RLock()
	remote, closed, existing := m.remote, m.closed, m.hosts[key]
	m.mu.RUnlock()
	if closed || remote == nil {
		return ErrHostUnavailable
	}
	if existing != nil {
		if existing.generation > generation {
			return ErrGenerationUnavailable
		}
		if existing.generation == generation && existing.remote && sameArtifactRelease(existing.ref, ref) {
			if session, ok := existing.bridge.(*packagebridge.GenerationSession); ok && !session.Closed() {
				return nil
			}
		}
	}
	factory, ok := m.bridgeFactory.(packagebridge.GenerationSessionFactory)
	if !ok {
		return fmt.Errorf("%w: the remote runtime needs the package bridge", ErrHostIncompatible)
	}
	identity, err := moduletls.Module(remote.config.Cluster, ref.PackageID)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrHostIncompatible, err)
	}
	session, err := factory.NewGenerationSession(packagebridge.HostIdentity{
		PackageID: ref.PackageID, Version: ref.Version, Generation: generation, Remote: true,
	})
	if err != nil {
		return fmt.Errorf("%w: create package bridge: %v", ErrHostUnavailable, err)
	}
	pool := newRemotePool(ref.PackageID, ref.Version, generation, identity, remote.config, m.maxResponse)
	host := &hostProcess{
		packageID: ref.PackageID, version: ref.Version, generation: generation, ref: ref,
		client: pool, leaseID: pool.leaseID, bridge: session, remote: true, maxResponseBytes: m.maxResponse,
	}
	if existing != nil && existing.version == ref.Version {
		m.mu.Lock()
		if current := m.hosts[key]; current == existing {
			m.retireHostLocked(ctx, key, existing)
		}
		m.mu.Unlock()
	}
	remote.mu.Lock()
	remote.pending[key] = host
	remote.mu.Unlock()
	defer func() {
		remote.mu.Lock()
		if remote.pending[key] == host {
			delete(remote.pending, key)
		}
		remote.mu.Unlock()
	}()
	if err := pool.awaitHealthy(ctx, remote.config.BindTimeout); err != nil {
		_ = session.Close()
		_ = pool.Close()
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		_ = session.Close()
		_ = pool.Close()
		return ErrHostUnavailable
	}
	if current := m.hosts[key]; current != nil {
		m.retireHostLocked(ctx, key, current)
	}
	m.hosts[key] = host
	m.updateStats(host, func(stats *HostStats) {
		stats.Starts++
		stats.State = HostStateRunning
	})
	m.logger()("plugin host [pkg:%s v:%s gen:%d] serving from remote instances", ref.PackageID, ref.Version, generation)
	return nil
}

// retireHostLocked stops a host that a lifecycle operation replaces. The
// caller holds m.mu.
func (m *Supervisor) retireHostLocked(ctx context.Context, key string, host *hostProcess) {
	m.cancelRecoveryLocked(host)
	host.retiring.Store(true)
	if err := host.stop(ctx); err != nil {
		m.logger()("plugin host [pkg:%s v:%s gen:%d] stop while replacing: %v", host.packageID, host.version, host.generation, err)
	}
	delete(m.hosts, key)
	m.recordStateLocked(host, HostStateStopped)
}

// remotePool is the hostRPCClient of a remote host: it spreads calls over
// the instances bound to its generation.
type remotePool struct {
	packageID  string
	version    string
	generation uint64
	identity   moduletls.Identity
	config     RemoteConfig
	maxRecv    int
	leaseID    string

	mu        sync.Mutex
	clients   map[string]*remoteClient
	next      int
	ejected   map[string]time.Time
	health    HostHealth
	healthAt  time.Time
	closed    bool
	probeLock sync.Mutex
}

type remoteClient struct {
	address string
	client  *hostClient
}

func newRemotePool(packageID, version string, generation uint64, identity moduletls.Identity, config RemoteConfig, maxResponse int64) *remotePool {
	return &remotePool{
		packageID: packageID, version: version, generation: generation, identity: identity, config: config,
		maxRecv: hostReceiveMessageLimit(maxResponse),
		// The pool's lease is stable for the generation; each instance's own
		// lease is checked against the one it bound with.
		leaseID: "remote-" + packageID + "-" + strconv.FormatUint(generation, 10),
		clients: make(map[string]*remoteClient), ejected: make(map[string]time.Time),
	}
}

func (p *remotePool) bound() []packagebridge.BoundInstance {
	return p.config.Instances.Instances(p.packageID, p.generation)
}

// clientFor returns the connection to an instance, redialing when its
// advertised address changed.
func (p *remotePool) clientFor(instance packagebridge.BoundInstance) (*hostClient, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil, ErrHostUnavailable
	}
	if existing := p.clients[instance.InstanceID]; existing != nil {
		if existing.address == instance.AdvertiseAddr {
			return existing.client, nil
		}
		_ = existing.client.Close()
		delete(p.clients, instance.InstanceID)
	}
	tlsConfig := p.config.TLS.ClientConfig(moduletls.AcceptExactly(p.identity))
	connection, err := grpc.NewClient(instance.AdvertiseAddr,
		grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(p.maxRecv)),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{Time: 10 * time.Second, Timeout: 5 * time.Second, PermitWithoutStream: true}),
	)
	if err != nil {
		return nil, hostClientError(err)
	}
	client := &hostClient{connection: connection, rpc: pluginhostv1.NewControlPackageHostClient(connection)}
	p.clients[instance.InstanceID] = &remoteClient{address: instance.AdvertiseAddr, client: client}
	return client, nil
}

// pick chooses the next bound instance that is not ejected.
func (p *remotePool) pick() (*hostClient, packagebridge.BoundInstance, error) {
	instances := p.bound()
	if len(instances) == 0 {
		return nil, packagebridge.BoundInstance{}, fmt.Errorf("%w: no instance of %s is bound", ErrHostUnavailable, p.packageID)
	}
	now := time.Now()
	p.mu.Lock()
	start := p.next % len(instances)
	p.next = (p.next + 1) % (1 << 30)
	p.mu.Unlock()
	for offset := range len(instances) {
		instance := instances[(start+offset)%len(instances)]
		p.mu.Lock()
		ejectedAt, ejected := p.ejected[instance.InstanceID]
		p.mu.Unlock()
		if ejected && now.Sub(ejectedAt) < remoteEjection {
			continue
		}
		client, err := p.clientFor(instance)
		if err != nil {
			p.eject(instance.InstanceID)
			continue
		}
		return client, instance, nil
	}
	return nil, packagebridge.BoundInstance{}, fmt.Errorf("%w: every instance of %s is failing", ErrHostUnavailable, p.packageID)
}

func (p *remotePool) eject(instanceID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ejected[instanceID] = time.Now()
	p.healthAt = time.Time{}
}

// observe ejects an instance whose call failed at the transport.
func (p *remotePool) observe(instance packagebridge.BoundInstance, err error) {
	if errors.Is(err, ErrHostUnavailable) {
		p.eject(instance.InstanceID)
	}
}

func (p *remotePool) Dispatch(ctx context.Context, input DispatchInput) (DispatchOutput, error) {
	client, instance, err := p.pick()
	if err != nil {
		return DispatchOutput{}, err
	}
	output, err := client.Dispatch(ctx, input)
	p.observe(instance, err)
	return output, err
}

func (p *remotePool) Migrate(ctx context.Context, input MigrationInput) (MigrationOutput, error) {
	client, instance, err := p.pick()
	if err != nil {
		return MigrationOutput{}, err
	}
	output, err := client.Migrate(ctx, input)
	p.observe(instance, err)
	return output, err
}

func (p *remotePool) OpenWebSocket(ctx context.Context, input WebSocketInput) (webSocketTransport, error) {
	client, instance, err := p.pick()
	if err != nil {
		return nil, err
	}
	stream, err := client.OpenWebSocket(ctx, input)
	p.observe(instance, err)
	return stream, err
}

// Health reports the generation healthy when at least one bound instance
// answers healthy with the lease it bound with. Results are cached briefly
// so that WebSocket relays, which check health per frame, stay local.
func (p *remotePool) Health(ctx context.Context, generation uint64) (HostHealth, error) {
	if generation != p.generation {
		return HostHealth{}, ErrGenerationUnavailable
	}
	p.mu.Lock()
	if !p.healthAt.IsZero() && time.Since(p.healthAt) < remoteHealthCache {
		health := p.health
		p.mu.Unlock()
		return health, nil
	}
	p.mu.Unlock()
	p.probeLock.Lock()
	defer p.probeLock.Unlock()
	health := HostHealth{LeaseID: p.leaseID}
	for _, instance := range p.bound() {
		client, err := p.clientFor(instance)
		if err != nil {
			continue
		}
		probeCtx, cancel := context.WithTimeout(ctx, remoteProbeLimit)
		answer, err := client.Health(probeCtx, generation)
		cancel()
		if err != nil || !answer.Healthy || answer.LeaseID != instance.LeaseID {
			p.eject(instance.InstanceID)
			continue
		}
		p.mu.Lock()
		delete(p.ejected, instance.InstanceID)
		p.mu.Unlock()
		if !health.Healthy {
			health.Healthy, health.DetailsJSON = true, answer.DetailsJSON
		}
	}
	p.mu.Lock()
	p.health, p.healthAt = health, time.Now()
	p.mu.Unlock()
	return health, nil
}

// Drain drains every bound instance; the generation is drained when all of
// them are.
func (p *remotePool) Drain(ctx context.Context, generation uint64, deadline time.Time) (DrainResult, error) {
	result := DrainResult{Drained: true}
	for _, instance := range p.bound() {
		client, err := p.clientFor(instance)
		if err != nil {
			continue
		}
		answer, err := client.Drain(ctx, generation, deadline)
		if err != nil {
			continue
		}
		result.Drained = result.Drained && answer.Drained
		result.InFlight += answer.InFlight
	}
	return result, nil
}

func (p *remotePool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	var closeErr error
	for id, client := range p.clients {
		closeErr = errors.Join(closeErr, client.client.Close())
		delete(p.clients, id)
	}
	return closeErr
}

// awaitHealthy waits until an instance has bound and answers healthy.
func (p *remotePool) awaitHealthy(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	for {
		if len(p.bound()) > 0 {
			p.mu.Lock()
			p.healthAt = time.Time{}
			p.mu.Unlock()
			if health, err := p.Health(ctx, p.generation); err == nil && health.Healthy {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%w: no healthy instance of %s %s bound within %s", ErrHostUnavailable, p.packageID, p.version, timeout)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%w: %v", ErrHostUnavailable, ctx.Err())
		case <-time.After(remoteBindPoll):
		}
	}
}
