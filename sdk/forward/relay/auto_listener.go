package relay

import (
	"context"
	"errors"
	"net"
	"strconv"
	"sync"

	"github.com/AnixOps/anix-control/sdk/forward/relay/link"
)

// AutoListener is the listener of an AUTO link (anixops-protocol.md section
// 5.4): TLS over TCP and QUIC on the same port number, TCP for TLS_TCP and UDP
// for QUIC. Its carriers come from both; every rule of the two listeners holds
// for each (sources, peers, handshake limits, carriers that never outlive their
// listener), and a change of sources or peers applies to both. There is no
// plaintext here: PLAIN is never part of AUTO.
type AutoListener struct {
	tcp  *Listener
	quic *QUICListener

	ready  chan Carrier
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	once   sync.Once
	err    error
}

var _ CarrierListener = (*AutoListener)(nil)

// ListenAuto listens on address for TLS carriers (TCP) and QUIC carriers (UDP)
// at the same port. A port of 0 picks a free one for both, which tests use.
// lcfg configures both listeners (the handshake limit applies to each), and
// qcfg both kinds of carriers: its Config limits the TLS ones too.
func ListenAuto(address string, lcfg link.ListenerConfig, qcfg QUICConfig) (*AutoListener, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	attempts := 1
	if port == "0" {
		attempts = 16 // a free TCP port may be taken for UDP
	}
	var lastErr error
	for range attempts {
		tcp, err := Listen(address, lcfg, qcfg.Config)
		if err != nil {
			return nil, err
		}
		qaddr := net.JoinHostPort(host, strconv.Itoa(tcp.Addr().(*net.TCPAddr).Port))
		q, err := ListenQUIC(qaddr, lcfg, qcfg)
		if err != nil {
			_ = tcp.Close()
			lastErr = err
			continue
		}
		return newAutoListener(tcp, q), nil
	}
	return nil, lastErr
}

func newAutoListener(tcp *Listener, q *QUICListener) *AutoListener {
	l := &AutoListener{tcp: tcp, quic: q, ready: make(chan Carrier)}
	l.ctx, l.cancel = context.WithCancel(context.Background())
	for _, accept := range []func(context.Context) (Carrier, error){tcp.Accept, q.Accept} {
		l.wg.Add(1)
		go func() {
			defer l.wg.Done()
			for {
				c, err := accept(l.ctx)
				if err != nil {
					return
				}
				select {
				case l.ready <- c:
				case <-l.ctx.Done():
					_ = c.Close()
					return
				}
			}
		}()
	}
	return l
}

// TCP and QUIC return the two listeners, for their statistics.
func (l *AutoListener) TCP() *Listener      { return l.tcp }
func (l *AutoListener) QUIC() *QUICListener { return l.quic }

// Addr returns the TCP listening address; the UDP address has the same port
// (QUIC().Addr()).
func (l *AutoListener) Addr() net.Addr { return l.tcp.Addr() }

// Accept returns the next carrier of either kind. It returns net.ErrClosed
// after Close.
func (l *AutoListener) Accept(ctx context.Context) (Carrier, error) {
	select {
	case c := <-l.ready:
		return c, nil
	case <-l.ctx.Done():
		return nil, net.ErrClosed
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// SetSources replaces the admitted source addresses of both listeners.
func (l *AutoListener) SetSources(items []string) error {
	return errors.Join(l.tcp.SetSources(items), l.quic.SetSources(items))
}

// SetPeers replaces the identities that may dial on both listeners and closes
// the carriers of the identities removed, whichever way they came.
func (l *AutoListener) SetPeers(ids []string) (removed []string, err error) {
	removed, err = l.tcp.SetPeers(ids)
	if err != nil {
		return nil, err
	}
	if _, err := l.quic.SetPeers(ids); err != nil {
		return removed, err
	}
	return removed, nil
}

// Stats returns the two listeners' counters added up.
func (l *AutoListener) Stats() ListenerStats {
	a, b := l.tcp.Stats(), l.quic.Stats()
	s := ListenerStats{
		Link:           a.Link,
		Carriers:       a.Carriers + b.Carriers,
		Accepted:       a.Accepted + b.Accepted,
		SettingsFailed: a.SettingsFailed + b.SettingsFailed,
		Overloaded:     a.Overloaded + b.Overloaded,
	}
	s.Link.Accepted += b.Link.Accepted
	s.Link.Pending += b.Link.Pending
	s.Link.Retries += b.Link.Retries
	for i := range s.Link.Failures {
		s.Link.Failures[i] += b.Link.Failures[i]
	}
	return s
}

// Close stops both listeners and everything they accepted (L1), concurrently,
// and returns when both have ended.
func (l *AutoListener) Close() error {
	l.once.Do(func() {
		l.cancel()
		var wg sync.WaitGroup
		errs := make([]error, 2)
		for i, f := range []func() error{l.tcp.Close, l.quic.Close} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				errs[i] = f()
			}()
		}
		wg.Wait()
		l.wg.Wait()
		l.err = errors.Join(errs...)
	})
	return l.err
}
