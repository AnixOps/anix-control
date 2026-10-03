package fake

import (
	"context"
	"fmt"
	"maps"
	"net/netip"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/driver"
)

// Op names a driver call, for fault injection.
type Op string

// The driver calls a fault can be injected into.
const (
	OpCapabilities Op = "capabilities"
	OpApply        Op = "apply"
	OpObserve      Op = "observe"
	OpSetUpstreams Op = "set-upstreams"
	OpRemove       Op = "remove"
)

// Traffic is simulated traffic through one hop, added to its counters.
type Traffic struct {
	UpBytes, DownBytes     uint64
	UpPackets, DownPackets uint64
	// NewConns adds to total_conns; ActiveConns replaces active_conns.
	NewConns    uint64
	ActiveConns uint32
}

// Host is the simulated machine a fake Driver runs on: its owned objects,
// the objects that belong to others, and the injected faults. Several
// Driver instances may share one Host; a new instance on the same Host is
// an Agent restart. Every method is safe for concurrent use.
type Host struct {
	mu       sync.Mutex
	now      func() time.Time
	epochSeq uint64
	owned    *owned
	impostor bool
	foreign  map[string]string
	ports    map[uint32]string // listen ports held by foreign objects
	faults   map[Op][]error
	applies  int
	conns    map[netip.AddrPort]uint64 // what ActiveConns answers
}

// owned is the state the driver owns on the host: what a real driver would
// find by listing its table or reading its gost config.
type owned struct {
	nodeRef    string
	generation uint64
	stateHash  string
	digest     string
	hops       map[driver.HopKey]*hopObject
}

// hopObject is one hop's set of objects (rules, maps, counters).
type hopObject struct {
	spec        *forwardv1.NodeHop
	epoch       string
	upBytes     uint64
	downBytes   uint64
	upPackets   uint64
	downPackets uint64
	activeConns uint32
	totalConns  uint64
	rotation    []driver.Upstream
	health      map[upKey]*forwardv1.UpstreamHealth
}

type upKey struct {
	address string
	port    uint32
}

// NewHost answers an empty host. now is the clock (time.Now when nil).
func NewHost(now func() time.Time) *Host {
	if now == nil {
		now = time.Now
	}
	return &Host{
		now:     now,
		foreign: map[string]string{},
		ports:   map[uint32]string{},
		faults:  map[Op][]error{},
	}
}

// FailNext makes the next call of op fail with err (queued: several calls
// fail in order). A failed Apply changes nothing, as a real atomic apply.
func (h *Host) FailNext(op Op, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.faults[op] = append(h.faults[op], err)
}

func (h *Host) takeFault(op Op) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	q := h.faults[op]
	if len(q) == 0 {
		return nil
	}
	h.faults[op] = q[1:]
	return q[0]
}

// PlantForeign adds an object the driver does not own (another table, a
// service). The driver must never change it.
func (h *Host) PlantForeign(name, value string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.foreign[name] = value
}

// PlantConflict makes a foreign object hold a listen port.
func (h *Host) PlantConflict(port uint32, holder string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.ports[port] = holder
}

// PlantImpostor creates an object with the driver's name but without its
// ownership mark. It panics when the driver owns objects already, which a
// real host cannot reach either.
func (h *Host) PlantImpostor() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.owned != nil {
		panic("fake: impostor planted over owned objects")
	}
	h.impostor = true
}

// Foreign lists the objects the driver does not own, sorted.
func (h *Host) Foreign() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []string
	for k, v := range h.foreign {
		out = append(out, fmt.Sprintf("object %s=%s", k, v))
	}
	for p, v := range h.ports {
		out = append(out, fmt.Sprintf("port %d held by %s", p, v))
	}
	if h.impostor {
		out = append(out, "impostor table")
	}
	sort.Strings(out)
	return out
}

