package relay

import (
	"context"
	"fmt"
	"net/netip"
	"os"
	"strconv"
	"testing"

	"github.com/AnixOps/anix-control/sdk/forward/relay/link"
	"github.com/AnixOps/anix-control/sdk/forward/relay/relaytest"
)

// quicEnv is a QUIC carrier listener for forward-2 on loopback that admits
// forward-1 (and the given extra peers) from 127.0.0.1.
type quicEnv struct {
	t      *testing.T
	ca     *relaytest.PKI
	server *link.Credentials
	l      *QUICListener
}

func newQUICEnv(t *testing.T, qcfg QUICConfig, peers ...string) *quicEnv {
	t.Helper()
	ca, err := relaytest.New("a")
	if err != nil {
		t.Fatal(err)
	}
	e := &quicEnv{t: t, ca: ca, server: linkCreds(t, ca, ca.CAPEM(), "forward-2")}
	e.l, err = ListenQUIC("127.0.0.1:0", link.ListenerConfig{
		Credentials: e.server,
		Sources:     []string{"127.0.0.1"},
		Peers:       append([]string{nodeID("forward-1")}, peers...),
	}, qcfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.l.Close() })
	return e
}

func (e *quicEnv) dial(creds *link.Credentials, qcfg QUICConfig) (*QUICCarrier, error) {
	ctx, cancel := context.WithTimeout(context.Background(), testWait)
	defer cancel()
	return DialQUIC(ctx, e.l.Addr().String(), link.DialConfig{Credentials: creds, ServerName: "forward-2", PeerIdentity: nodeID("forward-2")}, qcfg)
}

// pair dials as forward-1 and returns both ends of the carrier.
func (e *quicEnv) pair(qcfg QUICConfig) (d, a *QUICCarrier) {
	e.t.Helper()
	d, err := e.dial(linkCreds(e.t, e.ca, e.ca.CAPEM(), "forward-1"), qcfg)
	if err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(func() { _ = d.Close() })
	a = acceptQUIC(e.t, e.l)
	return d, a
}

// acceptQUIC takes the next carrier of a QUIC listener.
func acceptQUIC(t testing.TB, l *QUICListener) *QUICCarrier {
	t.Helper()
	c, err := l.Accept(ctxTimeout(t))
	if err != nil {
		t.Fatal(err)
	}
	return c.(*QUICCarrier)
}

// errDeadlineExceeded is what a deadline that passed returns.
var errDeadlineExceeded = os.ErrDeadlineExceeded

func netipAP(s string) netip.AddrPort { return netip.MustParseAddrPort(s) }

func newTestPKI(t testing.TB) *relaytest.PKI {
	t.Helper()
	ca, err := relaytest.New("a")
	if err != nil {
		t.Fatal(err)
	}
	return ca
}

func itoa(n int) string { return strconv.Itoa(n) }

// dump describes the carrier's streams, for a failing test.
func (c *QUICCarrier) dump() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := fmt.Sprintf("carrier %v active=%d streams=%d err=%v goAway(sent=%v recv=%v)\n", c.role, c.active, len(c.streams), c.err, c.goAwaySent, c.goAwayRecv)
	for id, s := range c.streams {
		out += fmt.Sprintf("  stream %d %v params=%s answered=%v awaiting=%v settled=%v err=%v closedByApp=%v readDone=%v writeDone=%v remoteFin=%v removed=%v out=%d/%v recv=%d\n",
			id, s.kind, s.params.RouteID, s.answered, s.awaiting, s.settled, s.err, s.closedByApp, s.readDone, s.writeDone, s.remoteFin, s.removed, s.outLen, s.finQueued, s.recvLen)
	}
	return out
}
