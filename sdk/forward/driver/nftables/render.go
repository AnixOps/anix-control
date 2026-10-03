package nftables

import (
	"fmt"
	"net/netip"
	"slices"
	"strconv"
	"strings"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/driver"
)

// Render turns the state's nftables hops into one `nft -f` script; see the
// package documentation for its shape. Hops it cannot run come back as hop
// errors in a *driver.RenderError next to the artifact of the others.
func (d *Driver) Render(state *forwardv1.NodeForwardState) (driver.Artifact, error) {
	if state == nil {
		return driver.Artifact{}, fmt.Errorf("%w: nil state", driver.ErrInvalidState)
	}
	hops, hopErrs := driver.EngineHops(state, d.Engine())
	var plans []*hopPlan
	for _, h := range hops {
		p, err := d.planHop(h)
		if err != nil {
			hopErrs = append(hopErrs, &driver.HopError{Key: driver.KeyOf(h), Engine: d.Engine(), Err: err})
			continue
		}
		plans = append(plans, p)
	}
	plans, collisions := rejectCollisions(plans)
	hopErrs = append(hopErrs, collisions...)
	slices.SortStableFunc(hopErrs, func(a, b *driver.HopError) int { return a.Key.Compare(b.Key) })

	keys := make([]driver.HopKey, len(plans))
	for i, p := range plans {
		keys[i] = p.key
	}
	return driver.NewArtifact(d.Engine(), state, keys, d.script(plans)), driver.NewRenderError(hopErrs)
}

// writer builds the script line by line with tab indentation.
type writer struct {
	b     strings.Builder
	depth int
}

func (w *writer) line(format string, args ...any) {
	for range w.depth {
		w.b.WriteByte('\t')
	}
	fmt.Fprintf(&w.b, format, args...)
	w.b.WriteByte('\n')
}

func (w *writer) open(format string, args ...any) {
	w.line(format+" {", args...)
	w.depth++
}

func (w *writer) close() {
	w.depth--
	w.line("}")
}

func (w *writer) blank() { w.b.WriteByte('\n') }

const header = `#!/usr/sbin/nft -f
# Rendered by sdk/forward/driver/nftables (anixops forward driver v1); do not edit.
# One transaction that touches table inet anixops_fwd only: declare every
# object, flush the rules and the balancing and admission elements, then add
# them again. Counters, quotas and connection counts keep their values.
# Apply checks the table's ownership comment before running it, adds the
# state it records on the host and deletes the objects of removed hops in the
# same transaction. The anixops-hop and anixops-upstream lines are the
# manifest Apply reads: every rendered upstream, the bandwidth for tc.
`

const emptyNote = "# No hops: applying this artifact removes table inet anixops_fwd.\n"

// script renders the hops, sorted by key, into the nft script.
func (d *Driver) script(plans []*hopPlan) []byte {
	w := &writer{}
	w.b.WriteString(header)
	if len(plans) == 0 {
		w.b.WriteString(emptyNote)
		return []byte(w.b.String())
	}
	tbl := Family + " " + Table

	w.blank()
	d.writeManifest(w, plans)

	// 1. Declare every object, without elements or rules, so the flushes
	// below find them on a host that has none yet.
	w.blank()
	w.open("table %s", tbl)
	w.line("comment %q", OwnerComment)
	for _, p := range plans {
		d.declareObjects(w, p, false)
	}
	for _, p := range plans {
		w.open("chain %s_dnat", p.base)
		w.close()
		w.open("chain %s_acct", p.base)
		w.close()
	}
	d.baseChains(w, nil)
	w.close()

	// 2. Flush the rules of every chain and the elements of the balancing
	// and admission sets. The connection-count sets keep their entries.
	w.blank()
	w.line("flush table %s", tbl)
	for _, p := range plans {
		for _, name := range p.flushedSets() {
			w.line("flush %s %s %s", name.kind, tbl, name.name)
		}
	}

	// 3. Add the elements and the rules.
	w.blank()
	w.open("table %s", tbl)
	for _, p := range plans {
		d.declareObjects(w, p, true)
	}
	for _, p := range plans {
		d.hopChains(w, p)
	}
	d.baseChains(w, plans)
	w.close()
	return []byte(w.b.String())
}

type setName struct{ kind, name string }

// flushedSets answers the hop's sets and maps whose elements the render
// owns.
func (p *hopPlan) flushedSets() []setName {
	var out []setName
	if len(p.src4) > 0 {
		out = append(out, setName{"set", p.base + "_src4"})
	}
	if len(p.src6) > 0 {
		out = append(out, setName{"set", p.base + "_src6"})
	}
	if len(p.ups4) > 0 {
		out = append(out, setName{"map", p.base + "_lb4"})
	}
	if len(p.ups6) > 0 {
		out = append(out, setName{"map", p.base + "_lb6"})
	}
	return out
}

