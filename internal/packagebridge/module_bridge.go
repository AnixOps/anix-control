package packagebridge

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"sort"
	"sync"
	"time"

	packagebridgev1 "github.com/AnixOps/anix-control/v4/api/packagebridge/v1"
	"github.com/AnixOps/anix-control/v4/pkg/moduletls"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// SessionMetadataKey carries the Bind session token on every later bridge
// call of a remote module instance.
const SessionMetadataKey = "x-anix-bridge-session"

// Module bridge timing defaults.
const (
	DefaultHeartbeatInterval = 5 * time.Second
	DefaultSessionTTL        = 15 * time.Second
	sessionTokenBytes        = 32
)

// ErrNotRemote means the package has no enabled installation that runs in the
// remote runtime at the requested version.
var ErrNotRemote = errors.New("package has no enabled remote installation at this version")

// InstanceBinding is what a remote module instance reports when it binds.
type InstanceBinding struct {
	PackageID     string
	Version       string
	InstanceID    string
	AdvertiseAddr string
	LeaseID       string
	// ImageDigest is advisory: the kernel cannot prove what a remote
	// process runs.
	ImageDigest string
}

// Binder admits a remote instance and returns the generation it serves. The
// remote runtime manager implements it; it must check that the installation
// is enabled, runs remotely and desires this version.
type Binder interface {
	BindInstance(ctx context.Context, binding InstanceBinding) (*GenerationSession, error)
}

// ModuleBridgeOptions configures a ModuleBridge.
type ModuleBridgeOptions struct {
	// Cluster is the SPIFFE cluster whose modules may bind.
	Cluster           string
	HeartbeatInterval time.Duration
	SessionTTL        time.Duration
	Now               func() time.Time
}

// BoundInstance is one remote module process attached to a generation.
type BoundInstance struct {
	InstanceBinding
	Identity   moduletls.Identity
	Generation uint64
	BoundAt    time.Time
	LastSeen   time.Time

	token      string
	generation *GenerationSession
}

// ModuleBridge serves KernelPackageBridge to remote module instances over
// the kernel's mTLS module listener. An instance is authenticated by its
// client certificate identity; Bind admits it to a generation and returns a
// session token that every later call presents in SessionMetadataKey. The
// token is pinned to the module identity rather than to one certificate,
// because instances renew their certificates while the session lives.
type ModuleBridge struct {
	packagebridgev1.UnimplementedKernelPackageBridgeServer

	binder   Binder
	cluster  string
	interval time.Duration
	ttl      time.Duration
	now      func() time.Time

	mu        sync.Mutex
	instances map[string]*BoundInstance
}

var _ packagebridgev1.KernelPackageBridgeServer = (*ModuleBridge)(nil)

// NewModuleBridge returns a bridge that admits instances through binder. A
// nil binder rejects every Bind.
func NewModuleBridge(binder Binder, options ModuleBridgeOptions) (*ModuleBridge, error) {
	if !moduletls.ValidCluster(options.Cluster) {
		return nil, errors.New("module bridge needs a valid cluster name")
	}
	bridge := &ModuleBridge{
		binder: binder, cluster: options.Cluster, interval: options.HeartbeatInterval, ttl: options.SessionTTL,
		now: options.Now, instances: make(map[string]*BoundInstance),
	}
	if bridge.interval <= 0 {
		bridge.interval = DefaultHeartbeatInterval
	}
	if bridge.ttl <= bridge.interval {
		bridge.ttl = 3 * bridge.interval
	}
	if bridge.now == nil {
		bridge.now = time.Now
	}
	return bridge, nil
}

