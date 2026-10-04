package gost

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/netip"
	"os"
	"slices"
	"strconv"
	"sync"
	"time"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/driver"
	"github.com/AnixOps/anix-control/sdk/forward/validate"
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
	// retired receives the last counters of the epochs a reload ends.
	retired func([]*forwardv1.Counters)
	now     func() time.Time
	client  *http.Client // the metrics socket
	api     *http.Client // the web API socket
	// apiFault, set by tests only (export_test.go), refuses a changing
	// web API request when it answers an error.
	apiFault func(method, path string) error

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

// WithRetiredCounters calls f after an Apply that reloaded or restarted a
// running gost, with every hop's counters read just before: a reload
// re-creates every service, so their counter epochs end there, and f
// gets their last values (the Agent reports them, as for the nftables
// driver's removed hops). Traffic between that read and the reload, and
// what connections that survive the reload move afterwards, is not
// counted.
func WithRetiredCounters(f func([]*forwardv1.Counters)) Option {
	return func(d *Driver) { d.retired = f }
}

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
	d.api = newAPIClient(cfg.apiPath())
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
// TLS, WSS, QUIC and gRPC links need the link certificate. The quota is
// the soft one (Config.SoftQuota, EnforceQuotas): gost has no byte quota.
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
		Quota:          d.cfg.SoftQuota,
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

