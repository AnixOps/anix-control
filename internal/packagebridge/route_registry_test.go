package packagebridge

import (
	"context"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRouteRegistryAllowsOnlyItsRegisteredPackageOperation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	registry := NewRouteRegistry()
	allowlist := registry.Allowlist()
	call := Call{
		Host: HostIdentity{PackageID: "knowledge", Version: "4.0.0", Generation: 1},
		Request: Request{
			RequestID: "route-registry-1", RouteID: "knowledge.article.list", Method: "GET",
			PrincipalJSON: []byte(`{"actor_id":7,"admin":false,"package_id":"knowledge"}`),
			MetadataJSON:  []byte(`{"path":"/api/v2/user/knowledge"}`),
			Deadline:      time.Now().Add(time.Second),
		},
		Operation: "knowledge.article.list",
	}

	_, err := allowlist.Invoke(context.Background(), call)
	require.ErrorIs(t, err, ErrCapabilityRejected)

	require.NoError(t, registry.Register("knowledge", "knowledge.article.list", func(c *gin.Context) {
		c.JSON(200, gin.H{"items": []string{"article-1"}})
	}))
	response, err := allowlist.Invoke(context.Background(), call)
	require.NoError(t, err)
	require.EqualValues(t, 200, response.StatusCode)
	require.JSONEq(t, `{"items":["article-1"]}`, string(response.Body))

	call.Host.PackageID = "ticket"
	call.Request.PrincipalJSON = []byte(`{"actor_id":7,"admin":false,"package_id":"ticket"}`)
	_, err = allowlist.Invoke(context.Background(), call)
	require.ErrorIs(t, err, ErrCapabilityRejected)
}

func TestRouteRegistryResolvesOnlyItsRegisteredWebSocketOperation(t *testing.T) {
	registry := NewRouteRegistry()
	require.NoError(t, registry.RegisterWebSocket("machine-telemetry", "telemetry.monitor.ws", func(*gin.Context) {}))

	handler, ok := registry.ResolveWebSocket("machine-telemetry", "telemetry.monitor.ws", "telemetry.monitor.ws")
	require.True(t, ok)
	require.NotNil(t, handler)
	_, ok = registry.ResolveWebSocket("proxy-node", "telemetry.monitor.ws", "telemetry.monitor.ws")
	require.False(t, ok)
	_, ok = registry.ResolveWebSocket("machine-telemetry", "telemetry.monitor.ws", "telemetry.monitor.other")
	require.False(t, ok)
}
