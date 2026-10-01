package middleware

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRedactAuditBody(t *testing.T) {
	cases := []struct {
		name, path, body, want string
	}{
		{"user password", "/api/v2/admin/users",
			`{"email":"a@example.test","password":"hunter2","plan_id":3}`,
			`{"email":"a@example.test","password":"[REDACTED]","plan_id":3}`},
		{"gateway settings sent as a JSON string", "/api/v2/admin/payment/gateways",
			`{"name":"epay","config":"{\"pid\":\"1001\",\"key\":\"merchant-key\",\"api_url\":\"https://pay.example.test\"}"}`,
			`{"config":"{\"api_url\":\"https://pay.example.test\",\"key\":\"[REDACTED]\",\"pid\":\"1001\"}","name":"epay"}`},
		{"gateway settings as an object", "/api/v2/admin/payment/gateways/2",
			`{"config":{"secret_key":"sk_live","webhook_secret":"whsec","publishable_key":"pk_live","private_key":"-----BEGIN"}}`,
			`{"config":{"private_key":"[REDACTED]","publishable_key":"pk_live","secret_key":"[REDACTED]","webhook_secret":"[REDACTED]"}}`},
		{"sensitive setting by path", "/api/v2/admin/system/configs/smtp_password",
			`{"value":"mail-pass","group":"mail"}`,
			`{"group":"mail","value":"[REDACTED]"}`},
		{"plain setting by path", "/api/v2/admin/system/configs/site_name",
			`{"value":"Anix"}`,
			`{"value":"Anix"}`},
		{"settings as key and value pairs", "/api/v2/admin/system/configs",
			`[{"key":"telegram_bot_token","value":"123:abc"},{"key":"site_name","value":"Anix"}]`,
			`[{"key":"telegram_bot_token","value":"[REDACTED]"},{"key":"site_name","value":"Anix"}]`},
		{"backup storage keys", "/api/v2/admin/system/backup/config",
			`{"s3_bucket":"b","s3_access_key":"AKIA","s3_secret_key":"s3"}`,
			`{"s3_access_key":"[REDACTED]","s3_bucket":"b","s3_secret_key":"[REDACTED]"}`},
		{"large numbers keep their digits", "/api/v2/admin/users/1",
			`{"transfer_enable":107374182400000000001}`,
			`{"transfer_enable":107374182400000000001}`},
		{"not JSON", "/api/v2/admin/upload", `name=x&password=y`, `[body omitted: 17 bytes, not JSON]`},
		{"cut JSON", "/api/v2/admin/users", `{"password":"hunt`, `[body omitted: 17 bytes, not JSON]`},
		{"empty", "/api/v2/admin/users/1/ban", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.want, redactAuditBody([]byte(c.body), c.path))
		})
	}
}

// Every credential field an admin v2 request can carry today.
func TestRedactAuditBodyCoversKnownSecretFields(t *testing.T) {
	fields := []string{
		"password", "new_password", "obfs_password", "smtp_password", "token", "api_token",
		"access_token", "bot_token", "api_key", "api_v3_key", "private_key", "secret",
		"secret_key", "secret_id", "client_secret", "webhook_secret", "totp_secret",
		"server_key", "ss_server_key", "auth_key", "key", "uuid", "s3_access_key",
	}
	for _, field := range fields {
		body, err := json.Marshal(map[string]string{field: "credential-value"})
		require.NoError(t, err)
		redacted := redactAuditBody(body, "/api/v2/admin/x")
		require.NotContains(t, redacted, "credential-value", field)
	}
}

func TestAuditLogRecordsOnlyTheRedactedBody(t *testing.T) {
	previousMode := gin.Mode()
	gin.SetMode(gin.ReleaseMode)
	t.Cleanup(func() { gin.SetMode(previousMode) })
	var logged bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logged, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	router := gin.New()
	router.Use(AuditLog())
	router.POST("/api/v2/admin/users", func(c *gin.Context) {
		var body map[string]any
		require.NoError(t, c.ShouldBindJSON(&body))
		require.Equal(t, "hunter2", body["password"], "the handler still sees the real body")
		c.Status(http.StatusOK)
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v2/admin/users",
		strings.NewReader(`{"email":"a@example.test","password":"hunter2"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(httptest.NewRecorder(), request)

	require.Contains(t, logged.String(), `[REDACTED]`)
	require.NotContains(t, logged.String(), "hunter2")
}