// serves reports whether gost runs the configuration p as st recorded
// it, and answers the running configuration when it does: gost serves
// p's metrics path, or the one it loaded at the last start or reload when
// Apply changed services through the web API since (st.Served); it holds
// every listener of p; and it runs exactly p's services and hops. A web
// API that does not answer counts as not running p.
func (d *Driver) serves(ctx context.Context, st *hostState, p *parsed) (bool, *liveConfig, error) {
	path := d.live(ctx, p.metricsPath)
	if !path && st != nil && st.Served != "" && st.Served != p.metricsPath {
		path = d.live(ctx, st.Served)
	}
	if !path {
		return false, nil, ctx.Err()
	}
	socks, err := listSockets(ctx, d.runner)
	if err != nil {
		return false, nil, err
	}
	if !bound(socks, listenerSet(p.hops)) {
		return false, nil, nil
	}
	// gost starts its web API before it serves metrics: one request.
	var live liveConfig
	if err := d.call(ctx, http.MethodGet, "/config", nil, &live); err != nil {
		return false, nil, ctx.Err()
	}
	var want, got []string
	for _, h := range p.hops {
		want = append(want, h.Services...)
		if live.hop(hopName(h.key())) == nil {
			return false, nil, nil
		}
	}
	for _, s := range live.Services {
		got = append(got, s.Name)
	}
	slices.Sort(want)
	slices.Sort(got)
	if !slices.Equal(want, got) {
		return false, nil, nil
	}
	return true, &live, nil
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
//     content, and gost running it (serves). All equal is a no-op that at
//     most records the new generation and state_hash;
//  4. refuse a listener whose port a socket the driver does not run holds
//     (ErrConflict);
//  5. when gost runs the recorded configuration, its web API answers and
//     the artifact keeps the configuration's globals (log, API, metrics
//     address), change it through the web API object by object
//     (applyAPI): the services, chains, admissions and limiters that
//     changed are created, re-created or deleted, every hop is put, and
//     every other service runs on untouched;
//  6. otherwise write the configuration (temporary file and rename),
//     reload a running gost (restart it when it serves the artifact's
//     structure already, since a reload could not be told from none) or
//     start it, and wait until it serves the content;
//  7. when 5 fails, put the objects it changed back through the web API
//     (or, when that fails too, the previous configuration back with a
//     restart); when 6 fails, put the previous configuration back and
//     restart gost on it (or stop it after a first apply); and fail;
//  8. record the state.
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
		ok, _, err := d.serves(ctx, st, p)
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
	next := &hostState{Owner: OwnerMark, Node: a.NodeRef, Generation: a.Generation, StateHash: a.StateHash, Digest: a.Digest, Hops: p.hops}
	if h.owned {
		next.carry(h.state)
	} else {
		// Mark the directory before the first configuration lands in it.
		if err := d.writeState(&hostState{Owner: OwnerMark}); err != nil {
			return driver.ApplyResult{}, err
		}
	}

	if status.Running {
		prev, live := d.apiReady(ctx, h, a.Content)
		if err := ctx.Err(); err != nil {
			return driver.ApplyResult{}, err
		}
		if live != nil {
			if err := d.applyAPI(ctx, h, prev, a, next, live, status.Instance); err != nil {
				return driver.ApplyResult{}, err
			}
			res.Changed = true
			return res, nil
		}
	}

	var last []*forwardv1.Counters
	if d.retired != nil && status.Running && status.Instance != "" && h.state.applied() {
		if live, err := d.readLive(ctx); err == nil {
			last = d.hopCounters(h.state, live, status.Instance)
		}
	}
	if err := writeFileAtomic(d.cfg.configPath(), a.Content, 0o640); err != nil {
		return driver.ApplyResult{}, fmt.Errorf("gost driver: write configuration: %w", err)
	}
	started := !status.Running
	switch {
	case started:
		if !status.Starting {
			d.removeStaleSocket()
		}
		err = d.sup.Start(ctx)
	case d.live(ctx, p.metricsPath):
		// gost serves this structure already (an earlier apply was cut
		// short, or the recorded state is gone): a reload would not move
		// the metrics path, so whether it took could not be seen.
		started = true
		if err = d.sup.Stop(ctx); err == nil {
			d.removeStaleSocket()
			err = d.sup.Start(ctx)
		}
	default:
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
	next.Loads++
	next.Served = p.metricsPath
	if err := d.writeState(next); err != nil {
		return driver.ApplyResult{}, err
	}
	if len(last) > 0 {
		d.retired(last)
	}
	res.Changed = true
	return res, nil
}

// apiReady answers the previous configuration's objects and the running
// configuration when Apply may take gost to content through the web API:
// gost runs the configuration the driver recorded (the file is the
// applied one and serves finds it running) and content keeps its
// globals. live is nil otherwise, and Apply reloads.
func (d *Driver) apiReady(ctx context.Context, h *host, content []byte) (prev *gostConfig, live *liveConfig) {
	st := h.state
	if !h.owned || !st.applied() || h.config == nil || driver.Digest(h.config) != st.Digest {
		return nil, nil
	}
	pp, err := parseContent(driver.Artifact{Content: h.config, Hops: keysOf(st.Hops)})
	if err != nil {
		return nil, nil
	}
	prev, err = decodeConfig(h.config)
	if err != nil {
		return nil, nil
	}
	next, err := decodeConfig(content)
	if err != nil {
		return nil, nil
	}
	g1, err1 := globals(prev)
	g2, err2 := globals(next)
	if err1 != nil || err2 != nil || !bytes.Equal(g1, g2) {
		return nil, nil
	}
	ok, live, err := d.serves(ctx, st, pp)
	if !ok || err != nil {
		return nil, nil
	}
	if st.Served == "" {
		// A state recorded before F4c: gost serves the file's path.
		st.Served = pp.metricsPath
	}
	return prev, live
}

// applyAPI takes the running gost, which runs the recorded configuration
// prev (live is its running state), to the artifact a through the web
// API, and records next (sync.go): it writes the configuration first, so
// a restart loads it; it creates, re-creates and deletes the services,
// chains, admissions and limiters that changed and puts every hop. Every
// other service keeps its listener, connections, UDP sessions, mux
// carriers and statistics. The hops whose services it deleted or created
// start a new counter epoch, and the WithRetiredCounters hook gets their
// counters read just before.
//
// When gost refuses a change, applyAPI puts the objects it changed back
// as prev has them, and the previous configuration file; when that fails
// too, it restarts gost on the previous configuration. Either way it
// fails, and the state records the previous artifact.
func (d *Driver) applyAPI(ctx context.Context, h *host, prev *gostConfig, a driver.Artifact, next *hostState, live *liveConfig, instance string) error {
	nextCfg, err := decodeConfig(a.Content)
	if err != nil {
		return err
	}
	from, err := objects(prev)
	if err != nil {
		return err
	}
	to, err := objects(nextCfg)
	if err != nil {
		return err
	}
	// hopOf maps every service of either configuration to its hop.
	hopOf := map[string]string{}
	for _, hops := range [][]manifestHop{h.state.Hops, next.Hops} {
		for _, mh := range hops {
			for _, s := range mh.Services {
				hopOf[s] = hopName(mh.key())
			}
		}
	}
	var before []*forwardv1.Counters
	if d.retired != nil {
		before = d.hopCounters(h.state, live, instance)
	}
	hopsOf := func(services map[string]bool) map[string]bool {
		out := map[string]bool{}
		for s := range services {
			if hop, ok := hopOf[s]; ok {
				out[hop] = true
			}
		}
		return out
	}
	// retire hands the counters read before the first change of the
	// recorded hops whose epochs end to the hook.
	retire := func(services map[string]bool) {
		hops := hopsOf(services)
		if d.retired == nil || len(hops) == 0 {
			return
		}
		var out []*forwardv1.Counters
		for i, mh := range h.state.Hops {
			if hops[hopName(mh.key())] {
				out = append(out, before[i])
			}
		}
		if len(out) > 0 {
			d.retired(out)
		}
	}

	if err := writeFileAtomic(d.cfg.configPath(), a.Content, 0o640); err != nil {
		return fmt.Errorf("gost driver: write configuration: %w", err)
	}
	log := newSyncLog()
	err = d.sync(ctx, to, changes(from, to), live.names(), log)
	if err == nil {
		next.Served = h.state.Served
		next.recreated(hopsOf(log.services))
		if err := d.writeState(next); err != nil {
			return err
		}
		retire(log.services)
		return nil
	}
	err = fmt.Errorf("gost driver: apply generation %d: %w", a.Generation, err)

	// Roll back, whatever the caller's context says, bounded.
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), recoverTimeout)
	defer cancel()
	back := newSyncLog()
	rerr := d.rollback(rctx, from, log, back)
	if rerr == nil {
		rerr = writeFileAtomic(d.cfg.configPath(), h.config, 0o640)
	}
	if rerr != nil {
		return errors.Join(err, rerr, d.recoverPrevious(ctx, h))
	}
	services := log.services
	for s := range back.services {
		services[s] = true
	}
	kept := *h.state
	kept.recreated(hopsOf(services))
	if werr := d.writeState(&kept); werr != nil {
		return errors.Join(err, werr)
	}
	retire(services)
	return err
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