// declareObjects writes the hop's counters, quota and sets; with elements
// it writes the sets and maps again with their elements.
func (d *Driver) declareObjects(w *writer, p *hopPlan, elements bool) {
	if !elements {
		w.open("counter %s_up", p.base)
		w.close()
		w.open("counter %s_down", p.base)
		w.close()
		if p.quotaBytes > 0 {
			w.open("quota %s_quota", p.base)
			w.line("over %d bytes", p.quotaBytes)
			w.close()
		}
		if p.maxConns > 0 {
			w.open("set %s_conns", p.base)
			w.line("type mark")
			w.line("flags dynamic")
			w.close()
		}
	}
	srcSet := func(name, typ string, ps []netip.Prefix) {
		if len(ps) == 0 {
			return
		}
		w.open("set %s", name)
		w.line("type %s", typ)
		w.line("flags interval")
		if elements {
			w.open("elements =")
			for _, pf := range ps {
				w.line("%s,", prefixText(pf))
			}
			w.close()
		}
		w.close()
	}
	srcSet(p.base+"_src4", "ipv4_addr", p.src4)
	srcSet(p.base+"_src6", "ipv6_addr", p.src6)
	lbMap := func(name, typ string, ups []upstream) {
		if len(ups) == 0 {
			return
		}
		w.open("map %s", name)
		w.line("type mark : %s . inet_service", typ)
		w.line("flags interval")
		if elements {
			runs, held := balanceRuns(strategyLayout(p.balance), ups, d.cfg.Slots)
			w.open("elements =")
			for _, r := range runs {
				u := held[r.idx]
				w.line("%s : %s . %d,", slotText(r), u.addr, u.port)
			}
			w.close()
		}
		w.close()
	}
	lbMap(p.base+"_lb4", "ipv4_addr", p.ups4)
	lbMap(p.base+"_lb6", "ipv6_addr", p.ups6)
}

// strategyLayout answers how a strategy fills its map.
func strategyLayout(s forwardv1.BalanceStrategy) interleaving {
	switch s {
	case forwardv1.BalanceStrategy_BALANCE_STRATEGY_FAILOVER:
		return interleaving{interleave: true, primaryOnly: true}
	case forwardv1.BalanceStrategy_BALANCE_STRATEGY_ROUND_ROBIN:
		return interleaving{interleave: true}
	default: // RANDOM, LEAST_CONN, IP_HASH: one run per upstream
		return interleaving{}
	}
}

// selector answers the expression that picks a slot for a new connection.
func (d *Driver) selector(s forwardv1.BalanceStrategy, fam int) string {
	n := d.cfg.Slots
	switch s {
	case forwardv1.BalanceStrategy_BALANCE_STRATEGY_RANDOM, forwardv1.BalanceStrategy_BALANCE_STRATEGY_LEAST_CONN:
		return fmt.Sprintf("numgen random mod %d", n)
	case forwardv1.BalanceStrategy_BALANCE_STRATEGY_IP_HASH:
		if fam == 4 {
			return fmt.Sprintf("jhash ip saddr mod %d seed 0x0", n)
		}
		return fmt.Sprintf("jhash ip6 saddr mod %d seed 0x0", n)
	default: // ROUND_ROBIN, FAILOVER
		return fmt.Sprintf("numgen inc mod %d", n)
	}
}

