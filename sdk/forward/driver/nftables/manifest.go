package nftables

import (
	"bufio"
	"bytes"
	"fmt"
	"net/netip"
	"regexp"
	"slices"
	"strconv"
	"strings"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/driver"
)

// The manifest is a block of comment lines Render writes after the header:
// what Apply needs about each hop that the nft objects do not carry (every
// rendered upstream with its weight and priority, failover's backups
// included; the strategy; the bandwidth for tc; the listener for the
// conflict check). nft ignores comments, so the script stays one
// transaction, and the digest covers the manifest. Like the rest of the
// script it holds checked literals only, never the state's identity.
//
//	# anixops-hop route=<id> hop=<n> mark=<n> balance=<STRATEGY> listen=<tcp|udp|tcp,udp>/<port> address=<addr|any> bandwidth=<bps>
//	# anixops-upstream route=<id> hop=<n> address=<addr> port=<n> weight=<n> priority=<n>
const (
	manifestHop      = "# anixops-hop "
	manifestUpstream = "# anixops-upstream "
)

// writeManifest writes the manifest lines of the hops.
func (d *Driver) writeManifest(w *writer, plans []*hopPlan) {
	for _, p := range plans {
		var proto []string
		if p.tcp {
			proto = append(proto, "tcp")
		}
		if p.udp {
			proto = append(proto, "udp")
		}
		addr := "any"
		if p.listen.IsValid() {
			addr = p.listen.String()
		}
		var bw uint64
		if p.bandwidth {
			bw = p.bandwidthBps
		}
		w.line("%sroute=%s hop=%d mark=%d balance=%s listen=%s/%d address=%s bandwidth=%d",
			manifestHop, p.key.RouteID, p.key.HopIndex, p.markIndex, p.balance, strings.Join(proto, ","), p.port, addr, bw)
		for _, ups := range [][]upstream{p.ups4, p.ups6} {
			for _, u := range ups {
				w.line("%sroute=%s hop=%d address=%s port=%d weight=%d priority=%d",
					manifestUpstream, p.key.RouteID, p.key.HopIndex, u.addr, u.port, u.weight, u.priority)
			}
		}
	}
}

// manifest is what Apply reads back from an artifact.
type manifest struct {
	hops []*manifestHopInfo
	// declared lists the objects the script declares, as "kind name".
	declared map[string]bool
}

type manifestHopInfo struct {
	key       driver.HopKey
	mark      uint32
	balance   forwardv1.BalanceStrategy
	tcp, udp  bool
	port      uint32
	listen    netip.Addr // invalid for any
	bandwidth uint64
	ups       []upstream // rendered order: IPv4 then IPv6, each by priority, address, port
}

func (h *manifestHopInfo) base() string { return hopName(h.key) }

func (h *manifestHopInfo) listenText() string {
	var protos []string
	if h.tcp {
		protos = append(protos, "tcp")
	}
	if h.udp {
		protos = append(protos, "udp")
	}
	return strings.Join(protos, ",") + "/" + strconv.FormatUint(uint64(h.port), 10)
}

// declaration matches an object declaration at the first depth of a table
// block of the rendered script.
var declaration = regexp.MustCompile(`^\t(chain|counter|quota|set|map) ([A-Za-z0-9_]+) \{$`)

// parseManifest reads the manifest and the declared objects of an artifact
// this driver rendered, and checks them against the artifact's hops.
func parseManifest(a driver.Artifact) (*manifest, error) {
	bad := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", driver.ErrInvalidArtifact, fmt.Sprintf(format, args...))
	}
	m := &manifest{declared: map[string]bool{}}
	byKey := map[driver.HopKey]*manifestHopInfo{}
	sc := bufio.NewScanner(bytes.NewReader(a.Content))
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, manifestHop):
			h, err := parseHopLine(strings.TrimPrefix(line, manifestHop))
			if err != nil {
				return nil, bad("line %d: %v", n, err)
			}
			if byKey[h.key] != nil {
				return nil, bad("line %d: hop %s twice", n, h.key)
			}
			byKey[h.key] = h
			m.hops = append(m.hops, h)
		case strings.HasPrefix(line, manifestUpstream):
			k, u, err := parseUpstreamLine(strings.TrimPrefix(line, manifestUpstream))
			if err != nil {
				return nil, bad("line %d: %v", n, err)
			}
			h := byKey[k]
			if h == nil {
				return nil, bad("line %d: upstream of unknown hop %s", n, k)
			}
			h.ups = append(h.ups, u)
		default:
			if g := declaration.FindStringSubmatch(line); g != nil {
				m.declared[g[1]+" "+g[2]] = true
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, bad("%v", err)
	}
	keys := make([]driver.HopKey, len(m.hops))
	for i, h := range m.hops {
		keys[i] = h.key
		if len(h.ups) == 0 {
			return nil, bad("hop %s without upstreams", h.key)
		}
	}
	slices.SortFunc(keys, driver.HopKey.Compare)
	if !slices.Equal(keys, a.Hops) {
		return nil, bad("manifest hops %v, artifact hops %v", keys, a.Hops)
	}
	slices.SortFunc(m.hops, func(x, y *manifestHopInfo) int { return x.key.Compare(y.key) })
	return m, nil
}