// removeStaleSocket deletes the metrics and API sockets a gost that was
// killed left behind, which would stop the next gost from listening on
// them. gost is not running when it is called. Under systemd the unit's
// RuntimeDirectory goes with the process, and the Agent may lack the
// right to delete in it.
func (d *Driver) removeStaleSocket() {
	_ = os.Remove(d.cfg.metricsPath())
	_ = os.Remove(d.cfg.apiPath())
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

// Observe reads the recorded state, gost's status and, when gost runs,
// its running configuration through the web API: the applied identity,
// one Counters per hop and the rotation.
//
// Counters are the sums of the hop's services' statistics: up_bytes what
// they read from their clients, down_bytes what they wrote back (payload,
// or the carrier's bytes on an encrypted or multiplexed ingress),
// total_conns and active_conns the accepted connections (UDP sessions
// and mux streams are not connections). gost counts no packets: they are
// 0. The counter epoch names the services' statistics objects: the gost
// instance, the starts and reloads Apply made, and the services' creation
// times. A start, a reload or the re-creation of a service ends it; a hot
// change (applyHot, SetUpstreams, EnforceQuotas) keeps it. While gost
// does not run, every hop reports 0 in the epoch "stopped".
//
// The rotation is the one gost runs: a SetUpstreams selection stays in
// the hop's metadata until a start, a reload or a changing Apply puts the
// rendered upstreams back. gost's own fail marking is not exposed, so
// Health stays empty and the Agent's checks are the source of truth.
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
	var live *liveConfig
	if status.Running && status.Instance != "" {
		if live, err = d.readLive(ctx); err != nil {
			return driver.Observation{}, err
		}
	}
	st := h.state
	o.Applied, o.NodeRef, o.Generation, o.StateHash, o.Digest = true, st.Node, st.Generation, st.StateHash, st.Digest
	if live != nil {
		o.Counters = d.hopCounters(st, live, status.Instance)
	}
	for i, mh := range st.Hops {
		if live == nil {
			o.Counters = append(o.Counters, &forwardv1.Counters{
				RouteId: mh.Route, HopIndex: mh.Hop, NodeRef: st.Node,
				CounterEpoch: "stopped", ObservedAtUnixMs: now.UnixMilli(),
			})
		} else {
			o.Counters[i].ObservedAtUnixMs = now.UnixMilli()
		}
		active := renderedRotation(mh)
		if live != nil {
			if r, ok := liveRotation(live.hop(hopName(mh.key()))); ok {
				active = r
			}
		}
		o.Rotation = append(o.Rotation, driver.HopRotation{RouteID: mh.Route, HopIndex: mh.Hop, Active: active})
	}
	return o, nil
}

