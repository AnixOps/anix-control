package agente2e

import (
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// tcpProxy sits between the Agent and Control's gRPC listener: the Agent's
// GRPCHost is the proxy. Drop is a network partition (the listener closes
// and every connection is cut, so the Agent cannot reconnect); Restore heals
// it on the same address. It needs no privileges.
type tcpProxy struct {
	addr   string
	target string

	mu       sync.Mutex
	listener net.Listener
	conns    map[net.Conn]struct{}
	accepted int
}

func newTCPProxy(t *testing.T, target string) *tcpProxy {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	p := &tcpProxy{addr: listener.Addr().String(), target: target, conns: map[net.Conn]struct{}{}}
	p.serve(listener)
	t.Cleanup(p.Drop)
	return p
}

func (p *tcpProxy) Addr() string { return p.addr }

// Accepted counts the connections the proxy has taken (each a dial of the
// Agent).
func (p *tcpProxy) Accepted() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.accepted
}

func (p *tcpProxy) serve(listener net.Listener) {
	p.mu.Lock()
	p.listener = listener
	p.mu.Unlock()
	go func() {
		for {
			client, err := listener.Accept()
			if err != nil {
				return
			}
			p.mu.Lock()
			if p.listener != listener {
				p.mu.Unlock()
				_ = client.Close()
				return
			}
			p.accepted++
			p.conns[client] = struct{}{}
			p.mu.Unlock()
			go p.pipe(client)
		}
	}()
}

func (p *tcpProxy) pipe(client net.Conn) {
	upstream, err := net.DialTimeout("tcp", p.target, 5*time.Second)
	if err != nil {
		p.forget(client)
		_ = client.Close()
		return
	}
	p.mu.Lock()
	if p.listener == nil {
		p.mu.Unlock()
		_ = client.Close()
		_ = upstream.Close()
		return
	}
	p.conns[upstream] = struct{}{}
	p.mu.Unlock()
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(upstream, client); done <- struct{}{} }()
	go func() { _, _ = io.Copy(client, upstream); done <- struct{}{} }()
	<-done
	_ = client.Close()
	_ = upstream.Close()
	p.forget(client)
	p.forget(upstream)
}

func (p *tcpProxy) forget(conn net.Conn) {
	p.mu.Lock()
	delete(p.conns, conn)
	p.mu.Unlock()
}

// Drop partitions the Agent from Control.
func (p *tcpProxy) Drop() {
	p.mu.Lock()
	listener := p.listener
	p.listener = nil
	conns := p.conns
	p.conns = map[net.Conn]struct{}{}
	p.mu.Unlock()
	if listener != nil {
		_ = listener.Close()
	}
	for conn := range conns {
		_ = conn.Close()
	}
}

// Restore heals the partition on the same address.
func (p *tcpProxy) Restore(t *testing.T) {
	t.Helper()
	var listener net.Listener
	var err error
	for i := 0; i < 50; i++ {
		listener, err = net.Listen("tcp", p.addr)
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.NoError(t, err, "re-listen on %s", p.addr)
	p.serve(listener)
}

// freePort answers a TCP port that was free a moment ago.
func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = listener.Close() }()
	return listener.Addr().(*net.TCPAddr).Port
}
