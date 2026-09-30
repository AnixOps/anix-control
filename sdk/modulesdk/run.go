// Package modulesdk runs a package host as a local child process of the
// kernel or as a network module.
//
// The same binary supports both. Without ANIX_MODULE_MODE it expects the
// environment the kernel gives a local host (socket and inherited bridge).
// With ANIX_MODULE_MODE=remote it runs as a service:
//   - it enrolls with the kernel's module PKI (or uses externally issued
//     certificates), keeps its certificate renewed and serves the host
//     protocol over mTLS;
//   - it binds to the kernel's module listener, heartbeats and binds again
//     after fences and kernel restarts;
//   - it answers /livez and /readyz for the platform and drains on SIGTERM.
package modulesdk

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	pluginhostv1 "github.com/AnixOps/anix-control/sdk/api/pluginhost/v1"
	"github.com/AnixOps/anix-control/sdk/moduletls"
	"github.com/AnixOps/anix-control/sdk/packagebridgesdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// Environment variables of a network module.
const (
	EnvMode            = "ANIX_MODULE_MODE"
	EnvKernelAddr      = "ANIX_MODULE_KERNEL_ADDR"
	EnvListenAddr      = "ANIX_MODULE_LISTEN_ADDR"
	EnvAdvertiseAddr   = "ANIX_MODULE_ADVERTISE_ADDR"
	EnvCluster         = "ANIX_MODULE_CLUSTER"
	EnvInstanceID      = "ANIX_MODULE_INSTANCE_ID"
	EnvCertDir         = "ANIX_MODULE_CERT_DIR"
	EnvTrustBundleFile = "ANIX_MODULE_TRUST_BUNDLE_FILE"
	EnvEnrollmentFile  = "ANIX_MODULE_ENROLL_CREDENTIAL_FILE"
	EnvCertFile        = "ANIX_MODULE_CERT_FILE"
	EnvKeyFile         = "ANIX_MODULE_KEY_FILE"
	EnvHealthAddr      = "ANIX_MODULE_HEALTH_ADDR"
	EnvShutdownGrace   = "ANIX_MODULE_SHUTDOWN_GRACE"
	EnvImageDigest     = "ANIX_MODULE_IMAGE_DIGEST"

	// ModeRemote selects the network module mode.
	ModeRemote = "remote"
)

// Bridge is the package bridge a host package uses in either mode.
type Bridge interface {
	pluginhostsdk.RouterBridge
	pluginhostsdk.RouterWebSocketBridge
	LeaseStorage(ctx context.Context) (packagebridgesdk.StorageLease, error)
}

var (
	_ Bridge = (*packagebridgesdk.Client)(nil)
	_ Bridge = (*packagebridgesdk.NetworkClient)(nil)
)

// Host is what a package receives to build itself.
type Host struct {
	PackageID string
	Bridge    Bridge
	// LeaseID identifies this process in Health answers.
	LeaseID string
	Logf    func(string, ...any)
	// Remote reports the network module mode.
	Remote bool
}

// Options configures Run.
type Options struct {
	PackageID      string
	PackageVersion string
	// Build returns the package the host serves. A package that implements
	// Run(context.Context) is run for the life of the host, and one that
	// implements pluginhostsdk.ResumablePackage is resumed whenever it binds
	// to a generation.
	Build func(Host) (pluginhostsdk.Package, error)
	Logf  func(string, ...any)
	// Getenv defaults to os.Getenv.
	Getenv func(string) string
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
	if options.Getenv(EnvMode) == ModeRemote {
		return runRemote(ctx, options)
	}
	return runLocal(ctx, options)
}

