package link

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/quic-go/quic-go"
)

// The QUIC counterparts of the helpers in harness_test.go.

// quicTestConfig is the transport configuration the tests use: datagrams on,
// and a keepalive and idle timeout short enough that nothing waits for the
// defaults, with test deadlines far longer.
func quicTestConfig() *quic.Config {
	return &quic.Config{EnableDatagrams: true, KeepAlivePeriod: 200 * time.Millisecond, MaxIdleTimeout: 5 * time.Second, MaxIncomingStreams: 16, MaxIncomingUniStreams: 16}
}

// listenQUIC starts an encrypted QUIC listener on loopback admitting 127.0.0.1.
func listenQUIC(t testing.TB, creds *Credentials, peers []string, mod ...func(*QUICListenerConfig)) *QUICListener {
	t.Helper()
	cfg := QUICListenerConfig{
		ListenerConfig: ListenerConfig{Credentials: creds, Protocol: testProto, Sources: []string{"127.0.0.1"}, Peers: peers},
		QUIC:           quicTestConfig(),
	}
	for _, m := range mod {
		m(&cfg)
	}
	l, err := ListenQUIC("127.0.0.1:0", cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	return l
}

func dialQUIC(t testing.TB, l *QUICListener, creds *Credentials, name, peer string) (*QUICConn, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), testWait)
	defer cancel()
	return DialQUIC(ctx, l.Addr().String(), QUICDialConfig{
		DialConfig: DialConfig{Credentials: creds, ServerName: name, PeerIdentity: peer, Protocol: testProto, HandshakeTimeout: testWait},
		QUIC:       quicTestConfig(),
	})
}

// acceptQUIC returns the next connection the listener hands out, or fails.
func acceptQUIC(t testing.TB, l *QUICListener) *QUICConn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), testWait)
	defer cancel()
	c, err := l.Accept(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// expectNoAcceptQUIC fails if the listener hands out a connection within d.
func expectNoAcceptQUIC(t testing.TB, l *QUICListener, d time.Duration) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	if c, err := l.Accept(ctx); err == nil {
		_ = c.Close()
		t.Fatal("Accept returned a connection that should have been refused")
	}
}

// rawQUICTLS is the TLS configuration of a hand-made QUIC client: the link CA
// as its roots, forward-2 as the name, and the given client certificate (none
// when nil).
func rawQUICTLS(t testing.TB, f *fixture, cert *tls.Certificate) *tls.Config {
	t.Helper()
	roots := x509.NewCertPool()
	roots.AddCert(mustParse(t, f.ca.CAPEM()))
	c := &tls.Config{RootCAs: roots, ServerName: "forward-2", MinVersion: tls.VersionTLS13, NextProtos: []string{testProto}}
	if cert != nil {
		c.Certificates = []tls.Certificate{*cert}
	}
	return c
}

// rawQUICDial dials with a hand-made client and returns what quic-go says. On
// success the connection is closed with the test.
func rawQUICDial(t testing.TB, addr string, tlsConf *tls.Config, qcfg *quic.Config) (*quic.Conn, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), testWait)
	defer cancel()
	if qcfg == nil {
		qcfg = quicTestConfig()
	}
	c, err := quic.DialAddr(ctx, addr, tlsConf, qcfg)
	if err == nil {
		t.Cleanup(func() { _ = c.CloseWithError(0, "") })
	}
	return c, err
}

// expectRefused checks that a connection the listener should have refused is
// gone: the client finishes its handshake first (as with TLS 1.3), so the
// refusal arrives right behind, as the peer's close.
func expectRefused(t testing.TB, c *quic.Conn) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), testWait)
	defer cancel()
	// The test listener opens no stream, so this returns only when the
	// connection ends.
	if _, err := c.AcceptUniStream(ctx); err == nil || errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("the connection was not closed by the listener: %v", err)
	}
}

// stalledClient starts a QUIC handshake that stops after the listener's first
// flight: the client certificate callback blocks until the returned release
// function is called, so the listener's handshake stays in flight and holds a
// slot. release is also run at the end of the test.
func stalledClient(t testing.TB, f *fixture, addr string, creds *Credentials) (release func()) {
	t.Helper()
	gate := make(chan struct{})
	cert := creds.state.Load().cert
	conf := rawQUICTLS(t, f, nil)
	conf.GetClientCertificate = func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
		<-gate
		return &cert, nil
	}
	var once sync.Once
	release = func() { once.Do(func() { close(gate) }) }
	quit := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		ctx, cancel := context.WithTimeout(context.Background(), 3*testWait)
		defer cancel()
		// A connection that completes stays open until the test ends, so the
		// listener can hand it out.
		if c, err := quic.DialAddr(ctx, addr, conf, quicTestConfig()); err == nil {
			<-quit
			_ = c.CloseWithError(0, "")
		}
	}()
	t.Cleanup(func() { release(); close(quit); <-done })
	return release
}

// quicFailures is a listener's failure count for a reason.
func quicFailures(l *QUICListener, r Reason) uint64 { return l.Stats().Failures[r] }

// udpAddrOf returns the loopback UDP address of a QUIC listener.
func udpAddrOf(l *QUICListener) *net.UDPAddr { return l.Addr().(*net.UDPAddr) }
