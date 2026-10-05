package relayd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"slices"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/AnixOps/anix-control/sdk/forward/driver/anixops/relayctl"
	"github.com/AnixOps/anix-control/sdk/forward/relay"
	"github.com/AnixOps/anix-control/sdk/forward/relay/link"
)

// Errors of Apply and SetRotation; the control API maps them to its codes.
var (
	errInvalid  = errors.New("invalid")
	errNotFound = errors.New("not found")
	// ErrConflict means a port another process holds.
	ErrConflict = errors.New("a port is held by another process")
)

// Options configure a Relay.
type Options struct {
	// Dial opens the connections to targets (network "tcp" or "udp"); a
	// net.Dialer with ConnectTimeout when nil. Tests redirect it.
	Dial func(ctx context.Context, network, address string) (net.Conn, error)
	// ConnectTimeout bounds a dial to a target; 5 s when zero.
	ConnectTimeout time.Duration
	// UDPIdle is the idle timeout of a UDP association; DefaultUDPIdle when
	// zero.
	UDPIdle time.Duration
	// Carrier configures every carrier the relay makes or accepts; the zero
	// value is the document's defaults.
	Carrier relay.Config
	// Logger receives the relay's lifecycle events (JSON to the journal in
	// the unit); nothing when nil. Client addresses are never logged.
	Logger *slog.Logger
	// Now is the clock of breakers and buckets; time.Now when nil.
	Now func() time.Time
	// Fault, set by tests, is called at the start of every Apply; an error
	// fails the Apply before it changes anything.
	Fault func() error
}

// Relay is the hop runtime of one node.
type Relay struct {
	opts     Options
	instance string
	log      *slog.Logger
	udpIdle  time.Duration

	// mu serialises Apply, SetRotation and ReloadCredentials; Observe and
	// Status take it only to read the hop table.
	mu      sync.Mutex
	cfg     *relayctl.Config
	digest  string
	applies uint64
	hops    map[relayctl.Key]*hop
	seq     uint64
	retired []relayctl.HopObs
	closed  bool

	credMu    sync.RWMutex
	creds     *link.Credentials
	credFiles relayctl.LinkFiles
	reset     *[32]byte
}

