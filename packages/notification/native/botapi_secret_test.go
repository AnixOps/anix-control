package native

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Telegram checks this value against the secret_token registered with setWebhook;
// it must match service.TelegramWebhookSecret.
func TestTelegramWebhookSecretIsPinned(t *testing.T) {
	assert.Equal(t, "32852c38c821d604e9de8554d9b81702dea87614dab3b034a389dbe2fc654b64", telegramWebhookSecret("123:abc"))
}

type recordingTransport struct{ body []byte }

func (r *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r.body, _ = io.ReadAll(req.Body)
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Request: req,
		Body: io.NopCloser(strings.NewReader(`{"ok":true,"result":true}`))}, nil
}

// setWebhook must register the secret, or the legacy receiver rejects every update.
func TestSetWebhookRegistersSecret(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Bot{}))
	require.NoError(t, db.Create(&Bot{Name: "b", Token: "123:abc"}).Error)

	previous := botAPIClient.Transport
	defer func() { botAPIClient.Transport = previous }()
	recorder := &recordingTransport{}
	botAPIClient.Transport = recorder

	require.NoError(t, setWebhook(db, "https://panel.example.test/api/v2/telegram/webhook"))
	assert.Contains(t, string(recorder.body), `"secret_token":"32852c38c821d604e9de8554d9b81702dea87614dab3b034a389dbe2fc654b64"`)
}
