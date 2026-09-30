package packagebridgesdk

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"
	"time"

	packagebridgev1 "github.com/AnixOps/anix-control/sdk/api/packagebridge/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// SessionMetadataKey carries the Bind session token (unpadded base64url) on
// every bridge call of a remote module.
const SessionMetadataKey = "x-anix-bridge-session"

// Rebind backoff bounds.
const (
	minRebindDelay = 500 * time.Millisecond
	maxRebindDelay = 10 * time.Second
)

// ErrNotBound means the module has no session with the kernel yet.
var ErrNotBound = errors.New("package bridge session is not bound")

// NetworkConfig configures the bridge of a network module.
type NetworkConfig struct {
	// KernelAddr is the kernel's module listener, host:port.
	KernelAddr string
	// TLS authenticates the module with its certificate and the kernel by
	// its SPIFFE identity (see pkg/moduletls).
	TLS            *tls.Config
	PackageID      string
	PackageVersion string
	// InstanceID is unique per process, for example the pod name.
	InstanceID string
	// AdvertiseAddr is where the kernel dials this instance.
	AdvertiseAddr string
	// LeaseID is the per-process lease the host also reports from Health.
	LeaseID     string
	ImageDigest string
	// OnBind runs after every successful Bind with the generation served.
	OnBind func(generation uint64)
	Logf   func(string, ...any)
}

// NetworkClient is the package bridge of a network module. It embeds Client,
// so Invoke, OpenWebSocket, GetPackageConfig and LeaseStorage behave as for a
// local host; a session token from Bind travels with every call, and a call
// rejected for an unknown session binds again and retries once.
type NetworkClient struct {
	*Client
	config NetworkConfig

	bindMu     sync.Mutex
	mu         sync.Mutex
	token      string
	generation uint64
	interval   time.Duration
	bound      bool
}

// DialNetwork connects to the kernel's module listener. It does not bind;
// call Run (or Bind) next.
func DialNetwork(config NetworkConfig) (*NetworkClient, error) {
	if config.KernelAddr == "" || config.TLS == nil || config.PackageID == "" || config.PackageVersion == "" ||
		config.InstanceID == "" || config.AdvertiseAddr == "" || config.LeaseID == "" {
		return nil, fmt.Errorf("%w: network bridge configuration is incomplete", ErrBridgeUnavailable)
	}
	if config.Logf == nil {
		config.Logf = func(string, ...any) {}
	}
	client := &NetworkClient{config: config, interval: 5 * time.Second}
	connection, err := grpc.NewClient(config.KernelAddr,
		grpc.WithTransportCredentials(credentials.NewTLS(config.TLS)),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(maxBridgeMessageBytes)),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{Time: 10 * time.Second, Timeout: 5 * time.Second, PermitWithoutStream: true}),
		grpc.WithChainUnaryInterceptor(client.unarySession),
		grpc.WithChainStreamInterceptor(client.streamSession),
	)
	if err != nil {
		return nil, bridgeError(err)
	}
	client.Client = &Client{connection: connection, rpc: packagebridgev1.NewKernelPackageBridgeClient(connection)}
	return client, nil
}

// Bound returns the generation the session serves.
func (c *NetworkClient) Bound() (uint64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.generation, c.bound
}

// Bind attaches this instance to the generation its package runs.
func (c *NetworkClient) Bind(ctx context.Context) error {
	c.bindMu.Lock()
	defer c.bindMu.Unlock()
	return c.bindLocked(ctx)
}

// rebind binds again unless another call already replaced the session
// identified by staleToken.
func (c *NetworkClient) rebind(ctx context.Context, staleToken string) error {
	c.bindMu.Lock()
	defer c.bindMu.Unlock()
	c.mu.Lock()
	refreshed := c.bound && c.token != "" && c.token != staleToken
	c.mu.Unlock()
	if refreshed {
		return nil
	}
	return c.bindLocked(ctx)
}

