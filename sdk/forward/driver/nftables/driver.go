package nftables

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"slices"
	"sync"
	"time"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/driver"
)

// Names of what the driver owns on a host.
const (
	// Family and Table name the one nftables table the driver owns.
	Family = "inet"
	Table  = "anixops_fwd"
	// OwnerComment is the table's comment, the ownership mark of the
	// driver contract: a table "inet anixops_fwd" without it is foreign.
	// nft cannot change a table's comment in place, so it never changes;
	// the applied generation and digest are recorded in the table's state
	// set (see hoststate.go).
	OwnerComment = "anixops-forward-driver v1"
)

// undoTimeout bounds the rollback of tc steps after a failed apply, which
// runs whatever the caller's context says.
const undoTimeout = 30 * time.Second

// Driver is the nftables forward driver. It keeps no state besides its
// configuration: everything it applied is read back from the host. Apply,
// SetUpstreams and Remove are serialised; Observe runs between them.
type Driver struct {
	cfg     Config
	runner  Runner
	retired func([]*forwardv1.Counters)
	now     func() time.Time
	nonce   func() string

	mu sync.RWMutex
}

var _ driver.Driver = (*Driver)(nil)

// Option configures New.
type Option func(*Driver)

// WithRunner runs nft and tc through r instead of ExecRunner (tests, a
// network namespace).
func WithRunner(r Runner) Option { return func(d *Driver) { d.runner = r } }

// WithRetiredCounters calls f after an Apply that deleted hops, with their
// last counters (read just before the transaction that deleted them), so
// the Agent can report the end of their counter epochs. It runs after
// Apply released the driver, so it may call it.
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
	cfg.MSSClampInterfaces = slices.Clone(cfg.MSSClampInterfaces)
	slices.Sort(cfg.MSSClampInterfaces)
	cfg.LimitInterfaces = slices.Clone(cfg.LimitInterfaces)
	slices.Sort(cfg.LimitInterfaces)
	d := &Driver{cfg: cfg, runner: ExecRunner{}, now: time.Now, nonce: randomNonce}
	for _, o := range opts {
		o(d)
	}
	return d, nil
}

func randomNonce() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Engine answers ENGINE_NFTABLES.
func (d *Driver) Engine() forwardv1.Engine { return forwardv1.Engine_ENGINE_NFTABLES }

// Config answers a copy of the driver's configuration.
func (d *Driver) Config() Config {
	c := d.cfg
	c.Strategies = slices.Clone(c.Strategies)
	c.MSSClampInterfaces = slices.Clone(c.MSSClampInterfaces)
	c.LimitInterfaces = slices.Clone(c.LimitInterfaces)
	return c
}

// Capabilities answers the capabilities of the configuration, which Probe
// filled from the host: a configuration without a Version is unavailable.
func (d *Driver) Capabilities(ctx context.Context) (*forwardv1.EngineCapabilities, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c := &forwardv1.EngineCapabilities{
		Engine:         forwardv1.Engine_ENGINE_NFTABLES,
		Version:        d.cfg.Version,
		Available:      d.cfg.Version != "",
		Ipv6:           d.cfg.IPv6,
		Udp:            d.cfg.UDP,
		Strategies:     slices.Clone(d.cfg.Strategies),
		LinkSecurities: []forwardv1.LinkSecurity{forwardv1.LinkSecurity_LINK_SECURITY_RAW},
		BandwidthLimit: d.cfg.BandwidthLimit,
		Quota:          d.cfg.Quota,
		MaxConns:       d.cfg.MaxConns,
	}
	slices.Sort(c.Strategies)
	if !c.Available {
		c.UnavailableReason = d.cfg.Unavailable
		if c.UnavailableReason == "" {
			c.UnavailableReason = "nftables host not probed: the configuration has no version (nftables.Probe fills it)"
		}
	}
	return c, nil
}

// run runs one command through the runner.
func (d *Driver) run(ctx context.Context, name string, stdin []byte, args ...string) ([]byte, error) {
	return d.runner.Run(ctx, name, args, stdin)
}

// host is the driver's table as Apply, Observe and SetUpstreams find it.
type host struct {
	present bool     // a table inet anixops_fwd exists
	owned   bool     // and carries OwnerComment
	table   *listing // its objects, when owned
	doc     *hostDoc // the recorded state, when owned and readable
	seal    string   // the recorded fingerprint
	docErr  error    // why the state could not be read
}

