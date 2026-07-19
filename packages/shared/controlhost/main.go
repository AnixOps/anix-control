package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"net"
	"os"
	"strings"
	"time"

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
	service := newGenericService(bridge, leaseID)
	host, err := pluginhostsdk.NewServer(pluginhostsdk.ServerConfig{
		PackageID: packageID, PackageVersion: packageVersion,
	}, service)
	if err != nil {
		return err
	}
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return err
	}
	defer func() { _ = listener.Close() }()
	if err := os.Chmod(socketPath, 0o600); err != nil {
		return err
	}
	server := grpc.NewServer()
	pluginhostv1.RegisterControlPackageHostServer(server, host)
	return server.Serve(listener)
}

func newLeaseID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}
