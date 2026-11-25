package regionrelay

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/proxyproto"
)

// Forward is one running TCP passthrough.
type Forward struct {
	config   ForwardConfig
	listener net.Listener
	logger   *slog.Logger
	dialer   func(ctx context.Context, network, address string) (net.Conn, error)

	// preferred is the index of the upstream that answered last.
	preferred atomic.Int32
	slots     chan struct{}

	mu     sync.Mutex
	conns  map[net.Conn]struct{}
	closed bool
	wg     sync.WaitGroup
}

// ListenForward binds the forward's listen address.
func ListenForward(config ForwardConfig, logger *slog.Logger) (*Forward, error) {
	listener, err := net.Listen("tcp", config.Listen)
	if err != nil {
		return nil, fmt.Errorf("forward %q: %w", config.Name, err)
	}
	return newForward(config, listener, logger), nil
}

func newForward(config ForwardConfig, listener net.Listener, logger *slog.Logger) *Forward {
	if logger == nil {
		logger = slog.Default()
	}
	dialer := &net.Dialer{Timeout: config.DialTimeout, KeepAlive: config.KeepAlive}
	return &Forward{
		config:   config,
		listener: listener,
		logger:   logger.With("forward", config.Name),
		dialer:   dialer.DialContext,
		slots:    make(chan struct{}, config.MaxConnections),
		conns:    map[net.Conn]struct{}{},
	}
}

// Addr is the bound listen address.
func (f *Forward) Addr() net.Addr { return f.listener.Addr() }

// Serve accepts node connections until Close.
func (f *Forward) Serve() error {
	f.logger.Info("relay forward listening", "listen", f.listener.Addr().String(), "upstreams", f.config.Upstreams, "proxy_protocol", f.config.ProxyProtocol)
	var backoff time.Duration
	for {
		conn, err := f.listener.Accept()
		if err != nil {
			if f.isClosed() {
				return nil
			}
			if errors.Is(err, net.ErrClosed) {
				return err
			}
			// Out of file descriptors and the like: back off and retry.
			backoff = min(max(backoff*2, 5*time.Millisecond), time.Second)
			f.logger.Warn("accept failed; retrying", "error", err, "backoff", backoff)
			time.Sleep(backoff)
			continue
		}
		backoff = 0
		select {
		case f.slots <- struct{}{}:
		default:
			f.logger.Warn("refused a node connection: max_connections reached", "remote", conn.RemoteAddr().String(), "max_connections", f.config.MaxConnections)
			_ = conn.Close()
			continue
		}
		if !f.track(conn) {
			<-f.slots
			_ = conn.Close()
			return nil
		}
		f.wg.Add(1)
		go func() {
			defer f.wg.Done()
			defer func() { <-f.slots }()
			defer f.untrack(conn)
			f.relay(conn)
		}()
	}
}

// Close stops accepting, closes every open connection and waits for them.
func (f *Forward) Close() error {
	f.mu.Lock()
	f.closed = true
	conns := make([]net.Conn, 0, len(f.conns))
	for conn := range f.conns {
		conns = append(conns, conn)
	}
	f.mu.Unlock()
	err := f.listener.Close()
	for _, conn := range conns {
		_ = conn.Close()
	}
	f.wg.Wait()
	return err
}

func (f *Forward) isClosed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

func (f *Forward) track(conn net.Conn) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return false
	}
	f.conns[conn] = struct{}{}
	return true
}

func (f *Forward) untrack(conn net.Conn) {
	f.mu.Lock()
	delete(f.conns, conn)
	f.mu.Unlock()
	_ = conn.Close()
}