// readHost reads the driver's table.
func (d *Driver) readHost(ctx context.Context) (*host, error) {
	out, err := d.run(ctx, "nft", nil, "-j", "list", "table", Family, Table)
	if err != nil {
		if cerr := ctx.Err(); cerr != nil {
			return nil, cerr
		}
		if notFound(err) {
			return &host{}, nil
		}
		return nil, fmt.Errorf("nftables driver: read table: %w", err)
	}
	l, err := parseListing(out)
	if err != nil {
		return nil, err
	}
	l = l.in(Family, Table)
	t, ok := l.table(Family, Table)
	if !ok {
		return &host{}, nil
	}
	h := &host{present: true, owned: t.str("comment") == OwnerComment}
	if !h.owned {
		return h, nil
	}
	h.table = l
	if s, ok := l.object("set", stateSet); ok {
		h.doc, h.docErr = decodeDoc(s.elementComments())
	}
	if s, ok := l.object("set", sealSet); ok {
		h.seal = s.elementComments()[0]
	}
	return h, nil
}

// notOwned is Apply's error for a foreign table of the driver's name.
func notOwned() error {
	return fmt.Errorf("%w: table %s %s exists without the comment %q", driver.ErrNotOwned, Family, Table, OwnerComment)
}

// Apply makes the host run the artifact (driver package documentation):
//
//  1. read the table; a table without OwnerComment is ErrNotOwned (the
//     script's flush would empty it);
//  2. check the generation against the recorded one;
//  3. compare: the recorded digest, the table's fingerprint with the seal
//     recorded after the last change, and the tc objects with the ones the
//     artifact needs. All equal is a no-op that at most records the new
//     generation and state_hash;
//  4. refuse foreign DNAT rules on the artifact's listeners (ErrConflict)
//     and foreign root qdiscs on the limit interfaces;
//  5. `nft -c`, then add the tc qdisc and classes the artifact needs, run
//     the transaction (counter epochs for new counters, the script, the
//     deletion of every undeclared object, the state document) with
//     `nft -f`, undoing the tc additions when it fails;
//  6. record the new fingerprint, then delete the tc classes and qdiscs
//     no longer needed.
func (d *Driver) Apply(ctx context.Context, a driver.Artifact) (driver.ApplyResult, error) {
	if err := ctx.Err(); err != nil {
		return driver.ApplyResult{}, err
	}
	if err := a.Verify(d.Engine()); err != nil {
		return driver.ApplyResult{}, err
	}
	m, err := parseManifest(a)
	if err != nil {
		return driver.ApplyResult{}, err
	}
	// The hook runs after the lock is released, so it may call the driver.
	var retired []*forwardv1.Counters
	defer func() {
		if d.retired != nil && len(retired) > 0 {
			d.retired(retired)
		}
	}()
	d.mu.Lock()
	defer d.mu.Unlock()

	h, err := d.readHost(ctx)
	if err != nil {
		return driver.ApplyResult{}, err
	}
	if h.present && !h.owned {
		return driver.ApplyResult{}, notOwned()
	}
	if doc := h.doc; doc != nil {
		switch {
		case a.Generation < doc.Gen:
			return driver.ApplyResult{}, fmt.Errorf("%w: artifact generation %d, host runs %d", driver.ErrStaleGeneration, a.Generation, doc.Gen)
		case a.Generation == doc.Gen && a.Digest != doc.Digest:
			return driver.ApplyResult{}, fmt.Errorf("%w: generation %d", driver.ErrGenerationConflict, a.Generation)
		}
	}
	res := driver.ApplyResult{Generation: a.Generation, StateHash: a.StateHash, Digest: a.Digest}
	if a.Empty() {
		return d.applyEmpty(ctx, h, res, &retired)
	}

	tcp, err := d.planTC(ctx, m)
	if err != nil {
		return driver.ApplyResult{}, err
	}
	if h.doc != nil && h.doc.Digest == a.Digest && h.seal == h.table.fingerprint() && tcp.noop() {
		if h.doc.Gen != a.Generation || h.doc.Hash != a.StateHash || h.doc.Node != a.NodeRef {
			doc := *h.doc
			doc.Gen, doc.Hash, doc.Node = a.Generation, a.StateHash, a.NodeRef
			w := &writer{}
			writeDoc(w, &doc)
			if _, err := d.run(ctx, "nft", []byte(w.b.String()), "-f", "-"); err != nil {
				return driver.ApplyResult{}, fmt.Errorf("nftables driver: record generation: %w", err)
			}
		}
		return res, nil
	}
	if err := d.checkConflicts(ctx, m); err != nil {
		return driver.ApplyResult{}, err
	}

	script, gone := d.transaction(a, m, h)
	if _, err := d.run(ctx, "nft", script, "-c", "-f", "-"); err != nil {
		return driver.ApplyResult{}, fmt.Errorf("nftables driver: nft -c refused the transaction: %w", err)
	}
	if _, err := d.runTC(ctx, tcp.pre); err != nil {
		return driver.ApplyResult{}, fmt.Errorf("nftables driver: tc: %w", err)
	}
	if _, err := d.run(ctx, "nft", script, "-f", "-"); err != nil {
		d.undoTC(tcp.pre)
		if cerr := ctx.Err(); cerr != nil {
			return driver.ApplyResult{}, cerr
		}
		return driver.ApplyResult{}, fmt.Errorf("nftables driver: nft -f refused the transaction: %w", err)
	}
	// From here the host runs the artifact; what follows tidies up, and a
	// cancelled caller must not stop it halfway.
	tidy, cancel := context.WithTimeout(context.WithoutCancel(ctx), undoTimeout)
	defer cancel()
	if err := d.seal(tidy); err != nil {
		return driver.ApplyResult{}, err
	}
	if _, err := d.runTC(tidy, tcp.post); err != nil {
		return driver.ApplyResult{}, fmt.Errorf("nftables driver: tc cleanup: %w", err)
	}
	retired = gone
	res.Changed = true
	return res, nil
}

