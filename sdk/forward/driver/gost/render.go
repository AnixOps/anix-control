package gost

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/netip"
	"slices"
	"strconv"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/driver"
)

// Render turns the state's gost hops into one gost configuration (JSON);
// see the package documentation for its shape. Hops it cannot run come
// back as hop errors in a *driver.RenderError next to the artifact of the
// others.
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
	content, err := d.render(plans)
	if err != nil {
		return driver.Artifact{}, err
	}
	return driver.NewArtifact(d.Engine(), state, keys, content), driver.NewRenderError(hopErrs)
}

// The configuration's shape: the subset of gost v3's configuration
// (github.com/go-gost/x/config) the driver writes, with gost's own key
// spellings. Field order is fixed, so encoding/json writes the same bytes
// for the same plans.
type (
	gostConfig struct {
		// AnixOps is the manifest: what Apply, Observe and SetUpstreams
		// need and the gost objects do not say. gost ignores the key.
		AnixOps    *manifest     `json:"anixops"`
		Services   []service     `json:"services,omitempty"`
		Chains     []chain       `json:"chains,omitempty"`
		Admissions []admission   `json:"admissions,omitempty"`
		Limiters   []limiter     `json:"limiters,omitempty"`
		CLimiters  []limiter     `json:"climiters,omitempty"`
		Log        *logConfig    `json:"log,omitempty"`
		Metrics    *metricsBlock `json:"metrics,omitempty"`
	}
	service struct {
		Name      string    `json:"name"`
		Addr      string    `json:"addr"`
		Admission string    `json:"admission,omitempty"`
		Limiter   string    `json:"limiter,omitempty"`
		CLimiter  string    `json:"climiter,omitempty"`
		Handler   handler   `json:"handler"`
		Listener  endpoint  `json:"listener"`
		Forwarder forwarder `json:"forwarder"`
	}
	handler struct {
		Type  string `json:"type"`
		Chain string `json:"chain,omitempty"`
	}
	// endpoint is a listener or a dialer.
	endpoint struct {
		Type     string            `json:"type"`
		TLS      *tlsConfig        `json:"tls,omitempty"`
		Metadata map[string]string `json:"metadata,omitempty"`
	}
	tlsConfig struct {
		CertFile   string `json:"certFile"`
		KeyFile    string `json:"keyFile"`
		CAFile     string `json:"caFile"`
		Secure     bool   `json:"secure,omitempty"`
		ServerName string `json:"serverName,omitempty"`
	}
	forwarder struct {
		Nodes    []node    `json:"nodes"`
		Selector *selector `json:"selector,omitempty"`
	}
	node struct {
		Name      string            `json:"name"`
		Addr      string            `json:"addr"`
		Connector *endpoint         `json:"connector,omitempty"`
		Dialer    *endpoint         `json:"dialer,omitempty"`
		Metadata  map[string]string `json:"metadata,omitempty"`
	}
	selector struct {
		Strategy    string `json:"strategy"`
		MaxFails    uint32 `json:"maxFails"`
		FailTimeout string `json:"failTimeout"`
	}
	chain struct {
		Name string     `json:"name"`
		Hops []chainHop `json:"hops"`
	}
	chainHop struct {
		Name     string    `json:"name"`
		Selector *selector `json:"selector"`
		Nodes    []node    `json:"nodes"`
	}
	admission struct {
		Name      string   `json:"name"`
		Whitelist bool     `json:"whitelist"`
		Matchers  []string `json:"matchers"`
	}
	limiter struct {
		Name   string   `json:"name"`
		Limits []string `json:"limits"`
	}
	logConfig struct {
		Level  string `json:"level"`
		Format string `json:"format"`
		Output string `json:"output"`
	}
	metricsBlock struct {
		Addr string `json:"addr"`
		Path string `json:"path"`
	}
)

// placeholder is the one forwarder node of a hop whose upstreams are
// dialled through a chain: the handler needs a target, the chain picks the
// next node and gost's relay connector sends this address, which the next
// node's relay handler ignores because it forwards to its own upstreams.
const placeholder = "0.0.0.0:0"

