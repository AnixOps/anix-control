package pluginhost

import (
	"bytes"
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestCompiledHostHonorsConfiguredResponseLimit drives a legacy handler's
// response through the real package bridge session, the compiled identity
// host (pluginhostsdk), and the kernel host client.
func TestCompiledHostHonorsConfiguredResponseLimit(t *testing.T) {
	ref := buildIdentityPlatformArtifactRef(t)
	tests := []struct {
		name         string
		limit        int64
		responseSize int
		wantTooLarge bool
	}{
		{name: "configured 3 MiB carries a 2 MiB response", limit: 3 << 20, responseSize: 2 << 20},
		{name: "configured 6 MiB carries a 5 MiB response above the gRPC default", limit: 6 << 20, responseSize: 5 << 20},
		{name: "default 1 MiB rejects a 1.5 MiB response", limit: 0, responseSize: 3 << 19, wantTooLarge: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			legacy := func(c *gin.Context) {
				c.Data(http.StatusOK, "application/json", bytes.Repeat([]byte("x"), test.responseSize))
			}
			allowlist, err := packagebridge.NewAllowlistWithFallback(nil, packagebridge.Operation{
				PackageID: "identity-platform", RouteID: "identity.auth.login", Name: "identity.auth.login",
				Handler: packagebridge.NewHTTPAdapter(legacy),
			})
			require.NoError(t, err)
			factory := packagebridge.NewFactory(allowlist).WithSessionOptions(packagebridge.SessionOptions{MaxResponseBodyBytes: test.limit})
			manager, err := NewManager(ManagerConfig{
				RuntimeDir: shortHostTempDir(t), BridgeFactory: factory, StartupTimeout: 10 * time.Second, MaxResponseBytes: test.limit,
			})
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, manager.Shutdown(context.Background())) })
			require.NoError(t, manager.Start(context.Background(), ref, 7))

			response, err := manager.Dispatch(context.Background(), DispatchInput{
				PackageID: "identity-platform", Version: "4.0.0", Generation: 7,
				RequestID: "identity-limit-1", RouteID: "identity.auth.login", Method: http.MethodPost,
				Body:          []byte(`{"email":"u@example.test","password":"secret"}`),
				PrincipalJSON: []byte(`{"actor_id":0,"admin":false,"package_id":"identity-platform"}`),
				Metadata:      RequestMetadata{Path: "/api/v2/login"},
				Deadline:      time.Now().Add(10 * time.Second),
			})
			if test.wantTooLarge {
				require.ErrorIs(t, err, ErrResponseTooLarge)
				require.ErrorIs(t, err, ErrHostUnavailable)
				return
			}
			require.NoError(t, err)
			require.EqualValues(t, http.StatusOK, response.StatusCode)
			require.Len(t, response.Body, test.responseSize)
		})
	}
}

func TestHostReceiveMessageLimitCoversConfiguredResponses(t *testing.T) {
	require.Equal(t, 4<<20, hostReceiveMessageLimit(0))
	require.Equal(t, 4<<20, hostReceiveMessageLimit(3<<20))
	require.Equal(t, 6<<20+64<<10, hostReceiveMessageLimit(6<<20))
	require.Equal(t, int64(1<<20), normalizeMaxResponseBytes(-1))
	require.Equal(t, int64(1<<20), normalizeMaxResponseBytes(128<<20))
}