// New makes a relay with no configuration: it runs nothing until Apply.
func New(opts Options) *Relay {
	var id [8]byte
	_, _ = rand.Read(id[:])
	r := &Relay{
		opts:     opts,
		instance: hex.EncodeToString(id[:]),
		log:      opts.Logger,
		udpIdle:  opts.UDPIdle,
		hops:     map[relayctl.Key]*hop{},
	}
	if r.log == nil {
		r.log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if r.udpIdle == 0 {
		r.udpIdle = DefaultUDPIdle
	}
	return r
}

func (r *Relay) now() time.Time {
	if r.opts.Now != nil {
		return r.opts.Now()
	}
	return time.Now()
}

func (r *Relay) dial(ctx context.Context, network, address string) (net.Conn, error) {
	if r.opts.Dial != nil {
		return r.opts.Dial(ctx, network, address)
	}
	d := net.Dialer{Timeout: r.opts.ConnectTimeout}
	if d.Timeout == 0 {
		d.Timeout = 5 * time.Second
	}
	return d.DialContext(ctx, network, address)
}

func (r *Relay) carrierConfig() relay.Config { return r.opts.Carrier }

func (r *Relay) credentials() *link.Credentials {
	r.credMu.RLock()
	defer r.credMu.RUnlock()
	return r.creds
}

func (r *Relay) resetKey() *[32]byte {
	r.credMu.RLock()
	defer r.credMu.RUnlock()
	return r.reset
}

// Instance is the id of this relay process.
func (r *Relay) Instance() string { return r.instance }

// newUpstream makes the state of one upstream: for an AnixOps egress, a
// Selector and a pool of carriers.
func (r *Relay) newUpstream(id string, d relayctl.Upstream, nextHop uint32) *upstream {
	u := &upstream{def: d, id: id, nextHop: nextHop}
	if d.Egress.Security != relayctl.SecurityAnixOps {
		return u
	}
	choice, _ := relay.ParseCarrierChoice(d.Egress.Carrier)
	ccfg := r.carrierConfig()
	scfg := relay.SelectorConfig{
		Choice:  choice,
		Address: id,
		Carrier: ccfg,
		QUIC:    relay.QUICConfig{Config: ccfg},
		Plain:   link.PlainDialConfig{TrustedLink: true},
	}
	creds := r.credentials()
	if choice != relay.ChoicePlain {
		scfg.Link = link.DialConfig{Credentials: creds, ServerName: d.Egress.ServerName, PeerIdentity: d.PeerIdentity, Protocol: relay.ALPN}
	}
	sel, err := relay.NewSelector(scfg)
	if err != nil {
		// Apply checked the configuration; a selector that still fails
		// leaves the upstream undialable, which its breaker reports.
		r.log.Error("upstream has no carrier selector", "upstream", id, "err", err)
		u.pool = newPool(nil, creds)
		u.pool.closed = true
		return u
	}
	u.pool = newPool(sel, creds)
	return u
}

// Apply validates and runs a configuration document (the encoded bytes of a
// relayctl.Config). It binds every new listener before it changes anything, so
// a failure (ErrConflict for a port another process holds) changes nothing.
// changed is false when the relay already runs the document.
func (r *Relay) Apply(content []byte) (changed bool, digest string, err error) {
	cfg, err := relayctl.Parse(content)
	if err != nil {
		return false, "", fmt.Errorf("%w: %w", errInvalid, err)
	}
	digest = relayctl.Digest(content)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return false, "", errors.New("relay is closed")
	}
	if r.opts.Fault != nil {
		if err := r.opts.Fault(); err != nil {
			return false, "", err
		}
	}
	if digest == r.digest && r.allListening() {
		return false, digest, nil
	}
	if err := r.loadLink(cfg); err != nil {
		return false, "", err
	}
	if err := r.checkLink(cfg); err != nil {
		return false, "", err
	}

	// Plan: which hops keep their listener, which get a new one.
	next := map[relayctl.Key]relayctl.Hop{}
	for _, d := range cfg.Hops {
		next[d.Key()] = d
	}
	var fresh []relayctl.Hop // need a listener
	var releasing []*hop     // lose theirs (removed, or replaced)
	for _, d := range cfg.Hops {
		old := r.hops[d.Key()]
		if old != nil && old.listenerKey == d.ListenerKey() && old.listening.Load() {
			continue
		}
		fresh = append(fresh, d)
		if old != nil {
			releasing = append(releasing, old)
		}
	}
	for k, old := range r.hops {
		if _, ok := next[k]; !ok {
			releasing = append(releasing, old)
		}
	}

	// Bind what does not overlap what is being released; the overlap waits
	// for the release.
	bound := map[relayctl.Key]*sockets{}
	var later []relayctl.Hop
	closeBound := func() {
		for _, s := range bound {
			s.close()
		}
	}
	for _, d := range fresh {
		if overlaps(d, releasing) {
			later = append(later, d)
			continue
		}
		s, err := r.bind(d)
		if err != nil {
			closeBound()
			return false, "", bindError(d, err)
		}
		bound[d.Key()] = s
	}

	// Commit. From here on the hops that were running are being replaced.
	prev := r.cfg
	for _, old := range releasing {
		r.retire(old)
		old.close()
		delete(r.hops, old.key)
	}
	for _, d := range later {
		s, err := r.bindRetry(d)
		if err != nil {
			closeBound()
			r.restore(prev)
			return false, "", bindError(d, err)
		}
		bound[d.Key()] = s
	}
	for _, d := range cfg.Hops {
		if s := bound[d.Key()]; s != nil {
			r.seq++
			r.hops[d.Key()] = r.newHop(d, s, r.seq)
			r.log.Info("hop listening", "route", d.Route, "hop", d.Hop, "listen", listenAddr(d.Listen), "ingress", d.Ingress.Security+"/"+d.Ingress.Carrier)
			continue
		}
		r.hops[d.Key()].update(d)
	}
	r.cfg, r.digest = cfg, digest
	r.applies++
	return true, digest, nil
}