// Owned lists the driver's objects, sorted: the applied identity, every
// hop's spec, and every hop's rotation. Counter values and epochs are not
// listed, so two hosts that run the same artifact list the same objects.
func (h *Host) Owned() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.owned == nil {
		return nil
	}
	o := h.owned
	out := []string{fmt.Sprintf("state node=%s generation=%d hash=%s digest=%s", o.nodeRef, o.generation, o.stateHash, o.digest)}
	for k, ho := range o.hops {
		b, _ := proto.MarshalOptions{Deterministic: true}.Marshal(ho.spec)
		out = append(out, fmt.Sprintf("hop %s spec=%s", k, driver.Digest(b)))
		out = append(out, fmt.Sprintf("rotation %s %s", k, formatRotation(ho.rotation)))
	}
	sort.Strings(out)
	return out
}

// Applies counts the applies that changed the host.
func (h *Host) Applies() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.applies
}

// Damage simulates partial state, as a crash halfway through a
// non-atomic apply or an operator deleting one object leaves it: the
// owned hop that sorts first loses its objects, counters included. It
// reports whether there was anything to damage.
func (h *Host) Damage() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.owned == nil || len(h.owned.hops) == 0 {
		return false
	}
	keys := sortedKeys(h.owned.hops)
	delete(h.owned.hops, keys[0])
	return true
}

// AddTraffic simulates traffic through an applied hop.
func (h *Host) AddTraffic(key driver.HopKey, t Traffic) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	ho, err := h.hopLocked(key)
	if err != nil {
		return err
	}
	ho.upBytes += t.UpBytes
	ho.downBytes += t.DownBytes
	ho.upPackets += t.UpPackets
	ho.downPackets += t.DownPackets
	ho.totalConns += t.NewConns
	ho.activeConns = t.ActiveConns
	return nil
}

// SetUpstreamConns sets the live connections per upstream address and
// port that ActiveConns answers, as an engine that counts them would
// (least-connections re-weighting, sdk/forward/leastconn).
func (h *Host) SetUpstreamConns(conns map[netip.AddrPort]uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.conns = maps.Clone(conns)
}

// SetHealth records the engine's own view of one upstream of an applied
// hop, as Observe will answer it.
func (h *Host) SetHealth(key driver.HopKey, health *forwardv1.UpstreamHealth) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	ho, err := h.hopLocked(key)
	if err != nil {
		return err
	}
	k := upKey{health.GetAddress(), health.GetPort()}
	if !slices.ContainsFunc(ho.spec.GetUpstreams(), func(u *forwardv1.Upstream) bool { return upKey{u.GetAddress(), u.GetPort()} == k }) {
		return fmt.Errorf("%w: upstream %s:%d of %s", driver.ErrNotFound, k.address, k.port, key)
	}
	c := proto.Clone(health).(*forwardv1.UpstreamHealth)
	c.RouteId, c.HopIndex = key.RouteID, key.HopIndex
	ho.health[k] = c
	return nil
}

func (h *Host) hopLocked(key driver.HopKey) (*hopObject, error) {
	if h.owned == nil {
		return nil, fmt.Errorf("%w: nothing applied", driver.ErrNotFound)
	}
	ho, ok := h.owned.hops[key]
	if !ok {
		return nil, fmt.Errorf("%w: hop %s", driver.ErrNotFound, key)
	}
	return ho, nil
}

func (h *Host) newEpochLocked() string {
	h.epochSeq++
	return fmt.Sprintf("fake-%d", h.epochSeq)
}

// Options configure a fake Driver.
type Options struct {
	// Engine the driver serves (ENGINE_NFTABLES when unset).
	Engine forwardv1.Engine
	// Capabilities the driver answers and renders against. When nil, those
	// of a v4.2 nftables driver (RAW links, every strategy, UDP, IPv6,
	// every limit). Engine is overwritten with Options.Engine.
	Capabilities *forwardv1.EngineCapabilities
}

// NFTablesCapabilities are the default capabilities: what the v4.2
// nftables driver offers (forward-sdk.md section 6.5).
func NFTablesCapabilities() *forwardv1.EngineCapabilities {
	return &forwardv1.EngineCapabilities{
		Engine:    forwardv1.Engine_ENGINE_NFTABLES,
		Version:   "fake 1",
		Available: true,
		Ipv6:      true,
		Udp:       true,
		Strategies: []forwardv1.BalanceStrategy{
			forwardv1.BalanceStrategy_BALANCE_STRATEGY_ROUND_ROBIN,
			forwardv1.BalanceStrategy_BALANCE_STRATEGY_RANDOM,
			forwardv1.BalanceStrategy_BALANCE_STRATEGY_IP_HASH,
			forwardv1.BalanceStrategy_BALANCE_STRATEGY_LEAST_CONN,
			forwardv1.BalanceStrategy_BALANCE_STRATEGY_FAILOVER,
		},
		LinkSecurities: []forwardv1.LinkSecurity{forwardv1.LinkSecurity_LINK_SECURITY_RAW},
		BandwidthLimit: true,
		Quota:          true,
		MaxConns:       true,
	}
}

