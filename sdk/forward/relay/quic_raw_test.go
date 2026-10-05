package relay

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/quic-go/quic-go"

	"github.com/AnixOps/anix-control/sdk/forward/relay/link"
	"github.com/AnixOps/anix-control/sdk/forward/relay/relaytest"
)

// A raw QUIC peer is a verified link connection driven by hand: it makes (or
// takes) the QUIC connection, does the control stream exchange as the test
// says, and sends and reads what the mapping of quicwire.go puts on the wire,
// well-formed or not. It stands in for a peer that breaks a rule.
type qraw struct {
	t    testing.TB
	conn *link.QUICConn
	out  *quic.SendStream    // the raw peer's control stream
	in   *quic.ReceiveStream // the carrier's control stream, once accepted
	// Settings is what the carrier under test announced.
	Settings Settings
}

// rawDiallerConn dials the environment's listener as forward-1 and returns the
// bare connection: no control stream, nothing sent.
func rawDiallerConn(t testing.TB, e *quicEnv, qcfg QUICConfig) *link.QUICConn {
	t.Helper()
	tr, err := qcfg.Transport(RoleDialer)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), testWait)
	defer cancel()
	conn, err := link.DialQUIC(ctx, e.l.Addr().String(), link.QUICDialConfig{
		DialConfig: link.DialConfig{Credentials: linkCreds(t, e.ca, e.ca.CAPEM(), "forward-1"), ServerName: "forward-2", PeerIdentity: nodeID("forward-2"), Protocol: ALPN},
		QUIC:       tr,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// startControl opens the raw peer's control stream, sends settings on it and
// reads the carrier's. A nil settings sends the defaults.
func (p *qraw) startControl(settings *Settings) {
	p.t.Helper()
	s := DefaultSettings()
	if settings != nil {
		s = *settings
	}
	out, err := p.conn.OpenUniStream()
	if err != nil {
		p.t.Fatal(err)
	}
	p.out = out
	p.send(quicFrameBytes(TypeSettings, s.Marshal()))
	ctx, cancel := context.WithTimeout(context.Background(), testWait)
	defer cancel()
	in, err := p.conn.AcceptUniStream(ctx)
	if err != nil {
		p.t.Fatalf("raw peer: the carrier's control stream: %v", err)
	}
	p.in = in
	_ = in.SetReadDeadline(time.Now().Add(testWait))
	f, err := readQUICFrame(in, MaxMaxFrame)
	if err != nil || f.Type != TypeSettings {
		p.t.Fatalf("raw peer: SETTINGS: %v %v", f.Type, err)
	}
	if p.Settings, err = ParseSettings(f.Payload); err != nil {
		p.t.Fatal(err)
	}
}

// send writes bytes on the raw peer's control stream.
func (p *qraw) send(b []byte) {
	p.t.Helper()
	_ = p.out.SetWriteDeadline(time.Now().Add(testWait))
	if _, err := p.out.Write(b); err != nil && p.conn.Context().Err() == nil {
		p.t.Fatalf("raw peer: control write: %v", err)
	} // else the carrier has already ended the connection on what came before
}

// recvControl returns the next frame of the carrier's control stream.
func (p *qraw) recvControl() (Frame, error) {
	_ = p.in.SetReadDeadline(time.Now().Add(testWait))
	return readQUICFrame(p.in, MaxMaxFrame)
}

// expectGoAway reads the carrier's control stream until its GOAWAY and returns
// its reason.
func (p *qraw) expectGoAway() GoAwayReason {
	p.t.Helper()
	for {
		f, err := p.recvControl()
		if err != nil {
			p.t.Fatalf("raw peer: waiting for GOAWAY: %v", err)
		}
		if f.Type == TypeGoAway {
			_, reason, err := parseGoAway(f.Payload)
			if err != nil {
				p.t.Fatal(err)
			}
			return reason
		}
	}
}

// expectEnd waits for the connection to end and returns the application error
// code the carrier closed it with.
func (p *qraw) expectEnd() GoAwayReason {
	p.t.Helper()
	return expectConnEnd(p.t, p.conn.Conn)
}

// expectConnEnd waits for a QUIC connection to end by the peer's application
// close and returns its code.
func expectConnEnd(t testing.TB, c *quic.Conn) GoAwayReason {
	t.Helper()
	select {
	case <-c.Context().Done():
	case <-time.After(testWait):
		t.Fatal("the connection did not end")
	}
	var app *quic.ApplicationError
	if !errors.As(context.Cause(c.Context()), &app) || !app.Remote {
		t.Fatalf("the connection ended with %v, want the peer's application close", context.Cause(c.Context()))
	}
	return GoAwayReason(app.ErrorCode)
}

// openRaw opens a stream and sends bytes on it.
func (p *qraw) openRaw(b ...byte) *quic.Stream {
	p.t.Helper()
	st, err := p.conn.OpenStream()
	if err != nil {
		p.t.Fatal(err)
	}
	if len(b) > 0 {
		_ = st.SetWriteDeadline(time.Now().Add(testWait))
		if _, err := st.Write(b); err != nil && p.conn.Context().Err() == nil {
			var se *quic.StreamError
			if !errors.As(err, &se) { // the carrier may have reset the stream already
				p.t.Fatalf("raw peer: stream write: %v", err)
			}
		}
	}
	return st
}

// openStream opens a stream and sends a valid OPEN for params on it.
func (p *qraw) openStream(params OpenParams) *quic.Stream {
	p.t.Helper()
	payload, err := params.MarshalBinary()
	if err != nil {
		p.t.Fatal(err)
	}
	return p.openRaw(quicFrameBytes(TypeOpen, payload)...)
}

// expectReset reads from a stream until it fails and checks that the failure is
// the carrier's reset with the given reason.
func expectReset(t testing.TB, st *quic.Stream, want ResetReason) {
	t.Helper()
	_ = st.SetReadDeadline(time.Now().Add(testWait))
	_, err := io.Copy(io.Discard, st)
	var se *quic.StreamError
	if !errors.As(err, &se) || !se.Remote || ResetReason(se.ErrorCode) != want {
		t.Fatalf("stream ended with %v, want a remote reset (%s)", err, want)
	}
}

// readResultFrame reads the RESULT the carrier answers a stream with.
func readResultFrame(t testing.TB, st *quic.Stream) ResultCode {
	t.Helper()
	_ = st.SetReadDeadline(time.Now().Add(testWait))
	f, err := readQUICFrame(st, resultPayloadLen)
	if err != nil || f.Type != TypeResult {
		t.Fatalf("RESULT: %v %v", f.Type, err)
	}
	code, err := parseResult(f.Payload)
	if err != nil {
		t.Fatal(err)
	}
	return code
}

// rawDialler makes a carrier of the environment's listener and a raw dialling
// peer that did the control exchange, returning the listener's carrier.
func rawDialler(t *testing.T, e *quicEnv, qcfg QUICConfig) (*QUICCarrier, *qraw) {
	t.Helper()
	p := &qraw{t: t, conn: rawDiallerConn(t, e, qcfg)}
	p.startControl(nil)
	a := acceptQUIC(t, e.l)
	t.Cleanup(func() { _ = a.Close() })
	return a, p
}

// rawAcceptorEnv is a listener of the link layer (no carrier on it) for a
// dialler under test to reach: the raw acceptor.
type rawAcceptorEnv struct {
	t    *testing.T
	env  *quicEnv
	ll   *link.QUICListener
	qcfg QUICConfig
}

// newRawAcceptor starts a link-layer QUIC listener for forward-2.
func newRawAcceptor(t *testing.T, qcfg QUICConfig) *rawAcceptorEnv {
	t.Helper()
	ca, err := relaytest.New("a")
	if err != nil {
		t.Fatal(err)
	}
	e := &quicEnv{t: t, ca: ca}
	e.server = linkCreds(t, e.ca, e.ca.CAPEM(), "forward-2")
	tr, err := qcfg.Transport(RoleAcceptor)
	if err != nil {
		t.Fatal(err)
	}
	ll, err := link.ListenQUIC("127.0.0.1:0", link.QUICListenerConfig{
		ListenerConfig: link.ListenerConfig{Credentials: e.server, Protocol: ALPN, Sources: []string{"127.0.0.1"}, Peers: []string{nodeID("forward-1")}},
		QUIC:           tr,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ll.Close() })
	return &rawAcceptorEnv{t: t, env: e, ll: ll, qcfg: qcfg}
}

// dial runs DialQUIC (the dialler under test) against the raw acceptor in the
// background and returns the raw end once the QUIC connection is up; the
// returned channel yields the dial's result after the raw peer has done its
// part of the control exchange.
func (r *rawAcceptorEnv) dial(dcfg QUICConfig) (*qraw, <-chan dialResult) {
	r.t.Helper()
	res := make(chan dialResult, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), testWait)
		defer cancel()
		c, err := DialQUIC(ctx, r.ll.Addr().String(), link.DialConfig{Credentials: linkCreds(r.t, r.env.ca, r.env.ca.CAPEM(), "forward-1"), ServerName: "forward-2", PeerIdentity: nodeID("forward-2")}, dcfg)
		res <- dialResult{c, err}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), testWait)
	defer cancel()
	conn, err := r.ll.Accept(ctx)
	if err != nil {
		r.t.Fatal(err)
	}
	r.t.Cleanup(func() { _ = conn.Close() })
	return &qraw{t: r.t, conn: conn}, res
}

