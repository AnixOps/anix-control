package packagebridge

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/agentws"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestHTTPAdapterUsesKernelBoundRequestState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adapter := NewHTTPAdapter(func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		userID, _ := c.Get("user_id")
		admin, _ := c.Get("is_admin")
		nodeID, _ := c.Get("node_id")
		c.Header("Retry-After", "1")
		c.JSON(200, gin.H{
			"body": string(body), "ip": c.ClientIP(), "user_agent": c.GetHeader("User-Agent"),
			"signature": c.GetHeader("Stripe-Signature"), "content_type": c.GetHeader("Content-Type"),
			"query": c.QueryArray("tag"), "user_id": userID, "admin": admin, "node_id": nodeID,
		})
	})
	response, err := adapter(context.Background(), Call{
		Host: HostIdentity{PackageID: "identity-platform", Version: "4.0.0", Generation: 7},
		Request: Request{
			RequestID: "request-http-1", RouteID: "identity.auth.login", Method: "POST",
			Body:          []byte(`{"email":"trusted@example.test"}`),
			PrincipalJSON: []byte(`{"actor_id":42,"admin":true,"package_id":"identity-platform"}`),
			MetadataJSON:  []byte(`{"path":"/api/v2/login","query":{"tag":["stable","v4"]},"headers":{"Content-Type":["application/json"],"Stripe-Signature":["signed-payload"]},"client_ip":"198.51.100.42","user_agent":"AnixOps-Test/1.0","node_id":9}`),
			Deadline:      time.Now().Add(time.Second),
		},
		Operation: "identity.auth.login",
		Payload:   []byte(`{"email":"host-controlled@example.test"}`),
	})

	require.NoError(t, err)
	require.EqualValues(t, 200, response.StatusCode)
	require.Equal(t, []Header{{Name: "Content-Type", Value: "application/json; charset=utf-8"}, {Name: "Retry-After", Value: "1"}}, response.Headers)
	require.JSONEq(t, `{"body":"{\"email\":\"trusted@example.test\"}","ip":"198.51.100.42","user_agent":"AnixOps-Test/1.0","signature":"signed-payload","content_type":"application/json","query":["stable","v4"],"user_id":42,"admin":true,"node_id":9}`, string(response.Body))
}

func TestHTTPAdapterRejectsUntrustedMetadata(t *testing.T) {
	adapter := NewHTTPAdapter(func(*gin.Context) {})
	_, err := adapter(context.Background(), Call{
		Host:    HostIdentity{PackageID: "identity-platform", Version: "4.0.0", Generation: 7},
		Request: Request{RequestID: "request-http-2", RouteID: "identity.auth.login", Method: "POST", MetadataJSON: []byte(`{"client_ip":"not-an-ip"}`)},
	})
	require.ErrorIs(t, err, ErrCapabilityRejected)
}

func TestHTTPAdapterRejectsSensitiveHeaderSnapshot(t *testing.T) {
	adapter := NewHTTPAdapter(func(*gin.Context) {})
	_, err := adapter(context.Background(), Call{
		Host: HostIdentity{PackageID: "payment", Version: "4.0.0", Generation: 7},
		Request: Request{
			RequestID: "request-http-sensitive", RouteID: "payment.callback", Method: "POST",
			PrincipalJSON: []byte(`{"actor_id":0,"admin":false,"package_id":"payment"}`),
			MetadataJSON:  []byte(`{"path":"/api/v2/payment/callback/stripe","headers":{"Authorization":["Bearer secret"]}}`),
			Deadline:      time.Now().Add(time.Second),
		},
		Operation: "payment.callback",
	})
	require.ErrorIs(t, err, ErrCapabilityRejected)
}

func TestBridgeResponseStatusCodeRejectsUnrepresentableStatus(t *testing.T) {
	status, err := bridgeResponseStatusCode(200)
	require.NoError(t, err)
	require.EqualValues(t, 200, status)

	_, err = bridgeResponseStatusCode(-1)
	require.Error(t, err)
	_, err = bridgeResponseStatusCode(int(^uint32(0)) + 1)
	require.Error(t, err)
}

