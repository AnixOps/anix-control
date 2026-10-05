package service

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/assert"
)

const probeTestToken = "123456789:SECRETtokenValueForTests-0123456789abc"

func TestClassifyTelegramAnswer(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   TelegramProbeResult
	}{
		{"ok", 200, `{"ok":true,"result":{}}`, TelegramProbeResult{Class: TelegramProbeOK}},
		{"ok but 4xx status", 400, `{"ok":true}`, TelegramProbeResult{Class: TelegramProbeUnknown, ErrorCode: 400}},
		{"unauthorized", 401, `{"ok":false,"error_code":401,"description":"Unauthorized"}`, TelegramProbeResult{Class: TelegramProbeInvalidToken, ErrorCode: 401}},
		{"the error code of the body wins", 200, `{"ok":false,"error_code":403,"description":"Forbidden"}`, TelegramProbeResult{Class: TelegramProbeBotBlocked, ErrorCode: 403}},
		{"not found", 404, `{"ok":false,"error_code":404,"description":"Not Found"}`, TelegramProbeResult{Class: TelegramProbeInvalidToken, ErrorCode: 404}},
		{"chat not found", 400, `{"ok":false,"error_code":400,"description":"Bad Request: CHAT NOT FOUND"}`, TelegramProbeResult{Class: TelegramProbeChatNotFound, ErrorCode: 400}},
		{"peer id invalid", 400, `{"ok":false,"error_code":400,"description":"Bad Request: PEER_ID_INVALID"}`, TelegramProbeResult{Class: TelegramProbeChatNotFound, ErrorCode: 400}},
		{"chat not found is only a 400", 500, `{"ok":false,"error_code":500,"description":"chat not found"}`, TelegramProbeResult{Class: TelegramProbeUnknown, ErrorCode: 500}},
		{"forbidden", 403, `{"ok":false,"error_code":403,"description":"Forbidden: bot was blocked by the user"}`, TelegramProbeResult{Class: TelegramProbeBotBlocked, ErrorCode: 403}},
		{"rate limited", 429, `{"ok":false,"error_code":429}`, TelegramProbeResult{Class: TelegramProbeRateLimited, ErrorCode: 429}},
		{"empty answer", 500, ``, TelegramProbeResult{Class: TelegramProbeUnknown, ErrorCode: 500}},
		{"redirect", 307, ``, TelegramProbeResult{Class: TelegramProbeUnknown, ErrorCode: 307}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, classifyTelegramAnswer(tc.status, []byte(tc.body)))
		})
	}
}

func TestTelegramProbeMessagesAreFixed(t *testing.T) {
	for _, class := range []TelegramProbeClass{TelegramProbeOK, TelegramProbeInvalidToken, TelegramProbeChatNotFound, TelegramProbeBotBlocked,
		TelegramProbeRateLimited, TelegramProbeNotConfigured, TelegramProbeNetworkError, TelegramProbeUnknown} {
		message := TelegramProbeResult{Class: class}.Message()
		assert.NotEmpty(t, message, class)
		assert.NotContains(t, message, "api.telegram.org")
	}
	assert.True(t, TelegramProbeResult{Class: TelegramProbeOK}.OK())
	assert.False(t, TelegramProbeResult{Class: TelegramProbeNetworkError}.OK())
}

func TestTelegramNetworkReason(t *testing.T) {
	quoted := func(err error) error {
		return &url.Error{Op: "Post", URL: "https://api.telegram.org/bot" + probeTestToken + "/sendMessage", Err: err}
	}
	timeout := &net.OpError{Op: "read", Err: timeoutError{}}
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"dns", quoted(&net.DNSError{Err: "no such host", Name: "api.telegram.org", IsNotFound: true}), TelegramReasonDNS},
		{"refused", quoted(&net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}), TelegramReasonConnect},
		{"net timeout", quoted(timeout), TelegramReasonTimeout},
		{"deadline", quoted(context.DeadlineExceeded), TelegramReasonTimeout},
		{"canceled", quoted(context.Canceled), TelegramReasonCanceled},
		{"unknown authority", quoted(x509.UnknownAuthorityError{}), TelegramReasonTLS},
		{"hostname", quoted(x509.HostnameError{Host: "api.telegram.org"}), TelegramReasonTLS},
		{"other", quoted(errors.New("proxyconnect tcp: weird")), TelegramReasonOther},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, telegramNetworkReason(tc.err))
		})
	}
}

type timeoutError struct{}

func (timeoutError) Error() string   { return "i/o timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

func TestProbeSendWithoutABot(t *testing.T) {
	service := NewTelegramBotService(nil)
	assert.Equal(t, TelegramProbeNotConfigured, service.ProbeSend(t.Context(), nil, 1, "x").Class)
	assert.Equal(t, TelegramProbeNotConfigured, service.ProbeSend(t.Context(), &model.TelegramBot{Token: " \t"}, 1, "x").Class)
	assert.Equal(t, TelegramProbeInvalidToken, service.ProbeSend(t.Context(), &model.TelegramBot{Token: "../x"}, 1, "x").Class)
}

// A certificate the system does not trust is a tls network error, and the
// answer does not carry the URL that net/http's error quotes.
func TestProbeSendTLSFailureIsScrubbed(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	t.Cleanup(server.Close)
	service := NewTelegramBotService(nil)
	service.SetAPIBase(server.URL + "/")
	result := service.ProbeSend(t.Context(), &model.TelegramBot{Token: probeTestToken}, 1, "x")
	assert.Equal(t, TelegramProbeResult{Class: TelegramProbeNetworkError, Reason: TelegramReasonTLS}, result)
	assert.NotContains(t, fmt.Sprintf("%+v", result), "SECRET")
}

// The probe gives up when its context ends, long before its own timeout.
func TestProbeSendHonoursTheContext(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
	t.Cleanup(func() { close(release); server.Close() })
	service := NewTelegramBotService(nil)
	service.SetAPIBase(server.URL)
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	result := service.ProbeSend(ctx, &model.TelegramBot{Token: probeTestToken}, 1, "x")
	assert.Equal(t, TelegramProbeResult{Class: TelegramProbeNetworkError, Reason: TelegramReasonTimeout}, result)
	assert.Less(t, time.Since(started), 3*time.Second)
}

func TestProbeDefaultBase(t *testing.T) {
	service := NewTelegramBotService(nil)
	assert.Equal(t, "https://api.telegram.org", service.probeBase())
	service.SetAPIBase("http://127.0.0.1:1/")
	assert.Equal(t, "http://127.0.0.1:1", service.probeBase())
}
