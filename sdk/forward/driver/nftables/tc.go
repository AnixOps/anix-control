package nftables

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/AnixOps/anix-control/sdk/forward/driver"
)

// Bandwidth limits with tc (forward-sdk.md section 6.1). On every
// Config.LimitInterfaces device the driver owns one HTB root qdisc with the
// major handle Config.TCHandle, and for each rate-limited hop two classes,
// minor 2*mark (up: the hop's packets in the original direction) and
// 2*mark+1 (down: reply packets), each at the hop's bandwidth_bps. One fw
// filter per class selects it by the packet mark the _acct chain sets: the
// hop's mark, plus Config.DirectionBit on reply packets, within
// MarkMask|DirectionBit. Unclassified traffic is not shaped (HTB default 0).
// Both directions of a forwarded connection leave the node as egress, so
// no ingress qdisc or ifb is needed.
//
// The driver touches only that qdisc, its classes and filters. A root qdisc
// with another non-zero handle on a limit interface is foreign: an artifact
// with rate-limited hops is then ErrConflict. The kernel's default root
// qdisc (handle 0:) is replaced, and comes back when the driver deletes its
// own.

const tcPrio = "10"

// tcClassSpec is one class the artifact needs.
type tcClassSpec struct {
	minor uint16
	bps   uint64
	mark  uint32 // the packet mark its filter matches
}

// tcIface is what the driver owns on one interface now.
type tcIface struct {
	name    string
	exists  bool              // the interface exists
	qdisc   bool              // the driver's root qdisc is there
	foreign string            // a foreign root qdisc ("htb 1:"), or ""
	classes map[uint16]uint64 // minor -> rate in bytes per second
	filters map[uint32]uint16 // mark -> class minor
}

// tcStep is one tc command and the command that undoes it.
type tcStep struct {
	args, undo []string
}

// tcPlan is what Apply runs: pre before the nft transaction, post after it,
// undo of pre when the transaction fails.
type tcPlan struct {
	pre, post []tcStep
}

func (p *tcPlan) noop() bool { return len(p.pre) == 0 && len(p.post) == 0 }

func (d *Driver) tcMajor() string { return strconv.FormatUint(uint64(d.cfg.TCHandle), 16) + ":" }

func (d *Driver) tcClassID(minor uint16) string {
	return d.tcMajor() + strconv.FormatUint(uint64(minor), 16)
}

// tcSpecs answers the classes the manifest's rate-limited hops need.
func (d *Driver) tcSpecs(m *manifest) []tcClassSpec {
	var out []tcClassSpec
	if m == nil {
		return nil
	}
	for _, h := range m.hops {
		if h.bandwidth == 0 {
			continue
		}
		mark := h.mark << d.cfg.markShift()
		out = append(out,
			tcClassSpec{minor: uint16(2 * h.mark), bps: h.bandwidth, mark: mark},                        // #nosec G115 -- mark <= maxTCMark
			tcClassSpec{minor: uint16(2*h.mark + 1), bps: h.bandwidth, mark: mark | d.cfg.DirectionBit}, // #nosec G115 -- as above
		)
	}
	return out
}

// readTC reads what the driver owns on one interface.
func (d *Driver) readTC(ctx context.Context, name string) (*tcIface, error) {
	t := &tcIface{name: name, classes: map[uint16]uint64{}, filters: map[uint32]uint16{}}
	out, err := d.run(ctx, "tc", nil, "-j", "qdisc", "show", "dev", name)
	if err != nil {
		if s := stderrOf(err); strings.Contains(s, "Cannot find device") || strings.Contains(s, "does not exist") {
			return t, nil
		}
		return nil, err
	}
	t.exists = true
	var qdiscs []map[string]any
	if err := decodeJSON(out, &qdiscs); err != nil {
		return nil, fmt.Errorf("nftables driver: tc qdisc show %s: %w", name, err)
	}
	for _, q := range qdiscs {
		if root, _ := q["root"].(bool); !root {
			continue
		}
		kind, _ := q["kind"].(string)
		handle, _ := q["handle"].(string)
		switch {
		case handle == d.tcMajor() && kind == "htb":
			t.qdisc = true
		case handle != "0:" && handle != "":
			t.foreign = kind + " " + handle
		}
	}
	if !t.qdisc {
		return t, nil
	}
	out, err = d.run(ctx, "tc", nil, "-j", "class", "show", "dev", name)
	if err != nil {
		return nil, err
	}
	var classes []map[string]any
	if err := decodeJSON(out, &classes); err != nil {
		return nil, fmt.Errorf("nftables driver: tc class show %s: %w", name, err)
	}
	for _, c := range classes {
		h, _ := c["handle"].(string)
		minor, ok := d.ownMinor(h)
		if !ok {
			continue
		}
		t.classes[minor] = item{fields: c}.num("rate")
	}
	out, err = d.run(ctx, "tc", nil, "-j", "filter", "show", "dev", name, "parent", d.tcMajor())
	if err != nil {
		return nil, err
	}
	var filters []map[string]any
	if err := decodeJSON(out, &filters); err != nil {
		return nil, fmt.Errorf("nftables driver: tc filter show %s: %w", name, err)
	}
	for _, f := range filters {
		mark, minor, ok := d.parseFilter(f)
		if ok {
			t.filters[mark] = minor
		}
	}
	return t, nil
}

