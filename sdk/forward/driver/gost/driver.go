package gost

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"slices"
	"sync"
	"time"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/driver"
)

// startSettle is how long Apply waits after gost started serving before
// it may send it SIGHUP: gost arms its reload handler just after its
// services are up, and a SIGHUP before that would end the process.
const startSettle = 250 * time.Millisecond

// recoverTimeout bounds putting the previous configuration back after a
// failed apply, whatever the caller's context says.
const recoverTimeout = 30 * time.Second

// Driver is the gost forward driver. It keeps no state besides its
// configuration: what it applied is read back from its directory and from
// the gost process. Apply, SetUpstreams and Remove are serialised; Observe
// runs between them.
type Driver struct {
	cfg    Config
	runner Runner
	sup    Supervisor
	now    func() time.Time
	client *http.Client

	mu sync.RWMutex
}

var _ driver.Driver = (*Driver)(nil)

// Option configures New.
type Option func(*Driver)

// WithRunner runs ss (and the SystemdSupervisor's systemctl) through r
// instead of ExecRunner (tests, a network namespace).
func WithRunner(r Runner) Option { return func(d *Driver) { d.runner = r } }

// WithSupervisor runs gost through s instead of a SystemdSupervisor on
// the driver's runner.
func WithSupervisor(s Supervisor) Option { return func(d *Driver) { d.sup = s } }

// New answers a driver with the given configuration, or ErrInvalidConfig.
// The configuration usually comes from Probe.
func New(cfg Config, opts ...Option) (*Driver, error) {
	if err := cfg.check(); err != nil {
		return nil, err
	}
	cfg.Strategies = slices.Clone(cfg.Strategies)
	d := &Driver{cfg: cfg, runner: ExecRunner{}, now: time.Now}
	for _, o := range opts {
		o(d)
	}
	if d.sup == nil {
		d.sup = SystemdSupervisor{Runner: d.runner}
	}
	sock := cfg.metricsPath()
	d.client = &http.Client{
		Timeout: 2 * time.Second,
		Transport: &http.Transport{
			Proxy:             nil,
			DisableKeepAlives: true,
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var dl net.Dialer
				return dl.DialContext(ctx, "unix", sock)
			},
		},
	}
	return d, nil
}

// Engine answers ENGINE_GOST.
func (d *Driver) Engine() forwardv1.Engine { return forwardv1.Engine_ENGINE_GOST }

// Config answers a copy of the driver's configuration.
func (d *Driver) Config() Config {
	c := d.cfg
	c.Strategies = slices.Clone(c.Strategies)
	return c
}

// Capabilities answers the capabilities of the configuration, which Probe
// filled from the host: a configuration without a Version is unavailable.
// TLS, WSS, QUIC and gRPC links need the link certificate; gost has no
// byte quota.
func (d *Driver) Capabilities(ctx context.Context) (*forwardv1.EngineCapabilities, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c := &forwardv1.EngineCapabilities{
		Engine:         forwardv1.Engine_ENGINE_GOST,
		Version:        d.cfg.Version,
		Available:      d.cfg.Version != "",
		Ipv6:           d.cfg.IPv6,
		Udp:            d.cfg.UDP,
		Strategies:     slices.Clone(d.cfg.Strategies),
		LinkSecurities: []forwardv1.LinkSecurity{forwardv1.LinkSecurity_LINK_SECURITY_RAW},
		BandwidthLimit: d.cfg.BandwidthLimit,
		MaxConns:       d.cfg.MaxConns,
	}
	if d.cfg.linkTLS() {
		c.LinkSecurities = append(c.LinkSecurities,
			forwardv1.LinkSecurity_LINK_SECURITY_TLS, forwardv1.LinkSecurity_LINK_SECURITY_WSS,
			forwardv1.LinkSecurity_LINK_SECURITY_QUIC, forwardv1.LinkSecurity_LINK_SECURITY_GRPC)
	}
	slices.Sort(c.Strategies)
	if !c.Available {
		c.UnavailableReason = d.cfg.Unavailable
		if c.UnavailableReason == "" {
			c.UnavailableReason = "gost host not probed: the configuration has no version (gost.Probe fills it)"
		}
	}
	return c, nil
}

