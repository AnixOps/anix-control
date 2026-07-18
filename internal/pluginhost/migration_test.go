package pluginhost

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	pluginhostv1 "github.com/AnixOps/anix-control/v4/api/pluginhost/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

func TestManagerMigratePersistsCheckpointBeforeInterruptionAndResume(t *testing.T) {
	server := &migrationTestHostServer{
		responses: []*pluginhostv1.MigrationResponse{
			{Checkpoint: "opaque-step-1", Complete: false},
			{Checkpoint: "opaque-step-2", ValidationDigest: "opaque-validation", Complete: true},
		},
		healthLeaseID: "lease-5",
	}
	client := dialMigrationTestHost(t, server)
	manager := newTestManager(t, &hostProcess{
		packageID: "order", version: "4.0.0", generation: 5, client: client, leaseID: "lease-5",
	})
	t.Cleanup(func() { require.NoError(t, client.Close()) })

	var checkpoints []MigrationOutput
	recorder := func(_ context.Context, output MigrationOutput) error {
		checkpoints = append(checkpoints, output)
		return nil
	}
	first, err := manager.Migrate(context.Background(), MigrationInput{
		PackageID: "order", Version: "4.0.0", MigrationID: "order-v4", Generation: 5,
	}, recorder)
	require.NoError(t, err)
	require.False(t, first.Complete)
	require.Equal(t, "opaque-step-1", first.Checkpoint)

	second, err := manager.Migrate(context.Background(), MigrationInput{
		PackageID: "order", Version: "4.0.0", MigrationID: "order-v4", Generation: 5, Checkpoint: first.Checkpoint,
	}, recorder)
	require.NoError(t, err)
	require.True(t, second.Complete)
	require.Equal(t, "opaque-validation", second.ValidationDigest)
	require.Equal(t, []MigrationOutput{
		{Checkpoint: "opaque-step-1"},
		{Checkpoint: "opaque-step-2", ValidationDigest: "opaque-validation", Complete: true},
	}, checkpoints)
	require.Len(t, server.requests, 2)
	require.Equal(t, "opaque-step-1", server.requests[1].GetCheckpoint())
	require.Len(t, server.healthRequests, 1)
	require.EqualValues(t, 5, server.healthRequests[0].GetRouteGeneration())
}

func TestManagerMigrateRejectsActivationWhenHealthLeaseChanges(t *testing.T) {
	server := &migrationTestHostServer{
		responses:     []*pluginhostv1.MigrationResponse{{Checkpoint: "opaque-done", Complete: true}},
		healthLeaseID: "replacement-lease",
	}
	client := dialMigrationTestHost(t, server)
	manager := newTestManager(t, &hostProcess{
		packageID: "order", version: "4.0.0", generation: 5, client: client, leaseID: "lease-5",
	})
	t.Cleanup(func() { require.NoError(t, client.Close()) })

	persisted := false
	_, err := manager.Migrate(context.Background(), MigrationInput{
		PackageID: "order", Version: "4.0.0", MigrationID: "order-v4", Generation: 5,
	}, func(context.Context, MigrationOutput) error {
		persisted = true
		return nil
	})

	require.ErrorIs(t, err, ErrHostIncompatible)
	require.True(t, persisted, "the host reply must be durable before activation is considered")
}

type migrationTestHostServer struct {
	pluginhostv1.UnimplementedControlPackageHostServer

	mu             sync.Mutex
	responses      []*pluginhostv1.MigrationResponse
	requests       []*pluginhostv1.MigrationRequest
	healthRequests []*pluginhostv1.HealthRequest
	healthLeaseID  string
}

func (s *migrationTestHostServer) Migrate(_ context.Context, request *pluginhostv1.MigrationRequest) (*pluginhostv1.MigrationResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, request)
	if len(s.responses) == 0 {
		return &pluginhostv1.MigrationResponse{}, nil
	}
	response := s.responses[0]
	s.responses = s.responses[1:]
	return response, nil
}

func (s *migrationTestHostServer) Health(_ context.Context, request *pluginhostv1.HealthRequest) (*pluginhostv1.HealthResponse, error) {
	s.mu.Lock()
	s.healthRequests = append(s.healthRequests, request)
	s.mu.Unlock()
	return &pluginhostv1.HealthResponse{Healthy: true, LeaseId: s.healthLeaseID}, nil
}

func dialMigrationTestHost(t *testing.T, server pluginhostv1.ControlPackageHostServer) *hostClient {
	t.Helper()
	socketPath := filepath.Join(t.TempDir(), "migration.sock")
	listener, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	require.NoError(t, os.Chmod(socketPath, 0o600))
	grpcServer := grpc.NewServer()
	pluginhostv1.RegisterControlPackageHostServer(grpcServer, server)
	go func() { _ = grpcServer.Serve(listener) }()
	t.Cleanup(grpcServer.Stop)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	client, err := dialHostClient(ctx, socketPath)
	require.NoError(t, err)
	return client
}