type dialResult struct {
	c   *QUICCarrier
	err error
}

// rawAcceptor dials the raw acceptor with a carrier under test and does the
// control exchange by hand, returning the dialling carrier.
func rawAcceptor(t *testing.T, dcfg QUICConfig) (*QUICCarrier, *qraw) {
	t.Helper()
	r := newRawAcceptor(t, dcfg)
	p, res := r.dial(dcfg)
	p.startControl(nil)
	dr := <-res
	if dr.err != nil {
		t.Fatal(dr.err)
	}
	t.Cleanup(func() { _ = dr.c.Close() })
	return dr.c, p
}

// acceptRaw returns the next stream the carrier opened on the raw peer's side
// and its OPEN parameters.
func (p *qraw) acceptStream() (*quic.Stream, OpenParams) {
	p.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), testWait)
	defer cancel()
	st, err := p.conn.AcceptStream(ctx)
	if err != nil {
		p.t.Fatalf("raw peer: AcceptStream: %v", err)
	}
	_ = st.SetReadDeadline(time.Now().Add(testWait))
	f, err := readQUICFrame(st, maxOpenPayload)
	if err != nil || f.Type != TypeOpen {
		p.t.Fatalf("raw peer: OPEN: %v %v", f.Type, err)
	}
	var params OpenParams
	if err := params.UnmarshalBinary(f.Payload); err != nil {
		p.t.Fatal(err)
	}
	return st, params
}

// answer writes a RESULT on a stream.
func answer(t testing.TB, st *quic.Stream, code ResultCode) {
	t.Helper()
	_ = st.SetWriteDeadline(time.Now().Add(testWait))
	if _, err := st.Write(quicFrameBytes(TypeResult, marshalResult(code))); err != nil {
		t.Fatal(err)
	}
}
