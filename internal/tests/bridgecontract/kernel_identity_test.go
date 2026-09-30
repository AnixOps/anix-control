package bridgecontract

import (
	"context"
	"testing"

	kernelidentityv1 "github.com/AnixOps/anix-control/sdk/api/kernelidentity/v1"
	"github.com/AnixOps/anix-control/sdk/packagebridgesdk"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type settingsRecorder struct {
	kernelidentityv1.UnimplementedKernelIdentityServer
	host packagebridge.HostIdentity
	seen *[]packagebridge.HostIdentity
}

func (r settingsRecorder) GetIdentitySettings(context.Context, *kernelidentityv1.GetIdentitySettingsRequest) (*kernelidentityv1.GetIdentitySettingsResponse, error) {
	*r.seen = append(*r.seen, r.host)
	return &kernelidentityv1.GetIdentitySettingsResponse{SettingsJson: []byte(`{"settings_schema_version":1}`)}, nil
}

func dialSession(t *testing.T, options packagebridge.SessionOptions) *packagebridgesdk.Client {
	t.Helper()
	allowlist, err := packagebridge.NewAllowlistWithFallback(nil)
	require.NoError(t, err)
	session, child, err := packagebridge.NewSessionWithOptions(
		packagebridge.HostIdentity{PackageID: "identity-platform", Version: "4.1.0", Generation: 5}, allowlist, options,
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, session.Close()) })
	client, err := packagebridgesdk.DialFile(context.Background(), child)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	return client
}

// A local host reaches KernelIdentity over its bridge connection, and the
// kernel serves it as the session's host identity.
func TestKernelIdentityOverTheLocalBridge(t *testing.T) {
	var seen []packagebridge.HostIdentity
	client := dialSession(t, packagebridge.SessionOptions{
		KernelIdentity: func(host packagebridge.HostIdentity) kernelidentityv1.KernelIdentityServer {
			return settingsRecorder{host: host, seen: &seen}
		},
	})
	response, err := kernelidentityv1.NewKernelIdentityClient(client.Conn()).GetIdentitySettings(context.Background(), &kernelidentityv1.GetIdentitySettingsRequest{})
	require.NoError(t, err)
	require.JSONEq(t, `{"settings_schema_version":1}`, string(response.GetSettingsJson()))
	require.Equal(t, []packagebridge.HostIdentity{{PackageID: "identity-platform", Version: "4.1.0", Generation: 5}}, seen)
}

func TestKernelIdentityIsAbsentWithoutAProvider(t *testing.T) {
	client := dialSession(t, packagebridge.SessionOptions{})
	_, err := kernelidentityv1.NewKernelIdentityClient(client.Conn()).GetIdentitySettings(context.Background(), &kernelidentityv1.GetIdentitySettingsRequest{})
	require.Equal(t, codes.Unimplemented, status.Code(err))
}
