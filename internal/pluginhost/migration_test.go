package pluginhost

import (
	"context"
	"sync"
	"testing"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/stretchr/testify/require"
)

func TestManagerMigratePersistsCheckpointBeforeInterruptionAndResume(t *testing.T) {
	server := &migrationTestHostServer{
		responses: []pluginhostsdk.MigrationResponse{
			{Checkpoint: "opaque-step-1", Complete: false},
			{Checkpoint: "opaque-step-2", ValidationDigest: "opaque-validation", Complete: true},
		},
		healthLeaseID: "lease-5",
	}
	client := newPackageClient(server, 0)
	manager := newTestManager(t, &hostProcess{
		packageID: "order", version: "4.0.0", generation: 5, client: client, leaseID: "lease-5",
	})

	var checkpoints []MigrationOutput
	recorder := func(_ context.Context, output MigrationOutput) error {
		server.recordEvent("checkpoint")
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
	require.Equal(t, "opaque-step-1", server.requests[1].Checkpoint)
	require.Equal(t, 1, server.healthCalls)
	require.Equal(t, []string{"checkpoint", "checkpoint", "health"}, server.eventSnapshot())
	require.Equal(t, "lease-5", second.HealthLeaseID)
	require.EqualValues(t, 5, second.HealthGeneration)
}

func TestManagerMigrateRejectsActivationWhenHealthLeaseChanges(t *testing.T) {
	server := &migrationTestHostServer{
		responses:     []pluginhostsdk.MigrationResponse{{Checkpoint: "opaque-done", Complete: true}},
		healthLeaseID: "replacement-lease",
	}
	client := newPackageClient(server, 0)
	manager := newTestManager(t, &hostProcess{
		packageID: "order", version: "4.0.0", generation: 5, client: client, leaseID: "lease-5",
	})

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
	fakePackage

	mu            sync.Mutex
	responses     []pluginhostsdk.MigrationResponse
	requests      []pluginhostsdk.MigrationRequest
	healthCalls   int
	events        []string
	healthLeaseID string
}

func (s *migrationTestHostServer) Migrate(_ context.Context, request pluginhostsdk.MigrationRequest) (pluginhostsdk.MigrationResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, request)
	if len(s.responses) == 0 {
		return pluginhostsdk.MigrationResponse{}, nil
	}
	response := s.responses[0]
	s.responses = s.responses[1:]
	return response, nil
}

func (s *migrationTestHostServer) Health(context.Context) (pluginhostsdk.HealthResponse, error) {
	s.mu.Lock()
	s.healthCalls++
	s.events = append(s.events, "health")
	s.mu.Unlock()
	return pluginhostsdk.HealthResponse{Healthy: true, LeaseID: s.healthLeaseID}, nil
}

func (s *migrationTestHostServer) recordEvent(event string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
}

func (s *migrationTestHostServer) eventSnapshot() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.events...)
}
