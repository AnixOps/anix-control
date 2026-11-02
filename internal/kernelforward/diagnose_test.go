package kernelforward

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// steps answers the steps of a kind, optionally of one node.
func steps(answer *forwardv1.DiagnoseRouteResponse, kind forwardv1.ProbeKind, node string) []*forwardv1.DiagnoseStep {
	var out []*forwardv1.DiagnoseStep
	for _, step := range answer.GetSteps() {
		if step.GetKind() == kind && (node == "*" || step.GetNodeRef() == node) {
			out = append(out, step)
		}
	}
	return out
}

func (f *fixture) diagnose(routeID string, timeout time.Duration) *forwardv1.DiagnoseRouteResponse {
	f.t.Helper()
	answer, err := f.service.DiagnoseRoute(f.ctx, routeID, timeout)
	require.NoError(f.t, err)
	return answer
}

// Too many diagnoses at once are refused; a caller of a running diagnosis
// shares it.
func TestDiagnoseBusyAndShared(t *testing.T) {
	resetDiagnosesForTest()
	f := newFixture(t, openSQLite(t))
	id := f.create("c1", twoHop(31000)).GetId()
	diagnoses.running = MaxConcurrentDiagnoses
	_, err := f.service.DiagnoseRoute(f.ctx, id, 0)
	require.ErrorIs(t, err, ErrDiagnoseBusy)
	diagnoses.running = 0

	release := make(chan struct{})
	started := make(chan struct{}, 8)
	f.service.Probes.Dial = func(context.Context, string, string, time.Duration) (net.Conn, error) {
		started <- struct{}{}
		<-release
		return nil, errors.New("connection refused (test)")
	}
	first := make(chan *forwardv1.DiagnoseRouteResponse, 1)
	go func() {
		answer, _ := f.service.DiagnoseRoute(context.Background(), id, 0)
		first <- answer
	}()
	<-started
	second := make(chan *forwardv1.DiagnoseRouteResponse, 1)
	go func() {
		answer, _ := f.service.DiagnoseRoute(context.Background(), id, 0)
		second <- answer
	}()
	time.Sleep(50 * time.Millisecond)
	close(release)
	a, b := <-first, <-second
	require.NotNil(t, a)
	require.NotNil(t, b)
	assert.False(t, a.GetCached())
	assert.True(t, b.GetCached(), "the second caller shares the running diagnosis")
	assert.Equal(t, a.GetStartedAtUnixMs(), b.GetStartedAtUnixMs())
}
