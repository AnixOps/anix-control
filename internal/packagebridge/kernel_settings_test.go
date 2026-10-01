package packagebridge

import (
	"context"
	"testing"

	kernelsettingsv1 "github.com/AnixOps/anix-control/sdk/api/kernelsettings/v1"
	"github.com/AnixOps/anix-control/sdk/packagebridgesdk"
	"github.com/stretchr/testify/require"
)

// settingsHost answers GetSettings with the host it was built for.
type settingsHost struct {
	kernelsettingsv1.UnimplementedKernelSettingsServer
	host HostIdentity
}

func (s settingsHost) GetSettings(context.Context, *kernelsettingsv1.GetSettingsRequest) (*kernelsettingsv1.GetSettingsResponse, error) {
	return &kernelsettingsv1.GetSettingsResponse{Settings: []*kernelsettingsv1.Setting{{Key: s.host.PackageID, Value: s.host.Version}}}, nil
}

// A local session serves KernelSettings on the host's bridge connection, as
// the session's own host identity.
func TestSessionServesKernelSettingsAsItsHost(t *testing.T) {
	allowlist, err := NewAllowlistWithFallback(nil)
	require.NoError(t, err)
	identity := HostIdentity{PackageID: "notification", Version: "4.1.0", Generation: 3}
	session, child, err := NewSessionWithOptions(identity, allowlist, SessionOptions{
		KernelSettings: func(host HostIdentity) kernelsettingsv1.KernelSettingsServer { return settingsHost{host: host} },
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, session.Close()) })
	client, err := packagebridgesdk.DialFile(context.Background(), child)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Close()) })

	response, err := kernelsettingsv1.NewKernelSettingsClient(client.Conn()).GetSettings(context.Background(), &kernelsettingsv1.GetSettingsRequest{Namespace: "mail"})
	require.NoError(t, err)
	require.Equal(t, "notification", response.GetSettings()[0].GetKey())
	require.Equal(t, "4.1.0", response.GetSettings()[0].GetValue())
}
