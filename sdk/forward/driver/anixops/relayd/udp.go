package relayd

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AnixOps/anix-control/sdk/forward/relay"
)

// DefaultUDPIdle is how long a UDP association lives after its last datagram
// (anixops-protocol.md section 4.8). The driver owns the table: the library
// leaves it to us.
const DefaultUDPIdle = 60 * time.Second

// maxDatagram is the largest UDP payload.
const maxDatagram = 65535

// dgramConn is the upstream side of a UDP association: a connected UDP socket
// to a target, or a UDP stream to the next hop.
type dgramConn interface {
	// Send passes one datagram on; a datagram a full stream drops is dropped
	// as a full socket buffer would drop it, not an error.
	Send(p []byte) error
	// Recv returns the next datagram; buf may be used for it.
	Recv(buf []byte) ([]byte, error)
	Close() error
}

type sockDgram struct{ c net.Conn }

func (s sockDgram) Send(p []byte) error { _, err := s.c.Write(p); return err }
func (s sockDgram) Recv(buf []byte) ([]byte, error) {
	n, err := s.c.Read(buf)
	return buf[:n], err
}
func (s sockDgram) Close() error { return s.c.Close() }

type streamDgram struct{ s relay.Stream }

func (s streamDgram) Send(p []byte) error {
	if err := s.s.WriteDatagram(p); err != nil && !errors.Is(err, relay.ErrDatagramDropped) {
		return err
	}
	return nil
}
func (s streamDgram) Recv([]byte) ([]byte, error) { return s.s.ReadDatagram() }
func (s streamDgram) Close() error                { return s.s.Close() }

// idler closes something once nothing touched it for a timeout.
type idler struct {
	last    atomic.Int64
	timeout time.Duration
	timer   *time.Timer
}

func newIdler(timeout time.Duration, fire func()) *idler {
	i := &idler{timeout: timeout}
	i.touch()
	i.timer = time.AfterFunc(timeout, func() {
		idle := time.Since(time.Unix(0, i.last.Load()))
		if idle >= i.timeout {
			fire()
			return
		}
		i.timer.Reset(i.timeout - idle)
	})
	return i
}

func (i *idler) touch() { i.last.Store(time.Now().UnixNano()) }
func (i *idler) stop()  { i.timer.Stop() }

// assoc is one client's UDP association on a RAW listener.
type assoc struct {
	h      *hop
	client netip.AddrPort
	out    dgramConn
	up     *upstream
	idle   *idler
	once   sync.Once
}

func (a *assoc) close() {
	a.once.Do(func() {
		a.idle.stop()
		_ = a.out.Close()
		a.h.assocs.remove(a.client, a)
		a.up.active.Add(-1)
		a.h.release()
	})
}

type assocTable struct {
	mu     sync.Mutex
	m      map[netip.AddrPort]*assoc
	closed bool
}

func newAssocTable() *assocTable { return &assocTable{m: map[netip.AddrPort]*assoc{}} }

func (t *assocTable) get(k netip.AddrPort) *assoc {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.m[k]
}

// add stores a; it reports false when the table is closed or the client
// already has one (a race between two datagrams).
func (t *assocTable) add(a *assoc) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed || t.m[a.client] != nil {
		return false
	}
	t.m[a.client] = a
	return true
}

func (t *assocTable) remove(k netip.AddrPort, a *assoc) {
	t.mu.Lock()
	if t.m[k] == a {
		delete(t.m, k)
	}
	t.mu.Unlock()
}

func (t *assocTable) closeAll() {
	t.mu.Lock()
	t.closed = true
	as := make([]*assoc, 0, len(t.m))
	for _, a := range t.m {
		as = append(as, a)
	}
	t.mu.Unlock()
	for _, a := range as {
		a.close()
	}
}

// serveRawUDP serves the clients of a RAW UDP listener: one association per
// client address, with the upstream picked when its first datagram arrives.
func (h *hop) serveRawUDP() {
	defer h.wg.Done()
	buf := make([]byte, maxDatagram)
	for {
		n, client, err := h.socks.udp.ReadFromUDPAddrPort(buf)
		if err != nil {
			h.acceptEnded(err)
			return
		}
		client = netip.AddrPortFrom(client.Addr().Unmap(), client.Port())
		if !h.sourceAllowed(client.Addr()) {
			continue
		}
		a := h.assocs.get(client)
		if a == nil {
			if a = h.newAssoc(client); a == nil {
				continue
			}
		}
		if h.quotaUsed() {
			a.close()
			continue
		}
		a.idle.touch()
		h.upLim.wait(h.ctx.Done(), n)
		if err := a.out.Send(buf[:n]); err != nil {
			a.close()
			continue
		}
		h.add(dirUp, uint64(n), 1) // #nosec G115 -- a datagram length
	}
}

// newAssoc admits a client's first datagram: it takes a slot, picks and
// connects an upstream, and starts the association's downstream reader. nil
// means the datagram is dropped (paused, quota, limit, nothing to connect to).
func (h *hop) newAssoc(client netip.AddrPort) *assoc {
	if _, ok := h.admit(); !ok {
		return nil
	}
	ctx, cancel := context.WithTimeout(h.ctx, resultWait)
	defer cancel()
	out, u, await, err := h.connectDgram(ctx, client)
	if err != nil {
		h.release()
		return nil
	}
	a := &assoc{h: h, client: client, out: out, up: u}
	a.idle = newIdler(h.r.udpIdle, a.close)
	if !h.assocs.add(a) {
		a.idle.stop()
		_ = out.Close()
		u.active.Add(-1)
		h.release()
		return h.assocs.get(client)
	}
	go func() {
		if await != nil {
			if err := await(h.ctx); err != nil {
				s := h.snap.Load()
				u.failure(h.r.now(), s.maxFails, s.openFor)
				a.close()
				return
			}
		}
		a.readLoop()
	}()
	return a
}