// live reports whether gost serves the configuration whose metrics path is
// path: the path answers only once a start or reload loaded it.
func (d *Driver) live(ctx context.Context, path string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://gost"+path, nil)
	if err != nil {
		return false
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<20))
	return resp.StatusCode == http.StatusOK
}

// runs reports whether the host runs a configuration: gost serves its
// metrics path and holds every listener.
func (d *Driver) runs(ctx context.Context, p *parsed) (bool, error) {
	if !d.live(ctx, p.metricsPath) {
		return false, ctx.Err()
	}
	socks, err := listSockets(ctx, d.runner)
	if err != nil {
		return false, err
	}
	return bound(socks, listenerSet(p.hops)), nil
}

// waitServing waits until gost serves p, or fails: gost stopped, or the
// ready timeout passed.
func (d *Driver) waitServing(ctx context.Context, p *parsed) error {
	deadline := time.Now().Add(d.cfg.ReadyTimeout)
	for {
		ok, err := d.runs(ctx, p)
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
		st, err := d.sup.Status(ctx)
		if err != nil {
			return err
		}
		if !st.Running && !st.Starting {
			return errors.New("gost driver: gost exited instead of serving the configuration")
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("gost driver: gost did not serve the configuration within %v", d.cfg.ReadyTimeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// status answers the supervisor's status once gost is not starting: a
// start in progress (systemd restarting a crashed gost) is waited for, at
// most ReadyTimeout, so Apply then reloads a running gost or starts a
// stopped one.
func (d *Driver) status(ctx context.Context) (Status, error) {
	deadline := time.Now().Add(d.cfg.ReadyTimeout)
	for {
		st, err := d.sup.Status(ctx)
		if err != nil || !st.Starting || time.Now().After(deadline) {
			return st, err
		}
		select {
		case <-ctx.Done():
			return Status{}, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// Apply makes the host run the artifact (driver package documentation):
//
//  1. read the directory; files of the driver's name without its state
//     file are ErrNotOwned, and so is a unit without OwnerMark;
//  2. check the generation against the recorded one;
//  3. compare: the recorded digest, the configuration file with the
//     content, and gost serving the content's metrics path and holding
//     every listener. All equal is a no-op that at most records the new
//     generation and state_hash;
//  4. refuse a listener whose port a socket the driver does not run holds
//     (ErrConflict);
//  5. write the configuration (temporary file and rename), reload a
//     running gost or start it, and wait until it serves the content;
//  6. when it does not, put the previous configuration back and restart
//     gost on it (or stop it after a first apply), and fail;
//  7. record the state.
func (d *Driver) Apply(ctx context.Context, a driver.Artifact) (driver.ApplyResult, error) {
	if err := ctx.Err(); err != nil {
		return driver.ApplyResult{}, err
	}
	if err := a.Verify(d.Engine()); err != nil {
		return driver.ApplyResult{}, err
	}
	p, err := parseContent(a)
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
	if err := d.sup.Check(ctx); err != nil {
		return driver.ApplyResult{}, err
	}
	if st := h.state; st.applied() {
		switch {
		case a.Generation < st.Generation:
			return driver.ApplyResult{}, fmt.Errorf("%w: artifact generation %d, host runs %d", driver.ErrStaleGeneration, a.Generation, st.Generation)
		case a.Generation == st.Generation && a.Digest != st.Digest:
			return driver.ApplyResult{}, fmt.Errorf("%w: generation %d", driver.ErrGenerationConflict, a.Generation)
		}
	}
	res := driver.ApplyResult{Generation: a.Generation, StateHash: a.StateHash, Digest: a.Digest}
	if a.Empty() {
		return d.applyEmpty(ctx, h, res)
	}
	status, err := d.status(ctx)
	if err != nil {
		return driver.ApplyResult{}, err
	}

	if st := h.state; st.applied() && st.Digest == a.Digest && bytes.Equal(h.config, a.Content) && status.Running {
		ok, err := d.runs(ctx, p)
		if err != nil {
			return driver.ApplyResult{}, err
		}
		if ok {
			if st.Generation != a.Generation || st.StateHash != a.StateHash || st.Node != a.NodeRef {
				next := *st
				next.Generation, next.StateHash, next.Node = a.Generation, a.StateHash, a.NodeRef
				if err := d.writeState(&next); err != nil {
					return driver.ApplyResult{}, err
				}
			}
			return res, nil
		}
	}

	socks, err := listSockets(ctx, d.runner)
	if err != nil {
		return driver.ApplyResult{}, err
	}
	var ours []manifestListener
	if (status.Running || status.Starting) && h.state.applied() {
		ours = listenerSet(h.state.Hops)
	}
	if err := conflicts(socks, listenerSet(p.hops), ours); err != nil {
		return driver.ApplyResult{}, err
	}

	if err := os.MkdirAll(d.cfg.Dir, 0o750); err != nil {
		return driver.ApplyResult{}, fmt.Errorf("gost driver: %w", err)
	}
	if !h.owned {
		// Mark the directory before the first configuration lands in it.
		if err := d.writeState(&hostState{Owner: OwnerMark}); err != nil {
			return driver.ApplyResult{}, err
		}
	}
	if err := writeFileAtomic(d.cfg.configPath(), a.Content, 0o640); err != nil {
		return driver.ApplyResult{}, fmt.Errorf("gost driver: write configuration: %w", err)
	}
	started := !status.Running
	if started {
		if !status.Starting {
			d.removeStaleSocket()
		}
		err = d.sup.Start(ctx)
	} else {
		err = d.sup.Reload(ctx)
	}
	if err == nil {
		err = d.waitServing(ctx, p)
	}
	if err != nil {
		return driver.ApplyResult{}, errors.Join(fmt.Errorf("gost driver: apply generation %d: %w", a.Generation, err), d.recoverPrevious(ctx, h))
	}
	if started {
		time.Sleep(startSettle)
	}
	if err := d.writeState(&hostState{Owner: OwnerMark, Node: a.NodeRef, Generation: a.Generation, StateHash: a.StateHash, Digest: a.Digest, Hops: p.hops}); err != nil {
		return driver.ApplyResult{}, err
	}
	res.Changed = true
	return res, nil
}

// recoverPrevious puts the configuration h held back after a failed
// apply: gost restarts on it (a failed reload may leave gost with some of
// its services closed), or stops when there was none. The caller's
// context may be done; recovery runs anyway, bounded.
func (d *Driver) recoverPrevious(ctx context.Context, h *host) error {
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), recoverTimeout)
	defer cancel()
	if !h.state.applied() || h.config == nil {
		// Nothing ran before: stop gost and leave the files as they were
		// (none, when the driver did not own the directory yet).
		if err := d.sup.Stop(rctx); err != nil {
			return err
		}
		if !h.owned {
			return d.removeFiles()
		}
		if h.config == nil {
			if err := os.Remove(d.cfg.configPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("gost driver: recover: %w", err)
			}
		} else if err := writeFileAtomic(d.cfg.configPath(), h.config, 0o640); err != nil {
			return fmt.Errorf("gost driver: recover: %w", err)
		}
		return nil
	}
	if err := writeFileAtomic(d.cfg.configPath(), h.config, 0o640); err != nil {
		return fmt.Errorf("gost driver: recover: %w", err)
	}
	if err := d.sup.Stop(rctx); err != nil {
		return fmt.Errorf("gost driver: recover: %w", err)
	}
	d.removeStaleSocket()
	if err := d.sup.Start(rctx); err != nil {
		return fmt.Errorf("gost driver: recover: %w", err)
	}
	prev, err := parseContent(driver.Artifact{Content: h.config, Hops: keysOf(h.state.Hops)})
	if err != nil {
		return fmt.Errorf("gost driver: recover: %w", err)
	}
	if err := d.waitServing(rctx, prev); err != nil {
		return fmt.Errorf("gost driver: recover the previous configuration: %w", err)
	}
	time.Sleep(startSettle)
	return nil
}

// removeStaleSocket deletes the metrics socket a gost that was killed left
// behind, which would stop the next gost from listening on it. gost is not
// running when it is called. Under systemd the unit's RuntimeDirectory goes
// with the process, and the Agent may lack the right to delete in it.
func (d *Driver) removeStaleSocket() {
	_ = os.Remove(d.cfg.metricsPath())
}

func keysOf(hops []manifestHop) []driver.HopKey {
	out := make([]driver.HopKey, len(hops))
	for i, h := range hops {
		out[i] = h.key()
	}
	return out
}

// applyEmpty stops gost and removes the configuration and the state.
func (d *Driver) applyEmpty(ctx context.Context, h *host, res driver.ApplyResult) (driver.ApplyResult, error) {
	status, err := d.sup.Status(ctx)
	if err != nil {
		return driver.ApplyResult{}, err
	}
	if !h.present && !status.Running && !status.Starting {
		return res, nil
	}
	if err := d.sup.Stop(ctx); err != nil {
		return driver.ApplyResult{}, err
	}
	if err := d.removeFiles(); err != nil {
		return driver.ApplyResult{}, err
	}
	res.Changed = true
	return res, nil
}

// Observe reads the recorded state and gost's status: the applied
// identity, one Counters per hop and the rotation. Counter values are 0
// until F4b reads them from gost's metrics; the counter epoch is the gost
// instance, which a reload keeps and a restart ends.
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
	now := d.now()
	o := driver.Observation{Engine: d.Engine(), ObservedAt: now}
	if !h.owned || !h.state.applied() {
		return o, nil
	}
	status, err := d.sup.Status(ctx)
	if err != nil {
		return driver.Observation{}, err
	}
	epoch := "stopped"
	if status.Running && status.Instance != "" {
		epoch = status.Instance
	}
	st := h.state
	o.Applied, o.NodeRef, o.Generation, o.StateHash, o.Digest = true, st.Node, st.Generation, st.StateHash, st.Digest
	for _, mh := range st.Hops {
		o.Counters = append(o.Counters, &forwardv1.Counters{
			RouteId: mh.Route, HopIndex: mh.Hop, NodeRef: st.Node,
			CounterEpoch: epoch, ObservedAtUnixMs: now.UnixMilli(),
		})
		active := make([]driver.Upstream, 0, len(mh.Upstreams))
		for _, u := range mh.Upstreams {
			active = append(active, driver.Upstream{Address: u.Address, Port: u.Port, Weight: u.Weight})
		}
		o.Rotation = append(o.Rotation, driver.HopRotation{RouteID: mh.Route, HopIndex: mh.Hop, Active: active})
	}
	return o, nil
}

// SetUpstreams checks the selection against the applied hop's rendered
// upstreams and then refuses: changing gost's nodes without a reload goes
// through gost's web API, which F4b adds.
func (d *Driver) SetUpstreams(ctx context.Context, routeID string, hopIndex uint32, active []driver.Upstream) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(active) == 0 {
		return fmt.Errorf("%w: no upstream selected", driver.ErrInvalidArgument)
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	h, err := d.readHost()
	if err != nil {
		return err
	}
	k := driver.HopKey{RouteID: routeID, HopIndex: hopIndex}
	if !h.owned || !h.state.applied() {
		return fmt.Errorf("%w: hop %s (nothing applied)", driver.ErrNotFound, k)
	}
	mh := h.state.hop(k)
	if mh == nil {
		return fmt.Errorf("%w: hop %s", driver.ErrNotFound, k)
	}
	seen := map[string]bool{}
	for _, u := range active {
		addr, err := parseLiteral(u.Address)
		key := ""
		if err == nil {
			key = fmt.Sprintf("%s|%d", addr, u.Port)
		}
		if err != nil || !slices.ContainsFunc(mh.Upstreams, func(r manifestUpstream) bool { return r.Address == addr.String() && r.Port == u.Port }) {
			return fmt.Errorf("%w: upstream %s:%d is not one hop %s was rendered with", driver.ErrNotFound, u.Address, u.Port, k)
		}
		if seen[key] {
			return fmt.Errorf("%w: upstream %s:%d selected twice", driver.ErrInvalidArgument, u.Address, u.Port)
		}
		seen[key] = true
	}
	return fmt.Errorf("%w: changing a gost hop's upstreams without an apply needs gost's web API (F4b)", driver.ErrUnsupported)
}

// Remove stops gost and deletes the configuration and the state file when
// they are the driver's. Someone else's files, or a unit without
// OwnerMark, are left alone.
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
	if h.present && !h.owned {
		return nil
	}
	switch err := d.sup.Check(ctx); {
	case errors.Is(err, driver.ErrNotOwned):
		return nil // someone else's unit: never stopped
	case errors.Is(err, ErrNoUnit):
		// Without the unit nothing runs gost: delete the files only.
	case err != nil:
		return err
	default:
		if err := d.sup.Stop(ctx); err != nil {
			return err
		}
	}
	return d.removeFiles()
}