// render writes the configuration of the plans, sorted by key.
func (d *Driver) render(plans []*hopPlan) ([]byte, error) {
	cfg := &gostConfig{AnixOps: &manifest{Driver: OwnerMark, Note: manifestNote, Hops: []manifestHop{}}}
	for _, p := range plans {
		d.renderHop(cfg, p)
	}
	if len(plans) > 0 {
		cfg.Log = &logConfig{Level: "warn", Format: "json", Output: "stderr"}
		// The metrics path names the configuration (a hash of everything
		// above it), so Apply knows gost serves this one and not the
		// previous: a reload that failed keeps the old path.
		body, err := encode(cfg)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(body)
		cfg.Metrics = &metricsBlock{Addr: "unix://" + d.cfg.metricsPath(), Path: "/anixops-" + hex.EncodeToString(sum[:8])}
	}
	return encode(cfg)
}

func encode(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, fmt.Errorf("gost driver: encode configuration: %w", err)
	}
	return b.Bytes(), nil
}

// renderHop adds a hop's services, chain, admission and limiters.
func (d *Driver) renderHop(cfg *gostConfig, p *hopPlan) {
	mh := manifestHop{
		Route:   p.key.RouteID,
		Hop:     p.key.HopIndex,
		Balance: p.balance.String(),
		Paused:  p.paused,
	}
	var adm, lim, clim string
	if len(p.sources) > 0 || p.paused {
		adm = p.name
		a := admission{Name: adm, Whitelist: true, Matchers: []string{}}
		if !p.paused { // a paused hop admits nobody: it keeps its listener and refuses every connection
			for _, s := range p.sources {
				a.Matchers = append(a.Matchers, prefixText(s))
			}
		}
		cfg.Admissions = append(cfg.Admissions, a)
	}
	if p.bandwidthBps > 0 {
		lim = p.name
		rate := strconv.FormatUint(max(1, p.bandwidthBps/8), 10) + "B"
		cfg.Limiters = append(cfg.Limiters, limiter{Name: lim, Limits: []string{"$ " + rate + " " + rate}})
	}
	if p.maxConns > 0 {
		clim = p.name
		cfg.CLimiters = append(cfg.CLimiters, limiter{Name: clim, Limits: []string{"$ " + strconv.FormatUint(uint64(p.maxConns), 10)}})
	}

	sel := &selector{Strategy: strategyName(p.balance), MaxFails: p.maxFails, FailTimeout: p.failTimeout.String()}
	nodes := d.nodes(p)
	fwd := forwarder{Nodes: nodes, Selector: sel}
	h := handler{}
	if p.chain {
		fwd = forwarder{Nodes: []node{{Name: "next", Addr: placeholder}}}
		h.Chain = p.name
		cfg.Chains = append(cfg.Chains, chain{Name: p.name, Hops: []chainHop{{Name: p.name, Selector: sel, Nodes: nodes}}})
	}

	addr := ":" + strconv.FormatUint(uint64(p.port), 10)
	if p.listen.IsValid() {
		addr = netip.AddrPortFrom(p.listen, uint16(p.port)).String() // #nosec G115 -- checked to be a port
	}
	for _, l := range p.listeners {
		s := service{
			Name:      p.name + "-" + l.gostType,
			Addr:      addr,
			Admission: adm,
			Limiter:   lim,
			CLimiter:  clim,
			Handler:   handler{Type: l.handler, Chain: h.Chain},
			Listener:  d.listenerEndpoint(l, p.ingress),
			Forwarder: fwd,
		}
		cfg.Services = append(cfg.Services, s)
		mh.Services = append(mh.Services, s.Name)
		ml := manifestListener{Network: l.network, Port: p.port}
		if p.listen.IsValid() {
			ml.Address = p.listen.String()
		}
		mh.Listeners = append(mh.Listeners, ml)
	}
	for _, u := range p.upstreams {
		mh.Upstreams = append(mh.Upstreams, manifestUpstream{Address: u.addr.String(), Port: u.port, Weight: u.weight, Priority: u.priority})
	}
	cfg.AnixOps.Hops = append(cfg.AnixOps.Hops, mh)
}

// strategyName maps a balance strategy to gost's selector strategy.
// LEAST_CONN is weighted random, which the Agent re-weights every
// model.DefaultLeastConnReweight (H21) through SetUpstreams (F4b, L1).
func strategyName(s forwardv1.BalanceStrategy) string {
	switch s {
	case forwardv1.BalanceStrategy_BALANCE_STRATEGY_RANDOM, forwardv1.BalanceStrategy_BALANCE_STRATEGY_LEAST_CONN:
		return "rand"
	case forwardv1.BalanceStrategy_BALANCE_STRATEGY_IP_HASH:
		return "hash"
	case forwardv1.BalanceStrategy_BALANCE_STRATEGY_FAILOVER:
		return "fifo"
	}
	return "round"
}

