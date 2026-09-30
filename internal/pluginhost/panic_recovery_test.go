//go:build unix

package pluginhost

import (
	"context"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/stretchr/testify/require"
)

type panickingPackage struct{}

func (panickingPackage) Dispatch(context.Context, pluginhostsdk.DispatchRequest) (pluginhostsdk.DispatchResponse, error) {
	panic("dispatch bug")
}

func (panickingPackage) Migrate(context.Context, pluginhostsdk.MigrationRequest) (pluginhostsdk.MigrationResponse, error) {
	panic("migrate bug")
}

func (panickingPackage) Health(context.Context) (pluginhostsdk.HealthResponse, error) {
	panic("health bug")
}

func (panickingPackage) Drain(context.Context) (pluginhostsdk.DrainResponse, error) {
	panic("drain bug")
}

func TestClientMapsPackagePanicToPackageFailure(t *testing.T) {
	host, err := pluginhostsdk.NewServer(pluginhostsdk.ServerConfig{PackageID: "knowledge", PackageVersion: "4.0.0"}, panickingPackage{})
	require.NoError(t, err)
	socketPath := startTestHostServerWithService(t, host)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := dialHostClient(ctx, socketPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, client.Close()) })

	for range 2 {
		_, err = client.Dispatch(ctx, DispatchInput{
			PackageID: "knowledge", Version: "4.0.0", Generation: 7, RequestID: "request-panic",
			RouteID: "knowledge.article.list", Method: "GET", PrincipalJSON: []byte(`{"actor_id":7}`),
			Deadline: time.Now().Add(time.Second),
		})
		require.ErrorIs(t, err, ErrHostUnavailable)
		require.ErrorIs(t, err, ErrPackageFailed)
		require.NotErrorIs(t, err, ErrHostIncompatible)
	}

	_, err = client.Migrate(ctx, MigrationInput{
		PackageID: "knowledge", Version: "4.0.0", MigrationID: "migration-panic", Generation: 7,
		Deadline: time.Now().Add(time.Second),
	})
	require.ErrorIs(t, err, ErrPackageFailed)

	_, err = client.Health(ctx, 7)
	require.ErrorIs(t, err, ErrPackageFailed)

	_, err = client.Drain(ctx, 7, time.Now().Add(time.Second))
	require.ErrorIs(t, err, ErrPackageFailed)
}