// relay copies one node connection to an upstream and back.
func (f *Forward) relay(node net.Conn) {
	setKeepAlive(node, f.config.KeepAlive)
	upstream, address, err := f.dialUpstream()
	if err != nil {
		f.logger.Warn("no upstream answered; closing the node connection", "remote", node.RemoteAddr().String(), "error", err)
		return
	}
	if !f.track(upstream) {
		_ = upstream.Close()
		return
	}
	defer f.untrack(upstream)
	if f.config.ProxyProtocol {
		header, err := proxyHeader(node)
		if err != nil {
			f.logger.Warn("could not describe the node connection", "remote", node.RemoteAddr().String(), "error", err)
			return
		}
		if err := writeFull(upstream, header, f.config.DialTimeout); err != nil {
			f.logger.Warn("upstream refused the PROXY header", "upstream", address, "error", err)
			return
		}
	}
	pipe(node, upstream)
}

// dialUpstream dials the preferred upstream first, then the others in
// order, and remembers the one that answered.
func (f *Forward) dialUpstream() (net.Conn, string, error) {
	upstreams := f.config.Upstreams
	start := int(f.preferred.Load())
	var errs []error
	for i := range upstreams {
		index := (start + i) % len(upstreams)
		ctx, cancel := context.WithTimeout(context.Background(), f.config.DialTimeout)
		conn, err := f.dialer(ctx, "tcp", upstreams[index])
		cancel()
		if err == nil {
			if index != start {
				f.logger.Info("switched upstream", "from", upstreams[start], "to", upstreams[index])
				f.preferred.Store(int32(index)) // #nosec G115 -- index is below len(upstreams), a short configured list.
			}
			return conn, upstreams[index], nil
		}
		errs = append(errs, fmt.Errorf("%s: %w", upstreams[index], err))
	}
	return nil, "", errors.Join(errs...)
}

// proxyHeader is the PROXY v2 header of the node connection: its source is
// the node, its destination the address the node dialed on the relay.
func proxyHeader(node net.Conn) ([]byte, error) {
	source, ok := node.RemoteAddr().(*net.TCPAddr)
	if !ok {
		return nil, fmt.Errorf("remote address %v is not TCP", node.RemoteAddr())
	}
	destination, ok := node.LocalAddr().(*net.TCPAddr)
	if !ok {
		return nil, fmt.Errorf("local address %v is not TCP", node.LocalAddr())
	}
	// A dual-stack listener reports an IPv4 node next to an IPv4-mapped
	// local address; both then encode as IPv4.
	if source.IP.To4() != nil && destination.IP.To4() == nil {
		destination = &net.TCPAddr{IP: net.IPv4zero, Port: destination.Port}
	}
	if source.IP.To4() == nil && destination.IP.To4() != nil {
		destination = &net.TCPAddr{IP: net.IPv6unspecified, Port: destination.Port}
	}
	return proxyproto.Encode(source, destination)
}

func writeFull(conn net.Conn, data []byte, timeout time.Duration) error {
	_ = conn.SetWriteDeadline(time.Now().Add(timeout))
	defer func() { _ = conn.SetWriteDeadline(time.Time{}) }()
	_, err := conn.Write(data)
	return err
}

func setKeepAlive(conn net.Conn, period time.Duration) {
	if tcp, ok := conn.(*net.TCPConn); ok {
		_ = tcp.SetKeepAlive(true)
		_ = tcp.SetKeepAlivePeriod(period)
	}
}

// pipe copies both ways. When one side ends its stream, the other side's
// write half is closed so the end reaches it; when both are done, or either
// fails, both connections close.
func pipe(a, b net.Conn) {
	done := make(chan struct{}, 2)
	copyHalf := func(dst, src net.Conn) {
		_, err := io.Copy(dst, src)
		if err == nil {
			if tcp, ok := dst.(*net.TCPConn); ok {
				_ = tcp.CloseWrite()
			} else {
				_ = dst.Close()
			}
		} else {
			_ = dst.Close()
			_ = src.Close()
		}
		done <- struct{}{}
	}
	go copyHalf(b, a)
	go copyHalf(a, b)
	<-done
	<-done
	_ = a.Close()
	_ = b.Close()
}
