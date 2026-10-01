package native

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The same cases as the kernel's TestGatewayConfigRedaction
// (internal/handler/payment_ownership_test.go): both sides redact and keep
// secrets alike.
func TestGatewayConfigRedaction(t *testing.T) {
	for _, key := range []string{"key", "secret_key", "webhook_secret", "client_secret", "private_key", "api_key", "api_v3_key", "sign_key", "access_token", "password"} {
		assert.True(t, gatewaySecretKey(key), key)
	}
	for _, key := range []string{"public_key", "publishable_key", "client_id", "webhook_id", "app_id", "pid", "api_url", "key_path", "wallet_address", "network"} {
		assert.False(t, gatewaySecretKey(key), key)
	}
	require.Equal(t, "", redactGatewayConfig(""))
	require.Equal(t, `{"pid":"1"}`, redactGatewayConfig(`{"pid":"1"}`), "a configuration without a secret is unchanged")
	require.Equal(t, `{"key":""}`, redactGatewayConfig(`{"key":""}`), "an empty secret is shown as empty")
	require.Equal(t, `{"confirm_blocks":12345678901234567890,"nested":{"api_key":"********"},"url":"a?b=1&c=2"}`,
		redactGatewayConfig(`{"url":"a?b=1&c=2","confirm_blocks":12345678901234567890,"nested":{"api_key":"k"}}`))
	require.Equal(t, gatewaySecretPlaceholder, redactGatewayConfig(`key=secret`), "an unreadable configuration is hidden whole")
	require.Equal(t, gatewaySecretPlaceholder, redactGatewayConfig(`{"key":"a"} trailing`))

	require.Equal(t, `{"key":"old","pid":"2"}`, keepGatewaySecrets(`{"pid":"2","key":"********"}`, `{"key":"old","pid":"1"}`))
	require.Equal(t, `{"pid":"2","key":"new"}`, keepGatewaySecrets(`{"pid":"2","key":"new"}`, `{"key":"old"}`), "a new secret is stored as sent")
	require.Equal(t, `{"note":"********"}`, keepGatewaySecrets(`{"note":"********"}`, `{"note":"x"}`), "only secrets are restored")
	require.Equal(t, `{"key":""}`, keepGatewaySecrets(`{"key":"********"}`, `not json`))
	require.Equal(t, `not json`, keepGatewaySecrets(`not json`, `{"key":"old"}`))
	require.Equal(t, "********", gatewaySecretPlaceholder)
}

func TestAmountPaysOrder(t *testing.T) {
	require.True(t, amountPaysOrder(100, 10000))
	require.True(t, amountPaysOrder(0.1+0.2, 30))
	require.False(t, amountPaysOrder(1, 10000))
	require.False(t, amountPaysOrder(100.01, 10000))
}
