package packagebridge

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/packagebridgesdk"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func legacyBodyHandler(size int) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Data(http.StatusOK, "application/octet-stream", bytes.Repeat([]byte("r"), size))
	}
}

func invokeLegacyThroughSession(t *testing.T, options SessionOptions, handler gin.HandlerFunc, payload []byte) (packagebridgesdk.Response, error) {
	t.Helper()
	allowlist, err := NewAllowlistWithFallback(nil, Operation{
		PackageID: "knowledge", RouteID: "knowledge.article.list", Name: "knowledge.article.list",
		Handler: NewHTTPAdapter(handler),
	})
	require.NoError(t, err)
	session, child, err := NewSessionWithOptions(HostIdentity{PackageID: "knowledge", Version: "4.0.0", Generation: 7}, allowlist, options)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, session.Close()) })
	capability, err := session.Mint(Request{
		RequestID: "request-limit", RouteID: "knowledge.article.list", Method: http.MethodPost, Body: payload,
		PrincipalJSON: []byte(`{"actor_id":1,"admin":false,"package_id":"knowledge"}`),
		MetadataJSON:  []byte(`{"path":"/api/v2/user/knowledge"}`), Deadline: time.Now().Add(10 * time.Second),
	})
	require.NoError(t, err)
	client, err := packagebridgesdk.DialFile(context.Background(), child)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	return client.Invoke(context.Background(), capability, "knowledge.article.list", payload)
}

func TestSessionRejectsLegacyResponseAboveDefaultLimit(t *testing.T) {
	_, err := invokeLegacyThroughSession(t, SessionOptions{}, legacyBodyHandler(3<<19), nil)
	require.ErrorIs(t, err, packagebridgesdk.ErrResponseTooLarge)
	require.Equal(t, codes.ResourceExhausted, status.Code(err))
}

func TestSessionHonorsConfiguredLimitsAboveGRPCDefault(t *testing.T) {
	// 5 MiB each way exceeds grpc-go's 4 MiB default receive size, so this
	// also proves the session and SDK size their gRPC windows.
	const size = 5 << 20
	received := make(chan int, 1)
	handler := func(c *gin.Context) {
		body, _ := io.ReadAll(c.Request.Body)
		received <- len(body)
		legacyBodyHandler(size)(c)
	}
	options := SessionOptions{MaxResponseBodyBytes: 6 << 20, MaxRequestBodyBytes: 6 << 20}
	response, err := invokeLegacyThroughSession(t, options, handler, bytes.Repeat([]byte("q"), size))
	require.NoError(t, err)
	require.Len(t, response.Body, size)
	require.Equal(t, size, <-received)
}

func TestSessionOptionsReceiveLimit(t *testing.T) {
	require.Equal(t, defaultGRPCMessageBytes, SessionOptions{}.receiveMessageLimit())
	require.Equal(t, int(8<<20+messageEnvelopeBytes), SessionOptions{MaxRequestBodyBytes: 8 << 20}.receiveMessageLimit())
	require.Equal(t, int(12<<20+messageEnvelopeBytes), SessionOptions{MaxResponseBodyBytes: 12 << 20, MaxRequestBodyBytes: 2 << 20}.receiveMessageLimit())
}