// hopCounters answers the counters of every recorded hop from the running
// configuration: the sums of each hop's services' statistics, in the
// epoch of their statistics objects.
func (d *Driver) hopCounters(st *hostState, live *liveConfig, instance string) []*forwardv1.Counters {
	now := d.now().UnixMilli()
	out := make([]*forwardv1.Counters, 0, len(st.Hops))
	for _, mh := range st.Hops {
		c := &forwardv1.Counters{RouteId: mh.Route, HopIndex: mh.Hop, NodeRef: st.Node, ObservedAtUnixMs: now}
		var active uint64
		created := make([]int64, 0, len(mh.Services))
		for _, name := range mh.Services {
			ls := live.service(name)
			if ls == nil || ls.Status == nil {
				created = append(created, -1)
				continue
			}
			created = append(created, ls.Status.CreateTime)
			if x := ls.Status.Stats; x != nil {
				c.UpBytes += x.InputBytes
				c.DownBytes += x.OutputBytes
				c.TotalConns += x.TotalConns
				active += x.CurrentConns
			}
		}
		c.ActiveConns = uint32(min(active, math.MaxUint32)) // #nosec G115 -- clamped
		c.CounterEpoch = counterEpoch(instance, st.Loads, st.Created[hopName(mh.key())], created)
		out = append(out, c)
	}
	return out
}

// counterEpoch names the statistics objects of a hop's services: they
// start from zero when gost starts (a new instance), when Apply reloads it
// (loads), when Apply deletes or creates one of the hop's services through
// the web API (seq, hostState.Created) and when a service is re-created
// otherwise (its creation time; -1 for a service gost does not run).
func counterEpoch(instance string, loads, seq uint64, created []int64) string {
	sum := sha256.New()
	_, _ = fmt.Fprintf(sum, "%s\n%d", instance, loads)
	if seq > 0 {
		_, _ = fmt.Fprintf(sum, "\ns%d", seq)
	}
	for _, c := range created {
		_, _ = fmt.Fprintf(sum, "\n%d", c)
	}
	return "g" + hex.EncodeToString(sum.Sum(nil)[:8])
}

// rotationKey is the hop metadata in which SetUpstreams records the
// selection it put in rotation (gost ignores metadata it does not know).
const rotationKey = "anixopsRotation"

// renderedRotation answers every rendered upstream of a hop with its
// rendered weight.
func renderedRotation(mh manifestHop) []driver.Upstream {
	out := make([]driver.Upstream, 0, len(mh.Upstreams))
	for _, u := range mh.Upstreams {
		out = append(out, driver.Upstream{Address: u.Address, Port: u.Port, Weight: u.Weight})
	}
	return out
}

// liveRotation reads the selection SetUpstreams recorded in a running
// hop; ok is false when the hop runs its rendered upstreams.
func liveRotation(h *liveHop) ([]driver.Upstream, bool) {
	if h == nil {
		return nil, false
	}
	raw, ok := h.Metadata[rotationKey].(string)
	if !ok {
		return nil, false
	}
	var us []driver.Upstream
	if json.Unmarshal([]byte(raw), &us) != nil || len(us) == 0 {
		return nil, false
	}
	return us, true
}

