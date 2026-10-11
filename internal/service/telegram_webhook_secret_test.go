package service

import "testing"

// Telegram checks this value against the secret_token registered with setWebhook.
// Both the legacy service and the native package must derive the same value.
func TestTelegramWebhookSecretIsPinned(t *testing.T) {
	const want = "32852c38c821d604e9de8554d9b81702dea87614dab3b034a389dbe2fc654b64"
	if got := TelegramWebhookSecret("123:abc"); got != want {
		t.Fatalf("TelegramWebhookSecret = %s, want %s", got, want)
	}
}
