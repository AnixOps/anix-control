package service

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/require"
)

// A diagnosis starts no probe after its context ended, and waits for those
// it started.
func TestDiagnosisProbesStopStartingWhenTheContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var mu sync.Mutex
	var ran []int
	err := DiagnosisProbes{Concurrency: 1}.each(ctx, 10, func(i int) {
		mu.Lock()
		ran = append(ran, i)
		mu.Unlock()
		if i == 2 {
			cancel()
		}
	})
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, []int{0, 1, 2}, ran)

	require.NoError(t, DiagnosisProbes{}.each(context.Background(), 0, func(int) { t.Fatal("no probe to run") }))
}

// A diagnosis runs at most Concurrency probes at once, and every probe when
// its context does not end.
func TestDiagnosisProbesBoundTheirConcurrency(t *testing.T) {
	for _, limit := range []int{1, 4, 0} {
		var mu sync.Mutex
		inFlight, most, total := 0, 0, 0
		require.NoError(t, DiagnosisProbes{Concurrency: limit}.each(context.Background(), 40, func(int) {
			mu.Lock()
			inFlight++
			total++
			most = max(most, inFlight)
			mu.Unlock()
			time.Sleep(2 * time.Millisecond)
			mu.Lock()
			inFlight--
			mu.Unlock()
		}))
		want := limit
		if want == 0 {
			want = DefaultDiagnosisConcurrency
		}
		require.Equal(t, 40, total)
		require.LessOrEqual(t, most, want, "limit %d", limit)
	}
}

// A forward or tunnel diagnosis whose context ends answers the probes that
// completed and the context's error: a probe interrupted by the end is not
// reported as a failed dial.
func (s *PanelForwardServiceTestSuite) TestDiagnosisStopsWhenItsContextEnds() {
	db := database.Get()
	entry := s.createForwardNode("Stop Entry", "198.51.100.61", model.ForwardNodeStatusOnline)
	exit := s.createForwardNodeOfType("Stop Exit", "198.51.100.62", model.ForwardNodeStatusOnline, model.ForwardNodeTypeExit)
	tunnel := &model.ForwardTunnel{Name: "Stop Tunnel", InNodeID: entry.ID, OutNodeID: &exit.ID, InIP: entry.Host, Type: 2, Protocol: "tcp", Status: model.ForwardTunnelStatusActive}
	s.Require().NoError(db.Create(tunnel).Error)
	forward := &model.Forward{UserID: 1, Name: "Stop Forward", TunnelID: tunnel.ID, InPort: 31000, RemoteAddr: "203.0.113.61:80,203.0.113.62:80", Status: model.ForwardStatusActive}
	s.Require().NoError(db.Create(forward).Error)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var mu sync.Mutex
	dialled := map[string]bool{}
	probes := DiagnosisProbes{Dial: func(ctx context.Context, network, address string, timeout time.Duration) (net.Conn, error) {
		mu.Lock()
		dialled[address] = true
		if len(dialled) == 2 {
			// Both probes started: the refused one completes, and the
			// diagnosis ends while the other hangs.
			time.AfterFunc(20*time.Millisecond, cancel)
		}
		mu.Unlock()
		switch address {
		case "203.0.113.61:80", "198.51.100.61:20001":
			<-ctx.Done()
			return nil, &net.OpError{Op: "dial", Net: network, Err: ctx.Err()}
		}
		return nil, fmt.Errorf("dial tcp %s: connect: connection refused", address)
	}}

	report, err := s.svc.DiagnoseForwardTargets(ctx, probes, forward.ID, false)
	s.Require().ErrorIs(err, context.Canceled)
	s.Require().Len(report.Results, 1)
	s.Equal("203.0.113.62", report.Results[0].TargetIP)
	s.Equal("dial tcp 203.0.113.62:80: connect: connection refused", report.Results[0].Message)

	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	dialled = map[string]bool{}
	tunnelReport, err := s.svc.DiagnoseTunnelContext(ctx, probes, tunnel.ID)
	s.Require().ErrorIs(err, context.Canceled)
	s.Require().Len(tunnelReport.Results, 1)
	s.Equal("管理端->出口节点", tunnelReport.Results[0].Description)
}
