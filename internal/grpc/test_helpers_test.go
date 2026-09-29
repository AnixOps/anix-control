package grpc

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/database"
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

// connectionForTest returns a registered node connection for assertions.
func (m *NodeConnectionManager) connectionForTest(nodeID uint32) (*NodeConnection, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	conn, ok := m.connections[nodeID]
	return conn, ok
}

// activeNodesForTest returns the IDs of all registered node connections.
func (m *NodeConnectionManager) activeNodesForTest() []uint32 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	nodes := make([]uint32, 0, len(m.connections))
	for id := range m.connections {
		nodes = append(nodes, id)
	}
	return nodes
}

// configVersionForTest returns the last config version recorded for a node.
func (m *NodeConnectionManager) configVersionForTest(nodeID uint32) int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.configVer[nodeID]
}

// apiKeyHashForTest mirrors the stored node API key hash (hex SHA-256).
func apiKeyHashForTest(apiKey string) string {
	sum := sha256.Sum256([]byte(apiKey))
	return hex.EncodeToString(sum[:])
}
