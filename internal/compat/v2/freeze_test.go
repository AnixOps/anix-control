package v2

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/pluginhost"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// blockingDispatcher holds each dispatch until release is closed.
type blockingDispatcher struct {
	started chan struct{}
	release chan struct{}
}

func (d *blockingDispatcher) Dispatch(ctx context.Context, _ pluginhost.DispatchInput) (pluginhost.DispatchOutput, error) {
	d.started <- struct{}{}
	<-d.release
	return pluginhost.DispatchOutput{StatusCode: http.StatusOK, Body: []byte(`{}`)}, nil
}

func TestFrozenRoutesAnswer503AndDrainWaitsForDispatchedRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dispatcher := &blockingDispatcher{started: make(chan struct{}, 1), release: make(chan struct{})}
	freeze := &RouteFreeze{}
	gateway := Gateway{
		Registry: NewRegistry(registrySourceStub{route: Route{
			Method: http.MethodPost, LegacyPath: "/api/v2/login", PackageID: "identity-platform", Version: "4.1.0",
			Generation: 1, PackageRoute: "identity.auth.login", Envelope: EnvelopeData,
		}}),
		Dispatcher: dispatcher, Timeout: time.Second, Freeze: freeze,
	}
	router := gin.New()
	router.POST("/api/v2/login", gateway.Serve)
	serve := func() *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v2/login", nil))
		return recorder
	}

	done := make(chan int)
	go func() { done <- serve().Code }()
	<-dispatcher.started
	freeze.Freeze("identity-platform", []string{"identity.auth.login"})
	frozen := serve()
	require.Equal(t, http.StatusServiceUnavailable, frozen.Code)
	require.Equal(t, "5", frozen.Header().Get("Retry-After"))
	require.Contains(t, frozen.Body.String(), "package_route_frozen")

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, freeze.Drain(ctx, "identity-platform", []string{"identity.auth.login"}), context.DeadlineExceeded,
		"the dispatched request is still in flight")
	close(dispatcher.release)
	require.Equal(t, http.StatusOK, <-done)
	require.NoError(t, freeze.Drain(context.Background(), "identity-platform", []string{"identity.auth.login"}))

	freeze.Unfreeze("identity-platform", []string{"identity.auth.login"})
	require.Equal(t, http.StatusOK, serve().Code)
}