// readLoop carries the upstream's datagrams back to the client.
func (a *assoc) readLoop() {
	defer a.close()
	h := a.h
	buf := make([]byte, maxDatagram)
	for {
		d, err := a.out.Recv(buf)
		if err != nil {
			return
		}
		if len(d) == 0 {
			continue
		}
		if h.quotaUsed() {
			return
		}
		a.idle.touch()
		h.downLim.wait(h.ctx.Done(), len(d))
		if _, err := h.socks.udp.WriteToUDPAddrPort(d, a.client); err != nil {
			if h.ctx.Err() != nil {
				return
			}
			continue
		}
		h.add(dirDown, uint64(len(d)), 1) // #nosec G115 -- a datagram length
	}
}

// connectDgram is connect for UDP: it picks an upstream and opens the
// association's upstream side. For a stream to the next hop it does not wait
// for the answer (the first datagrams ride the stream behind the OPEN); await
// does, and is nil for a socket.
func (h *hop) connectDgram(ctx context.Context, client netip.AddrPort) (dgramConn, *upstream, func(context.Context) error, error) {
	s := h.snap.Load()
	rot := h.rot.Load()
	skip := map[*upstream]bool{}
	var last error
	for range rot.entries {
		u := rot.pick(s.def.Balance, client.Addr(), skip, h.r.now())
		if u == nil {
			break
		}
		skip[u] = true
		if u.pool == nil {
			c, err := h.r.dial(ctx, "udp", u.id)
			if err != nil {
				u.failure(h.r.now(), s.maxFails, s.openFor)
				h.logUpstreamFailure(u, err)
				last = err
				continue
			}
			u.success()
			u.active.Add(1)
			return sockDgram{c}, u, nil, nil
		}
		st, err := u.pool.open(ctx, relay.OpenParams{Kind: relay.StreamUDP, RouteID: h.key.Route, HopIndex: u.nextHop, Client: client})
		if err != nil {
			u.failure(h.r.now(), s.maxFails, s.openFor)
			h.logUpstreamFailure(u, err)
			last = err
			continue
		}
		u.active.Add(1)
		return streamDgram{st}, u, func(ctx context.Context) error {
			actx, cancel := context.WithTimeout(ctx, resultWait)
			defer cancel()
			if err := st.AwaitResult(actx); err != nil {
				return err
			}
			u.success()
			return nil
		}, nil
	}
	if last == nil {
		last = errors.New("no upstream in rotation")
	}
	return nil, nil, nil, last
}

// streamUDP serves a UDP stream of the previous hop: it connects the upstream
// side (and waits for the next hop's answer), answers the stream, and moves
// datagrams both ways until either side ends or the association is idle.
func (h *hop) streamUDP(ctx context.Context, s relay.Stream, p relay.OpenParams) {
	cctx, cancel := context.WithTimeout(ctx, resultWait)
	out, u, await, err := h.connectDgram(cctx, p.Client)
	cancel()
	if err != nil {
		_ = s.Reject(relay.ResultUpstreamUnreachable)
		return
	}
	defer u.active.Add(-1)
	if await != nil {
		if err := await(ctx); err != nil {
			snap := h.snap.Load()
			u.failure(h.r.now(), snap.maxFails, snap.openFor)
			_ = out.Close()
			_ = s.Reject(relay.ResultUpstreamUnreachable)
			return
		}
	}
	if err := s.Accept(); err != nil {
		_ = out.Close()
		return
	}
	var once sync.Once
	end := func() {
		once.Do(func() {
			_ = out.Close()
			_ = s.Close()
		})
	}
	idle := newIdler(h.r.udpIdle, end)
	defer idle.stop()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { // previous hop to upstream
		defer wg.Done()
		defer end()
		for {
			d, err := s.ReadDatagram()
			if err != nil {
				return
			}
			if h.quotaUsed() {
				return
			}
			idle.touch()
			h.upLim.wait(ctx.Done(), len(d))
			if err := out.Send(d); err != nil {
				return
			}
			h.add(dirUp, uint64(len(d)), 1) // #nosec G115 -- a datagram length
		}
	}()
	go func() { // upstream to previous hop
		defer wg.Done()
		defer end()
		buf := make([]byte, maxDatagram)
		for {
			d, err := out.Recv(buf)
			if err != nil {
				return
			}
			if len(d) == 0 {
				continue
			}
			if h.quotaUsed() {
				return
			}
			idle.touch()
			h.downLim.wait(ctx.Done(), len(d))
			if err := s.WriteDatagram(d); err != nil && !errors.Is(err, relay.ErrDatagramDropped) {
				return
			}
			h.add(dirDown, uint64(len(d)), 1) // #nosec G115 -- a datagram length
		}
	}()
	go func() {
		<-ctx.Done()
		end()
	}()
	wg.Wait()
}
