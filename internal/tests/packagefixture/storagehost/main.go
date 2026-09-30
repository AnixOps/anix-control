// Command storagehost is a test fixture package host. It uses only the public
// SDKs, like a real package: it leases its storage over the package bridge
// and applies its embedded migration index when the kernel asks.
package main

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"errors"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	pluginhostv1 "github.com/AnixOps/anix-control/v4/api/pluginhost/v1"
	"github.com/AnixOps/anix-control/v4/pkg/packagebridgesdk"
	"github.com/AnixOps/anix-control/v4/pkg/packagestoresdk"
	"github.com/AnixOps/anix-control/v4/pkg/pluginhostsdk"
	"google.golang.org/grpc"
)

const packageID = "storage-fixture"

var packageVersion = "4.1.0"

//go:embed migrations
var migrations embed.FS

func main() {
	if err := run(); err != nil {
		log.Printf("storage fixture host: %v", err)
		os.Exit(1)
	}
}

func run() error {
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
	var lease [16]byte
	if _, err := rand.Read(lease[:]); err != nil {
		return err
	}
	storage := packagestoresdk.SharedOpener(bridge)
	router, err := pluginhostsdk.NewRouter(pluginhostsdk.RouterConfig{
		PackageID: packageID, LeaseID: hex.EncodeToString(lease[:]), Bridge: bridge, Logf: log.Printf,
		IndexMigration: packagestoresdk.IndexMigrator(storage, migrations, "migrations/index.json"),
	})
	if err != nil {
		return err
	}
	host, err := pluginhostsdk.NewServer(pluginhostsdk.ServerConfig{PackageID: packageID, PackageVersion: packageVersion}, router)
	if err != nil {
		return err
	}
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return err
	}
	defer func() { _ = listener.Close() }()
	server := grpc.NewServer(pluginhostsdk.HostServerOptions()...)
	pluginhostv1.RegisterControlPackageHostServer(server, host)
	terminate := make(chan os.Signal, 1)
	signal.Notify(terminate, syscall.SIGTERM)
	defer signal.Stop(terminate)
	go func() {
		<-terminate
		server.GracefulStop()
	}()
	return server.Serve(listener)
}