// Driver is an in-memory driver.Driver. It keeps no state of its own: every
// call reads and writes its Host, so it behaves as a real driver that
// reads the kernel or the engine's configuration back.
type Driver struct {
	host   *Host
	engine forwardv1.Engine
	caps   *forwardv1.EngineCapabilities
}

var _ driver.Driver = (*Driver)(nil)

// New answers a driver on host.
func New(host *Host, opts Options) *Driver {
	e := opts.Engine
	if e == forwardv1.Engine_ENGINE_UNSPECIFIED {
		e = forwardv1.Engine_ENGINE_NFTABLES
	}
	caps := opts.Capabilities
	if caps == nil {
		caps = NFTablesCapabilities()
	} else {
		caps = proto.Clone(caps).(*forwardv1.EngineCapabilities)
	}
	caps.Engine = e
	return &Driver{host: host, engine: e, caps: caps}
}

// Engine implements driver.Driver.
func (d *Driver) Engine() forwardv1.Engine { return d.engine }

// Capabilities implements driver.Driver.
func (d *Driver) Capabilities(ctx context.Context) (*forwardv1.EngineCapabilities, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := d.host.takeFault(OpCapabilities); err != nil {
		return nil, err
	}
	return proto.Clone(d.caps).(*forwardv1.EngineCapabilities), nil
}

// Render implements driver.Driver. The content is the deterministic
// protobuf encoding of the accepted hops, sorted, with nothing else.
func (d *Driver) Render(state *forwardv1.NodeForwardState) (driver.Artifact, error) {
	if state == nil {
		return driver.Artifact{}, fmt.Errorf("%w: nil state", driver.ErrInvalidState)
	}
	hops, errs := driver.EngineHops(state, d.engine)
	accepted := make([]*forwardv1.NodeHop, 0, len(hops))
	keys := make([]driver.HopKey, 0, len(hops))
	for _, h := range hops {
		if err := d.check(h); err != nil {
			errs = append(errs, &driver.HopError{Key: driver.KeyOf(h), Engine: d.engine, Err: err})
			continue
		}
		accepted = append(accepted, h)
		keys = append(keys, driver.KeyOf(h))
	}
	content, err := proto.MarshalOptions{Deterministic: true}.Marshal(&forwardv1.NodeForwardState{Hops: accepted})
	if err != nil {
		return driver.Artifact{}, fmt.Errorf("fake: encode: %w", err)
	}
	slices.SortFunc(errs, func(a, b *driver.HopError) int { return a.Key.Compare(b.Key) })
	return driver.NewArtifact(d.engine, state, keys, content), driver.NewRenderError(errs)
}

