package packagebridge

import (
	"context"
	"io"
	"testing"
	"time"

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
