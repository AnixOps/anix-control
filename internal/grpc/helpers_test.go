package grpc

import (
	"math"
	"testing"

	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	"github.com/AnixOps/anix-control/v4/internal/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateTokenAuthenticationModes(t *testing.T) {
	const secret = "grpc-test-jwt-secret-with-enough-length"
	jwtToken, err := utils.GenerateToken(7, "admin@example.com", true, secret, 3600)
	require.NoError(t, err)
	otherSecretToken, err := utils.GenerateToken(7, "admin@example.com", true, "a-different-secret-value-1234567890", 3600)
	require.NoError(t, err)
	userToken, err := utils.GenerateToken(8, "user@example.com", false, secret, 3600)
	require.NoError(t, err)

	tests := []struct {
		name       string
		token      string
		apiToken   string
		jwtSecret  string
		wantOK     bool
		wantClaims bool
		wantReason string
	}{
		{name: "valid JWT", token: jwtToken, jwtSecret: secret, wantOK: true, wantClaims: true},
		{name: "invalid JWT without API token fallback", token: otherSecretToken, jwtSecret: secret, wantReason: "invalid or expired JWT token"},
		{name: "invalid JWT falls back to matching API token", token: "api.token", apiToken: "api.token", jwtSecret: secret, wantOK: true},
		{name: "invalid JWT and wrong API token", token: otherSecretToken, apiToken: "api-token", jwtSecret: secret, wantReason: "invalid token"},
		{name: "API token only", token: "api-token", apiToken: "api-token", wantOK: true},
		{name: "wrong API token", token: "nope", apiToken: "api-token", wantReason: "invalid token"},
		{name: "non-JWT token with JWT secret only", token: "no-dots", jwtSecret: secret, wantReason: "invalid token"},
		{name: "user JWT is refused", token: userToken, jwtSecret: secret, wantReason: "an administrator token is required"},
		{name: "user JWT is refused with an API token configured", token: userToken, apiToken: "api-token", jwtSecret: secret, wantReason: "an administrator token is required"},
		{name: "no authentication configured refuses every token", token: "anything", wantReason: "invalid token"},
		{name: "no authentication configured refuses an empty token", token: "", wantReason: "invalid token"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, claims, reason := validateToken(tt.token, tt.apiToken, tt.jwtSecret)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.wantClaims, claims != nil)
			assert.Equal(t, tt.wantReason, reason)
			if tt.wantClaims {
				assert.Equal(t, uint(7), claims.UserID)
			}
		})
	}
}

// Once identity is finalized the kernel's own HS256 tokens stop working, on
// the gRPC services as everywhere else: the shared JWT secret must not stay
// an administrator credential there.
func TestValidateTokenRefusesHS256AfterTheIdentityCutoverIsFinalized(t *testing.T) {
	const secret = "grpc-test-jwt-secret-with-enough-length"
	adminToken, err := utils.GenerateToken(7, "admin@example.com", true, secret, 3600)
	require.NoError(t, err)

	ok, _, reason := validateToken(adminToken, "", secret)
	require.True(t, ok, "before finalize the kernel's own administrator token works: %s", reason)

	previous := legacyTokensRefused
	legacyTokensRefused = func() bool { return true }
	t.Cleanup(func() { legacyTokensRefused = previous })
	ok, claims, reason := validateToken(adminToken, "", secret)
	require.False(t, ok, "after finalize an HS256 administrator token is refused")
	require.Nil(t, claims)
	require.Equal(t, "invalid or expired JWT token", reason)

	ok, _, _ = validateToken("shared-api-token", "shared-api-token", secret)
	require.True(t, ok, "the configured grpc.api_token is a different credential and is not part of this refusal")
}

func TestAnyToInt32ConvertsSupportedNumericTypes(t *testing.T) {
	got, ok, err := anyToInt32("port", int32(443))
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, int32(443), got)

	got, ok, err = anyToInt32("port", int64(8443))
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, int32(8443), got)

	_, ok, err = anyToInt32("port", int64(math.MaxInt32)+1)
	assert.Error(t, err)
	assert.False(t, ok)

	got, ok, err = anyToInt32("port", float64(80))
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, int32(80), got)

	for _, invalid := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		_, ok, err = anyToInt32("port", invalid)
		assert.Error(t, err)
		assert.False(t, ok)
	}
}

func TestAgentControlManagerRemovePendingOnlyRemovesMatchingWaiter(t *testing.T) {
	manager := NewAgentControlManager()
	key := agentOperationKey{nodeID: 3, operationID: "op-1"}
	current := make(chan *agentv1pb.OperationAck, 1)
	stale := make(chan *agentv1pb.OperationAck, 1)
	manager.pending[key] = current

	manager.removePending(key, stale)
	assert.Equal(t, current, manager.pending[key], "a stale waiter must not remove the current one")

	manager.removePending(key, current)
	_, exists := manager.pending[key]
	assert.False(t, exists)
}
