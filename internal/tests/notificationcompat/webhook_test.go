package notificationcompat

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
	"testing"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/internal/handler"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/tests/packagecompat"
	"github.com/AnixOps/anix-control/v4/packages/notification/native"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// botAPI stands for the Telegram Bot API. The kernel's and the package's
// clients both use http.DefaultTransport, which the test replaces: it
// records each call and answers JSON, or a body that is not JSON for the
// bot token "1:broken".
type botAPI struct {
	mu    sync.Mutex
	calls []string
}

func (b *botAPI) RoundTrip(request *http.Request) (*http.Response, error) {
	body, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, err
	}
	b.mu.Lock()
	b.calls = append(b.calls, request.Method+" "+request.URL.String()+" "+string(bytes.TrimSpace(body)))
	b.mu.Unlock()
	answer := `{"ok":true,"result":true,"description":"Webhook was set"}`
	if bytes.Contains([]byte(request.URL.Path), []byte("/bot1:broken/")) {
		answer = `<html>bad gateway</html>`
	}
	return &http.Response{
		StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}},
		Body: io.NopCloser(bytes.NewBufferString(answer)), Request: request,
	}, nil
}

// drain returns the calls since the last drain.
func (b *botAPI) drain() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	calls := b.calls
	b.calls = nil
	return calls
}

// withBotAPI replaces http.DefaultTransport with a botAPI for the test.
func withBotAPI(t *testing.T) *botAPI {
	t.Helper()
	api := &botAPI{}
	previous := http.DefaultTransport
	http.DefaultTransport = api
	t.Cleanup(func() { http.DefaultTransport = previous })
	return api
}

// seedBrokenBot is seedBot("[1]") with a token whose Bot API answer is not
// JSON.
func seedBrokenBot(t testing.TB, db *gorm.DB) {
	seedBot("[1]")(t, db)
	require.NoError(t, db.Model(&model.TelegramBot{}).Where("id = 1").Update("token", "1:broken").Error)
}

// The webhook's default URL is the administrator's request's scheme and
// host: the legacy handler reads them from the request, the native one from
// the request scheme and host the kernel sends, and both honour
// X-Forwarded-Proto. Each side's Bot API calls are part of its state.
func TestAdminSetWebhookParity(t *testing.T) {
	api := withBotAPI(t)
	r := route("POST", "/api/v2/admin/telegram/webhook", native.SetWebhookRouteID, telegram((*handler.TelegramHandler).SetWebhook))
	path := "/api/v2/admin/telegram/webhook"
	withCalls := func(t testing.TB, db *gorm.DB) any {
		return map[string]any{"rows": rows(t, db), "bot_api": api.drain()}
	}
	for _, c := range []packagecompat.Case{
		{Name: "no body: the request's scheme and host", Path: path, Principal: admin, Seed: seedBot("[1]")},
		{Name: "an empty object", Path: path, Principal: admin, Seed: seedBot("[1]"), Body: []byte(`{}`)},
		{Name: "a blank url", Path: path, Principal: admin, Seed: seedBot("[1]"), Body: []byte(`{"url":""}`)},
		{Name: "null", Path: path, Principal: admin, Seed: seedBot("[1]"), Body: []byte(`null`)},
		{Name: "a TLS request with a port", Path: path, Principal: admin, Seed: seedBot("[1]"), Host: "panel.example.test:8443", TLS: true},
		{Name: "an IPv6 host", Path: path, Principal: admin, Seed: seedBot("[1]"), Host: "[2001:db8::7]:8080"},
		{Name: "X-Forwarded-Proto", Path: path, Principal: admin, Seed: seedBot("[1]"), Host: "panel.example.test",
			RequestHeaders: map[string]string{"X-Forwarded-Proto": "https"}},
		{Name: "X-Forwarded-Proto over TLS", Path: path, Principal: admin, Seed: seedBot("[1]"), TLS: true,
			RequestHeaders: map[string]string{"X-Forwarded-Proto": " http "}},
		{Name: "a blank X-Forwarded-Proto", Path: path, Principal: admin, Seed: seedBot("[1]"), TLS: true,
			RequestHeaders: map[string]string{"X-Forwarded-Proto": "   "}},
		{Name: "an explicit url", Path: path, Principal: admin, Seed: seedBot("[1]"),
			Body: []byte(`{"url":"https://hooks.example.test/tg"}`)},
		{Name: "an explicit url wins over the request", Path: path, Principal: admin, Seed: seedBot("[1]"), TLS: true,
			RequestHeaders: map[string]string{"X-Forwarded-Proto": "https"}, Body: []byte(`{"url":"http://other.example.test/hook"}`)},
		{Name: "a url that is not one", Path: path, Principal: admin, Seed: seedBot("[1]"), Body: []byte(`{"url":"not a url"}`)},
		{Name: "a url that is a number", Path: path, Principal: admin, Seed: seedBot("[1]"), Body: []byte(`{"url":7}`)},
		{Name: "a body that does not parse", Path: path, Principal: admin, Seed: seedBot("[1]"), Body: []byte(`{"url":`)},
		{Name: "no bot", Path: path, Principal: admin},
		{Name: "a Bot API answer that is not JSON", Path: path, Principal: admin, Seed: seedBrokenBot},
	} {
		c.Snapshot = withCalls
		packagecompat.RunWrite(t, r, c)
	}
	require.Empty(t, api.drain())
}

// A kernel that sends no request scheme or host (one that predates them)
// leaves a webhook without a url to the legacy handler; one with a url
// needs neither.
func TestAdminSetWebhookDefersWithoutTheRequestAddress(t *testing.T) {
	withBotAPI(t)
	service := &native.Service{Open: func(ctx context.Context) (*gorm.DB, error) { return nil, errors.New("no storage") }}
	for _, metadata := range []pluginhostsdk.RequestMetadata{{}, {Scheme: "https"}, {Host: "panel.example.test"}} {
		_, err := service.AdminSetWebhook(context.Background(), pluginhostsdk.NativeRequest{
			RouteID: native.SetWebhookRouteID, Method: "POST", Principal: admin, Metadata: metadata,
		})
		require.ErrorIs(t, err, pluginhostsdk.ErrNativeUnavailable, "%+v", metadata)
	}
	response, err := service.AdminSetWebhook(context.Background(), pluginhostsdk.NativeRequest{
		RouteID: native.SetWebhookRouteID, Method: "POST", Principal: admin, Body: []byte(`{"url":"https://hooks.example.test/tg"}`),
	})
	require.NoError(t, err)
	require.Contains(t, string(response.Body), "no storage")
}