// SetUpstreams puts a selection of an applied hop's rendered upstreams in
// rotation, with weights (0 keeps the rendered one), by replacing the
// running gost's hop through the web API: no service is re-created and the
// configuration file, the recorded state, the digest and the counter
// epochs stay. The selection lives in gost (recorded in the hop's
// metadata, so a restarted Agent observes it) until gost starts or reloads
// or a changing Apply puts every rendered upstream back. Selecting every
// upstream with its rendered weight runs exactly the rendered hop.
func (d *Driver) SetUpstreams(ctx context.Context, routeID string, hopIndex uint32, active []driver.Upstream) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(active) == 0 {
		return fmt.Errorf("%w: no upstream selected", driver.ErrInvalidArgument)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
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
	weights := map[string]uint32{}
	for _, u := range active {
		addr, err := parseLiteral(u.Address)
		key := ""
		if err == nil {
			key = fmt.Sprintf("%s|%d", addr, u.Port)
		}
		if err != nil || !slices.ContainsFunc(mh.Upstreams, func(r manifestUpstream) bool { return r.Address == addr.String() && r.Port == u.Port }) {
			return fmt.Errorf("%w: upstream %s:%d is not one hop %s was rendered with", driver.ErrNotFound, u.Address, u.Port, k)
		}
		if _, dup := weights[key]; dup {
			return fmt.Errorf("%w: upstream %s:%d selected twice", driver.ErrInvalidArgument, u.Address, u.Port)
		}
		if u.Weight > validate.MaxWeight {
			return fmt.Errorf("%w: weight %d of %s:%d over %d", driver.ErrInvalidArgument, u.Weight, u.Address, u.Port, validate.MaxWeight)
		}
		weights[key] = u.Weight
	}
	body, err := d.selectionHop(h, mh, weights)
	if err != nil {
		return err
	}
	status, err := d.sup.Status(ctx)
	if err != nil {
		return err
	}
	if !status.Running {
		return fmt.Errorf("%w: hop %s keeps its rendered upstreams", errNotRunning, k)
	}
	return d.put(ctx, "hops", body.Name, body)
}

// selectionHop answers the running hop that puts the selected upstreams
// (rendered address and port to weight, 0 for the rendered weight) of mh
// in rotation: the applied configuration's hop with its nodes rebuilt
// for the selection, and the selection in its metadata unless it is
// every rendered upstream with its rendered weight.
func (d *Driver) selectionHop(h *host, mh *manifestHop, weights map[string]uint32) (apiHop, error) {
	cfg, err := decodeConfig(h.config)
	if err != nil {
		return apiHop{}, err
	}
	rendered := cfg.hop(hopName(mh.key()))
	if rendered == nil {
		return apiHop{}, fmt.Errorf("%w: the applied configuration has no hop %s", driver.ErrInvalidArtifact, hopName(mh.key()))
	}
	balance := forwardv1.BalanceStrategy(forwardv1.BalanceStrategy_value[mh.Balance])
	var ups []nodeUpstream
	var rotation []driver.Upstream
	same := len(weights) == len(mh.Upstreams)
	for i, u := range mh.Upstreams {
		w, ok := weights[fmt.Sprintf("%s|%d", u.Address, u.Port)]
		if !ok {
			continue
		}
		if w == 0 {
			w = u.Weight
		}
		same = same && w == u.Weight
		a, err := netip.ParseAddr(u.Address)
		if err != nil || u.Port == 0 || u.Port > 65535 {
			return apiHop{}, fmt.Errorf("%w: recorded upstream %s:%d of %s", driver.ErrInvalidArtifact, u.Address, u.Port, mh.key())
		}
		nu := nodeUpstream{index: i, addr: netip.AddrPortFrom(a, uint16(u.Port)).String(), weight: w, priority: u.Priority} // #nosec G115 -- checked
		if tmpl := templateNode(rendered.Nodes, i); tmpl != nil {
			nu.connector, nu.dialer = tmpl.Connector, tmpl.Dialer
		}
		ups = append(ups, nu)
		rotation = append(rotation, driver.Upstream{Address: u.Address, Port: u.Port, Weight: w})
	}
	sel := hopConfig{Name: rendered.Name, Selector: rendered.Selector, Nodes: buildNodes(balance, ups)}
	if !same {
		b, err := json.Marshal(rotation)
		if err != nil {
			return apiHop{}, err
		}
		sel.Metadata = map[string]string{rotationKey: string(b)}
	}
	return toAPIHop(sel)
}

// templateNode answers the rendered node of upstream i (u<i>, or its
// first entry u<i>-0), whose connector and dialer the selection keeps.
func templateNode(nodes []node, i int) *node {
	name := "u" + strconv.Itoa(i)
	for j := range nodes {
		if nodes[j].Name == name || nodes[j].Name == name+"-0" {
			return &nodes[j]
		}
	}
	return nil
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
