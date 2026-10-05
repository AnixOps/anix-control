package relay

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

// The in-process harness: carriers over loopback TCP (real buffering and
// half-close) or net.Pipe (none, which finds deadlocks), and a raw peer that
// speaks frames to a carrier under test.

const testWait = 5 * time.Second

// tcpPair returns the two ends of a loopback TCP connection.
func tcpPair(t testing.TB) (net.Conn, net.Conn) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	type res struct {
		c   net.Conn
		err error
	}
	ch := make(chan res, 1)
	go func() {
		c, err := ln.Accept()
		ch <- res{c, err}
	}()
	client, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	r := <-ch
	if r.err != nil {
		t.Fatal(r.err)
	}
	t.Cleanup(func() { _ = client.Close(); _ = r.c.Close() })
	return client, r.c
}

// connPair returns a connected pair of the named kind.
func connPair(t testing.TB, kind string) (net.Conn, net.Conn) {
	t.Helper()
	if kind == "pipe" {
		a, b := net.Pipe()
		t.Cleanup(func() { _ = a.Close(); _ = b.Close() })
		return a, b
	}
	return tcpPair(t)
}

// newPair returns a dialler and an acceptor carrier joined by conns of kind
// ("tcp" or "pipe"), closed when the test ends.
func newPair(t testing.TB, kind string, dcfg, acfg Config) (d, a *Carrier) {
	t.Helper()
	dc, ac := connPair(t, kind)
	type res struct {
		c   *Carrier
		err error
	}
	ch := make(chan res, 1)
	go func() {
		c, err := NewCarrier(ac, RoleAcceptor, acfg)
		ch <- res{c, err}
	}()
	d, err := NewCarrier(dc, RoleDialer, dcfg)
	if err != nil {
		t.Fatal(err)
	}
	r := <-ch
	if r.err != nil {
		t.Fatal(r.err)
	}
	a = r.c
	t.Cleanup(func() {
		_ = d.Close()
		_ = a.Close()
	})
	return d, a
}

func testParams() OpenParams {
	return OpenParams{Kind: StreamTCP, RouteID: "01HZZZZZZZZZZZZZZZZZZZZZZZ", HopIndex: 1}
}

func ctxTimeout(t testing.TB) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), testWait)
	t.Cleanup(cancel)
	return ctx
}

// serveEcho accepts streams on a and echoes every byte back, honouring
// half-close (it closes its write side when the peer's ends).
func serveEcho(t testing.TB, a *Carrier) {
	t.Helper()
	go func() {
		for {
			s, err := a.Accept(context.Background())
			if err != nil {
				return
			}
			go func() {
				if s.Accept() != nil {
					return
				}
				_, _ = io.Copy(s, s)
				_ = s.CloseWrite()
				_ = s.Close()
			}()
		}
	}()
}

// roundTrip writes data on a new stream of d, half-closes, and reads the
// echo to the end.
func roundTrip(t testing.TB, d *Carrier, data []byte) []byte {
	t.Helper()
	s, err := d.Open(testParams())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	go func() {
		_, _ = s.Write(data)
		_ = s.CloseWrite()
	}()
	_ = s.SetReadDeadline(time.Now().Add(testWait))
	got, err := io.ReadAll(s)
	if err != nil {
		t.Fatalf("round trip: %v (read %d of %d bytes)", err, len(got), len(data))
	}
	return got
}

func pattern(n int, seed byte) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i*7) ^ seed
	}
	return b
}

// rawPeer is a hand-driven end of a carrier: it has done the SETTINGS
// exchange and then sends and receives frames as the test says.
type rawPeer struct {
	t    testing.TB
	conn net.Conn
	// Settings is what the carrier under test announced.
	Settings Settings
	br       *bytesReader
}

// bytesReader is a buffered reader that survives read deadlines.
type bytesReader struct {
	conn net.Conn
	buf  []byte
}

func (b *bytesReader) Read(p []byte) (int, error) {
	if len(b.buf) > 0 {
		n := copy(p, b.buf)
		b.buf = b.buf[n:]
		return n, nil
	}
	return b.conn.Read(p)
}