// check refuses what the configured capabilities do not cover.
func (d *Driver) check(h *forwardv1.NodeHop) error {
	c := d.caps
	if !c.GetAvailable() {
		return fmt.Errorf("%w: engine unavailable: %s", driver.ErrUnsupported, c.GetUnavailableReason())
	}
	if h.GetRouteId() == "" {
		return fmt.Errorf("%w: empty route_id", driver.ErrInvalidState)
	}
	if !knownEnum(forwardv1.HopRole_name, int32(h.GetRole())) || h.GetRole() == forwardv1.HopRole_HOP_ROLE_UNSPECIFIED {
		return fmt.Errorf("%w: role %v", driver.ErrInvalidState, h.GetRole())
	}
	l := h.GetListen()
	if l.GetPort() == 0 || l.GetPort() > 65535 {
		return fmt.Errorf("%w: listen port %d", driver.ErrInvalidState, l.GetPort())
	}
	switch l.GetProtocol() {
	case forwardv1.L4Protocol_L4_PROTOCOL_TCP:
	case forwardv1.L4Protocol_L4_PROTOCOL_UDP, forwardv1.L4Protocol_L4_PROTOCOL_TCP_UDP:
		if !c.GetUdp() {
			return fmt.Errorf("%w: udp", driver.ErrUnsupported)
		}
	default:
		return fmt.Errorf("%w: protocol %v", driver.ErrUnsupported, l.GetProtocol())
	}
	if err := d.checkAddress(l.GetAddress(), true); err != nil {
		return err
	}
	if err := d.checkLink(h.GetIngress(), "ingress"); err != nil {
		return err
	}
	b := h.GetBalance()
	if !slices.Contains(c.GetStrategies(), b) {
		return fmt.Errorf("%w: balance strategy %v", driver.ErrUnsupported, b)
	}
	if len(h.GetUpstreams()) == 0 {
		return fmt.Errorf("%w: no upstreams", driver.ErrInvalidState)
	}
	seen := map[upKey]bool{}
	for _, u := range h.GetUpstreams() {
		k := upKey{u.GetAddress(), u.GetPort()}
		if k.address == "" || k.port == 0 || k.port > 65535 {
			return fmt.Errorf("%w: upstream %q port %d", driver.ErrInvalidState, k.address, k.port)
		}
		if seen[k] {
			return fmt.Errorf("%w: upstream %s:%d twice", driver.ErrInvalidState, k.address, k.port)
		}
		seen[k] = true
		if err := d.checkAddress(k.address, false); err != nil {
			return err
		}
		if err := d.checkLink(u.GetEgress(), "egress"); err != nil {
			return err
		}
	}
	lim := h.GetLimits()
	switch {
	case lim.GetBandwidthBps() > 0 && !c.GetBandwidthLimit():
		return fmt.Errorf("%w: bandwidth limit", driver.ErrUnsupported)
	case lim.GetQuotaBytes() > 0 && !c.GetQuota():
		return fmt.Errorf("%w: quota", driver.ErrUnsupported)
	case lim.GetMaxConns() > 0 && !c.GetMaxConns():
		return fmt.Errorf("%w: connection limit", driver.ErrUnsupported)
	}
	return nil
}

func (d *Driver) checkLink(t *forwardv1.LinkTransport, what string) error {
	s := t.GetSecurity()
	if s == forwardv1.LinkSecurity_LINK_SECURITY_UNSPECIFIED {
		s = forwardv1.LinkSecurity_LINK_SECURITY_RAW
	}
	if !slices.Contains(d.caps.GetLinkSecurities(), s) {
		return fmt.Errorf("%w: %s link security %v", driver.ErrUnsupported, what, s)
	}
	return nil
}

func (d *Driver) checkAddress(a string, listen bool) error {
	if a == "" {
		return nil
	}
	ip, err := netip.ParseAddr(a)
	if err != nil {
		if listen {
			return fmt.Errorf("%w: listen address %q", driver.ErrInvalidState, a)
		}
		return nil // a DNS name, resolved by the Agent
	}
	if ip.Is6() && !ip.Is4In6() && !d.caps.GetIpv6() {
		return fmt.Errorf("%w: ipv6 address %s", driver.ErrUnsupported, a)
	}
	return nil
}

func knownEnum(names map[int32]string, v int32) bool { _, ok := names[v]; return ok }