// runLocal serves the kernel-supervised child process mode.
func runLocal(ctx context.Context, options Options) error {
	socketPath := strings.TrimSpace(options.Getenv("ANIX_CONTROL_HOST_SOCKET"))
	if socketPath == "" {
		return errors.New("control host socket is required (or set ANIX_MODULE_MODE=remote)")
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
	// The kernel sends SIGTERM before it kills the host's process group, so
	// finish in-flight RPCs and exit cleanly within its grace period.
	go func() {
		<-ctx.Done()
		server.GracefulStop()
	}()
	return server.Serve(listener)
}

// runRemote serves the network module mode.
func runRemote(ctx context.Context, options Options) error {
	settings, err := loadSettings(options)
	if err != nil {
		return err
	}
	certs, err := newCertificates(ctx, settings)
	if err != nil {
		return err
	}
	source := certs.source()
	go certs.maintain(ctx, options.Logf)
	leaseID, err := newLeaseID()
	if err != nil {
		return err
	}
	kernel, err := moduletls.Kernel(settings.cluster)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", settings.listenAddr)
	if err != nil {
		return err
	}
	defer func() { _ = listener.Close() }()
	advertise := settings.advertiseAddr
	if advertise == "" {
		advertise = defaultAdvertiseAddr(listener.Addr(), settings.instanceID)
	}

	var packageImpl pluginhostsdk.Package
	var bound atomic.Bool
	bridge, err := packagebridgesdk.DialNetwork(packagebridgesdk.NetworkConfig{
		KernelAddr: settings.kernelAddr, TLS: source.ClientConfig(moduletls.AcceptExactly(kernel)),
		PackageID: settings.packageID, PackageVersion: settings.packageVersion, InstanceID: settings.instanceID,
		AdvertiseAddr: advertise, LeaseID: leaseID, ImageDigest: settings.imageDigest, Logf: options.Logf,
		OnBind: func(generation uint64) {
			bound.Store(true)
			// A drain belonged to the previous generation.
			if resumable, ok := packageImpl.(pluginhostsdk.ResumablePackage); ok {
				_ = resumable.Resume(context.WithoutCancel(ctx))
			}
			options.Logf("module %s bound to generation %d as %s", settings.packageID, generation, settings.instanceID)
		},
	})
	if err != nil {
		return err
	}
	defer func() { _ = bridge.Close() }()
	packageImpl, err = options.Build(Host{PackageID: settings.packageID, Bridge: bridge, LeaseID: leaseID, Logf: options.Logf, Remote: true})
	if err != nil {
		return err
	}
	host, err := pluginhostsdk.NewServer(pluginhostsdk.ServerConfig{
		PackageID: settings.packageID, PackageVersion: settings.packageVersion, MaxResponseBytes: settings.maxResponseBytes,
	}, packageImpl)
	if err != nil {
		return err
	}
	serverOptions := append(pluginhostsdk.HostServerOptions(),
		grpc.Creds(credentials.NewTLS(source.ServerConfig(moduletls.AcceptExactly(kernel), false))))
	server := grpc.NewServer(serverOptions...)
	pluginhostv1.RegisterControlPackageHostServer(server, host)

	serving := make(chan error, 1)
	go func() { serving <- server.Serve(listener) }()
	runCtx, stopRun := context.WithCancel(context.WithoutCancel(ctx))
	defer stopRun()
	runPackage(runCtx, packageImpl)
	go bridge.Run(runCtx)

	var shuttingDown atomic.Bool
	health := startHealthServer(settings.healthAddr, func() bool {
		if shuttingDown.Load() || !bound.Load() {
			return false
		}
		_, ok := bridge.Bound()
		return ok
	}, options.Logf)
	defer func() { _ = health.Close() }()

	select {
	case err := <-serving:
		return err
	case <-ctx.Done():
	}
	// Leave rotation: fail readiness and stop heartbeating so the kernel
	// drops this instance, while in-flight and late requests still finish.
	shuttingDown.Store(true)
	stopRun()
	options.Logf("module %s shutting down; serving for %s more", settings.packageID, settings.shutdownGrace)
	time.Sleep(settings.shutdownGrace)
	stopped := make(chan struct{})
	go func() {
		server.GracefulStop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(10 * time.Second):
		server.Stop()
	}
	return nil
}

func runPackage(ctx context.Context, packageImpl pluginhostsdk.Package) {
	if runner, ok := packageImpl.(interface{ Run(context.Context) }); ok {
		go runner.Run(ctx)
	}
}

func startHealthServer(address string, ready func() bool, logf func(string, ...any)) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/livez", func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte("ok\n"))
	})
	mux.HandleFunc("/readyz", func(writer http.ResponseWriter, _ *http.Request) {
		if !ready() {
			writer.WriteHeader(http.StatusServiceUnavailable)
			_, _ = writer.Write([]byte("not bound\n"))
			return
		}
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte("ok\n"))
	})
	server := &http.Server{Addr: address, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logf("module health server: %v", err)
		}
	}()
	return server
}

func defaultAdvertiseAddr(listen net.Addr, instanceID string) string {
	_, port, err := net.SplitHostPort(listen.String())
	if err != nil {
		return listen.String()
	}
	return net.JoinHostPort(instanceID, port)
}

func newLeaseID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

// settings is the resolved remote configuration.
type settings struct {
	packageID, packageVersion                string
	kernelAddr, listenAddr, advertiseAddr    string
	cluster, instanceID, certDir, healthAddr string
	trustBundleFile, enrollCredentialFile    string
	certFile, keyFile, imageDigest           string
	shutdownGrace                            time.Duration
	maxResponseBytes                         int
}

func (s settings) externalPKI() bool { return s.certFile != "" || s.keyFile != "" }

func loadSettings(options Options) (settings, error) {
	get := func(name, fallback string) string {
		if value := strings.TrimSpace(options.Getenv(name)); value != "" {
			return value
		}
		return fallback
	}
	hostname, _ := os.Hostname()
	s := settings{
		packageID: options.PackageID, packageVersion: options.PackageVersion,
		kernelAddr: get(EnvKernelAddr, ""), listenAddr: get(EnvListenAddr, ":7000"), advertiseAddr: get(EnvAdvertiseAddr, ""),
		cluster: get(EnvCluster, "default"), instanceID: get(EnvInstanceID, hostname),
		certDir: get(EnvCertDir, "/var/lib/anix-module/certs"), healthAddr: get(EnvHealthAddr, ":8081"),
		trustBundleFile: get(EnvTrustBundleFile, ""), enrollCredentialFile: get(EnvEnrollmentFile, ""),
		certFile: get(EnvCertFile, ""), keyFile: get(EnvKeyFile, ""), imageDigest: get(EnvImageDigest, ""),
		shutdownGrace: 20 * time.Second,
	}
	if s.kernelAddr == "" {
		return s, fmt.Errorf("%s is required in remote mode", EnvKernelAddr)
	}
	if s.instanceID == "" {
		return s, fmt.Errorf("%s is required when the host name is unknown", EnvInstanceID)
	}
	if s.externalPKI() && (s.certFile == "" || s.keyFile == "" || s.trustBundleFile == "") {
		return s, fmt.Errorf("external certificates need %s, %s and %s", EnvCertFile, EnvKeyFile, EnvTrustBundleFile)
	}
	if raw := options.Getenv(EnvShutdownGrace); raw != "" {
		grace, err := time.ParseDuration(raw)
		if err != nil || grace < 0 {
			return s, fmt.Errorf("invalid %s %q", EnvShutdownGrace, raw)
		}
		s.shutdownGrace = grace
	}
	maxResponse, err := pluginhostsdk.MaxResponseBytesFromEnvironment()
	if err != nil {
		return s, err
	}
	s.maxResponseBytes = maxResponse
	return s, nil
}