// ownMinor answers the minor of one of the driver's class ids.
func (d *Driver) ownMinor(classID string) (uint16, bool) {
	rest, ok := strings.CutPrefix(classID, d.tcMajor())
	if !ok || rest == "" {
		return 0, false
	}
	n, err := strconv.ParseUint(rest, 16, 16)
	if err != nil || n == 0 {
		return 0, false
	}
	return uint16(n), true
}

// parseFilter reads a fw filter of `tc -j filter show`: iproute2 prints
// {"options":{"fw":{"mark":"0x10000","mask":"0xfff0001"},"classid":"af00:2"}}
// (older versions "handle":"0x10000/0xfff0001").
func (d *Driver) parseFilter(f map[string]any) (uint32, uint16, bool) {
	if k, _ := f["kind"].(string); k != "fw" {
		return 0, 0, false
	}
	opts, _ := f["options"].(map[string]any)
	if opts == nil {
		return 0, 0, false
	}
	classID, _ := opts["classid"].(string)
	if classID == "" {
		classID, _ = opts["flowid"].(string)
	}
	minor, ok := d.ownMinor(classID)
	if !ok {
		return 0, 0, false
	}
	var markText string
	if fw, ok := opts["fw"].(map[string]any); ok {
		markText, _ = fw["mark"].(string)
	}
	if markText == "" {
		h, _ := f["handle"].(string)
		if h == "" {
			h, _ = opts["handle"].(string)
		}
		markText, _, _ = strings.Cut(h, "/")
	}
	mark, err := strconv.ParseUint(markText, 0, 32)
	if err != nil {
		return 0, 0, false
	}
	return uint32(mark), minor, true
}

// planTC compares what the driver owns on its limit interfaces with what
// the manifest needs (nil: nothing). It refuses with ErrConflict when a
// rate-limited hop needs an interface whose root qdisc is foreign.
func (d *Driver) planTC(ctx context.Context, m *manifest) (*tcPlan, error) {
	specs := d.tcSpecs(m)
	p := &tcPlan{}
	if len(specs) > 0 && len(d.cfg.LimitInterfaces) == 0 {
		return nil, fmt.Errorf("%w: rate-limited hops need Config.LimitInterfaces", driver.ErrUnsupported)
	}
	for _, name := range d.cfg.LimitInterfaces {
		t, err := d.readTC(ctx, name)
		if err != nil {
			return nil, err
		}
		if len(specs) > 0 {
			if !t.exists {
				return nil, fmt.Errorf("%w: limit interface %s does not exist", driver.ErrUnsupported, name)
			}
			if t.foreign != "" {
				return nil, fmt.Errorf("%w: interface %s has a foreign root qdisc %s; the driver needs its own (%s htb) for bandwidth limits", driver.ErrConflict, name, t.foreign, d.tcMajor())
			}
		}
		d.planIface(p, t, specs)
	}
	return p, nil
}