// Apply implements driver.Driver.
func (d *Driver) Apply(ctx context.Context, a driver.Artifact) (driver.ApplyResult, error) {
	if err := ctx.Err(); err != nil {
		return driver.ApplyResult{}, err
	}
	if err := a.Verify(d.engine); err != nil {
		return driver.ApplyResult{}, err
	}
	var decoded forwardv1.NodeForwardState
	if err := proto.Unmarshal(a.Content, &decoded); err != nil {
		return driver.ApplyResult{}, fmt.Errorf("%w: %v", driver.ErrInvalidArtifact, err)
	}
	specs := map[driver.HopKey]*forwardv1.NodeHop{}
	for _, hop := range decoded.GetHops() {
		specs[driver.KeyOf(hop)] = hop
	}
	if len(specs) != len(a.Hops) || !slices.EqualFunc(sortedKeys(specs), a.Hops, func(x, y driver.HopKey) bool { return x == y }) {
		return driver.ApplyResult{}, fmt.Errorf("%w: hops do not match content", driver.ErrInvalidArtifact)
	}
	if err := d.host.takeFault(OpApply); err != nil {
		return driver.ApplyResult{}, err
	}

	h := d.host
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return driver.ApplyResult{}, err
	}
	if h.impostor {
		return driver.ApplyResult{}, fmt.Errorf("%w: table exists without the ownership mark", driver.ErrNotOwned)
	}
	for _, k := range a.Hops {
		port := specs[k].GetListen().GetPort()
		if holder, ok := h.ports[port]; ok {
			return driver.ApplyResult{}, fmt.Errorf("%w: hop %s listen port %d held by %s", driver.ErrConflict, k, port, holder)
		}
	}
	res := driver.ApplyResult{Generation: a.Generation, StateHash: a.StateHash, Digest: a.Digest}
	if o := h.owned; o != nil {
		switch {
		case a.Generation < o.generation:
			return driver.ApplyResult{}, fmt.Errorf("%w: host runs generation %d, artifact has %d", driver.ErrStaleGeneration, o.generation, a.Generation)
		case a.Generation == o.generation && a.Digest != o.digest:
			return driver.ApplyResult{}, fmt.Errorf("%w: generation %d", driver.ErrGenerationConflict, a.Generation)
		}
		if a.Digest == o.digest && intact(o, specs) {
			o.nodeRef, o.generation, o.stateHash = a.NodeRef, a.Generation, a.StateHash
			return res, nil
		}
	}
	if a.Empty() && h.owned == nil {
		return res, nil
	}
	res.Changed = true
	h.applies++
	if a.Empty() {
		h.owned = nil
		return res, nil
	}
	next := &owned{nodeRef: a.NodeRef, generation: a.Generation, stateHash: a.StateHash, digest: a.Digest, hops: map[driver.HopKey]*hopObject{}}
	for k, spec := range specs {
		ho := &hopObject{spec: spec, health: map[upKey]*forwardv1.UpstreamHealth{}}
		if h.owned != nil {
			if prev, ok := h.owned.hops[k]; ok {
				// Named counters survive a rewrite.
				*ho = *prev
				ho.spec = spec
				ho.health = map[upKey]*forwardv1.UpstreamHealth{}
				for uk, v := range prev.health {
					if hasUpstream(spec, uk) {
						ho.health[uk] = v
					}
				}
			}
		}
		if ho.epoch == "" {
			ho.epoch = h.newEpochLocked()
		}
		ho.rotation = rendered(spec)
		next.hops[k] = ho
	}
	h.owned = next
	return res, nil
}

// intact reports whether the host's hops are exactly specs.
func intact(o *owned, specs map[driver.HopKey]*forwardv1.NodeHop) bool {
	if len(o.hops) != len(specs) {
		return false
	}
	for k, s := range specs {
		ho, ok := o.hops[k]
		if !ok || !proto.Equal(ho.spec, s) {
			return false
		}
	}
	return true
}

func hasUpstream(spec *forwardv1.NodeHop, k upKey) bool {
	return slices.ContainsFunc(spec.GetUpstreams(), func(u *forwardv1.Upstream) bool { return upKey{u.GetAddress(), u.GetPort()} == k })
}

func rendered(spec *forwardv1.NodeHop) []driver.Upstream {
	out := make([]driver.Upstream, 0, len(spec.GetUpstreams()))
	for _, u := range spec.GetUpstreams() {
		out = append(out, driver.Upstream{Address: u.GetAddress(), Port: u.GetPort(), Weight: max(u.GetWeight(), 1)})
	}
	return out
}