// Bind admits the calling instance to the generation its package runs.
func (b *ModuleBridge) Bind(ctx context.Context, request *packagebridgev1.BindRequest) (*packagebridgev1.BindResponse, error) {
	identity, err := b.peerIdentity(ctx)
	if err != nil {
		return nil, err
	}
	if identity.PackageID != request.GetPackageId() {
		return nil, status.Error(codes.PermissionDenied, "the client certificate belongs to another package")
	}
	binding := InstanceBinding{
		PackageID: request.GetPackageId(), Version: request.GetPackageVersion(), InstanceID: request.GetInstanceId(),
		AdvertiseAddr: request.GetAdvertiseAddr(), LeaseID: request.GetLeaseId(), ImageDigest: request.GetImageDigest(),
	}
	if !safeIdentifier(binding.Version) || !safeIdentifier(binding.InstanceID) || binding.AdvertiseAddr == "" ||
		len(binding.AdvertiseAddr) > 255 || binding.LeaseID == "" || len(binding.LeaseID) > 128 || len(binding.ImageDigest) > 128 {
		return nil, status.Error(codes.InvalidArgument, "bind request is invalid")
	}
	if b.binder == nil {
		return nil, status.Error(codes.FailedPrecondition, ErrNotRemote.Error())
	}
	generation, err := b.binder.BindInstance(ctx, binding)
	if err != nil {
		if errors.Is(err, ErrNotRemote) {
			return nil, status.Error(codes.FailedPrecondition, ErrNotRemote.Error())
		}
		if errors.Is(err, ErrHostFenced) {
			return nil, status.Error(codes.PermissionDenied, "package host generation is fenced")
		}
		return nil, status.Error(codes.Internal, "bind failed")
	}
	hostIdentity := generation.Identity()
	if generation.Closed() || hostIdentity.PackageID != binding.PackageID || hostIdentity.Version != binding.Version {
		return nil, status.Error(codes.PermissionDenied, "package host generation is fenced")
	}
	raw := make([]byte, sessionTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return nil, status.Error(codes.Internal, "bind failed")
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	now := b.now()
	instance := &BoundInstance{
		InstanceBinding: binding, Identity: identity, Generation: hostIdentity.Generation,
		BoundAt: now, LastSeen: now, token: token, generation: generation,
	}
	b.mu.Lock()
	// A rebinding process (after a kernel restart or a fenced generation)
	// replaces its previous session.
	for existing, bound := range b.instances {
		if bound.PackageID == binding.PackageID && bound.InstanceID == binding.InstanceID {
			delete(b.instances, existing)
		}
	}
	b.instances[token] = instance
	b.mu.Unlock()
	return &packagebridgev1.BindResponse{
		SessionToken: raw, RouteGeneration: hostIdentity.Generation,
		HeartbeatIntervalMillis: b.interval.Milliseconds(), SessionTtlMillis: b.ttl.Milliseconds(),
	}, nil
}

// Heartbeat keeps a session alive and reports whether its generation is
// fenced or draining. A fenced instance must Bind again.
func (b *ModuleBridge) Heartbeat(ctx context.Context, _ *packagebridgev1.HeartbeatRequest) (*packagebridgev1.HeartbeatResponse, error) {
	instance, err := b.instance(ctx, true)
	if err != nil {
		return nil, err
	}
	fenced := instance.generation.Closed()
	if fenced {
		b.drop(instance.token)
	}
	return &packagebridgev1.HeartbeatResponse{
		RouteGeneration: instance.Generation, Fenced: fenced, Draining: instance.generation.Draining(),
	}, nil
}

func (b *ModuleBridge) Invoke(ctx context.Context, request *packagebridgev1.InvokeRequest) (*packagebridgev1.InvokeResponse, error) {
	instance, err := b.instance(ctx, false)
	if err != nil {
		return nil, err
	}
	return instance.generation.invoke(ctx, request)
}

func (b *ModuleBridge) OpenWebSocket(stream packagebridgev1.KernelPackageBridge_OpenWebSocketServer) error {
	instance, err := b.instance(stream.Context(), false)
	if err != nil {
		return err
	}
	return instance.generation.openWebSocket(stream)
}

func (b *ModuleBridge) GetPackageConfig(ctx context.Context, request *packagebridgev1.GetPackageConfigRequest) (*packagebridgev1.GetPackageConfigResponse, error) {
	instance, err := b.instance(ctx, false)
	if err != nil {
		return nil, err
	}
	return instance.generation.getPackageConfig(ctx, request)
}

func (b *ModuleBridge) LeaseStorage(ctx context.Context, request *packagebridgev1.LeaseStorageRequest) (*packagebridgev1.LeaseStorageResponse, error) {
	instance, err := b.instance(ctx, false)
	if err != nil {
		return nil, err
	}
	return instance.generation.leaseStorage(ctx, request)
}

// Instances returns the live instances bound to generation of packageID,
// ordered by instance id, for dispatch.
func (b *ModuleBridge) Instances(packageID string, generation uint64) []BoundInstance {
	b.Sweep()
	b.mu.Lock()
	defer b.mu.Unlock()
	var instances []BoundInstance
	for _, instance := range b.instances {
		if instance.PackageID == packageID && instance.Generation == generation && !instance.generation.Closed() {
			copied := *instance
			copied.token = ""
			copied.generation = nil
			instances = append(instances, copied)
		}
	}
	sort.Slice(instances, func(i, j int) bool { return instances[i].InstanceID < instances[j].InstanceID })
	return instances
}

// Sweep drops sessions whose heartbeat expired or whose generation is
// fenced.
func (b *ModuleBridge) Sweep() {
	now := b.now()
	b.mu.Lock()
	defer b.mu.Unlock()
	for token, instance := range b.instances {
		if now.Sub(instance.LastSeen) > b.ttl || instance.generation.Closed() {
			delete(b.instances, token)
		}
	}
}

// Run sweeps expired sessions until ctx ends.
func (b *ModuleBridge) Run(ctx context.Context) {
	ticker := time.NewTicker(b.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			b.Sweep()
		}
	}
}

