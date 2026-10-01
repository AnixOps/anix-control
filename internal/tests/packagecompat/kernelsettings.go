package packagecompat

import (
	"context"
	"net"
	"testing"

	kernelsettingsv1 "github.com/AnixOps/anix-control/sdk/api/kernelsettings/v1"
	"github.com/AnixOps/anix-control/v4/internal/kernelsettings"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"gorm.io/gorm"
)

// SettingsGrants authorizes the capabilities a package's signed release
// declares, for that package's host only.
type SettingsGrants struct {
	Host         packagebridge.HostIdentity
	Capabilities []string
}

// AuthorizeCapability implements kernelsettings.Authorizer.
func (g SettingsGrants) AuthorizeCapability(_ context.Context, host packagebridge.HostIdentity, capability string) error {
	if host == g.Host {
		for _, granted := range g.Capabilities {
			if granted == capability {
				return nil
			}
		}
	}
	return service.ErrCapabilityNotAuthorized
}

// KernelSettings serves the kernel's KernelSettings (internal/kernelsettings)
// on db in process over gRPC, to grants' host, and returns a client for it,
// as the package host gets one over its bridge connection.
func KernelSettings(t testing.TB, db *gorm.DB, grants SettingsGrants) kernelsettingsv1.KernelSettingsClient {
	t.Helper()
	server := &kernelsettings.Server{DB: db, Authorizer: grants}
	listener := bufconn.Listen(1 << 20)
	grpcServer := grpc.NewServer()
	kernelsettingsv1.RegisterKernelSettingsServer(grpcServer, server.For(grants.Host))
	go func() { _ = grpcServer.Serve(listener) }()
	t.Cleanup(grpcServer.Stop)
	conn, err := grpc.NewClient("passthrough:///kernel", grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return kernelsettingsv1.NewKernelSettingsClient(conn)
}