// newRawPeer connects a raw peer to a carrier of the given role with the
// given config, returning both. The peer announces peerSettings.
func newRawPeer(t testing.TB, kind string, role Role, cfg Config, peerSettings Settings) (*Carrier, *rawPeer) {
	t.Helper()
	cc, pc := connPair(t, kind)
	type res struct {
		c   *Carrier
		err error
	}
	ch := make(chan res, 1)
	go func() {
		c, err := NewCarrier(cc, role, cfg)
		ch <- res{c, err}
	}()
	p := &rawPeer{t: t, conn: pc, br: &bytesReader{conn: pc}}
	werr := make(chan error, 1)
	go func() { werr <- WriteFrame(pc, Frame{Type: TypeSettings, Payload: peerSettings.Marshal()}) }()
	f, err := ReadFrame(p.br, MaxMaxFrame)
	if err != nil || f.Type != TypeSettings {
		t.Fatalf("raw peer: SETTINGS: %v %v", f.Type, err)
	}
	if p.Settings, err = ParseSettings(f.Payload); err != nil {
		t.Fatal(err)
	}
	if err := <-werr; err != nil {
		t.Fatal(err)
	}
	r := <-ch
	if r.err != nil {
		t.Fatal(r.err)
	}
	t.Cleanup(func() { _ = r.c.Close() })
	return r.c, p
}

func (p *rawPeer) send(f Frame) {
	p.t.Helper()
	_ = p.conn.SetWriteDeadline(time.Now().Add(testWait))
	if err := WriteFrame(p.conn, f); err != nil {
		p.t.Fatalf("raw peer send %v: %v", f.Type, err)
	}
}

func (p *rawPeer) sendRaw(b []byte) {
	p.t.Helper()
	_ = p.conn.SetWriteDeadline(time.Now().Add(testWait))
	if _, err := p.conn.Write(b); err != nil {
		p.t.Fatalf("raw peer send: %v", err)
	}
}

// recv returns the next frame, failing the test after a timeout.
func (p *rawPeer) recv() Frame {
	p.t.Helper()
	f, err := p.recvErr(testWait)
	if err != nil {
		p.t.Fatalf("raw peer recv: %v", err)
	}
	return f
}

func (p *rawPeer) recvErr(timeout time.Duration) (Frame, error) {
	_ = p.conn.SetReadDeadline(time.Now().Add(timeout))
	return ReadFrame(p.br, MaxMaxFrame)
}

// recvType returns the next frame of the given type, skipping others (such as
// WINDOW and PING).
func (p *rawPeer) recvType(typ FrameType) Frame {
	p.t.Helper()
	for {
		if f := p.recv(); f.Type == typ {
			return f
		}
	}
}

// open sends an OPEN for stream id.
func (p *rawPeer) open(id uint32) {
	p.t.Helper()
	payload, err := testParams().MarshalBinary()
	if err != nil {
		p.t.Fatal(err)
	}
	p.send(Frame{Type: TypeOpen, StreamID: id, Payload: payload})
}

// expectGoAway reads until the carrier's GOAWAY and returns its reason; the
// connection must then close.
func (p *rawPeer) expectGoAway() GoAwayReason {
	p.t.Helper()
	f := p.recvType(TypeGoAway)
	_, reason, err := parseGoAway(f.Payload)
	if err != nil {
		p.t.Fatal(err)
	}
	return reason
}

func waitDone(t testing.TB, c *Carrier) {
	t.Helper()
	select {
	case <-c.Done():
	case <-time.After(testWait):
		t.Fatalf("carrier did not end; err=%v", c.Err())
	}
}

func waitFor(t testing.TB, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(testWait)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func eqBytes(t testing.TB, got, want []byte) {
	t.Helper()
	if !bytes.Equal(got, want) {
		n := min(len(got), len(want))
		i := 0
		for i < n && got[i] == want[i] {
			i++
		}
		t.Fatalf("data differs: got %d bytes, want %d, first difference at %d", len(got), len(want), i)
	}
}

func isTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

var _ = binary.BigEndian
var _ sync.Mutex

func withSettings(f func(*Settings)) Settings {
	s := DefaultSettings()
	f(&s)
	return s
}
