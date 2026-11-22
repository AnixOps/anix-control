// Package modulesdk runs a package host as a local child process of the
// kernel: it serves the host protocol on the socket the kernel gives it and
// reaches the kernel over the inherited package bridge.
package modulesdk

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
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
	// LeaseID identifies this process in Health answers.
	LeaseID string
	Logf    func(string, ...any)
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
	// Getenv defaults to os.Getenv.
	Getenv func(string) string
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

// Run serves the host until SIGTERM, SIGINT or ctx ends.
func Run(ctx context.Context, options Options) error {
	if options.Build == nil || strings.TrimSpace(options.PackageID) == "" || strings.TrimSpace(options.PackageVersion) == "" {
		return errors.New("module package identity and Build are required")
	}
	if options.Logf == nil {
		options.Logf = log.Printf
	}
	if options.Getenv == nil {
		options.Getenv = os.Getenv
	}
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGTERM, os.Interrupt)
	defer stop()
	return runLocal(ctx, options)
}

// runLocal serves the kernel-supervised child process.
func runLocal(ctx context.Context, options Options) error {
	socketPath := strings.TrimSpace(options.Getenv("ANIX_CONTROL_HOST_SOCKET"))
	if socketPath == "" {
		return errors.New("control host socket is required")
	}
	dialContext, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	bridge, err := packagebridgesdk.DialFromEnvironment(dialContext)
	if err != nil {
		return err
	}
	defer func() { _ = bridge.Close() }()
	leaseID, err := newLeaseID()
	if err != nil {
		return err
	}
	packageImpl, err := options.Build(Host{PackageID: options.PackageID, Bridge: bridge, LeaseID: leaseID, Logf: options.Logf})
	if err != nil {
		return err
	}
	runPackage(ctx, packageImpl)
	maxResponseBytes, err := pluginhostsdk.MaxResponseBytesFromEnvironment()
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
	// The kernel sends SIGTERM before it kills the host's process group, so
	// finish in-flight RPCs and exit cleanly within its grace period.
	go func() {
		<-ctx.Done()
		server.GracefulStop()
	}()
	return server.Serve(listener)
}

func runPackage(ctx context.Context, packageImpl pluginhostsdk.Package) {
	if runner, ok := packageImpl.(interface{ Run(context.Context) }); ok {
		go runner.Run(ctx)
	}
}

func newLeaseID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}