// Observe implements driver.Driver.
func (d *Driver) Observe(ctx context.Context) (driver.Observation, error) {
	if err := ctx.Err(); err != nil {
		return driver.Observation{}, err
	}
	if err := d.host.takeFault(OpObserve); err != nil {
		return driver.Observation{}, err
	}
	h := d.host
	h.mu.Lock()
	defer h.mu.Unlock()
	now := h.now().UTC()
	obs := driver.Observation{Engine: d.engine, ObservedAt: now}
	o := h.owned
	if o == nil {
		return obs, nil
	}
	obs.Applied = true
	obs.NodeRef, obs.Generation, obs.StateHash, obs.Digest = o.nodeRef, o.generation, o.stateHash, o.digest
	for _, k := range sortedKeys(o.hops) {
		ho := o.hops[k]
		obs.Counters = append(obs.Counters, &forwardv1.Counters{
			RouteId:          k.RouteID,
			HopIndex:         k.HopIndex,
			NodeRef:          o.nodeRef,
			UpBytes:          ho.upBytes,
			DownBytes:        ho.downBytes,
			UpPackets:        ho.upPackets,
			DownPackets:      ho.downPackets,
			ActiveConns:      ho.activeConns,
			TotalConns:       ho.totalConns,
			CounterEpoch:     ho.epoch,
			ObservedAtUnixMs: now.UnixMilli(),
		})
		hk := make([]upKey, 0, len(ho.health))
		for uk := range ho.health {
			hk = append(hk, uk)
		}
		slices.SortFunc(hk, func(a, b upKey) int {
			if c := strings.Compare(a.address, b.address); c != 0 {
				return c
			}
			return int(a.port) - int(b.port)
		})
		for _, uk := range hk {
			obs.Health = append(obs.Health, proto.Clone(ho.health[uk]).(*forwardv1.UpstreamHealth))
		}
		obs.Rotation = append(obs.Rotation, driver.HopRotation{RouteID: k.RouteID, HopIndex: k.HopIndex, Active: slices.Clone(ho.rotation)})
	}
	return obs, nil
}

// SetUpstreams implements driver.Driver.
func (d *Driver) SetUpstreams(ctx context.Context, routeID string, hopIndex uint32, active []driver.Upstream) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(active) == 0 {
		return fmt.Errorf("%w: no upstream in rotation", driver.ErrInvalidArgument)
	}
	seen := map[upKey]bool{}
	for _, u := range active {
		k := upKey{u.Address, u.Port}
		if seen[k] {
			return fmt.Errorf("%w: upstream %s:%d twice", driver.ErrInvalidArgument, u.Address, u.Port)
		}
		seen[k] = true
	}
	if err := d.host.takeFault(OpSetUpstreams); err != nil {
		return err
	}
	h := d.host
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	ho, err := h.hopLocked(driver.HopKey{RouteID: routeID, HopIndex: hopIndex})
	if err != nil {
		return err
	}
	all := rendered(ho.spec)
	next := make([]driver.Upstream, 0, len(active))
	for _, u := range active {
		i := slices.IndexFunc(all, func(r driver.Upstream) bool { return r.Address == u.Address && r.Port == u.Port })
		if i < 0 {
			return fmt.Errorf("%w: upstream %s:%d is not rendered for %s/%d", driver.ErrNotFound, u.Address, u.Port, routeID, hopIndex)
		}
		if u.Weight == 0 {
			u.Weight = all[i].Weight
		}
		next = append(next, u)
	}
	ho.rotation = next
	return nil
}

// ActiveConns answers the connections Host.SetUpstreamConns set: the
// fake is a leastconn.Source.
func (d *Driver) ActiveConns(ctx context.Context) (map[netip.AddrPort]uint64, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	d.host.mu.Lock()
	defer d.host.mu.Unlock()
	out := maps.Clone(d.host.conns)
	if out == nil {
		out = map[netip.AddrPort]uint64{}
	}
	return out, nil
}

// Remove implements driver.Driver.
func (d *Driver) Remove(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := d.host.takeFault(OpRemove); err != nil {
		return err
	}
	h := d.host
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	h.owned = nil
	return nil
}

func sortedKeys[V any](m map[driver.HopKey]V) []driver.HopKey {
	keys := make([]driver.HopKey, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, driver.HopKey.Compare)
	return keys
}

func formatRotation(us []driver.Upstream) string {
	parts := make([]string, len(us))
	for i, u := range us {
		parts[i] = fmt.Sprintf("%s:%d*%d", u.Address, u.Port, u.Weight)
	}
	return strings.Join(parts, ",")
}
