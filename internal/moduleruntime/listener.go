// Package moduleruntime runs the kernel side of network modules: the mTLS
// module listener that serves ModulePKI and the remote package bridge.
package moduleruntime

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	kernelidentityv1 "github.com/AnixOps/anix-control/sdk/api/kernelidentity/v1"
	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	kernelorderv1 "github.com/AnixOps/anix-control/sdk/api/kernelorder/v1"
	kernelsettingsv1 "github.com/AnixOps/anix-control/sdk/api/kernelsettings/v1"
	kernelsubscriberv1 "github.com/AnixOps/anix-control/sdk/api/kernelsubscriber/v1"
	kerneltelemetryv1 "github.com/AnixOps/anix-control/sdk/api/kerneltelemetry/v1"
	modulepkiv1 "github.com/AnixOps/anix-control/sdk/api/modulepki/v1"
	packagebridgev1 "github.com/AnixOps/anix-control/sdk/api/packagebridge/v1"
	"github.com/AnixOps/anix-control/sdk/moduletls"
	"github.com/AnixOps/anix-control/v4/internal/modulepki"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/AnixOps/anix-control/v4/internal/panicrecovery"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// enrollMethod is the only RPC a client may call without a certificate.
const enrollMethod = modulepkiv1.ModulePKI_Enroll_FullMethodName

// revocationCacheTTL bounds how long a revocation takes to reach new calls.
const revocationCacheTTL = 30 * time.Second

// maxModuleMessageBytes bounds one message on the module listener.
const maxModuleMessageBytes = 64<<20 + 64<<10

// Listener serves the module listener.
type Listener struct {
	// TLS is the kernel's certificate and trust bundle.
	TLS     moduletls.Source
	Cluster string
	// PKI is the built-in CA; nil with an external PKI, which disables
	// ModulePKI.
	PKI    *modulepki.Authority
	Bridge *packagebridge.ModuleBridge
	// KernelIdentity, when set, is served to bound instances; see
	// ModuleBridge.KernelIdentityServer.
	KernelIdentity packagebridge.KernelIdentityProvider
	// KernelOrder, when set, is served to bound instances; see
	// ModuleBridge.KernelOrderServer.
	KernelOrder packagebridge.KernelOrderProvider
	// KernelSubscriber, when set, is served to bound instances; see
	// ModuleBridge.KernelSubscriberServer.
	KernelSubscriber packagebridge.KernelSubscriberProvider
	// KernelSettings, when set, is served to bound instances; see
	// ModuleBridge.KernelSettingsServer.
	KernelSettings packagebridge.KernelSettingsProvider
	// KernelTelemetry, when set, is served to bound instances; see
	// ModuleBridge.KernelTelemetryServer.
	KernelTelemetry packagebridge.KernelTelemetryProvider
	// KernelNodeOps, when set, is served to bound instances; see
	// ModuleBridge.KernelNodeOpsServer.
	KernelNodeOps packagebridge.KernelNodeOpsProvider

	revocations revocationCache
}

// Serve accepts connections on listener until ctx ends.
func (l *Listener) Serve(ctx context.Context, listener net.Listener) error {
	server, err := l.newServer()
	if err != nil {
		return err
	}
	go func() {
		<-ctx.Done()
		stopped := make(chan struct{})
		go func() {
			server.GracefulStop()
			close(stopped)
		}()
		select {
		case <-stopped:
		case <-time.After(5 * time.Second):
			server.Stop()
		}
	}()
	if err := server.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
		return err
	}
	return nil
}

