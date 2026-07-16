package grpc

import (
	"errors"
	"net"
	"testing"

	"github.com/AnixOps/anix-control/v3/internal/config"
	"github.com/AnixOps/anix-control/v3/internal/database"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type grpcCloseSender interface {
	CloseSend() error
}

func requireInMemoryDatabase(t testing.TB) {
	t.Helper()

	require.NoError(t, database.Init(&config.DatabaseConfig{
		Driver:   "sqlite",
		Database: ":memory:",
		LogLevel: "silent",
	}))
	sqlDB, err := database.Get().DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
}

func requireAutoMigrate(t testing.TB, models ...any) {
	t.Helper()

	require.NoError(t, database.AutoMigrate(models...))
}

func requireDatabaseClosed(t testing.TB) {
	t.Helper()

	require.NoError(t, database.Close())
}

func requireClientConnClosed(t testing.TB, conn *grpc.ClientConn) {
	t.Helper()

	require.NoError(t, conn.Close())
}

func requireCloseSend(t testing.TB, stream grpcCloseSender) {
	t.Helper()

	require.NoError(t, stream.CloseSend())
}

func serveGRPCServerForTest(t testing.TB, server *grpc.Server, lis net.Listener) <-chan error {
	t.Helper()

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Serve(lis)
	}()

	return errCh
}

func stopGRPCServerForTest(t testing.TB, server *grpc.Server, errCh <-chan error) {
	t.Helper()

	server.GracefulStop()
	if errCh == nil {
		return
	}

	err := <-errCh
	if err != nil && !errors.Is(err, grpc.ErrServerStopped) {
		require.NoError(t, err)
	}
}
