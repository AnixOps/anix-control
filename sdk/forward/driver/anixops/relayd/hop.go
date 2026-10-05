package relayd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AnixOps/anix-control/sdk/forward/driver/anixops/relayctl"
	"github.com/AnixOps/anix-control/sdk/forward/relay"
	"github.com/AnixOps/anix-control/sdk/forward/relay/link"
)

// resultWait bounds the wait for the next hop's answer to an OPEN.
const resultWait = 10 * time.Second

// snapshot is a hop's hot configuration: replaced as a whole by an apply, read
// without a lock by every connection.
type snapshot struct {
	def      relayctl.Hop
	ups      []*upstream
	sources  link.Sources
	filter   bool // the RAW listener admits only sources
	maxFails uint32
	openFor  time.Duration
}

// hop is one hop of one route on the node: its sockets, its hot
// configuration, its counters.
type hop struct {
	r           *Relay
	key         relayctl.Key
	listenerKey string
	epoch       string

	snap atomic.Pointer[snapshot]
	rot  atomic.Pointer[rotation]

	socks sockets
	// listening is true while the hop's accept loops run.
	listening atomic.Bool

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	upLim, downLim *bucket

	up, down, upPk, downPk, total atomic.Uint64
	active                        atomic.Int64

	assocs *assocTable
}

// sockets are the listening sockets of a hop.
type sockets struct {
	tcp net.Listener
	udp *net.UDPConn
	car relay.CarrierListener
}

func (s *sockets) close() {
	if s.tcp != nil {
		_ = s.tcp.Close()
	}
	if s.udp != nil {
		_ = s.udp.Close()
	}
	if s.car != nil {
		_ = s.car.Close()
	}
}

func listenAddr(l relayctl.Listen) string {
	return net.JoinHostPort(l.Address, strconv.FormatUint(uint64(l.Port), 10))
}