func (l *Listener) newServer() (*grpc.Server, error) {
	if l.TLS.Certificate == nil || l.TLS.Roots == nil {
		return nil, errors.New("module listener needs the kernel certificate and trust bundle")
	}
	if l.Bridge == nil {
		return nil, errors.New("module listener needs the module bridge")
	}
	// Client certificates are optional at the TLS layer so that a new module
	// can Enroll; when present they are fully verified there, and the
	// interceptors below require one for every other RPC.
	tlsConfig := l.TLS.ServerConfig(moduletls.AcceptModules(l.Cluster), true)
	options := append(panicrecovery.ServerOptions(),
		grpc.Creds(credentials.NewTLS(tlsConfig)),
		grpc.MaxRecvMsgSize(maxModuleMessageBytes),
		grpc.KeepaliveParams(keepalive.ServerParameters{Time: 10 * time.Second, Timeout: 5 * time.Second}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{MinTime: 5 * time.Second, PermitWithoutStream: true}),
		grpc.ChainUnaryInterceptor(l.unaryAuthentication),
		grpc.ChainStreamInterceptor(l.streamAuthentication),
	)
	server := grpc.NewServer(options...)
	if l.PKI != nil {
		modulepkiv1.RegisterModulePKIServer(server, &modulepki.Server{Authority: l.PKI})
	} else {
		modulepkiv1.RegisterModulePKIServer(server, &modulepki.Server{})
	}
	packagebridgev1.RegisterKernelPackageBridgeServer(server, l.Bridge)
	if l.KernelIdentity != nil {
		kernelidentityv1.RegisterKernelIdentityServer(server, l.Bridge.KernelIdentityServer(l.KernelIdentity))
	}
	if l.KernelOrder != nil {
		kernelorderv1.RegisterKernelOrderServer(server, l.Bridge.KernelOrderServer(l.KernelOrder))
	}
	if l.KernelSubscriber != nil {
		kernelsubscriberv1.RegisterKernelSubscriberServer(server, l.Bridge.KernelSubscriberServer(l.KernelSubscriber))
	}
	if l.KernelSettings != nil {
		kernelsettingsv1.RegisterKernelSettingsServer(server, l.Bridge.KernelSettingsServer(l.KernelSettings))
	}
	if l.KernelTelemetry != nil {
		kerneltelemetryv1.RegisterKernelTelemetryServer(server, l.Bridge.KernelTelemetryServer(l.KernelTelemetry))
	}
	if l.KernelNodeOps != nil {
		kernelnodeopsv1.RegisterKernelNodeOpsServer(server, l.Bridge.KernelNodeOpsServer(l.KernelNodeOps))
	}
	return server, nil
}

func (l *Listener) unaryAuthentication(ctx context.Context, request any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	if err := l.authenticate(ctx, info.FullMethod); err != nil {
		return nil, err
	}
	return handler(ctx, request)
}

func (l *Listener) streamAuthentication(server any, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	if err := l.authenticate(stream.Context(), info.FullMethod); err != nil {
		return err
	}
	return handler(server, stream)
}

// authenticate requires a verified, unrevoked module certificate for every
// method except Enroll.
func (l *Listener) authenticate(ctx context.Context, method string) error {
	if method == enrollMethod {
		return nil
	}
	remote, ok := peer.FromContext(ctx)
	if !ok {
		return status.Error(codes.Unauthenticated, "no transport peer")
	}
	info, ok := remote.AuthInfo.(credentials.TLSInfo)
	if !ok || len(info.State.PeerCertificates) == 0 {
		return status.Error(codes.Unauthenticated, "a module client certificate is required")
	}
	if l.PKI == nil {
		return nil
	}
	revoked, err := l.revocations.revoked(ctx, l.PKI, info.State.PeerCertificates[0])
	if err != nil {
		return status.Error(codes.Unavailable, "certificate revocation state is unavailable")
	}
	if revoked {
		return status.Error(codes.PermissionDenied, modulepki.ErrCertificateRevoked.Error())
	}
	return nil
}

// revocationCache remembers revocation lookups per serial for a short time.
// A revocation made by this kernel process clears it at once; revocations
// made elsewhere apply within revocationCacheTTL.
type revocationCache struct {
	mu      sync.Mutex
	entries map[string]revocationEntry
	epoch   uint64
}

type revocationEntry struct {
	revoked   bool
	checkedAt time.Time
}

func (c *revocationCache) revoked(ctx context.Context, authority *modulepki.Authority, certificate *x509.Certificate) (bool, error) {
	serial := modulepki.SerialString(certificate.SerialNumber)
	now := time.Now()
	c.mu.Lock()
	if epoch := modulepki.RevocationEpoch(); c.entries == nil || epoch != c.epoch {
		c.entries = make(map[string]revocationEntry)
		c.epoch = epoch
	}
	entry, ok := c.entries[serial]
	c.mu.Unlock()
	if ok && now.Sub(entry.checkedAt) < revocationCacheTTL {
		return entry.revoked, nil
	}
	revoked, err := authority.IsRevoked(ctx, serial)
	if err != nil {
		return false, fmt.Errorf("check certificate revocation: %w", err)
	}
	c.mu.Lock()
	c.entries[serial] = revocationEntry{revoked: revoked, checkedAt: now}
	for key, cached := range c.entries {
		if now.Sub(cached.checkedAt) > revocationCacheTTL {
			delete(c.entries, key)
		}
	}
	c.mu.Unlock()
	return revoked, nil
}
