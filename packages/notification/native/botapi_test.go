package native

import (
	"errors"
	"net"
	"net/http"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type failingTransport struct{ err error }

func (f failingTransport) RoundTrip(*http.Request) (*http.Response, error) { return nil, f.err }

// A transport failure must reach callers as the same fixed text the kernel's
// TelegramBotService returns (route-mode shadow parity), never as the
// *url.Error text that quotes the Bot API URL and with it the bot token.
func TestBotAPITransportErrorsCarryNoToken(t *testing.T) {
	const token = "123456789:FAKEtokenMustNeverLeak-0123456789abc"
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Bot{}))
	require.NoError(t, db.Create(&Bot{Name: "b", Token: token}).Error)

	previous := botAPIClient.Transport
	defer func() { botAPIClient.Transport = previous }()

	for _, tc := range []struct {
		name string
		fail error
		want string
	}{
		{"connect", &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}, "telegram API request failed: connect"},
		{"dns", &net.DNSError{Err: "no such host", Name: "api.telegram.org"}, "telegram API request failed: dns"},
		{"other", errors.New("proxyconnect tcp: weird"), "telegram API request failed: other"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			botAPIClient.Transport = failingTransport{err: tc.fail}
			for name, err := range map[string]error{
				"sendMessage":   sendMessage(db, 1, "hi"),
				"setWebhook":    setWebhook(db, "https://panel.example.test/hook"),
				"deleteWebhook": deleteWebhook(db),
			} {
				require.Error(t, err, name)
				assert.Equal(t, tc.want, err.Error(), name)
			}
		})
	}
}

func TestBotAPIUnparseableURLErrorCarriesNoToken(t *testing.T) {
	_, err := botAPIRequest("https://api.telegram.org/botFAKEtoken\x7fMustNeverLeak/sendMessage", nil)
	require.Error(t, err)
	assert.Equal(t, "telegram API request failed: other", err.Error())
}