func (d *Driver) planIface(p *tcPlan, t *tcIface, specs []tcClassSpec) {
	dev := []string{"dev", t.name}
	major := d.tcMajor()
	filterArgs := func(verb string, mark uint32, minor uint16) []string {
		a := append([]string{"filter", verb}, dev...)
		a = append(a, "parent", major, "protocol", "all", "prio", tcPrio,
			"handle", fmt.Sprintf("%#x/%#x", mark, d.cfg.MarkMask|d.cfg.DirectionBit), "fw")
		if verb != "del" {
			a = append(a, "classid", d.tcClassID(minor))
		}
		return a
	}
	classArgs := func(verb string, minor uint16, bps uint64) []string {
		a := append([]string{"class", verb}, dev...)
		a = append(a, "parent", major, "classid", d.tcClassID(minor))
		if verb != "del" {
			rate := strconv.FormatUint(bps, 10) + "bit"
			a = append(a, "htb", "rate", rate, "ceil", rate, "quantum", strconv.FormatUint(quantum(bps), 10))
		}
		return a
	}
	if len(specs) > 0 && !t.qdisc {
		p.pre = append(p.pre, tcStep{
			args: append(append([]string{"qdisc", "add"}, dev...), "root", "handle", major, "htb", "default", "0"),
			undo: append(append([]string{"qdisc", "del"}, dev...), "root", "handle", major),
		})
	}
	want := map[uint16]tcClassSpec{}
	wantFilters := map[uint32]uint16{}
	for _, s := range specs {
		want[s.minor] = s
		wantFilters[s.mark] = s.minor
		old, ok := t.classes[s.minor]
		switch {
		case !ok:
			p.pre = append(p.pre, tcStep{args: classArgs("add", s.minor, s.bps), undo: classArgs("del", s.minor, 0)})
		case old != s.bps/8:
			p.pre = append(p.pre, tcStep{args: classArgs("change", s.minor, s.bps), undo: classArgs("change", s.minor, old*8)})
		}
	}
	for _, s := range specs {
		old, ok := t.filters[s.mark]
		switch {
		case !ok:
			p.pre = append(p.pre, tcStep{args: filterArgs("add", s.mark, s.minor), undo: filterArgs("del", s.mark, 0)})
		case old != s.minor:
			p.pre = append(p.pre, tcStep{args: filterArgs("replace", s.mark, s.minor), undo: filterArgs("replace", s.mark, old)})
		}
	}
	if !t.qdisc {
		return
	}
	if len(specs) == 0 {
		// Deleting the root qdisc takes its classes and filters with it.
		p.post = append(p.post, tcStep{args: append(append([]string{"qdisc", "del"}, dev...), "root", "handle", major)})
		return
	}
	for _, mark := range slices.Sorted(maps.Keys(t.filters)) {
		if _, ok := wantFilters[mark]; !ok {
			p.post = append(p.post, tcStep{args: filterArgs("del", mark, 0)})
		}
	}
	for _, minor := range slices.Sorted(maps.Keys(t.classes)) {
		if _, ok := want[minor]; !ok {
			p.post = append(p.post, tcStep{args: classArgs("del", minor, 0)})
		}
	}
}

// quantum answers an HTB quantum for a rate: rate/10 bytes, as HTB's
// default r2q would, clamped to what HTB accepts without a warning.
func quantum(bps uint64) uint64 {
	return min(max(bps/8/10, 1600), 200000)
}

// runTC runs steps in order. On a failure it undoes the steps that ran, in
// reverse, and answers the error.
func (d *Driver) runTC(ctx context.Context, steps []tcStep) (ran int, err error) {
	for i, s := range steps {
		if _, err := d.run(ctx, "tc", nil, s.args...); err != nil {
			d.undoTC(steps[:i])
			return i, err
		}
	}
	return len(steps), nil
}

// undoTC undoes steps in reverse, best effort and whatever the caller's
// context says: it restores the previous state.
func (d *Driver) undoTC(steps []tcStep) {
	ctx, cancel := context.WithTimeout(context.Background(), undoTimeout)
	defer cancel()
	for i := len(steps) - 1; i >= 0; i-- {
		if steps[i].undo != nil {
			_, _ = d.run(ctx, "tc", nil, steps[i].undo...)
		}
	}
}

// removeTC deletes the driver's root qdisc on every limit interface.
func (d *Driver) removeTC(ctx context.Context) error {
	p, err := d.planTC(ctx, nil)
	if err != nil {
		return err
	}
	_, err = d.runTC(ctx, p.post)
	return err
}

// tcListing answers the driver's tc objects as stable strings, for tests
// and diagnostics.
func (d *Driver) tcListing(ctx context.Context) ([]string, error) {
	var out []string
	for _, name := range d.cfg.LimitInterfaces {
		t, err := d.readTC(ctx, name)
		if err != nil {
			return nil, err
		}
		if !t.qdisc {
			continue
		}
		out = append(out, fmt.Sprintf("tc %s qdisc htb %s", name, d.tcMajor()))
		for _, minor := range slices.Sorted(maps.Keys(t.classes)) {
			out = append(out, fmt.Sprintf("tc %s class %s rate %dbit", name, d.tcClassID(minor), t.classes[minor]*8))
		}
		for _, mark := range slices.SortedFunc(maps.Keys(t.filters), cmp.Compare[uint32]) {
			out = append(out, fmt.Sprintf("tc %s filter %#x -> %s", name, mark, d.tcClassID(t.filters[mark])))
		}
	}
	return out, nil
}

func decodeJSON(b []byte, v any) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	return dec.Decode(v)
}