// bind binds the sockets of def's listener. It changes nothing else.
func (r *Relay) bind(def relayctl.Hop) (*sockets, error) {
	s := &sockets{}
	addr := listenAddr(def.Listen)
	var err error
	switch def.Ingress.Security {
	case relayctl.SecurityRaw:
		if def.Listen.TCP {
			if s.tcp, err = net.Listen("tcp", addr); err != nil {
				return nil, err
			}
		}
		if def.Listen.UDP {
			var pc net.PacketConn
			if pc, err = net.ListenPacket("udp", addr); err != nil {
				s.close()
				return nil, err
			}
			s.udp = pc.(*net.UDPConn)
		}
	case relayctl.SecurityAnixOps:
		if s.car, err = r.listenCarriers(def, addr); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (r *Relay) listenCarriers(def relayctl.Hop, addr string) (relay.CarrierListener, error) {
	ccfg := r.carrierConfig()
	if def.Ingress.Carrier == relayctl.CarrierPlain {
		return relay.ListenPlain(addr, link.PlainListenerConfig{TrustedLink: true, Sources: def.Sources}, ccfg)
	}
	creds := r.credentials()
	if creds == nil {
		return nil, fmt.Errorf("%w: %s needs the link certificate", errInvalid, def.Ingress.Carrier)
	}
	lcfg := link.ListenerConfig{Credentials: creds, Protocol: relay.ALPN, Sources: def.Sources, Peers: def.Peers}
	qcfg := relay.QUICConfig{Config: ccfg, StatelessResetKey: r.resetKey()}
	switch def.Ingress.Carrier {
	case relayctl.CarrierTLSTCP:
		return relay.Listen(addr, lcfg, ccfg)
	case relayctl.CarrierQUIC:
		return relay.ListenQUIC(addr, lcfg, qcfg)
	default:
		return relay.ListenAuto(addr, lcfg, qcfg)
	}
}

// newHop builds the hop around its bound sockets and starts serving.
func (r *Relay) newHop(def relayctl.Hop, socks *sockets, seq uint64) *hop {
	ctx, cancel := context.WithCancel(context.Background())
	h := &hop{
		r:           r,
		key:         def.Key(),
		listenerKey: def.ListenerKey(),
		socks:       *socks,
		ctx:         ctx,
		cancel:      cancel,
		upLim:       newBucket(def.Limits.BandwidthBps, r.now),
		downLim:     newBucket(def.Limits.BandwidthBps, r.now),
		assocs:      newAssocTable(),
	}
	sum := sha256.Sum256([]byte(r.instance + "/" + def.Route + "/" + strconv.FormatUint(uint64(def.Hop), 10) + "/" + strconv.FormatUint(seq, 10)))
	h.epoch = hex.EncodeToString(sum[:8])
	h.update(def)
	h.listening.Store(true)
	if h.socks.tcp != nil {
		h.wg.Add(1)
		go h.serveRawTCP()
	}
	if h.socks.udp != nil {
		h.wg.Add(1)
		go h.serveRawUDP()
	}
	if h.socks.car != nil {
		h.wg.Add(1)
		go h.serveCarriers()
	}
	return h
}

// update installs def's hot configuration. Upstreams whose transport did not
// change keep their state (connections, breaker, carriers); the rotation goes
// back to every rendered upstream, with the rendered weights.
func (h *hop) update(def relayctl.Hop) {
	var oldBy map[string]*upstream
	if old := h.snap.Load(); old != nil {
		oldBy = make(map[string]*upstream, len(old.ups))
		for _, u := range old.ups {
			oldBy[u.id] = u
		}
	}
	nextHop := def.Hop + 1
	ups := make([]*upstream, 0, len(def.Upstreams))
	entries := make([]entry, 0, len(def.Upstreams))
	for _, ud := range def.Upstreams {
		id := net.JoinHostPort(ud.Address, strconv.FormatUint(uint64(ud.Port), 10))
		u := oldBy[id]
		if u != nil && u.same(ud, nextHop) {
			delete(oldBy, id)
		} else {
			u = h.r.newUpstream(id, ud, nextHop)
		}
		ups = append(ups, u)
		entries = append(entries, entry{up: u, weight: ud.Weight, priority: ud.Priority})
	}
	for _, u := range oldBy {
		if u.pool != nil {
			u.pool.retire()
		}
	}
	s := &snapshot{
		def:      def,
		ups:      ups,
		maxFails: def.Breaker.MaxFails,
		openFor:  time.Duration(def.Breaker.OpenMs) * time.Millisecond,
	}
	if len(def.Sources) > 0 {
		s.sources, _ = link.ParseSources(def.Sources)
		s.filter = true
	}
	h.upLim.setRate(def.Limits.BandwidthBps)
	h.downLim.setRate(def.Limits.BandwidthBps)
	h.snap.Store(s)
	h.rot.Store(newRotation(entries))
	if car := h.socks.car; car != nil && len(def.Sources) > 0 {
		_ = car.SetSources(def.Sources)
		if def.Ingress.Carrier != relayctl.CarrierPlain && len(def.Peers) > 0 {
			_, _ = car.SetPeers(def.Peers)
		}
	}
}

// setRotation puts the named upstreams in rotation.
func (h *hop) setRotation(active []relayctl.RotationEntry) error {
	s := h.snap.Load()
	if len(active) == 0 {
		return fmt.Errorf("%w: an empty selection", errInvalid)
	}
	byID := make(map[string]entry, len(s.ups))
	for _, ud := range s.def.Upstreams {
		id := net.JoinHostPort(ud.Address, strconv.FormatUint(uint64(ud.Port), 10))
		for _, u := range s.ups {
			if u.id == id {
				byID[id] = entry{up: u, weight: ud.Weight, priority: ud.Priority}
			}
		}
	}
	seen := map[string]bool{}
	entries := make([]entry, 0, len(active))
	for _, a := range active {
		id := net.JoinHostPort(a.Address, strconv.FormatUint(uint64(a.Port), 10))
		e, ok := byID[id]
		if !ok {
			return fmt.Errorf("%w: %s is not an upstream of hop %s", errNotFound, id, h.key)
		}
		if seen[id] {
			return fmt.Errorf("%w: %s twice", errInvalid, id)
		}
		seen[id] = true
		if a.Weight > 0 {
			e.weight = a.Weight
		}
		entries = append(entries, e)
	}
	h.rot.Store(newRotation(entries))
	return nil
}

// close stops the hop: its sockets close (a carrier listener sends GOAWAY to
// every carrier it accepted and lets them drain, L1), its loops end and its
// upstreams' pools are retired. The returned channel closes when a carrier
// listener has finished draining.
func (h *hop) close() <-chan struct{} {
	h.listening.Store(false)
	h.cancel()
	done := make(chan struct{})
	tcp, udp, car := h.socks.tcp, h.socks.udp, h.socks.car
	if tcp != nil {
		_ = tcp.Close()
	}
	if udp != nil {
		_ = udp.Close()
	}
	h.assocs.closeAll()
	go func() {
		defer close(done)
		if car != nil {
			_ = car.Close()
		}
		h.wg.Wait()
		if s := h.snap.Load(); s != nil {
			for _, u := range s.ups {
				if u.pool != nil {
					u.pool.retire()
				}
			}
		}
	}()
	return done
}

// obs reads the hop's counters and state.
func (h *hop) obs() relayctl.HopObs {
	s := h.snap.Load()
	o := relayctl.HopObs{
		Route:       h.key.Route,
		Hop:         h.key.Hop,
		Epoch:       h.epoch,
		UpBytes:     h.up.Load(),
		DownBytes:   h.down.Load(),
		UpPackets:   h.upPk.Load(),
		DownPackets: h.downPk.Load(),
		Active:      uint32(max(h.active.Load(), 0)), // #nosec G115 -- clamped, and a connection count
		Total:       h.total.Load(),
	}
	for _, e := range h.rot.Load().entries {
		o.Rotation = append(o.Rotation, relayctl.RotationEntry{Address: e.up.def.Address, Port: e.up.def.Port, Weight: e.weight})
	}
	now := h.r.now()
	for _, u := range s.ups {
		fails, until := u.health()
		if fails == 0 && u.active.Load() == 0 {
			continue
		}
		st := relayctl.UpstreamState{Address: u.def.Address, Port: u.def.Port, ConsecutiveFailures: fails, ActiveStreams: uint32(max(u.active.Load(), 0))} // #nosec G115 -- clamped
		if now.Before(until) {
			st.CircuitOpenUntilMs = until.UnixMilli()
		}
		o.Health = append(o.Health, st)
	}
	return o
}

func (h *hop) add(dir direction, bytes, packets uint64) {
	if dir == dirUp {
		h.up.Add(bytes)
		h.upPk.Add(packets)
	} else {
		h.down.Add(bytes)
		h.downPk.Add(packets)
	}
}

func (h *hop) limiter(dir direction) *bucket {
	if dir == dirUp {
		return h.upLim
	}
	return h.downLim
}

// quotaUsed reports whether the hop moved quota_bytes already in this epoch.
func (h *hop) quotaUsed() bool {
	q := h.snap.Load().def.Limits.QuotaBytes
	return q > 0 && h.up.Load()+h.down.Load() >= q
}

// admit takes a connection slot for a new connection or association, or
// answers why not.
func (h *hop) admit() (relay.ResultCode, bool) {
	s := h.snap.Load()
	switch {
	case s.def.Paused:
		return relay.ResultPaused, false
	case h.quotaUsed():
		return relay.ResultQuotaExceeded, false
	}
	max := int64(s.def.Limits.MaxConns)
	for {
		a := h.active.Load()
		if max > 0 && a >= max {
			return relay.ResultLimitExceeded, false
		}
		if h.active.CompareAndSwap(a, a+1) {
			h.total.Add(1)
			return relay.ResultOK, true
		}
	}
}

func (h *hop) release() { h.active.Add(-1) }

// sourceAllowed applies the RAW listener's admission.
func (h *hop) sourceAllowed(a netip.Addr) bool {
	s := h.snap.Load()
	return !s.filter || s.sources.Contains(a)
}

func addrOf(a net.Addr) netip.AddrPort {
	switch v := a.(type) {
	case *net.TCPAddr:
		return v.AddrPort()
	case *net.UDPAddr:
		return v.AddrPort()
	}
	ap, _ := netip.ParseAddrPort(a.String())
	return ap
}

// serveRawTCP accepts the clients of a RAW listener.
func (h *hop) serveRawTCP() {
	defer h.wg.Done()
	for {
		c, err := h.socks.tcp.Accept()
		if err != nil {
			h.acceptEnded(err)
			return
		}
		go h.handleRawConn(c)
	}
}

// acceptEnded notes that a listener failed on its own (not closed by us).
func (h *hop) acceptEnded(err error) {
	if h.ctx.Err() == nil && !errors.Is(err, net.ErrClosed) {
		h.r.log.Error("listener failed", "route", h.key.Route, "hop", h.key.Hop, "err", err)
	}
	if h.ctx.Err() == nil {
		h.listening.Store(false)
	}
}

func (h *hop) handleRawConn(c net.Conn) {
	defer func() { _ = c.Close() }()
	client := addrOf(c.RemoteAddr())
	if !h.sourceAllowed(client.Addr()) {
		return
	}
	if _, ok := h.admit(); !ok {
		return
	}
	defer h.release()
	ctx, cancel := context.WithCancel(h.ctx)
	defer cancel()
	out, u, _, err := h.connect(ctx, client, relay.StreamTCP)
	if err != nil {
		return
	}
	defer u.active.Add(-1)
	h.pipe(connDuplex{c}, out)
}

// serveCarriers accepts the carriers of the previous hop and their streams.
func (h *hop) serveCarriers() {
	defer h.wg.Done()
	for {
		c, err := h.socks.car.Accept(h.ctx)
		if err != nil {
			h.acceptEnded(err)
			return
		}
		go h.serveCarrier(c)
	}
}

func (h *hop) serveCarrier(c relay.Carrier) {
	for {
		s, err := c.Accept(h.ctx)
		if err != nil {
			return
		}
		go h.handleStream(s)
	}
}

// handleStream serves one stream of the previous hop: a TCP connection or a
// UDP association. The stream is answered exactly once.
func (h *hop) handleStream(s relay.Stream) {
	p := s.Params()
	if p.RouteID != h.key.Route || p.HopIndex != h.key.Hop {
		h.r.log.Warn("a stream names another route or hop", "route", h.key.Route, "hop", h.key.Hop, "peer_route", p.RouteID, "peer_hop", p.HopIndex)
		_ = s.Reject(relay.ResultRouteMismatch)
		return
	}
	code, ok := h.admit()
	if !ok {
		_ = s.Reject(code)
		return
	}
	defer h.release()
	ctx, cancel := context.WithCancel(h.ctx)
	defer cancel()
	switch p.Kind {
	case relay.StreamUDP:
		h.streamUDP(ctx, s, p)
	default:
		out, u, code, err := h.connect(ctx, p.Client, relay.StreamTCP)
		if err != nil {
			_ = s.Reject(code)
			return
		}
		defer u.active.Add(-1)
		if err := s.Accept(); err != nil {
			_ = out.Close()
			return
		}
		h.pipe(s, out)
	}
}

// connect opens the hop's outbound TCP connection: it picks an upstream by the
// strategy and dials it, and on failure counts it against the breaker and
// moves on to the next candidate. An exit's upstream is a target (dialled
// directly); a relay's or entry's is the next hop (a stream on a carrier,
// whose answer is awaited so a refusing next hop is another failure). The
// returned upstream has its connection counted; the caller must decrement it.
func (h *hop) connect(ctx context.Context, client netip.AddrPort, kind relay.StreamKind) (duplex, *upstream, relay.ResultCode, error) {
	s := h.snap.Load()
	rot := h.rot.Load()
	skip := map[*upstream]bool{}
	var last error
	for range rot.entries {
		now := h.r.now()
		u := rot.pick(s.def.Balance, client.Addr(), skip, now)
		if u == nil {
			break
		}
		skip[u] = true
		out, err := h.dial(ctx, u, client, kind)
		if err != nil {
			u.failure(h.r.now(), s.maxFails, s.openFor)
			h.logUpstreamFailure(u, err)
			last = err
			continue
		}
		u.success()
		u.active.Add(1)
		return out, u, relay.ResultOK, nil
	}
	if last == nil {
		last = errors.New("no upstream in rotation")
	}
	return nil, nil, relay.ResultUpstreamUnreachable, last
}

// logUpstreamFailure says, at most once in a while per upstream, that it could
// not be reached and why (the peer's identity and the reason code, never a
// client's address).
func (h *hop) logUpstreamFailure(u *upstream, err error) {
	if !u.shouldLog(h.r.now()) {
		return
	}
	attrs := []any{"route", h.key.Route, "hop", h.key.Hop, "upstream", u.id, "err", err}
	if u.def.PeerIdentity != "" {
		attrs = append(attrs, "peer_identity", u.def.PeerIdentity)
	}
	h.r.log.Warn("could not reach an upstream", attrs...)
}

// dial opens one connection to u.
func (h *hop) dial(ctx context.Context, u *upstream, client netip.AddrPort, kind relay.StreamKind) (duplex, error) {
	if u.pool == nil {
		c, err := h.r.dial(ctx, "tcp", u.id)
		if err != nil {
			return nil, err
		}
		return connDuplex{c}, nil
	}
	st, err := u.pool.open(ctx, relay.OpenParams{Kind: kind, RouteID: h.key.Route, HopIndex: u.nextHop, Client: client})
	if err != nil {
		return nil, err
	}
	actx, cancel := context.WithTimeout(ctx, resultWait)
	defer cancel()
	if err := st.AwaitResult(actx); err != nil {
		_ = st.Close()
		return nil, err
	}
	return st, nil
}