// fields splits "k=v k=v" into a map, refusing repeats and junk.
func fields(s string) (map[string]string, error) {
	out := map[string]string{}
	for _, f := range strings.Fields(s) {
		k, v, ok := strings.Cut(f, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("field %q is not key=value", f)
		}
		if _, dup := out[k]; dup {
			return nil, fmt.Errorf("field %q twice", k)
		}
		out[k] = v
	}
	return out, nil
}

func parseKey(f map[string]string) (driver.HopKey, error) {
	route := f["route"]
	if !routeIDPattern.MatchString(route) {
		return driver.HopKey{}, fmt.Errorf("bad route id %q", route)
	}
	idx, err := strconv.ParseUint(f["hop"], 10, 32)
	if err != nil {
		return driver.HopKey{}, fmt.Errorf("bad hop index %q", f["hop"])
	}
	return driver.HopKey{RouteID: route, HopIndex: uint32(idx)}, nil
}

func parseHopLine(s string) (*manifestHopInfo, error) {
	f, err := fields(s)
	if err != nil {
		return nil, err
	}
	k, err := parseKey(f)
	if err != nil {
		return nil, err
	}
	h := &manifestHopInfo{key: k}
	mark, err := strconv.ParseUint(f["mark"], 10, 32)
	if err != nil || mark == 0 {
		return nil, fmt.Errorf("bad mark %q", f["mark"])
	}
	h.mark = uint32(mark)
	b, ok := forwardv1.BalanceStrategy_value[f["balance"]]
	if !ok || !slices.Contains(AllStrategies(), forwardv1.BalanceStrategy(b)) {
		return nil, fmt.Errorf("bad balance %q", f["balance"])
	}
	h.balance = forwardv1.BalanceStrategy(b)
	protos, port, ok := strings.Cut(f["listen"], "/")
	if !ok {
		return nil, fmt.Errorf("bad listen %q", f["listen"])
	}
	for _, p := range strings.Split(protos, ",") {
		switch p {
		case "tcp":
			h.tcp = true
		case "udp":
			h.udp = true
		default:
			return nil, fmt.Errorf("bad listen protocol %q", p)
		}
	}
	pn, err := strconv.ParseUint(port, 10, 16)
	if err != nil || pn == 0 {
		return nil, fmt.Errorf("bad listen port %q", port)
	}
	h.port = uint32(pn)
	if a := f["address"]; a != "any" {
		addr, err := netip.ParseAddr(a)
		if err != nil {
			return nil, fmt.Errorf("bad listen address %q", a)
		}
		h.listen = addr
	}
	if h.bandwidth, err = strconv.ParseUint(f["bandwidth"], 10, 64); err != nil {
		return nil, fmt.Errorf("bad bandwidth %q", f["bandwidth"])
	}
	return h, nil
}

func parseUpstreamLine(s string) (driver.HopKey, upstream, error) {
	f, err := fields(s)
	if err != nil {
		return driver.HopKey{}, upstream{}, err
	}
	k, err := parseKey(f)
	if err != nil {
		return driver.HopKey{}, upstream{}, err
	}
	addr, err := netip.ParseAddr(f["address"])
	if err != nil {
		return driver.HopKey{}, upstream{}, fmt.Errorf("bad upstream address %q", f["address"])
	}
	var u upstream
	u.addr = addr
	nums := []struct {
		name string
		dst  *uint32
		bits int
	}{{"port", &u.port, 16}, {"weight", &u.weight, 32}, {"priority", &u.priority, 32}}
	for _, n := range nums {
		v, err := strconv.ParseUint(f[n.name], 10, n.bits)
		if err != nil {
			return driver.HopKey{}, upstream{}, fmt.Errorf("bad upstream %s %q", n.name, f[n.name])
		}
		*n.dst = uint32(v) // #nosec G115 -- parsed with at most 32 bits
	}
	if u.port == 0 || u.weight == 0 {
		return driver.HopKey{}, upstream{}, fmt.Errorf("upstream %s port or weight zero", addr)
	}
	return k, u, nil
}
