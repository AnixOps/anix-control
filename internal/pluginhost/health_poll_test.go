//go:build unix

package pluginhost

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type healthPollHostClient struct {
	mu     sync.Mutex
	health HostHealth
	err    error
	calls  int
}

func (c *healthPollHostClient) set(health HostHealth, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.health, c.err = health, err
}

func (*healthPollHostClient) Dispatch(context.Context, DispatchInput) (DispatchOutput, error) {
	return DispatchOutput{}, nil
}

func (*healthPollHostClient) Migrate(context.Context, MigrationInput) (MigrationOutput, error) {
	return MigrationOutput{}, nil
}

func (c *healthPollHostClient) Health(context.Context, uint64) (HostHealth, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	return c.health, c.err
}

func (*healthPollHostClient) Drain(context.Context, uint64, time.Time) (DrainResult, error) {
	return DrainResult{Drained: true}, nil
}

func (*healthPollHostClient) OpenWebSocket(context.Context, WebSocketInput) (webSocketTransport, error) {
	return nil, ErrHostUnavailable
}

func (*healthPollHostClient) Close() error { return nil }

func TestPollHealthRecordsDetailsAndKeepsThemOnFailure(t *testing.T) {
	client := &healthPollHostClient{}
	details := `{"routes":{"knowledge.article.list":{"mode":"shadow"}}}`
	client.set(HostHealth{Healthy: true, LeaseID: "lease-3", DetailsJSON: details}, nil)
	host := &hostProcess{packageID: "knowledge", version: "4.0.1", generation: 3, client: client, leaseID: "lease-3"}
	manager := &Supervisor{hosts: map[string]*hostProcess{hostKey(host.packageID, host.version): host}}

	manager.pollHealthOnce(context.Background())
	stats := manager.Stats()
	require.Len(t, stats, 1)
	require.Equal(t, "knowledge", stats[0].PackageID)
	require.Equal(t, "4.0.1", stats[0].Version)
	require.EqualValues(t, 3, stats[0].Generation)
	require.Equal(t, details, stats[0].HealthDetailsJSON)
	firstChecked := stats[0].HealthCheckedAt
	require.False(t, firstChecked.IsZero())

	// A failed call and a lease mismatch both leave the last good details.
	client.set(HostHealth{}, errors.New("host busy"))
	manager.pollHealthOnce(context.Background())
	client.set(HostHealth{Healthy: true, LeaseID: "lease-other", DetailsJSON: `{}`}, nil)
	manager.pollHealthOnce(context.Background())
	stats = manager.Stats()
	require.Equal(t, details, stats[0].HealthDetailsJSON)
	require.Equal(t, firstChecked, stats[0].HealthCheckedAt)
}

func TestPollHealthSkipsExitedHostsAndStopsWithContext(t *testing.T) {
	client := &healthPollHostClient{}
	client.set(HostHealth{Healthy: true, LeaseID: "lease-1", DetailsJSON: `{}`}, nil)
	host := &hostProcess{
		packageID: "knowledge", version: "4.0.1", generation: 1, client: client, leaseID: "lease-1",
		supervision: hostSupervisionRestarting,
	}
	manager := &Supervisor{hosts: map[string]*hostProcess{hostKey(host.packageID, host.version): host}}
	manager.pollHealthOnce(context.Background())
	require.Empty(t, manager.Stats())
	client.mu.Lock()
	require.Zero(t, client.calls)
	client.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		manager.PollHealth(ctx, time.Millisecond)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("PollHealth did not stop when its context ended")
	}
	var nilManager *Supervisor
	nilManager.PollHealth(context.Background(), time.Second)
}
