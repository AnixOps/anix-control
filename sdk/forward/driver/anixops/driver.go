package anixops

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"sync"
	"time"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/driver"
	"github.com/AnixOps/anix-control/sdk/forward/driver/anixops/relayctl"
)

// stoppedEpoch is the counter epoch of a hop while the relay does not run.
const stoppedEpoch = "stopped"

// controlTimeout bounds one call of the relay's control API (an Apply may wait
// for a QUIC listener's port to be released).
const controlTimeout = 30 * time.Second

// Driver is the anixops forward driver. It keeps no state besides its
// configuration: what it applied is read back from its directory and from the
// relay process. Apply, SetUpstreams, ReloadCredentials and Remove are
// serialised; Observe runs between them.
type Driver struct {
	cfg    Config
	runner Runner
	sup    Supervisor
	client *relayctl.Client
	// retired receives the last counters of the epochs a change ends.
	retired func([]*forwardv1.Counters)
	now     func() time.Time

	mu sync.RWMutex
}

var _ driver.Driver = (*Driver)(nil)

// Option configures New.
type Option func(*Driver)

// WithRunner runs the relay (Probe) and the SystemdSupervisor's systemctl
// through r instead of ExecRunner (tests).
func WithRunner(r Runner) Option { return func(d *Driver) { d.runner = r } }

// WithSupervisor runs the relay through s instead of a SystemdSupervisor on
// the driver's runner.
func WithSupervisor(s Supervisor) Option { return func(d *Driver) { d.sup = s } }

// WithRetiredCounters calls f from Observe with the final counters of the
// epochs that ended since the last Observe: a hop whose listener changed or
// that was removed. The Agent reports them, as for the nftables driver's
// removed hops. Without it the relay keeps them (up to 256) until a driver
// with the hook observes.
func WithRetiredCounters(f func([]*forwardv1.Counters)) Option {
	return func(d *Driver) { d.retired = f }
}

// New answers a driver with the given configuration, or ErrInvalidConfig. The
// configuration usually comes from Probe.
func New(cfg Config, opts ...Option) (*Driver, error) {
	if err := cfg.check(); err != nil {
		return nil, err
	}
	cfg.Strategies = slices.Clone(cfg.Strategies)
	cfg.Carriers = slices.Clone(cfg.Carriers)
	cfg.ProtocolVersions = slices.Clone(cfg.ProtocolVersions)
	d := &Driver{cfg: cfg, runner: ExecRunner{}, now: time.Now}
	for _, o := range opts {
		o(d)
	}
	if d.sup == nil {
		d.sup = SystemdSupervisor{Runner: d.runner}
	}
	d.client = relayctl.NewClient(cfg.socketPath())
	d.client.Timeout = controlTimeout
	return d, nil
}

// Engine answers ENGINE_ANIXOPS.
func (d *Driver) Engine() forwardv1.Engine { return forwardv1.Engine_ENGINE_ANIXOPS }

// Config answers a copy of the driver's configuration.
func (d *Driver) Config() Config {
	c := d.cfg
	c.Strategies = slices.Clone(c.Strategies)
	c.Carriers = slices.Clone(c.Carriers)
	c.ProtocolVersions = slices.Clone(c.ProtocolVersions)
	return c
}

// Capabilities answers the capabilities of the configuration, which Probe
// filled from the host: a configuration without a Version is unavailable. The
// link securities are RAW (the client side and the trusted networks, which the
// relay terminates and originates) and ANIXOPS; the carriers are the ones the
// host serves and dials, never AUTO. PROXY protocol v2 is not implemented yet.
func (d *Driver) Capabilities(ctx context.Context) (*forwardv1.EngineCapabilities, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c := &forwardv1.EngineCapabilities{
		Engine:     forwardv1.Engine_ENGINE_ANIXOPS,
		Version:    d.cfg.Version,
		Available:  d.cfg.Version != "",
		Ipv6:       d.cfg.IPv6,
		Udp:        d.cfg.UDP,
		Strategies: slices.Clone(d.cfg.Strategies),
		LinkSecurities: []forwardv1.LinkSecurity{
			forwardv1.LinkSecurity_LINK_SECURITY_RAW,
			forwardv1.LinkSecurity_LINK_SECURITY_ANIXOPS,
		},
		BandwidthLimit:   d.cfg.BandwidthLimit,
		Quota:            d.cfg.Quota,
		MaxConns:         d.cfg.MaxConns,
		Carriers:         slices.Clone(d.cfg.Carriers),
		ProxyProtocol:    false,
		ProtocolVersions: slices.Clone(d.cfg.ProtocolVersions),
	}
	slices.Sort(c.Strategies)
	slices.Sort(c.Carriers)
	if !c.Available {
		c.UnavailableReason = d.cfg.Unavailable
		if c.UnavailableReason == "" {
			c.UnavailableReason = "anixops relay host not probed: the configuration has no version (anixops.Probe fills it)"
		}
	}
	return c, nil
}