func (b *ModuleBridge) drop(token string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.instances, token)
}

// instance resolves the session of a call. heartbeat refreshes it and
// tolerates a fenced generation so the instance learns about the fence.
func (b *ModuleBridge) instance(ctx context.Context, heartbeat bool) (*BoundInstance, error) {
	identity, err := b.peerIdentity(ctx)
	if err != nil {
		return nil, err
	}
	values := metadata.ValueFromIncomingContext(ctx, SessionMetadataKey)
	if len(values) != 1 {
		return nil, status.Error(codes.Unauthenticated, "bind first: the bridge session token is missing")
	}
	now := b.now()
	b.mu.Lock()
	instance, ok := b.instances[values[0]]
	if ok && (instance.Identity != identity || now.Sub(instance.LastSeen) > b.ttl) {
		if now.Sub(instance.LastSeen) > b.ttl {
			delete(b.instances, values[0])
		}
		ok = false
	}
	if ok && heartbeat {
		instance.LastSeen = now
	}
	b.mu.Unlock()
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "the bridge session is unknown or expired; bind again")
	}
	if !heartbeat && instance.generation.Closed() {
		return nil, status.Error(codes.PermissionDenied, "package host generation is fenced")
	}
	return instance, nil
}

// peerIdentity returns the module identity of the verified client
// certificate. The module listener verifies the chain during the handshake.
func (b *ModuleBridge) peerIdentity(ctx context.Context) (moduletls.Identity, error) {
	remote, ok := peer.FromContext(ctx)
	if !ok {
		return moduletls.Identity{}, status.Error(codes.Unauthenticated, "no transport peer")
	}
	info, ok := remote.AuthInfo.(credentials.TLSInfo)
	if !ok || len(info.State.PeerCertificates) == 0 {
		return moduletls.Identity{}, status.Error(codes.Unauthenticated, "a module client certificate is required")
	}
	identity, err := moduletls.FromCertificate(info.State.PeerCertificates[0])
	if err != nil || identity.Kind != moduletls.KindModule || identity.Cluster != b.cluster {
		return moduletls.Identity{}, status.Error(codes.PermissionDenied, "the client certificate is not a module of this cluster")
	}
	return identity, nil
}