// restore puts the listeners of the previous document back after an apply
// that released them could not bind the new ones (a hop whose own listener
// changes can only be bound once the old one is closed). The restored hops
// start new epochs; the digest is the previous document's.
func (r *Relay) restore(prev *relayctl.Config) {
	if prev == nil {
		return
	}
	for _, d := range prev.Hops {
		if h := r.hops[d.Key()]; h != nil && h.listening.Load() {
			h.update(d)
			continue
		}
		s, err := r.bindRetry(d)
		if err != nil {
			r.log.Error("could not put a listener back", "route", d.Route, "hop", d.Hop, "err", err)
			continue
		}
		r.seq++
		r.hops[d.Key()] = r.newHop(d, s, r.seq)
	}
	for k, h := range r.hops {
		found := false
		for _, d := range prev.Hops {
			if d.Key() == k {
				found = true
			}
		}
		if !found {
			r.retire(h)
			h.close()
			delete(r.hops, k)
		}
	}
}

// retire records the final counters of a hop whose epoch ends.
func (r *Relay) retire(h *hop) {
	r.retired = append(r.retired, h.obs())
	if n := len(r.retired); n > 256 {
		r.retired = slices.Clone(r.retired[n-256:])
	}
}

func overlaps(d relayctl.Hop, hops []*hop) bool {
	for _, h := range hops {
		s := h.snap.Load()
		if s == nil {
			continue
		}
		o := s.def.Listen
		if o.Port != d.Listen.Port {
			continue
		}
		if (o.TCP || carrierTCP(s.def)) && (d.Listen.TCP || carrierTCP(d)) {
			return true
		}
		if (o.UDP || carrierUDP(s.def)) && (d.Listen.UDP || carrierUDP(d)) {
			return true
		}
	}
	return false
}

func carrierTCP(h relayctl.Hop) bool {
	return h.Ingress.Security == relayctl.SecurityAnixOps && h.Ingress.Carrier != relayctl.CarrierQUIC
}

func carrierUDP(h relayctl.Hop) bool {
	return h.Ingress.Security == relayctl.SecurityAnixOps && (h.Ingress.Carrier == relayctl.CarrierQUIC || h.Ingress.Carrier == relayctl.CarrierAuto)
}