// applyEmpty removes everything the driver owns.
func (d *Driver) applyEmpty(ctx context.Context, h *host, res driver.ApplyResult, retired *[]*forwardv1.Counters) (driver.ApplyResult, error) {
	tcp, err := d.planTC(ctx, nil)
	if err != nil {
		return driver.ApplyResult{}, err
	}
	if !h.present && tcp.noop() {
		return res, nil
	}
	if h.present {
		if _, err := d.run(ctx, "nft", nil, "delete", "table", Family, Table); err != nil && !notFound(err) {
			return driver.ApplyResult{}, fmt.Errorf("nftables driver: delete table: %w", err)
		}
	}
	tidy, cancel := context.WithTimeout(context.WithoutCancel(ctx), undoTimeout)
	defer cancel()
	if _, err := d.runTC(tidy, tcp.post); err != nil {
		return driver.ApplyResult{}, fmt.Errorf("nftables driver: tc cleanup: %w", err)
	}
	if h.doc != nil {
		*retired = d.counters(h, h.doc.Hops, h.doc.Node, d.now())
	}
	res.Changed = true
	return res, nil
}

// transaction answers the nft script of a changing apply and the last
// counters of the hops it deletes.
func (d *Driver) transaction(a driver.Artifact, m *manifest, h *host) ([]byte, []*forwardv1.Counters) {
	tbl := Family + " " + Table
	w := &writer{}
	// Counter epochs: a counter created by this transaction carries a new
	// nonce; nft keeps the comment of a counter that exists.
	nonce := d.nonce()
	w.open("table %s", tbl)
	w.line("comment %q", OwnerComment)
	for _, hop := range m.hops {
		for _, dir := range []string{"_up", "_down"} {
			w.open("counter %s%s", hop.base(), dir)
			w.line("comment %q", epochNote+nonce)
			w.close()
		}
	}
	declareStateSets(w)
	w.close()
	w.b.Write(a.Content)

	// Every object of the table the script does not declare belongs to a
	// removed hop (or to an older render): delete it. The script flushed
	// every rule, so nothing refers to it any more. Sets and maps first,
	// then chains, then counters and quotas.
	var retired []*forwardv1.Counters
	if h.table != nil {
		keep := map[string]bool{"set " + stateSet: true, "set " + sealSet: true}
		order := map[string]int{"set": 0, "map": 0, "chain": 1, "counter": 2, "quota": 2}
		var stale []item
		for _, it := range h.table.items {
			if _, ok := order[it.kind]; !ok {
				continue
			}
			if name := it.kind + " " + it.str("name"); !keep[name] && !m.declared[name] {
				stale = append(stale, it)
			}
		}
		slices.SortStableFunc(stale, func(x, y item) int { return order[x.kind] - order[y.kind] })
		for _, it := range stale {
			w.line("delete %s %s %s", it.kind, tbl, it.str("name"))
		}
		if h.doc != nil {
			var gone []*docHop
			for _, dh := range h.doc.Hops {
				if !slices.ContainsFunc(m.hops, func(x *manifestHopInfo) bool { return x.key == dh.key() }) {
					gone = append(gone, dh)
				}
			}
			retired = d.counters(h, gone, h.doc.Node, d.now())
		}
	}
	writeDoc(w, docFor(a, m))
	w.line("flush set %s %s", tbl, sealSet)
	return []byte(w.b.String()), retired
}

