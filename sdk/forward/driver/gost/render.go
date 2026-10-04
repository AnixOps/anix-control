package gost

import (
	"bytes"
	"cmp"
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
//
// The objects split in two. Services, chains, the log and the API are the
// configuration's structure: changing them needs gost to re-create its
// services (a reload), and the metrics path names them. Hops, admissions
// and limiters are referenced by name from the structure and resolved by
// gost at every connection, so Apply and SetUpstreams replace them
// through the web API without re-creating a service (hot objects).
type (
	gostConfig struct {
		// AnixOps is the manifest: what Apply, Observe and SetUpstreams
		// need and the gost objects do not say. gost ignores the key.
		AnixOps    *manifest     `json:"anixops"`
		Services   []service     `json:"services,omitempty"`
		Chains     []chain       `json:"chains,omitempty"`
		Hops       []hopConfig   `json:"hops,omitempty"`
		Admissions []admission   `json:"admissions,omitempty"`
		Limiters   []limiter     `json:"limiters,omitempty"`
		CLimiters  []limiter     `json:"climiters,omitempty"`
		Log        *logConfig    `json:"log,omitempty"`
		API        *apiBlock     `json:"api,omitempty"`
		Metrics    *metricsBlock `json:"metrics,omitempty"`
	}
	service struct {
		Name      string          `json:"name"`
		Addr      string          `json:"addr"`
		Admission string          `json:"admission,omitempty"`
		Limiter   string          `json:"limiter,omitempty"`
		CLimiter  string          `json:"climiter,omitempty"`
		Metadata  serviceMetadata `json:"metadata"`
		Handler   handler         `json:"handler"`
		Listener  endpoint        `json:"listener"`
		Forwarder forwarder       `json:"forwarder"`
	}
	// serviceMetadata turns on the service's statistics (connections and
	// bytes), which gost's web API reports and Observe reads.
	serviceMetadata struct {
		EnableStats bool `json:"enableStats"`
	}
	handler struct {
		Type     string            `json:"type"`
		Chain    string            `json:"chain,omitempty"`
		Metadata map[string]string `json:"metadata,omitempty"`
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
	// forwarder names the hop whose nodes a RAW service dials, or holds
	// the placeholder node of a service that dials through a chain.
	forwarder struct {
		Hop   string `json:"hop,omitempty"`
		Nodes []node `json:"nodes,omitempty"`
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
	// hopConfig is a top-level hop: the upstreams of one gost hop with
	// their selector. Services (RAW) and chains (relayed) name it, so
	// SetUpstreams replaces it alone. Metadata carries the rotation a
	// SetUpstreams put there (rotationKey); Render never sets it.
	hopConfig struct {
		Name     string            `json:"name"`
		Selector *selector         `json:"selector"`
		Nodes    []node            `json:"nodes"`
		Metadata map[string]string `json:"metadata,omitempty"`
	}
	chain struct {
		Name string     `json:"name"`
		Hops []chainHop `json:"hops"`
	}
	// chainHop names a top-level hop.
	chainHop struct {
		Name string `json:"name"`
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
	// apiBlock is gost's web API, on a unix socket in the runtime
	// directory and without authentication: the socket's permissions are
	// its only key (the unit's UMask 0007 and RuntimeDirectoryMode 0750
	// leave it to the gost user and its group, which the Agent is in).
	apiBlock struct {
		Addr string `json:"addr"`
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
		cfg.API = &apiBlock{Addr: "unix://" + d.cfg.apiPath()}
		path, err := structurePath(cfg)
		if err != nil {
			return nil, err
		}
		cfg.Metrics = &metricsBlock{Addr: "unix://" + d.cfg.metricsPath(), Path: path}
	}
	return encode(cfg)
}

// structurePath answers the metrics path of a configuration: a hash of its
// structure (services, chains, log, API), so gost serves a new path
// exactly when a start or a reload loaded a new structure (a reload that
// failed keeps the old one), and a change to hot objects alone keeps it.
func structurePath(cfg *gostConfig) (string, error) {
	body, err := encode(&gostConfig{Services: cfg.Services, Chains: cfg.Chains, Log: cfg.Log, API: cfg.API})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	return "/anixops-" + hex.EncodeToString(sum[:8]), nil
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

// renderHop adds a hop's services, hop, chain, admission and limiters.
func (d *Driver) renderHop(cfg *gostConfig, p *hopPlan) {
	mh := manifestHop{
		Route:   p.key.RouteID,
		Hop:     p.key.HopIndex,
		Balance: p.balance.String(),
		Paused:  p.paused,
		Quota:   p.quotaBytes,
	}
	// Every hop has an admission, so pausing and resuming it, and the soft
	// quota, replace the admission alone and never a service: a paused hop
	// keeps its listener and its counters (and their epoch) and admits
	// nobody; an entry without sources admits everybody (an empty
	// blacklist).
	adm := admissionOf(p)
	cfg.Admissions = append(cfg.Admissions, adm)
	var lim, clim string
	if p.bandwidthBps > 0 {
		lim = p.name
		rate := strconv.FormatUint(max(1, p.bandwidthBps/8), 10) + "B"
		cfg.Limiters = append(cfg.Limiters, limiter{Name: lim, Limits: []string{"$ " + rate + " " + rate}})
	}
	if p.maxConns > 0 {
		clim = p.name
		cfg.CLimiters = append(cfg.CLimiters, limiter{Name: clim, Limits: []string{"$ " + strconv.FormatUint(uint64(p.maxConns), 10)}})
	}

	cfg.Hops = append(cfg.Hops, hopConfig{Name: p.name, Selector: selectorOf(p), Nodes: d.nodes(p)})
	fwd := forwarder{Hop: p.name}
	h := handler{}
	if p.chain {
		fwd = forwarder{Nodes: []node{{Name: "next", Addr: placeholder}}}
		h.Chain = p.name
		cfg.Chains = append(cfg.Chains, chain{Name: p.name, Hops: []chainHop{{Name: p.name}}})
	}

	addr := ":" + strconv.FormatUint(uint64(p.port), 10)
	if p.listen.IsValid() {
		addr = netip.AddrPortFrom(p.listen, uint16(p.port)).String() // #nosec G115 -- checked to be a port
	}
	for _, l := range p.listeners {
		s := service{
			Name:      p.name + "-" + l.gostType,
			Addr:      addr,
			Admission: adm.Name,
			Limiter:   lim,
			CLimiter:  clim,
			Metadata:  serviceMetadata{EnableStats: true},
			Handler:   handler{Type: l.handler, Chain: h.Chain, Metadata: handlerMetadata(l.handler)},
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

// handlerMetadata answers a handler's metadata: a relay handler answers
// the relay request as soon as it dialled its upstream (nodelay), as the
// relay connector of the previous hop sends it (see nodes), so a protocol
// whose server speaks first works through the link.
func handlerMetadata(handler string) map[string]string {
	if handler == "relay" {
		return map[string]string{"nodelay": "true"}
	}
	return nil
}

// admissionOf answers a hop's admission: a whitelist of its sources, a
// whitelist of nobody when it is paused, an empty blacklist (everybody)
// for an entry without sources.
func admissionOf(p *hopPlan) admission {
	a := admission{Name: p.name, Whitelist: len(p.sources) > 0 || p.paused, Matchers: []string{}}
	if !p.paused {
		for _, s := range p.sources {
			a.Matchers = append(a.Matchers, prefixText(s))
		}
	}
	return a
}

// selectorOf answers the selector of a hop's upstreams, with the circuit
// breaker as its fail filter.
func selectorOf(p *hopPlan) *selector {
	return &selector{Strategy: strategyName(p.balance), MaxFails: p.maxFails, FailTimeout: p.failTimeout.String()}
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

// nodes answers a hop's gost nodes for all its upstreams.
func (d *Driver) nodes(p *hopPlan) []node {
	ups := make([]nodeUpstream, 0, len(p.upstreams))
	for _, u := range p.upstreams {
		n := nodeUpstream{index: u.index, addr: u.addrPort().String(), weight: u.weight, priority: u.priority}
		if !u.egress.raw() {
			// nodelay: the relay request goes out when the connection is
			// dialled, not with the client's first bytes, so a protocol
			// whose server speaks first (SSH, SMTP, the databases) works
			// through an encrypted or multiplexed link.
			n.connector = &endpoint{Type: "relay", Metadata: map[string]string{"nodelay": "true"}}
			n.dialer = d.dialerEndpoint(u.egress)
		}
		ups = append(ups, n)
	}
	return buildNodes(p.balance, ups)
}

// nodeUpstream is one upstream as a gost node needs it: Render makes them
// from the hop, SetUpstreams from the applied configuration.
type nodeUpstream struct {
	index     int    // in the state's upstream list: the node is u<index>
	addr      string // host:port
	weight    uint32 // at least 1
	priority  uint32
	connector *endpoint // relayed upstreams only
	dialer    *endpoint
}

// buildNodes answers the gost nodes of upstreams (in state order), named
// u<index>, in the order the strategy needs. FAILOVER lists them by
// priority (ties in state order) for gost's fifo; RANDOM and LEAST_CONN
// carry the weight as metadata; weighted ROUND_ROBIN and IP_HASH list an
// upstream once per entry of spread (u<index>-<k>), since gost's round
// and hash ignore weights.
func buildNodes(balance forwardv1.BalanceStrategy, ups []nodeUpstream) []node {
	ups = slices.Clone(ups)
	mk := func(u nodeUpstream, name string) node {
		return node{Name: name, Addr: u.addr, Connector: u.connector, Dialer: u.dialer}
	}
	name := func(u nodeUpstream) string { return "u" + strconv.Itoa(u.index) }
	switch balance {
	case forwardv1.BalanceStrategy_BALANCE_STRATEGY_FAILOVER:
		slices.SortStableFunc(ups, func(a, b nodeUpstream) int { return cmp.Compare(a.priority, b.priority) })
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
	e := endpoint{Type: l.gostType, Metadata: keepalive(l.gostType)}
	if in.encrypted() {
		e.TLS = &tlsConfig{CertFile: d.cfg.LinkCert, KeyFile: d.cfg.LinkKey, CAFile: d.cfg.LinkCA}
		if in.path != "" {
			if e.Metadata == nil {
				e.Metadata = map[string]string{}
			}
			e.Metadata["path"] = in.path
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
	e := &endpoint{Type: out.gostType(), Metadata: keepalive(out.gostType())}
	if out.encrypted() {
		e.TLS = &tlsConfig{CertFile: d.cfg.LinkCert, KeyFile: d.cfg.LinkKey, CAFile: d.cfg.LinkCA, Secure: true, ServerName: out.serverName}
		switch out.security {
		case forwardv1.LinkSecurity_LINK_SECURITY_WSS, forwardv1.LinkSecurity_LINK_SECURITY_GRPC:
			if e.Metadata == nil {
				e.Metadata = map[string]string{}
			}
			e.Metadata["host"] = out.serverName
			if out.path != "" {
				e.Metadata["path"] = out.path
			}
		}
	}
	return e
}

// Link keepalives (forward-sdk.md section 6.2): a mux carrier (smux) and
// a QUIC connection send a keepalive every linkKeepalive and are closed
// when nothing arrived for linkIdleTimeout, so an end whose peer vanished
// without closing (a host or path gone, a QUIC peer restarted: gost sends
// no stateless reset) drops the carrier and dials a new one. A peer gost
// that stops closes its mux carriers (TCP) at once; these bound only the
// silent case. They are gost's own defaults (smux 10 s / 30 s, quic-go
// 30 s idle), rendered on both ends so the configuration states them and
// both ends agree (QUIC negotiates the lower idle timeout; gost's QUIC
// listener sends no keepalive unless told to). gost's file loader keeps
// the dotted mux keys whole inside a service's or hop's metadata (gost -O
// json shows them as loaded); TestRenderIsPlainJSON allows those alone.
const (
	linkKeepalive   = "10s"
	linkIdleTimeout = "30s"
)

// muxCarrier reports whether a gost listener or dialer type carries many
// streams on one long-lived carrier: smux over TCP, TLS or WSS, and
// QUIC. A carrier a listener accepted outlives the service: deleting the
// service through the web API closes its listening socket only (and a
// QUIC listener's UDP socket stays bound while its connections live), so
// such a service is never re-created or deleted on a running gost
// (applyStrands).
func muxCarrier(typ string) bool {
	switch typ {
	case "mtcp", "mtls", "mwss", "quic":
		return true
	}
	return false
}

// keepalive answers the keepalive metadata of a mux or QUIC listener or
// dialer, nil for any other type.
func keepalive(typ string) map[string]string {
	switch typ {
	case "mtcp", "mtls", "mwss":
		return map[string]string{"mux.keepaliveInterval": linkKeepalive, "mux.keepaliveTimeout": linkIdleTimeout}
	case "quic":
		return map[string]string{"keepAlive": "true", "ttl": linkKeepalive, "maxIdleTimeout": linkIdleTimeout}
	}
	return nil
}

// prefixText formats an admission matcher: a bare address for a single
// address, the prefix otherwise.
func prefixText(p netip.Prefix) string {
	if p.IsSingleIP() {
		return p.Addr().String()
	}
	return p.String()
}