// bindRetry binds a listener that waited for the release of an old one, which
// may take a moment: a closed listener frees its port at once, a QUIC one
// after its carriers drained.
func (r *Relay) bindRetry(d relayctl.Hop) (*sockets, error) {
	deadline := time.Now().Add(relay.DefaultDrainTimeout + 3*time.Second)
	for {
		s, err := r.bind(d)
		if err == nil || !errors.Is(err, syscall.EADDRINUSE) || time.Now().After(deadline) {
			return s, err
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func bindError(d relayctl.Hop, err error) error {
	if errors.Is(err, syscall.EADDRINUSE) {
		return fmt.Errorf("%w: %s port %d: %v", ErrConflict, d.Key(), d.Listen.Port, err)
	}
	if errors.Is(err, errInvalid) {
		return err
	}
	return fmt.Errorf("listen for hop %s: %w", d.Key(), err)
}

func (r *Relay) allListening() bool {
	for _, h := range r.hops {
		if !h.listening.Load() {
			return false
		}
	}
	return true
}

// loadLink loads the link credentials the document names (once per set of
// paths) and the stateless reset key.
func (r *Relay) loadLink(cfg *relayctl.Config) error {
	r.credMu.Lock()
	defer r.credMu.Unlock()
	if cfg.Link != nil && (r.creds == nil || r.credFiles != *cfg.Link) {
		c, err := link.LoadCredentials(cfg.Link.Cert, cfg.Link.Key, cfg.Link.CA)
		if err != nil {
			return fmt.Errorf("%w: link credentials: %w", errInvalid, err)
		}
		r.creds, r.credFiles = c, *cfg.Link
	}
	if cfg.Link == nil {
		r.creds = nil
	}
	if cfg.StatelessResetKeyFile != "" {
		b, err := os.ReadFile(cfg.StatelessResetKeyFile) // #nosec G304 G703 -- the driver's own key file, named by the document it rendered
		if err != nil || len(b) != 32 {
			return fmt.Errorf("%w: stateless reset key %s must be a readable file of 32 bytes", errInvalid, cfg.StatelessResetKeyFile)
		}
		var k [32]byte
		copy(k[:], b)
		r.reset = &k
	}
	return nil
}

// checkLink refuses a document that needs the link certificate without one.
func (r *Relay) checkLink(cfg *relayctl.Config) error {
	if r.credentials() != nil {
		return nil
	}
	for _, h := range cfg.Hops {
		if h.Ingress.Security == relayctl.SecurityAnixOps && h.Ingress.Carrier != relayctl.CarrierPlain {
			return fmt.Errorf("%w: hop %s: an encrypted ingress needs the link certificate", errInvalid, h.Key())
		}
		for _, u := range h.Upstreams {
			if u.Egress.Security == relayctl.SecurityAnixOps && u.Egress.Carrier != relayctl.CarrierPlain {
				return fmt.Errorf("%w: hop %s: an encrypted egress needs the link certificate", errInvalid, h.Key())
			}
		}
	}
	return nil
}

// ReloadCredentials makes the relay read the link files again. Carriers whose
// peer is no longer trusted end (GOAWAY credentials_changed); every other
// connection runs on.
func (r *Relay) ReloadCredentials() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	creds := r.credentials()
	if creds == nil {
		return fmt.Errorf("%w: the relay has no link credentials", errInvalid)
	}
	r.credMu.RLock()
	f := r.credFiles
	r.credMu.RUnlock()
	if err := creds.ReloadFiles(f.Cert, f.Key, f.CA); err != nil {
		return fmt.Errorf("%w: %w", errInvalid, err)
	}
	return nil
}

// Status says what the relay runs.
func (r *Relay) Status() relayctl.Status {
	r.mu.Lock()
	defer r.mu.Unlock()
	st := relayctl.Status{Instance: r.instance, Digest: r.digest, Applies: r.applies}
	keys := make([]relayctl.Key, 0, len(r.hops))
	for k := range r.hops {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, func(a, b relayctl.Key) int {
		if a.Route != b.Route {
			return cmpString(a.Route, b.Route)
		}
		return cmpUint(a.Hop, b.Hop)
	})
	for _, k := range keys {
		st.Hops = append(st.Hops, relayctl.HopStatus{Route: k.Route, Hop: k.Hop, Listening: r.hops[k].listening.Load()})
	}
	return st
}

func cmpString(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func cmpUint(a, b uint32) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// Observe reads every hop's counters; drain also takes the counters of the
// epochs that ended since the last drain.
func (r *Relay) Observe(drain bool) relayctl.Observation {
	r.mu.Lock()
	hops := make([]*hop, 0, len(r.hops))
	for _, h := range r.hops {
		hops = append(hops, h)
	}
	o := relayctl.Observation{Instance: r.instance, Digest: r.digest}
	if drain {
		o.Retired, r.retired = r.retired, nil
	}
	r.mu.Unlock()
	slices.SortFunc(hops, func(a, b *hop) int {
		if a.key.Route != b.key.Route {
			return cmpString(a.key.Route, b.key.Route)
		}
		return cmpUint(a.key.Hop, b.key.Hop)
	})
	for _, h := range hops {
		o.Hops = append(o.Hops, h.obs())
	}
	return o
}

// SetRotation puts the named upstreams of one hop in rotation, with weights.
func (r *Relay) SetRotation(rot relayctl.Rotation) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	h := r.hops[relayctl.Key{Route: rot.Route, Hop: rot.Hop}]
	if h == nil {
		return fmt.Errorf("%w: hop %s/%s", errNotFound, rot.Route, strconv.FormatUint(uint64(rot.Hop), 10))
	}
	return h.setRotation(rot.Active)
}

// Close stops the relay (L3): every listener closes with GOAWAY to its
// carriers, every upstream's carriers close, and Close returns when they have
// ended.
func (r *Relay) Close() {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	r.closed = true
	hops := make([]*hop, 0, len(r.hops))
	for _, h := range r.hops {
		hops = append(hops, h)
	}
	r.hops = map[relayctl.Key]*hop{}
	r.digest = ""
	r.mu.Unlock()
	var wg sync.WaitGroup
	for _, h := range hops {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-h.close()
			if s := h.snap.Load(); s != nil {
				for _, u := range s.ups {
					if u.pool != nil {
						u.pool.close()
					}
				}
			}
		}()
	}
	wg.Wait()
}