// seal records the fingerprint of the table as it is now.
func (d *Driver) seal(ctx context.Context) error {
	h, err := d.readHost(ctx)
	if err != nil {
		return err
	}
	if !h.owned {
		return fmt.Errorf("nftables driver: table vanished before it was sealed")
	}
	tbl := Family + " " + Table
	script := fmt.Sprintf("flush set %s %s\nadd element %s %s { 0 comment %q }\n", tbl, sealSet, tbl, sealSet, h.table.fingerprint())
	if _, err := d.run(ctx, "nft", []byte(script), "-f", "-"); err != nil {
		return fmt.Errorf("nftables driver: seal: %w", err)
	}
	return nil
}

// counters answers the counters of hops as the host holds them.
func (d *Driver) counters(h *host, hops []*docHop, node string, now time.Time) []*forwardv1.Counters {
	out := make([]*forwardv1.Counters, 0, len(hops))
	for _, dh := range hops {
		base := hopName(dh.key())
		c := &forwardv1.Counters{RouteId: dh.Route, HopIndex: dh.Index, NodeRef: node, ObservedAtUnixMs: now.UnixMilli()}
		var ids [2]string
		for i, dir := range []string{"_up", "_down"} {
			it, ok := h.table.object("counter", base+dir)
			if !ok {
				ids[i] = "missing"
				continue
			}
			ids[i] = counterID(it)
			if i == 0 {
				c.UpPackets, c.UpBytes = it.num("packets"), it.num("bytes")
			} else {
				c.DownPackets, c.DownBytes = it.num("packets"), it.num("bytes")
			}
		}
		c.CounterEpoch = ids[0]
		if ids[1] != ids[0] {
			c.CounterEpoch = ids[0] + "." + ids[1]
		}
		out = append(out, c)
	}
	return out
}

// counterID answers what identifies one creation of a counter: the nonce
// of its comment, or its handle when it has none.
func counterID(it item) string {
	if c := it.str("comment"); len(c) > len(epochNote) && c[:len(epochNote)] == epochNote {
		return c[len(epochNote):]
	}
	return fmt.Sprintf("h%d", it.num("handle"))
}