func TestHTTPAdapterRestoresTheOriginalRequestAddress(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adapter := NewHTTPAdapter(func(c *gin.Context) {
		c.JSON(200, gin.H{"host": c.Request.Host, "tls": c.Request.TLS != nil})
	})
	call := func(metadata string) (Response, error) {
		return adapter(context.Background(), Call{
			Host: HostIdentity{PackageID: "forward", Version: "4.0.0", Generation: 7},
			Request: Request{
				RequestID: "request-http-host", RouteID: "forward.forward_agent.install_sh.get", Method: "GET",
				PrincipalJSON: []byte(`{"actor_id":0,"admin":false,"package_id":"forward"}`),
				MetadataJSON:  []byte(metadata),
				Deadline:      time.Now().Add(time.Second),
			},
			Operation: "forward.forward_agent.install_sh.get",
		})
	}

	response, err := call(`{"path":"/api/v2/forward-agent/install.sh","host":"panel.example.test:8443","tls":true}`)
	require.NoError(t, err)
	require.JSONEq(t, `{"host":"panel.example.test:8443","tls":true}`, string(response.Body))

	response, err = call(`{"path":"/api/v2/forward-agent/install.sh","host":"[2001:db8::1]:8080"}`)
	require.NoError(t, err)
	require.JSONEq(t, `{"host":"[2001:db8::1]:8080","tls":false}`, string(response.Body))

	_, err = call(`{"path":"/api/v2/forward-agent/install.sh","host":"evil.test/x?y"}`)
	require.ErrorIs(t, err, ErrCapabilityRejected)
}

// The kernel resolved the original scheme, host and client address; the
// legacy handler sees exactly those, even when a snapshot carries forwarding
// headers (the bridge engine trusts no proxy and drops them).
func TestHTTPAdapterIgnoresForwardingHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adapter := NewHTTPAdapter(func(c *gin.Context) {
		c.JSON(200, gin.H{"host": c.Request.Host, "tls": c.Request.TLS != nil, "ip": c.ClientIP(),
			"xfp": c.GetHeader("X-Forwarded-Proto"), "xfh": c.GetHeader("X-Forwarded-Host"), "trace": c.GetHeader("X-Trace")})
	})
	response, err := adapter(context.Background(), Call{
		Host: HostIdentity{PackageID: "forward", Version: "4.0.0", Generation: 7},
		Request: Request{
			RequestID: "request-forwarded", RouteID: "forward.forward_agent.install_sh.get", Method: "GET",
			PrincipalJSON: []byte(`{"actor_id":0,"admin":false,"package_id":"forward"}`),
			MetadataJSON: []byte(`{"path":"/api/v2/forward-agent/install.sh","host":"panel.example.test","client_ip":"127.0.0.1",` +
				`"headers":{"X-Forwarded-Proto":["https"],"X-Forwarded-Host":["evil.example"],"X-Forwarded-For":["198.51.100.66"],"X-Real-Ip":["198.51.100.67"],"X-Trace":["kept"]}}`),
			Deadline: time.Now().Add(time.Second),
		},
		Operation: "forward.forward_agent.install_sh.get",
	})
	require.NoError(t, err)
	require.JSONEq(t, `{"host":"panel.example.test","tls":false,"ip":"127.0.0.1","xfp":"","xfh":"","trace":"kept"}`, string(response.Body))
}

// The agent node identity the kernel verified before the gateway reaches the
// legacy agent handler as trusted, which the agent HTTP routes require.
func TestHTTPAdapterRelaysTheKernelVerifiedAgentNode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adapter := NewHTTPAdapter(func(c *gin.Context) {
		nodeID, _ := c.Get("node_id")
		trusted, _ := c.Get(agentws.TrustedContextKey)
		forward, _ := c.Get(agentws.ForwardNodeContextKey)
		c.JSON(200, gin.H{"node_id": nodeID, "trusted": trusted, "forward": forward})
	})
	call := func(metadata string) string {
		response, err := adapter(context.Background(), Call{
			Host: HostIdentity{PackageID: "protocol-runtime", Version: "4.0.0", Generation: 7},
			Request: Request{
				RequestID: "request-agent-http", RouteID: "protocol.agent.tasks.get", Method: "GET",
				PrincipalJSON: []byte(`{"actor_id":0,"admin":false,"package_id":"protocol-runtime"}`),
				MetadataJSON:  []byte(metadata),
				Deadline:      time.Now().Add(time.Second),
			},
			Operation: "protocol.agent.tasks.get",
		})
		require.NoError(t, err)
		return string(response.Body)
	}
	require.JSONEq(t, `{"node_id":12,"trusted":true,"forward":true}`,
		call(`{"path":"/api/v2/agent/tasks","node_id":12,"trusted_agent_websocket_auth":true,"trusted_agent_websocket_forward_node":true}`))
	require.JSONEq(t, `{"node_id":12,"trusted":true,"forward":false}`,
		call(`{"path":"/api/v2/agent/tasks","node_id":12,"trusted_agent_websocket_auth":true}`))
	require.JSONEq(t, `{"node_id":12,"trusted":null,"forward":null}`,
		call(`{"path":"/api/v2/agent/tasks","node_id":12}`))
}
