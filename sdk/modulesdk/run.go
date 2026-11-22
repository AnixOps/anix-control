// Package modulesdk runs a package host for the kernel: it serves the host
// protocol on the socket the kernel gives it and reaches the kernel over the
// package bridge. The kernel runs a built-in host inside its own process
// (WithInProcess); any other host is a child process that inherits the
// bridge and reads its settings from the environment.
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
	"runtime/debug"
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
	// LeaseID identifies this host instance in Health answers.
	LeaseID string
	Logf    func(string, ...any)
	// Getenv reads the host's settings: the child process environment, or
	// the entries the kernel gave an in-process host.
	Getenv func(string) string
}

// InProcess is what the kernel hands a host it runs inside its own process:
// the host's settings, as a child would find them in its environment, and
// its end of the package bridge.
type InProcess struct {
	Getenv func(string) string
	Bridge *os.File
	// Logf, when set, replaces the host's default logger (log.Printf), so
	// its lines name the package in Control's log.
	Logf func(string, ...any)
}

type inProcessKey struct{}

// stopGrace is how long a stopping host lets in-flight RPCs finish; the
// kernel waits as long (internal/pluginhost defaultHostStopGrace).
const stopGrace = 2 * time.Second

// WithInProcess marks ctx as an in-process host start: Run then takes its
// settings and bridge from host instead of the process environment, and
// leaves the process signals alone.
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
	inProcess, ok := ctx.Value(inProcessKey{}).(InProcess)
	if ok && options.Logf == nil {
		options.Logf = inProcess.Logf
	}
	if options.Logf == nil {
		options.Logf = log.Printf
	}
	if ok {
		options.Getenv = inProcess.Getenv
		return serve(ctx, options, func(ctx context.Context) (*packagebridgesdk.Client, error) {
			return packagebridgesdk.DialFile(ctx, inProcess.Bridge)
		})
	}
	if options.Getenv == nil {
		options.Getenv = os.Getenv
	}
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGTERM, os.Interrupt)
	defer stop()
	return serve(ctx, options, packagebridgesdk.DialFromEnvironment)
}

// serve runs the host until ctx ends.
func serve(ctx context.Context, options Options, dial func(context.Context) (*packagebridgesdk.Client, error)) error {
	socketPath := strings.TrimSpace(options.Getenv("ANIX_CONTROL_HOST_SOCKET"))
	if socketPath == "" {
		return errors.New("control host socket is required")
	}
	dialContext, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	bridge, err := dial(dialContext)
	if err != nil {
		return err
	}
	defer func() { _ = bridge.Close() }()
	leaseID, err := newLeaseID()
	if err != nil {
		return err
	}
	packageImpl, err := options.Build(Host{PackageID: options.PackageID, Bridge: bridge, LeaseID: leaseID, Logf: options.Logf, Getenv: options.Getenv})
	if err != nil {
		return err
	}
	runPackage(ctx, packageImpl, options.Logf)
	maxResponseBytes, err := pluginhostsdk.MaxResponseBytesFrom(options.Getenv)
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
	// The kernel ends ctx (SIGTERM for a child) and waits its grace period:
	// finish in-flight RPCs, and cut the rest (long WebSocket streams) when
	// the grace period is over.
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
// logged and ends that work only: in-process, it would end Control.
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
