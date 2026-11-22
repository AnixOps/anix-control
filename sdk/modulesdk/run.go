// Package modulesdk runs a package host for the kernel: it builds the
// package, hands it to the kernel, which calls it directly, and reaches the
// kernel over the package bridge. The kernel runs every host inside its own
// process and hands it its settings and bridge (WithInProcess).
package modulesdk

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"runtime/debug"
	"strings"

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
// the host's settings, its end of the package bridge, and where to hand the
// built package.
type InProcess struct {
	Getenv func(string) string
	Bridge grpc.ClientConnInterface
	// Logf, when set, replaces the host's default logger (log.Printf), so
	// its lines name the package in Control's log.
	Logf func(string, ...any)
	// Ready receives the built package; the kernel calls it from then on.
	Ready func(pluginhostsdk.Package)
}

type inProcessKey struct{}

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
	// implements pluginhostsdk.ResumablePackage is resumed after a drain,
	// and one that implements ServiceRegistrant serves its extra gRPC
	// services to the kernel (in process, with no transport).
	Build func(Host) (pluginhostsdk.Package, error)
	Logf  func(string, ...any)
}

// ServiceRegistrant is a package that serves gRPC services of its own, such
// as a contract the kernel calls.
type ServiceRegistrant interface {
	RegisterServices(grpc.ServiceRegistrar)
}

// Run serves the host the kernel started with ctx (WithInProcess) until ctx
// ends.
func Run(ctx context.Context, options Options) error {
	if options.Build == nil || strings.TrimSpace(options.PackageID) == "" || strings.TrimSpace(options.PackageVersion) == "" {
		return errors.New("module package identity and Build are required")
	}
	inProcess, ok := ctx.Value(inProcessKey{}).(InProcess)
	if !ok || inProcess.Getenv == nil || inProcess.Ready == nil {
		return errors.New("a package host runs only inside Control")
	}
	if options.Logf == nil {
		options.Logf = inProcess.Logf
	}
	if options.Logf == nil {
		options.Logf = log.Printf
	}
	bridge, err := packagebridgesdk.NewClient(inProcess.Bridge)
	if err != nil {
		return err
	}
	leaseID, err := newLeaseID()
	if err != nil {
		return err
	}
	packageImpl, err := options.Build(Host{PackageID: options.PackageID, Bridge: bridge, LeaseID: leaseID, Logf: options.Logf, Getenv: inProcess.Getenv})
	if err != nil {
		return err
	}
	runPackage(ctx, packageImpl, options.Logf)
	inProcess.Ready(packageImpl)
	<-ctx.Done()
	return nil
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