// hopChains writes the hop's DNAT and accounting chains.
func (d *Driver) hopChains(w *writer, p *hopPlan) {
	notMask := hexMark(^d.cfg.MarkMask)
	w.open("chain %s_dnat", p.base)
	switch {
	case p.paused:
		// Paused: no new connection gets through; nothing goes to local
		// input either.
		w.line("drop")
	default:
		if p.sources {
			for _, f := range []struct {
				fam, match, set string
				has             bool
			}{
				{"ipv4", "ip saddr", p.base + "_src4", len(p.src4) > 0},
				{"ipv6", "ip6 saddr", p.base + "_src6", len(p.src6) > 0},
			} {
				if f.has {
					w.line("meta nfproto %s %s != @%s drop", f.fam, f.match, f.set)
				} else {
					w.line("meta nfproto %s drop", f.fam)
				}
			}
		}
		w.line("ct mark set ct mark and %s or %s", notMask, hexMark(p.mark))
		l4 := p.l4Match()
		if len(p.ups4) > 0 {
			w.line("meta nfproto ipv4 %s dnat ip to %s map @%s_lb4", l4, d.selector(p.balance, 4), p.base)
		}
		if len(p.ups6) > 0 {
			w.line("meta nfproto ipv6 %s dnat ip6 to %s map @%s_lb6", l4, d.selector(p.balance, 6), p.base)
		}
		// A family without upstreams must not fall through to local input.
		w.line("drop")
	}
	w.close()

	w.open("chain %s_acct", p.base)
	if p.paused {
		// Established connections carry the mark: drop them uncounted.
		w.line("drop")
		w.close()
		return
	}
	if p.maxConns > 0 {
		w.line("ct state new add @%s_conns { ct mark ct count over %d } reject", p.base, p.maxConns)
	}
	if p.quotaBytes > 0 {
		w.line("quota name %q drop", p.base+"_quota")
	}
	up := fmt.Sprintf("ct direction original counter name %q", p.base+"_up")
	down := fmt.Sprintf("ct direction reply counter name %q", p.base+"_down")
	if p.bandwidth {
		// tc classifies on the packet mark: the hop's mark, plus the
		// direction bit on reply packets. Other packet mark bits are kept.
		keep := hexMark(^(d.cfg.MarkMask | d.cfg.DirectionBit))
		up += fmt.Sprintf(" meta mark set meta mark and %s or %s", keep, hexMark(p.mark))
		down += fmt.Sprintf(" meta mark set meta mark and %s or %s", keep, hexMark(p.mark|d.cfg.DirectionBit))
	}
	w.line("%s", up)
	w.line("%s", down)
	w.close()
}

// l4Match answers the protocol match of the DNAT rule.
func (p *hopPlan) l4Match() string {
	switch {
	case p.tcp && p.udp:
		return "meta l4proto { tcp, udp }"
	case p.udp:
		return "meta l4proto udp"
	default:
		return "meta l4proto tcp"
	}
}

// listenMatch answers the prerouting match of the hop's listener. One
// protocol uses "tcp dport"; "meta l4proto tcp th dport" is accepted by nft
// but not read back.
func (p *hopPlan) listenMatch() string {
	var b strings.Builder
	switch {
	case p.listen.Is4():
		fmt.Fprintf(&b, "ip daddr %s ", p.listen)
	case p.listen.Is6():
		fmt.Fprintf(&b, "ip6 daddr %s ", p.listen)
	}
	switch {
	case p.tcp && p.udp:
		fmt.Fprintf(&b, "meta l4proto { tcp, udp } th dport %d", p.port)
	case p.udp:
		fmt.Fprintf(&b, "udp dport %d", p.port)
	default:
		fmt.Fprintf(&b, "tcp dport %d", p.port)
	}
	if !p.listen.IsValid() {
		b.WriteString(" fib daddr type local")
	}
	return b.String()
}

// baseChains writes the hooked chains; without plans, only their
// declarations.
func (d *Driver) baseChains(w *writer, plans []*hopPlan) {
	mask := hexMark(d.cfg.MarkMask)

	w.open("chain prerouting")
	w.line("type nat hook prerouting priority dstnat; policy accept;")
	for _, p := range plans {
		w.line("%s goto %s_dnat", p.listenMatch(), p.base)
	}
	w.close()

	w.open("chain forward")
	w.line("type filter hook forward priority filter; policy accept;")
	if len(plans) > 0 {
		if ifs := d.cfg.MSSClampInterfaces; len(ifs) > 0 {
			quoted := make([]string, len(ifs))
			for i, name := range ifs {
				quoted[i] = strconv.Quote(name)
			}
			w.line("ct mark and %s != 0x0 oifname { %s } tcp flags & (syn | rst) == syn tcp option maxseg size set rt mtu", mask, strings.Join(quoted, ", "))
		}
		w.open("ct mark and %s vmap", mask)
		for _, p := range plans {
			w.line("%s : jump %s_acct,", hexMark(p.mark), p.base)
		}
		w.close()
	}
	w.close()

	w.open("chain postrouting")
	w.line("type nat hook postrouting priority srcnat; policy accept;")
	if len(plans) > 0 {
		w.open("ct mark and %s", mask)
		for _, p := range plans {
			w.line("%s,", hexMark(p.mark))
		}
		w.depth--
		w.line("} masquerade")
	}
	w.close()
}

func hexMark(m uint32) string { return fmt.Sprintf("0x%08x", m) }

func slotText(r slotRun) string {
	if r.lo == r.hi {
		return strconv.FormatUint(uint64(r.lo), 10)
	}
	return fmt.Sprintf("%d-%d", r.lo, r.hi)
}

func prefixText(p netip.Prefix) string {
	if p.IsSingleIP() {
		return p.Addr().String()
	}
	return p.String()
}
