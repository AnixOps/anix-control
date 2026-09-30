package main

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
	// Embed the zone database so the TZ passed by the kernel resolves even
	// without system zoneinfo.
	_ "time/tzdata"

	pluginhostv1 "github.com/AnixOps/anix-control/v4/api/pluginhost/v1"
	"github.com/AnixOps/anix-control/v4/pkg/packagebridgesdk"
	"github.com/AnixOps/anix-control/v4/pkg/pluginhostsdk"
	"google.golang.org/grpc"
)

var (
	packageID      = ""
	packageVersion = "4.0.0"
)

func main() {
	if err := run(); err != nil {
		log.Printf("generic control package host: %v", err)
		os.Exit(1)
	}
}

func run() error {
	if strings.TrimSpace(packageID) == "" || strings.TrimSpace(packageVersion) == "" {
		return errors.New("package identity is required")
	}
	socketPath := strings.TrimSpace(os.Getenv("ANIX_CONTROL_HOST_SOCKET"))
	if socketPath == "" {
		return errors.New("control host socket is required")
	}
	dialContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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
	service, err := newGenericService(packageID, bridge, leaseID)
	if err != nil {
		return err
	}
	// Poll the installation's route modes for the life of the host.
	routerContext, stopRouter := context.WithCancel(context.Background())
	defer stopRouter()
	go service.Run(routerContext)
	maxResponseBytes, err := pluginhostsdk.MaxResponseBytesFromEnvironment()
	if err != nil {
		return err
	}
	host, err := pluginhostsdk.NewServer(pluginhostsdk.ServerConfig{
		PackageID: packageID, PackageVersion: packageVersion, MaxResponseBytes: maxResponseBytes,
	}, service)
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
	terminate := make(chan os.Signal, 1)
	signal.Notify(terminate, syscall.SIGTERM)
	defer signal.Stop(terminate)
	go func() {
		<-terminate
		server.GracefulStop()
	}()
	return server.Serve(listener)
}

func newLeaseID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}