// Observe reads the table: the recorded identity, the hops' counters and
// their rotation. A host without the driver's table, or with a foreign
// one, observes as not applied.
func (d *Driver) Observe(ctx context.Context) (driver.Observation, error) {
	if err := ctx.Err(); err != nil {
		return driver.Observation{}, err
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	h, err := d.readHost(ctx)
	if err != nil {
		return driver.Observation{}, err
	}
	now := d.now()
	o := driver.Observation{Engine: d.Engine(), ObservedAt: now}
	if !h.owned || h.doc == nil {
		return o, nil
	}
	doc := h.doc
	o.Applied, o.NodeRef, o.Generation, o.StateHash, o.Digest = true, doc.Node, doc.Gen, doc.Hash, doc.Digest
	o.Counters = d.counters(h, doc.Hops, doc.Node, now)
	for _, dh := range doc.Hops {
		o.Rotation = append(o.Rotation, driver.HopRotation{RouteID: dh.Route, HopIndex: dh.Index, Active: dh.rotation()})
	}
	return o, nil
}

// SetUpstreams rewrites the balancing map elements of one applied hop for
// the selected upstreams (failover keeps only the best priority among
// them), records the rotation in the state document in the same
// transaction, and re-seals the table.
func (d *Driver) SetUpstreams(ctx context.Context, routeID string, hopIndex uint32, active []driver.Upstream) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(active) == 0 {
		return fmt.Errorf("%w: no upstream selected", driver.ErrInvalidArgument)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	h, err := d.readHost(ctx)
	if err != nil {
		return err
	}
	k := driver.HopKey{RouteID: routeID, HopIndex: hopIndex}
	if !h.owned || h.doc == nil {
		return fmt.Errorf("%w: hop %s (nothing applied)", driver.ErrNotFound, k)
	}
	dh := h.doc.hop(k)
	if dh == nil {
		return fmt.Errorf("%w: hop %s", driver.ErrNotFound, k)
	}
	sel, err := selectUpstreams(dh, active)
	if err != nil {
		return err
	}
	rendered := dh.rendered()
	newActive := make([]docUp, 0, len(sel))
	same := len(sel) == len(rendered)
	for _, u := range rendered {
		w, ok := sel[upKey(u)]
		if !ok {
			continue
		}
		if w == 0 {
			w = u.weight
		}
		if w != u.weight {
			same = false
		}
		newActive = append(newActive, docUp{Addr: u.addr.String(), Port: u.port, Weight: w})
	}
	doc := *h.doc
	doc.Hops = slices.Clone(h.doc.Hops)
	nh := *dh
	nh.Active = newActive
	if same {
		nh.Active = nil
	}
	for i := range doc.Hops {
		if doc.Hops[i].key() == k {
			doc.Hops[i] = &nh
		}
	}

	tbl := Family + " " + Table
	w := &writer{}
	layout := strategyLayout(dh.Balance)
	for _, fam := range []struct {
		suffix string
		is4    bool
	}{{"_lb4", true}, {"_lb6", false}} {
		var famRendered, famActive []upstream
		for _, u := range rendered {
			if u.addr.Is4() != fam.is4 {
				continue
			}
			famRendered = append(famRendered, u)
			if wt, ok := sel[upKey(u)]; ok {
				if wt != 0 {
					u.weight = wt
				}
				famActive = append(famActive, u)
			}
		}
		if len(famRendered) == 0 {
			continue
		}
		name := hopName(k) + fam.suffix
		w.line("flush map %s %s", tbl, name)
		if len(famActive) == 0 {
			continue // the family drops in the _dnat chain until it is back
		}
		runs, held := balanceRuns(layout, famActive, d.cfg.Slots)
		w.open("add element %s %s", tbl, name)
		for _, r := range runs {
			u := held[r.idx]
			w.line("%s : %s . %d,", slotText(r), u.addr, u.port)
		}
		w.close()
	}
	writeDoc(w, &doc)
	if _, err := d.run(ctx, "nft", []byte(w.b.String()), "-f", "-"); err != nil {
		if cerr := ctx.Err(); cerr != nil {
			return cerr
		}
		return fmt.Errorf("nftables driver: set upstreams: %w", err)
	}
	tidy, cancel := context.WithTimeout(context.WithoutCancel(ctx), undoTimeout)
	defer cancel()
	return d.seal(tidy)
}

func upKey(u upstream) string { return fmt.Sprintf("%s|%d", u.addr, u.port) }

// selectUpstreams checks a selection against the hop's rendered upstreams
// and answers it as upKey -> weight (0: the rendered weight).
func selectUpstreams(dh *docHop, active []driver.Upstream) (map[string]uint32, error) {
	rendered := map[string]bool{}
	for _, u := range dh.rendered() {
		rendered[upKey(u)] = true
	}
	sel := map[string]uint32{}
	for _, u := range active {
		addr, err := parseLiteral(u.Address)
		if err != nil {
			return nil, fmt.Errorf("%w: upstream %s:%d of hop %s", driver.ErrNotFound, u.Address, u.Port, dh.key())
		}
		key := upKey(upstream{addr: addr, port: u.Port})
		if _, dup := sel[key]; dup {
			return nil, fmt.Errorf("%w: upstream %s:%d selected twice", driver.ErrInvalidArgument, u.Address, u.Port)
		}
		if !rendered[key] {
			return nil, fmt.Errorf("%w: upstream %s:%d is not one hop %s was rendered with", driver.ErrNotFound, u.Address, u.Port, dh.key())
		}
		sel[key] = u.Weight
	}
	return sel, nil
}

// Remove deletes the driver's table when it carries OwnerComment, and its
// tc qdiscs. A foreign table of the same name is left alone.
func (d *Driver) Remove(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	h, err := d.readHost(ctx)
	if err != nil {
		return err
	}
	if h.owned {
		if _, err := d.run(ctx, "nft", nil, "delete", "table", Family, Table); err != nil && !notFound(err) {
			return fmt.Errorf("nftables driver: delete table: %w", err)
		}
	}
	return d.removeTC(ctx)
}
