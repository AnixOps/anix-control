// Package modulesdk runs a package host for the kernel: it serves the host
// protocol on the socket the kernel gives it and reaches the kernel over the
// package bridge. The kernel runs every host inside its own process and
// hands it its settings and bridge (WithInProcess).
package modulesdk

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"net"
	"os"
	"runtime/debug"
	"strings"
	"time"

	pluginhostv1 "github.com/AnixOps/anix-control/sdk/api/pluginhost/v1"
	"github.com/AnixOps/anix-control/sdk/packagebridgesdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"google.golang.org/grpc"
)

// Bridge is the package bridge a host package uses.
type Bridge interface {
	pluginhostsdk.RouterBridge
	pluginhostsdk.RouterWebSocketBridge
	LeaseStorage(ctx context.Context) (packagebridgesdk.StorageLease, error)
}

var _ Bridge = (*packagebridgesdk.Client)(nil)

// Host is what a package receives to build itself.
type Host struct {
	PackageID string
	Bridge    Bridge
	// LeaseID identifies this host instance in Health answers.
	LeaseID string
	Logf    func(string, ...any)
	// Getenv reads the settings the kernel gave the host. It never reads
	// Control's environment.
	Getenv func(string) string
}

// InProcess is what the kernel hands a host it runs inside its own process:
// the host's settings and its end of the package bridge.
type InProcess struct {
	Getenv func(string) string
	Bridge *os.File
	// Logf, when set, replaces the host's default logger (log.Printf), so
	// its lines name the package in Control's log.
	Logf func(string, ...any)
}

type inProcessKey struct{}

// stopGrace is how long a stopping host lets in-flight RPCs finish.
const stopGrace = 2 * time.Second

// WithInProcess carries the kernel's InProcess to Run.
func WithInProcess(ctx context.Context, host InProcess) context.Context {
	return context.WithValue(ctx, inProcessKey{}, host)
}

// Options configures Run.
type Options struct {
	PackageID      string
	PackageVersion string
	// Build returns the package the host serves. A package that implements
	// Run(context.Context) is run for the life of the host, one that
	// implements pluginhostsdk.ResumablePackage is resumed whenever it binds
	// to a generation, and one that implements ServiceRegistrant serves its
	// extra gRPC services next to the host protocol (only the kernel reaches
	// them, over the kernel-owned socket).
	Build func(Host) (pluginhostsdk.Package, error)
	Logf  func(string, ...any)
}

// ServiceRegistrant is a package that serves more gRPC services than the
// host protocol, such as a contract the kernel calls.
type ServiceRegistrant interface {
	RegisterServices(grpc.ServiceRegistrar)
}

func registerPackageServices(server *grpc.Server, packageImpl pluginhostsdk.Package) {
	if registrant, ok := packageImpl.(ServiceRegistrant); ok {
		registrant.RegisterServices(server)
	}
}

// Run serves the host the kernel started with ctx (WithInProcess) until ctx
// ends.
func Run(ctx context.Context, options Options) error {
	if options.Build == nil || strings.TrimSpace(options.PackageID) == "" || strings.TrimSpace(options.PackageVersion) == "" {
		return errors.New("module package identity and Build are required")
	}
	inProcess, ok := ctx.Value(inProcessKey{}).(InProcess)
	if !ok || inProcess.Getenv == nil {
		return errors.New("a package host runs only inside Control")
	}
	if options.Logf == nil {
		options.Logf = inProcess.Logf
	}
	if options.Logf == nil {
		options.Logf = log.Printf
	}
	getenv := inProcess.Getenv
	socketPath := strings.TrimSpace(getenv("ANIX_CONTROL_HOST_SOCKET"))
	if socketPath == "" {
		return errors.New("control host socket is required")
	}
	bridge, err := packagebridgesdk.DialFile(ctx, inProcess.Bridge)
	if err != nil {
		return err
	}
	defer func() { _ = bridge.Close() }()
	leaseID, err := newLeaseID()
	if err != nil {
		return err
	}
	packageImpl, err := options.Build(Host{PackageID: options.PackageID, Bridge: bridge, LeaseID: leaseID, Logf: options.Logf, Getenv: getenv})
	if err != nil {
		return err
	}
	runPackage(ctx, packageImpl, options.Logf)
	maxResponseBytes, err := pluginhostsdk.MaxResponseBytesFrom(getenv)
	if err != nil {
		return err
	}
	host, err := pluginhostsdk.NewServer(pluginhostsdk.ServerConfig{
		PackageID: options.PackageID, PackageVersion: options.PackageVersion, MaxResponseBytes: maxResponseBytes,
	}, packageImpl)
	if err != nil {
		return err
	}
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return err
	}
	defer func() { _ = listener.Close() }()
	// The descriptor-pinned parent supervisor verifies and secures this socket
	// before it can dispatch a request to the host.
	server := grpc.NewServer(pluginhostsdk.HostServerOptions()...)
	pluginhostv1.RegisterControlPackageHostServer(server, host)
	registerPackageServices(server, packageImpl)
	// The kernel ends ctx: finish in-flight RPCs, and cut the rest (long
	// WebSocket streams) when the grace period is over.
	go func() {
		<-ctx.Done()
		stopped := make(chan struct{})
		go func() {
			server.GracefulStop()
			close(stopped)
		}()
		select {
		case <-stopped:
		case <-time.After(stopGrace):
			server.Stop()
		}
	}()
	return server.Serve(listener)
}

// runPackage starts the package's background work. A panic in it is
// logged and ends that work only, not Control.
func runPackage(ctx context.Context, packageImpl pluginhostsdk.Package, logf func(string, ...any)) {
	if runner, ok := packageImpl.(interface{ Run(context.Context) }); ok {
		go func() {
			defer func() {
				if recovered := recover(); recovered != nil {
					logf("package background work stopped by a panic: %v\n%s", recovered, debug.Stack())
				}
			}()
			runner.Run(ctx)
		}()
	}
}

func newLeaseID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}