// answers reports whether the relay answers its control socket.
func (d *Driver) answers(ctx context.Context) (*relayctl.Status, error) {
	st, err := d.client.Status(ctx)
	if err != nil {
		if cerr := ctx.Err(); cerr != nil {
			return nil, cerr
		}
		return nil, nil
	}
	return st, nil
}

// runs reports whether the relay runs the configuration with this digest and
// every hop listens.
func (d *Driver) runs(ctx context.Context, digest string) (bool, error) {
	st, err := d.answers(ctx)
	if st == nil || err != nil {
		return false, err
	}
	if st.Digest != digest {
		return false, nil
	}
	for _, h := range st.Hops {
		if !h.Listening {
			return false, nil
		}
	}
	return true, nil
}

// waitAnswers waits until the relay answers, or fails: it exited instead, or
// the ready timeout passed.
func (d *Driver) waitAnswers(ctx context.Context) error {
	deadline := time.Now().Add(d.cfg.ReadyTimeout)
	for {
		st, err := d.answers(ctx)
		if err != nil {
			return err
		}
		if st != nil {
			return nil
		}
		ss, err := d.sup.Status(ctx)
		if err != nil {
			return err
		}
		if !ss.Running && !ss.Starting {
			return errors.New("anixops driver: the relay exited instead of answering")
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("anixops driver: the relay did not answer within %v", d.cfg.ReadyTimeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
}

// parseArtifact reads the document of an artifact the driver rendered and
// checks that its hops are the artifact's.
func parseArtifact(a driver.Artifact) (*relayctl.Config, error) {
	doc, err := relayctl.Parse(a.Content)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", driver.ErrInvalidArtifact, err)
	}
	if doc.Owner != OwnerMark {
		return nil, fmt.Errorf("%w: not a configuration of %s", driver.ErrInvalidArtifact, OwnerMark)
	}
	keys := make([]driver.HopKey, len(doc.Hops))
	for i, h := range doc.Hops {
		keys[i] = driver.HopKey{RouteID: h.Route, HopIndex: h.Hop}
	}
	if !slices.Equal(keys, a.Hops) {
		return nil, fmt.Errorf("%w: document hops %v, artifact hops %v", driver.ErrInvalidArtifact, keys, a.Hops)
	}
	return doc, nil
}

// Apply makes the host run the artifact (driver package documentation):
//
//  1. read the directory; files of the driver's name without its state file
//     are ErrNotOwned;
//  2. check the generation against the recorded one;
//  3. compare: the recorded digest, the configuration file with the content,
//     and the relay running it. All equal is a no-op that at most records the
//     new generation and state_hash;
//  4. otherwise make sure the relay runs (the unit needs a configuration file
//     to start, so a first apply writes an empty one), send it the artifact:
//     the relay validates the whole document and binds every new listener
//     before it changes anything (ErrConflict for a port another process
//     holds), then changes hop by hop, keeping the listeners, carriers,
//     connections and counter epochs of the hops it does not change;
//  5. write the configuration file (temporary file and rename, so a restart
//     of the relay loads it) and the state.
//
// A failed first apply stops the relay and removes what it wrote; a failed
// later one leaves the previous configuration running. An empty artifact
// stops the relay and deletes the files.
func (d *Driver) Apply(ctx context.Context, a driver.Artifact) (driver.ApplyResult, error) {
	if err := ctx.Err(); err != nil {
		return driver.ApplyResult{}, err
	}
	if err := a.Verify(d.Engine()); err != nil {
		return driver.ApplyResult{}, err
	}
	doc, err := parseArtifact(a)
	if err != nil {
		return driver.ApplyResult{}, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	h, err := d.readHost()
	if err != nil {
		return driver.ApplyResult{}, err
	}
	if h.present && !h.owned {
		return driver.ApplyResult{}, d.notOwned()
	}
	if st := h.state; st.applied() {
		switch {
		case a.Generation < st.Generation:
			return driver.ApplyResult{}, fmt.Errorf("%w: artifact generation %d, host runs %d", driver.ErrStaleGeneration, a.Generation, st.Generation)
		case a.Generation == st.Generation && a.Digest != st.Digest:
			return driver.ApplyResult{}, fmt.Errorf("%w: generation %d", driver.ErrGenerationConflict, a.Generation)
		}
	}
	result := func(changed bool) driver.ApplyResult {
		return driver.ApplyResult{Changed: changed, Generation: a.Generation, StateHash: a.StateHash, Digest: a.Digest}
	}

	if a.Empty() {
		if !h.present {
			return result(false), nil
		}
		if err := d.stop(ctx); err != nil {
			return driver.ApplyResult{}, err
		}
		if err := d.removeFiles(); err != nil {
			return driver.ApplyResult{}, err
		}
		return result(true), nil
	}

	if err := d.sup.Check(ctx); err != nil {
		return driver.ApplyResult{}, err
	}
	if st := h.state; st.applied() && st.Digest == a.Digest && bytes.Equal(h.config, a.Content) {
		ok, err := d.runs(ctx, a.Digest)
		if err != nil {
			return driver.ApplyResult{}, err
		}
		if ok {
			if st.Generation != a.Generation || st.StateHash != a.StateHash || st.Node != a.NodeRef {
				if err := d.writeState(&hostState{Owner: OwnerMark, Node: a.NodeRef, Generation: a.Generation, StateHash: a.StateHash, Digest: a.Digest}); err != nil {
					return driver.ApplyResult{}, err
				}
			}
			return result(false), nil
		}
	}

	first := !h.state.applied()
	if !h.present {
		// The state file first: a crash between the files never leaves the
		// driver's own directory looking foreign.
		if err := d.writeState(&hostState{Owner: OwnerMark}); err != nil {
			return driver.ApplyResult{}, err
		}
	}
	if doc.StatelessResetKeyFile != "" {
		if err := d.ensureResetKey(); err != nil {
			return driver.ApplyResult{}, err
		}
	}
	started := false
	st, err := d.answers(ctx)
	if err != nil {
		return driver.ApplyResult{}, err
	}
	if st == nil {
		if h.config == nil {
			empty, err := d.document(nil)
			if err != nil {
				return driver.ApplyResult{}, err
			}
			if err := writeFileAtomic(d.cfg.configPath(), empty, 0o640); err != nil {
				return driver.ApplyResult{}, fmt.Errorf("anixops driver: write configuration: %w", err)
			}
		}
		if err := d.sup.Start(ctx); err != nil {
			return driver.ApplyResult{}, d.undo(ctx, first, err)
		}
		started = true
		if err := d.waitAnswers(ctx); err != nil {
			return driver.ApplyResult{}, d.undo(ctx, first, err)
		}
	}
	if _, err := d.client.Apply(ctx, a.Content); err != nil {
		err = applyError(err)
		if started || first {
			return driver.ApplyResult{}, d.undo(ctx, first, err)
		}
		return driver.ApplyResult{}, err
	}
	if err := writeFileAtomic(d.cfg.configPath(), a.Content, 0o640); err != nil {
		return driver.ApplyResult{}, fmt.Errorf("anixops driver: write configuration: %w", err)
	}
	if err := d.writeState(&hostState{Owner: OwnerMark, Node: a.NodeRef, Generation: a.Generation, StateHash: a.StateHash, Digest: a.Digest}); err != nil {
		return driver.ApplyResult{}, err
	}
	return result(true), nil
}

// undo puts the host back after a first apply failed: the relay stops and the
// files the apply wrote go. A later apply that failed after it had to start the
// relay leaves the previous files (the relay loads them at its next start).
func (d *Driver) undo(ctx context.Context, first bool, cause error) error {
	uctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	if first {
		_ = d.stop(uctx)
		_ = d.removeFiles()
	}
	return cause
}

// applyError maps the relay's refusals to the driver's errors.
func applyError(err error) error {
	var e *relayctl.Error
	if errors.As(err, &e) {
		switch e.Code {
		case relayctl.CodeConflict:
			return fmt.Errorf("%w: %s", driver.ErrConflict, e.Message)
		case relayctl.CodeInvalid:
			return fmt.Errorf("%w: %s", driver.ErrInvalidArtifact, e.Message)
		}
	}
	return err
}

// stop stops the relay when the unit is the driver's; without the unit there
// is nothing to stop.
func (d *Driver) stop(ctx context.Context) error {
	if err := d.sup.Check(ctx); err != nil {
		if cerr := ctx.Err(); cerr != nil {
			return cerr
		}
		if errors.Is(err, ErrNoUnit) || errors.Is(err, driver.ErrNotOwned) {
			return nil
		}
		return err
	}
	return d.sup.Stop(ctx)
}

// Observe answers the recorded identity, one Counters per applied hop and the
// rotation. While the relay runs they are its own: payload bytes up (client to
// target) and down per hop, UDP datagrams as packets, connections and
// associations in flight and since the epoch began. The epoch is the relay's:
// a hash of its instance and the creation of the hop's listener, so a restart
// or a re-created listener ends it for that hop and nothing else does. While
// the relay does not run every hop reports 0 in the epoch "stopped".
func (d *Driver) Observe(ctx context.Context) (driver.Observation, error) {
	if err := ctx.Err(); err != nil {
		return driver.Observation{}, err
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	h, err := d.readHost()
	if err != nil {
		return driver.Observation{}, err
	}
	o := driver.Observation{Engine: d.Engine(), ObservedAt: d.now()}
	if !h.owned || !h.state.applied() {
		return o, nil
	}
	st := h.state
	o.Applied, o.NodeRef, o.Generation, o.StateHash, o.Digest = true, st.Node, st.Generation, st.StateHash, st.Digest
	doc, err := relayctl.Parse(h.config)
	if err != nil {
		return driver.Observation{}, fmt.Errorf("anixops driver: the applied configuration does not parse: %w", err)
	}
	obs, err := d.client.Observe(ctx, d.retired != nil)
	if err != nil {
		if cerr := ctx.Err(); cerr != nil {
			return driver.Observation{}, cerr
		}
		obs = nil
	}
	byKey := map[relayctl.Key]relayctl.HopObs{}
	if obs != nil {
		for _, ho := range obs.Hops {
			byKey[relayctl.Key{Route: ho.Route, Hop: ho.Hop}] = ho
		}
	}
	nowMs := o.ObservedAt.UnixMilli()
	for _, hop := range doc.Hops {
		ho, ok := byKey[hop.Key()]
		c := &forwardv1.Counters{
			RouteId: hop.Route, HopIndex: hop.Hop, NodeRef: st.Node,
			CounterEpoch: stoppedEpoch, ObservedAtUnixMs: nowMs,
		}
		rot := driver.HopRotation{RouteID: hop.Route, HopIndex: hop.Hop}
		if ok {
			c.UpBytes, c.DownBytes, c.UpPackets, c.DownPackets = ho.UpBytes, ho.DownBytes, ho.UpPackets, ho.DownPackets
			c.ActiveConns, c.TotalConns, c.CounterEpoch = ho.Active, ho.Total, ho.Epoch
			for _, r := range ho.Rotation {
				rot.Active = append(rot.Active, driver.Upstream{Address: r.Address, Port: r.Port, Weight: r.Weight})
			}
			for _, uh := range ho.Health {
				if uh.ConsecutiveFailures == 0 && uh.CircuitOpenUntilMs == 0 {
					continue
				}
				state := forwardv1.HealthState_HEALTH_STATE_UNHEALTHY
				if uh.CircuitOpenUntilMs != 0 {
					state = forwardv1.HealthState_HEALTH_STATE_CIRCUIT_OPEN
				}
				o.Health = append(o.Health, &forwardv1.UpstreamHealth{
					RouteId: hop.Route, HopIndex: hop.Hop, Address: uh.Address, Port: uh.Port,
					State: state, ConsecutiveFailures: uh.ConsecutiveFailures,
					CircuitOpenUntilUnixMs: uh.CircuitOpenUntilMs, CheckedAtUnixMs: nowMs,
				})
			}
		} else {
			for _, u := range hop.Upstreams {
				rot.Active = append(rot.Active, driver.Upstream{Address: u.Address, Port: u.Port, Weight: u.Weight})
			}
		}
		o.Counters = append(o.Counters, c)
		o.Rotation = append(o.Rotation, rot)
	}
	if obs != nil && d.retired != nil && len(obs.Retired) > 0 {
		retired := make([]*forwardv1.Counters, 0, len(obs.Retired))
		for _, ho := range obs.Retired {
			retired = append(retired, &forwardv1.Counters{
				RouteId: ho.Route, HopIndex: ho.Hop, NodeRef: st.Node,
				UpBytes: ho.UpBytes, DownBytes: ho.DownBytes, UpPackets: ho.UpPackets, DownPackets: ho.DownPackets,
				ActiveConns: ho.Active, TotalConns: ho.Total, CounterEpoch: ho.Epoch, ObservedAtUnixMs: nowMs,
			})
		}
		d.retired(retired)
	}
	return o, nil
}

// SetUpstreams puts the named upstreams of one applied hop in rotation, with
// weights, through the relay: no apply, no new generation, no epoch ends and
// established connections keep their upstream. The rotation lives in the
// running relay, so it survives an Agent restart and a no-op Apply; a changing
// Apply puts every rendered upstream back.
func (d *Driver) SetUpstreams(ctx context.Context, routeID string, hopIndex uint32, active []driver.Upstream) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	h, err := d.readHost()
	if err != nil {
		return err
	}
	notFound := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", driver.ErrNotFound, fmt.Sprintf(format, args...))
	}
	if !h.owned || !h.state.applied() {
		return notFound("nothing is applied")
	}
	doc, err := relayctl.Parse(h.config)
	if err != nil {
		return fmt.Errorf("anixops driver: the applied configuration does not parse: %w", err)
	}
	var hop *relayctl.Hop
	for i := range doc.Hops {
		if doc.Hops[i].Route == routeID && doc.Hops[i].Hop == hopIndex {
			hop = &doc.Hops[i]
		}
	}
	if hop == nil {
		return notFound("route %s hop %d is not applied", routeID, hopIndex)
	}
	rotation := relayctl.Rotation{Route: routeID, Hop: hopIndex}
	seen := map[string]bool{}
	for _, u := range active {
		addr, err := netip.ParseAddr(u.Address)
		if err != nil {
			return notFound("%q is not an upstream of route %s hop %d", u.Address, routeID, hopIndex)
		}
		id := netip.AddrPortFrom(addr.Unmap(), uint16(u.Port)).String() // #nosec G115 -- a port
		known := slices.ContainsFunc(hop.Upstreams, func(r relayctl.Upstream) bool {
			return r.Address == addr.Unmap().String() && r.Port == u.Port
		})
		if !known {
			return notFound("%s is not an upstream of route %s hop %d", id, routeID, hopIndex)
		}
		if seen[id] {
			return fmt.Errorf("%w: %s twice", driver.ErrInvalidArgument, id)
		}
		seen[id] = true
		rotation.Active = append(rotation.Active, relayctl.RotationEntry{Address: addr.Unmap().String(), Port: u.Port, Weight: u.Weight})
	}
	if len(active) == 0 {
		return fmt.Errorf("%w: an empty selection", driver.ErrInvalidArgument)
	}
	if err := d.client.SetRotation(ctx, rotation); err != nil {
		var e *relayctl.Error
		if errors.As(err, &e) {
			switch e.Code {
			case relayctl.CodeNotFound:
				return fmt.Errorf("%w: %s", driver.ErrNotFound, e.Message)
			case relayctl.CodeInvalid:
				return fmt.Errorf("%w: %s", driver.ErrInvalidArgument, e.Message)
			}
		}
		return fmt.Errorf("anixops driver: %w", err)
	}
	return nil
}

// ReloadCredentials makes the relay read the link files again, after the Agent
// replaced them on a renewal. No connection ends and no epoch changes: the
// relay swaps the certificate it presents and drops only the carriers whose
// peer is no longer trusted (section 3.5).
func (d *Driver) ReloadCredentials(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	st, err := d.answers(ctx)
	if err != nil {
		return err
	}
	if st == nil {
		return nil // not running: the next start reads the files
	}
	if err := d.client.ReloadCredentials(ctx); err != nil {
		return fmt.Errorf("anixops driver: %w", err)
	}
	return nil
}

// Remove stops the relay and deletes everything the driver owns (the
// configuration, the state file and the stateless reset key); it leaves the
// unit installed and foreign files alone, and is idempotent.
func (d *Driver) Remove(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	h, err := d.readHost()
	if err != nil {
		return err
	}
	if !h.present || !h.owned {
		return nil
	}
	if err := d.stop(ctx); err != nil {
		return err
	}
	return d.removeFiles()
}