// nodes answers a hop's gost nodes: one per upstream, named u<index>, in
// the order its strategy needs. FAILOVER lists them by priority (ties in
// state order) for gost's fifo; RANDOM and LEAST_CONN carry the weight as
// metadata; weighted ROUND_ROBIN and IP_HASH list an upstream once per
// entry of spread (u<index>-<k>), since gost's round and hash ignore
// weights.
func (d *Driver) nodes(p *hopPlan) []node {
	ups := slices.Clone(p.upstreams)
	mk := func(u upstream, name string) node {
		n := node{Name: name, Addr: u.addrPort().String()}
		if !u.egress.raw() {
			n.Connector = &endpoint{Type: "relay"}
			n.Dialer = d.dialerEndpoint(u.egress)
		}
		return n
	}
	name := func(u upstream) string { return "u" + strconv.Itoa(u.index) }
	switch p.balance {
	case forwardv1.BalanceStrategy_BALANCE_STRATEGY_FAILOVER:
		slices.SortStableFunc(ups, func(a, b upstream) int {
			switch {
			case a.priority < b.priority:
				return -1
			case a.priority > b.priority:
				return 1
			}
			return 0
		})
	case forwardv1.BalanceStrategy_BALANCE_STRATEGY_RANDOM, forwardv1.BalanceStrategy_BALANCE_STRATEGY_LEAST_CONN:
		out := make([]node, 0, len(ups))
		for _, u := range ups {
			n := mk(u, name(u))
			n.Metadata = map[string]string{"weight": strconv.FormatUint(uint64(u.weight), 10)}
			out = append(out, n)
		}
		return out
	default: // ROUND_ROBIN, IP_HASH
		weights := make([]uint32, len(ups))
		equal := true
		for i, u := range ups {
			weights[i] = u.weight
			equal = equal && u.weight == ups[0].weight
		}
		if !equal {
			order := spread(weights, MaxEntries)
			count := make([]int, len(ups))
			out := make([]node, 0, len(order))
			for _, i := range order {
				out = append(out, mk(ups[i], name(ups[i])+"-"+strconv.Itoa(count[i])))
				count[i]++
			}
			return out
		}
	}
	out := make([]node, 0, len(ups))
	for _, u := range ups {
		out = append(out, mk(u, name(u)))
	}
	return out
}

// listenerEndpoint answers a service's listener: plain for RAW, the link's
// carrier with the node's link certificate and the CA its peers' client
// certificates must chain to (mutual TLS) otherwise.
func (d *Driver) listenerEndpoint(l listener, in link) endpoint {
	e := endpoint{Type: l.gostType}
	if in.encrypted() {
		e.TLS = &tlsConfig{CertFile: d.cfg.LinkCert, KeyFile: d.cfg.LinkKey, CAFile: d.cfg.LinkCA}
		if in.path != "" {
			e.Metadata = map[string]string{"path": in.path}
		}
	}
	if l.gostType == "udp" {
		// keepalive keeps a client's session for ttl after its last
		// datagram (without it gost answers once and closes); 64 KiB
		// reads take any datagram whole.
		e.Metadata = map[string]string{"keepalive": "true", "ttl": "60s", "readBufferSize": "65536"}
	}
	return e
}

// dialerEndpoint answers the dialer of a relayed upstream: the carrier,
// and for encrypted links the node's link certificate as client
// certificate and verification of the server's for the server name.
func (d *Driver) dialerEndpoint(out link) *endpoint {
	e := &endpoint{Type: out.gostType()}
	if out.encrypted() {
		e.TLS = &tlsConfig{CertFile: d.cfg.LinkCert, KeyFile: d.cfg.LinkKey, CAFile: d.cfg.LinkCA, Secure: true, ServerName: out.serverName}
		switch out.security {
		case forwardv1.LinkSecurity_LINK_SECURITY_WSS, forwardv1.LinkSecurity_LINK_SECURITY_GRPC:
			e.Metadata = map[string]string{"host": out.serverName}
			if out.path != "" {
				e.Metadata["path"] = out.path
			}
		}
	}
	return e
}

// prefixText formats an admission matcher: a bare address for a single
// address, the prefix otherwise.
func prefixText(p netip.Prefix) string {
	if p.IsSingleIP() {
		return p.Addr().String()
	}
	return p.String()
}