func (c *NetworkClient) bindLocked(ctx context.Context) error {
	response, err := c.rpc.Bind(ctx, &packagebridgev1.BindRequest{
		PackageId: c.config.PackageID, PackageVersion: c.config.PackageVersion, InstanceId: c.config.InstanceID,
		AdvertiseAddr: c.config.AdvertiseAddr, LeaseId: c.config.LeaseID, ImageDigest: c.config.ImageDigest,
	})
	if err != nil {
		c.unbind()
		return err
	}
	interval := time.Duration(response.GetHeartbeatIntervalMillis()) * time.Millisecond
	if interval <= 0 {
		interval = 5 * time.Second
	}
	c.mu.Lock()
	c.token = base64.RawURLEncoding.EncodeToString(response.GetSessionToken())
	c.generation = response.GetRouteGeneration()
	c.interval = interval
	c.bound = true
	c.mu.Unlock()
	if c.config.OnBind != nil {
		c.config.OnBind(response.GetRouteGeneration())
	}
	return nil
}

func (c *NetworkClient) unbind() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.bound = false
	c.token = ""
}

// Run keeps the session: it binds (with backoff while the kernel has no
// remote generation for this version), heartbeats, and binds again after a
// fence or a kernel restart, until ctx ends.
func (c *NetworkClient) Run(ctx context.Context) {
	delay := minRebindDelay
	for ctx.Err() == nil {
		if _, bound := c.Bound(); !bound {
			if err := c.rebind(ctx, ""); err != nil {
				if ctx.Err() == nil {
					c.config.Logf("package bridge: bind failed, retrying in %s: %v", delay, err)
				}
				if !sleepContext(ctx, delay) {
					return
				}
				delay = min(2*delay, maxRebindDelay)
				continue
			}
			delay = minRebindDelay
		}
		c.mu.Lock()
		interval := c.interval
		c.mu.Unlock()
		if !sleepContext(ctx, interval) {
			return
		}
		response, err := c.rpc.Heartbeat(ctx, &packagebridgev1.HeartbeatRequest{})
		switch {
		case err == nil && response.GetFenced():
			c.config.Logf("package bridge: generation %d is fenced; binding again", response.GetRouteGeneration())
			c.unbind()
		case status.Code(err) == codes.Unauthenticated:
			c.unbind()
		case err != nil && ctx.Err() == nil:
			c.config.Logf("package bridge: heartbeat failed: %v", err)
		}
	}
}

func sleepContext(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (c *NetworkClient) withSession(ctx context.Context) context.Context {
	c.mu.Lock()
	token := c.token
	c.mu.Unlock()
	if token == "" {
		return ctx
	}
	return metadata.AppendToOutgoingContext(ctx, SessionMetadataKey, token)
}

func (c *NetworkClient) unarySession(ctx context.Context, method string, request, reply any, connection *grpc.ClientConn, invoker grpc.UnaryInvoker, options ...grpc.CallOption) error {
	if method == packagebridgev1.KernelPackageBridge_Bind_FullMethodName {
		return invoker(ctx, method, request, reply, connection, options...)
	}
	c.mu.Lock()
	token := c.token
	c.mu.Unlock()
	err := invoker(c.withSession(ctx), method, request, reply, connection, options...)
	if status.Code(err) != codes.Unauthenticated || method == packagebridgev1.KernelPackageBridge_Heartbeat_FullMethodName {
		return err
	}
	// The kernel no longer knows the session (it restarted, or the session
	// expired): the call was rejected before any capability was consumed, so
	// binding again and retrying once is safe.
	if bindErr := c.rebind(ctx, token); bindErr != nil {
		return err
	}
	return invoker(c.withSession(ctx), method, request, reply, connection, options...)
}

func (c *NetworkClient) streamSession(ctx context.Context, description *grpc.StreamDesc, connection *grpc.ClientConn, method string, streamer grpc.Streamer, options ...grpc.CallOption) (grpc.ClientStream, error) {
	return streamer(c.withSession(ctx), description, connection, method, options...)
}
