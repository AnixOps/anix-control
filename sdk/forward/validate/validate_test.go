package validate

import (
	"net/netip"
	"strings"
	"testing"
	"time"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/model"
)

var allStrategies = []model.BalanceStrategy{
	model.BalanceRoundRobin, model.BalanceRandom, model.BalanceIPHash, model.BalanceLeastConn, model.BalanceFailover,
}

func nftNode(ref string, addrs ...string) model.NodeInfo {
	return model.NodeInfo{
		NodeRef: ref, Addresses: addrs, PortRange: model.PortRange{First: 40000, Last: 49999},
		Engines: []model.EngineCapabilities{{
			Engine: model.EngineNFTables, Available: true, UDP: true, Strategies: allStrategies,
			LinkSecurities: []model.LinkSecurity{model.LinkSecurityRaw}, BandwidthLimit: true, Quota: true, MaxConns: true,
		}},
	}
}

func gostNode(ref string, addrs ...string) model.NodeInfo {
	return model.NodeInfo{
		NodeRef: ref, Addresses: addrs, PortRange: model.PortRange{First: 20000, Last: 20999},
		Engines: []model.EngineCapabilities{{
			Engine: model.EngineGost, Available: true, IPv6: true, UDP: true, Strategies: allStrategies,
			LinkSecurities: []model.LinkSecurity{
				model.LinkSecurityRaw, model.LinkSecurityTLS, model.LinkSecurityWSS, model.LinkSecurityQUIC, model.LinkSecurityGRPC,
			},
			BandwidthLimit: true, MaxConns: true,
		}},
	}
}

func testNodes() []model.NodeInfo {
	return []model.NodeInfo{
		nftNode("forward-21", "192.0.2.21"),
		gostNode("forward-31", "198.51.100.31"),
		gostNode("forward-41", "203.0.113.41"),
		gostNode("forward-42", "203.0.113.42"),
	}
}

// validRoute is the fixture plan-nft-entry-gost-relay-exit-failover's
// route: nftables entry, gost relay, two gost exits over TLS.
func validRoute() model.Route {
	return model.Route{
		ID: "01JF1B000000000000000000B1", Owner: "user:1042", Name: "sh-to-jp",
		Listen: model.Listen{Port: 40001, Protocol: model.L4ProtocolTCP, EntryHostname: "r1042.fwd.example.com"},
		Hops: []model.Hop{
			{Role: model.HopRoleEntry, Engine: model.EngineNFTables, NodeRefs: []string{"forward-21"}},
			{
				Role: model.HopRoleRelay, Engine: model.EngineGost, NodeRefs: []string{"forward-31"},
				Ingress: model.LinkTransport{Security: model.LinkSecurityRaw}, DialAddress: "172.16.5.31",
			},
			{
				Role: model.HopRoleExit, Engine: model.EngineGost, NodeRefs: []string{"forward-41", "forward-42"},
				Ingress: model.LinkTransport{Security: model.LinkSecurityTLS, Mux: true},
			},
		},
		Targets: []model.Target{{Host: "198.51.100.7", Port: 8443}, {Host: "origin.example.net", Port: 8443, Priority: 10}},
		Policy: model.Policy{
			NextHop: model.BalanceFailover, Target: model.BalanceFailover,
			CircuitBreaker: model.CircuitBreaker{FailureThreshold: 3, OpenFor: 30 * time.Second},
		},
		Limits: model.Limits{BandwidthBPS: 50_000_000, MaxConns: 500, ExpiresAt: time.UnixMilli(1798761600000)},
		Labels: map[string]string{"link": "iepl"},
	}
}

func TestValidRoutes(t *testing.T) {
	r := validRoute()
	for name, opts := range map[string]Options{
		"without inventory": {},
		"with inventory":    {Nodes: testNodes()},
		"on create":         {Nodes: testNodes(), OnCreate: true, Now: time.UnixMilli(1790000000000)},
	} {
		if vs := Route(&r, opts); len(vs) != 0 {
			t.Errorf("%s: %v", name, vs)
		}
	}
	if err := Route(&r, Options{}).Err(); err != nil {
		t.Fatal(err)
	}
	single := model.Route{
		Owner:   "admin",
		Listen:  model.Listen{Protocol: model.L4ProtocolTCPUDP},
		Hops:    []model.Hop{{Role: model.HopRoleEntry, Engine: model.EngineNFTables, NodeRefs: []string{"forward-21"}}},
		Targets: []model.Target{{Host: "10.88.0.20", Port: 443}, {Host: "2001:db8::20", Port: 443}},
		Policy:  model.Policy{TargetPolicy: model.TargetPolicyAllowPrivate, Direct: model.DirectForced},
	}
	if vs := Route(&single, Options{}); len(vs) != 0 {
		t.Errorf("single hop: %v", vs)
	}
	// One node may serve two hops: an nftables entry handing over to a
	// gost relay on the same host.
	hairpin := validRoute()
	hairpin.Hops[1].NodeRefs = []string{"forward-21"}
	hairpin.Hops[1].DialAddress = ""
	if vs := Route(&hairpin, Options{}); len(vs) != 0 {
		t.Errorf("hairpin: %v", vs)
	}
}

type ruleCase struct {
	name  string
	edit  func(r *model.Route, o *Options)
	field string
	code  Code
}

func ruleCases() []ruleCase {
	return []ruleCase{
		// Route metadata.
		{"bad id", func(r *model.Route, _ *Options) { r.ID = "route-1" }, "id", CodeInvalidFormat},
		{"lower-case id", func(r *model.Route, _ *Options) { r.ID = strings.ToLower(r.ID) }, "id", CodeInvalidFormat},
		{"no owner", func(r *model.Route, _ *Options) { r.Owner = "" }, "owner", CodeRequired},
		{"bad owner", func(r *model.Route, _ *Options) { r.Owner = "root" }, "owner", CodeInvalidFormat},
		{"empty user id", func(r *model.Route, _ *Options) { r.Owner = "user:" }, "owner", CodeInvalidFormat},
		{"long name", func(r *model.Route, _ *Options) { r.Name = strings.Repeat("名", MaxNameRunes+1) }, "name", CodeTooLong},
		{"control in name", func(r *model.Route, _ *Options) { r.Name = "a\nb" }, "name", CodeInvalidFormat},
		{"invalid utf-8 name", func(r *model.Route, _ *Options) { r.Name = "\xff" }, "name", CodeInvalidFormat},
		{"negative created", func(r *model.Route, _ *Options) { r.CreatedAt = time.UnixMilli(-1) }, "created_at_unix_ms", CodeOutOfRange},
		{"negative updated", func(r *model.Route, _ *Options) { r.UpdatedAt = time.UnixMilli(-1) }, "updated_at_unix_ms", CodeOutOfRange},
		{"too large", func(r *model.Route, _ *Options) { r.Labels["big"] = strings.Repeat("x", MaxRouteBytes) }, "", CodeTooLarge},

		// Listen.
		{"listen host name", func(r *model.Route, _ *Options) { r.Listen.Address = "example.com" }, "listen.address", CodeInvalidFormat},
		{"listen zone", func(r *model.Route, _ *Options) { r.Listen.Address = "fe80::1%eth0" }, "listen.address", CodeInvalidFormat},
		{"listen port", func(r *model.Route, _ *Options) { r.Listen.Port = 70000 }, "listen.port", CodeOutOfRange},
		{"no protocol", func(r *model.Route, _ *Options) { r.Listen.Protocol = 0 }, "listen.protocol", CodeRequired},
		{"unknown protocol", func(r *model.Route, _ *Options) { r.Listen.Protocol = 9 }, "listen.protocol", CodeInvalidEnum},
		{"bad entry hostname", func(r *model.Route, _ *Options) { r.Listen.EntryHostname = "-x.example" }, "listen.entry_hostname", CodeInvalidFormat},
		{"entry HA without hostname", func(r *model.Route, _ *Options) {
			r.Listen.EntryHostname = ""
			r.Hops[0].NodeRefs = []string{"forward-21", "forward-22"}
		}, "listen.entry_hostname", CodeRequired},

		// Hops.
		{"no hops", func(r *model.Route, _ *Options) { r.Hops = nil }, "hops", CodeRequired},
		{"too many hops", func(r *model.Route, _ *Options) {
			relay := r.Hops[1]
			r.Hops = append([]model.Hop{r.Hops[0]}, r.Hops[1:]...)
			for len(r.Hops) <= MaxHops {
				relay.NodeRefs = []string{"forward-" + string(rune('5'+len(r.Hops)))}
				r.Hops = append(r.Hops[:1], append([]model.Hop{relay}, r.Hops[1:]...)...)
			}
		}, "hops", CodeTooMany},
		{"first hop not entry", func(r *model.Route, _ *Options) { r.Hops[0].Role = model.HopRoleRelay }, "hops[0].role", CodeInvalidRole},
		{"last hop not exit", func(r *model.Route, _ *Options) { r.Hops[2].Role = model.HopRoleRelay }, "hops[2].role", CodeInvalidRole},
		{"middle hop exit", func(r *model.Route, _ *Options) { r.Hops[1].Role = model.HopRoleExit }, "hops[1].role", CodeInvalidRole},
		{"one hop as exit", func(r *model.Route, _ *Options) {
			r.Hops = r.Hops[:1]
			r.Hops[0].Role = model.HopRoleExit
		}, "hops[0].role", CodeInvalidRole},
		{"no role", func(r *model.Route, _ *Options) { r.Hops[1].Role = 0 }, "hops[1].role", CodeRequired},
		{"unknown role", func(r *model.Route, _ *Options) { r.Hops[1].Role = 8 }, "hops[1].role", CodeInvalidEnum},
		{"no engine", func(r *model.Route, _ *Options) { r.Hops[0].Engine = 0 }, "hops[0].engine", CodeRequired},
		{"unknown engine", func(r *model.Route, _ *Options) { r.Hops[0].Engine = 9 }, "hops[0].engine", CodeInvalidEnum},
		{"anixops engine", func(r *model.Route, _ *Options) { r.Hops[2].Engine = model.EngineAnixOps }, "hops[2].engine", CodeEngineNotEnabled},
		{"no nodes", func(r *model.Route, _ *Options) { r.Hops[1].NodeRefs = nil }, "hops[1].node_refs", CodeRequired},
		{"too many nodes", func(r *model.Route, _ *Options) {
			r.Hops[2].NodeRefs = nil
			for i := 0; i <= MaxNodesPerHop; i++ {
				r.Hops[2].NodeRefs = append(r.Hops[2].NodeRefs, "forward-"+string(rune('a'+i)))
			}
		}, "hops[2].node_refs", CodeTooMany},
		{"bad node ref", func(r *model.Route, _ *Options) { r.Hops[1].NodeRefs = []string{"node-31"} }, "hops[1].node_refs[0]", CodeInvalidFormat},
		{"zero node id", func(r *model.Route, _ *Options) { r.Hops[1].NodeRefs = []string{"forward-0"} }, "hops[1].node_refs[0]", CodeInvalidFormat},
		{"node twice in a hop", func(r *model.Route, _ *Options) { r.Hops[2].NodeRefs = []string{"forward-41", "forward-41"} }, "hops[2].node_refs[1]", CodeDuplicate},
		{"hop port", func(r *model.Route, _ *Options) { r.Hops[1].Port = 65536 }, "hops[1].port", CodeOutOfRange},
		{"dial_address with several nodes", func(r *model.Route, _ *Options) { r.Hops[2].DialAddress = "203.0.113.41" }, "hops[2].dial_address", CodeRequiresSingleNode},
		{"bad dial_address", func(r *model.Route, _ *Options) { r.Hops[1].DialAddress = "172.16.5.31:20000" }, "hops[1].dial_address", CodeInvalidFormat},
		{"no ingress security", func(r *model.Route, _ *Options) { r.Hops[1].Ingress.Security = 0 }, "hops[1].ingress.security", CodeRequired},
		{"unknown ingress security", func(r *model.Route, _ *Options) { r.Hops[1].Ingress.Security = 40 }, "hops[1].ingress.security", CodeInvalidEnum},
		{"anixops link", func(r *model.Route, _ *Options) { r.Hops[2].Ingress.Security = model.LinkSecurityAnixOps }, "hops[2].ingress.security", CodeEngineNotEnabled},
		{"server name on raw", func(r *model.Route, _ *Options) { r.Hops[1].Ingress.ServerName = "x.example" }, "hops[1].ingress.server_name", CodeNotApplicable},
		{"bad server name", func(r *model.Route, _ *Options) { r.Hops[2].Ingress.ServerName = "a b" }, "hops[2].ingress.server_name", CodeInvalidFormat},
		{"path on tls", func(r *model.Route, _ *Options) { r.Hops[2].Ingress.Path = "/ws" }, "hops[2].ingress.path", CodeNotApplicable},
		{"bad wss path", func(r *model.Route, _ *Options) {
			r.Hops[2].Ingress.Security = model.LinkSecurityWSS
			r.Hops[2].Ingress.Path = "ws"
		}, "hops[2].ingress.path", CodeInvalidFormat},
		{"bad grpc service", func(r *model.Route, _ *Options) {
			r.Hops[2].Ingress.Security = model.LinkSecurityGRPC
			r.Hops[2].Ingress.Path = "/svc"
		}, "hops[2].ingress.path", CodeInvalidFormat},
		{"long path", func(r *model.Route, _ *Options) {
			r.Hops[2].Ingress.Security = model.LinkSecurityWSS
			r.Hops[2].Ingress.Path = "/" + strings.Repeat("p", MaxPathBytes)
		}, "hops[2].ingress.path", CodeTooLong},

		// Links (section 4.2).
		{"nftables cannot dial TLS", func(r *model.Route, _ *Options) {
			r.Hops = []model.Hop{r.Hops[0], r.Hops[2]}
		}, "hops[1].ingress.security", CodeLinkUnsupported},
		{"nftables cannot terminate TLS", func(r *model.Route, _ *Options) {
			r.Hops[2].Engine = model.EngineNFTables
			r.Hops[2].Ingress.Mux = false
		}, "hops[2].ingress.security", CodeLinkUnsupported},
		{"gost cannot carry anixops", func(r *model.Route, o *Options) {
			o.EnableAnixOps = true
			r.Hops[2].Ingress.Security = model.LinkSecurityAnixOps
		}, "hops[2].ingress.security", CodeLinkUnsupported},
		{"nftables cannot mux", func(r *model.Route, _ *Options) { r.Hops[1].Ingress.Mux = true }, "hops[1].ingress.mux", CodeLinkUnsupported},

		// Targets and the target policy (section 14).
		{"no targets", func(r *model.Route, _ *Options) { r.Targets = nil }, "targets", CodeRequired},
		{"too many targets", func(r *model.Route, _ *Options) {
			for i := 0; i <= MaxTargets; i++ {
				r.Targets = append(r.Targets, model.Target{Host: "t.example", Port: uint32(1000 + i)})
			}
		}, "targets", CodeTooMany},
		{"no host", func(r *model.Route, _ *Options) { r.Targets[0].Host = "" }, "targets[0].host", CodeRequired},
		{"host with port", func(r *model.Route, _ *Options) { r.Targets[0].Host = "198.51.100.7:8443" }, "targets[0].host", CodeInvalidFormat},
		{"bracketed v6", func(r *model.Route, _ *Options) { r.Targets[0].Host = "[2001:db8::1]" }, "targets[0].host", CodeInvalidFormat},
		{"bad host name", func(r *model.Route, _ *Options) { r.Targets[0].Host = "bad_host.example" }, "targets[0].host", CodeInvalidFormat},
		{"numeric host", func(r *model.Route, _ *Options) { r.Targets[0].Host = "127.1" }, "targets[0].host", CodeInvalidFormat},
		{"hex host", func(r *model.Route, _ *Options) { r.Targets[0].Host = "0x7f000001" }, "targets[0].host", CodeInvalidFormat},
		{"zoned target", func(r *model.Route, _ *Options) { r.Targets[0].Host = "fe80::1%eth0" }, "targets[0].host", CodeInvalidFormat},
		{"localhost", func(r *model.Route, _ *Options) { r.Targets[0].Host = "LocalHost." }, "targets[0].host", CodeTargetNotAllowed},
		{"localhost subdomain", func(r *model.Route, _ *Options) { r.Targets[0].Host = "db.localhost" }, "targets[0].host", CodeTargetNotAllowed},
		{"private for a user", func(r *model.Route, _ *Options) { r.Targets[0].Host = "10.0.0.5" }, "targets[0].host", CodeTargetNotAllowed},
		{"cgnat for a user", func(r *model.Route, _ *Options) { r.Targets[0].Host = "100.64.1.1" }, "targets[0].host", CodeTargetNotAllowed},
		{"ula for a user", func(r *model.Route, _ *Options) { r.Targets[0].Host = "fd00::1" }, "targets[0].host", CodeTargetNotAllowed},
		{"allow private for a user", func(r *model.Route, _ *Options) { r.Policy.TargetPolicy = model.TargetPolicyAllowPrivate }, "policy.target_policy", CodeForbidden},
		{"allow private for a user still checks", func(r *model.Route, _ *Options) {
			r.Policy.TargetPolicy = model.TargetPolicyAllowPrivate
			r.Targets[0].Host = "10.0.0.5"
		}, "targets[0].host", CodeTargetNotAllowed},
		{"unknown target policy", func(r *model.Route, _ *Options) { r.Policy.TargetPolicy = 7 }, "policy.target_policy", CodeInvalidEnum},
		{"admin loopback", func(r *model.Route, _ *Options) { adminPrivate(r); r.Targets[0].Host = "127.0.0.1" }, "targets[0].host", CodeTargetNotAllowed},
		{"admin mapped loopback", func(r *model.Route, _ *Options) { adminPrivate(r); r.Targets[0].Host = "::ffff:127.0.0.1" }, "targets[0].host", CodeTargetNotAllowed},
		{"admin v6 loopback", func(r *model.Route, _ *Options) { adminPrivate(r); r.Targets[0].Host = "::1" }, "targets[0].host", CodeTargetNotAllowed},
		{"admin link-local", func(r *model.Route, _ *Options) { adminPrivate(r); r.Targets[0].Host = "169.254.169.254" }, "targets[0].host", CodeTargetNotAllowed},
		{"admin multicast", func(r *model.Route, _ *Options) { adminPrivate(r); r.Targets[0].Host = "224.0.0.1" }, "targets[0].host", CodeTargetNotAllowed},
		{"admin unspecified", func(r *model.Route, _ *Options) { adminPrivate(r); r.Targets[0].Host = "0.0.0.0" }, "targets[0].host", CodeTargetNotAllowed},
		{"admin this network", func(r *model.Route, _ *Options) { adminPrivate(r); r.Targets[0].Host = "0.1.2.3" }, "targets[0].host", CodeTargetNotAllowed},
		{"admin broadcast", func(r *model.Route, _ *Options) { adminPrivate(r); r.Targets[0].Host = "255.255.255.255" }, "targets[0].host", CodeTargetNotAllowed},
		{"admin nat64", func(r *model.Route, _ *Options) { adminPrivate(r); r.Targets[0].Host = "64:ff9b::7f00:1" }, "targets[0].host", CodeTargetNotAllowed},
		{"no target port", func(r *model.Route, _ *Options) { r.Targets[0].Port = 0 }, "targets[0].port", CodeRequired},
		{"target port", func(r *model.Route, _ *Options) { r.Targets[0].Port = 65536 }, "targets[0].port", CodeOutOfRange},
		{"weight", func(r *model.Route, _ *Options) { r.Targets[0].Weight = MaxWeight + 1 }, "targets[0].weight", CodeOutOfRange},
		{"duplicate target", func(r *model.Route, _ *Options) {
			r.Targets[1] = model.Target{Host: "ORIGIN.example.net.", Port: 8443}
			r.Targets[0].Host = "origin.example.net"
		}, "targets[1]", CodeDuplicate},
		{"duplicate mapped target", func(r *model.Route, _ *Options) { r.Targets[1] = model.Target{Host: "::ffff:198.51.100.7", Port: 8443} }, "targets[1]", CodeDuplicate},

		// Policy.
		{"unknown next_hop", func(r *model.Route, _ *Options) { r.Policy.NextHop = 6 }, "policy.next_hop", CodeInvalidEnum},
		{"unknown target strategy", func(r *model.Route, _ *Options) { r.Policy.Target = -1 }, "policy.target", CodeInvalidEnum},
		{"unknown direct", func(r *model.Route, _ *Options) { r.Policy.Direct = 4 }, "policy.direct", CodeInvalidEnum},
		{"forced direct with later hops", func(r *model.Route, _ *Options) { r.Policy.Direct = model.DirectForced }, "policy.direct", CodeUnusedHops},
		{"interval too short", func(r *model.Route, _ *Options) { r.Policy.Health.Interval = time.Millisecond }, "policy.health.interval_ms", CodeOutOfRange},
		{"interval too long", func(r *model.Route, _ *Options) { r.Policy.Health.Interval = 2 * time.Hour }, "policy.health.interval_ms", CodeOutOfRange},
		{"timeout too long", func(r *model.Route, _ *Options) { r.Policy.Health.Timeout = 2 * time.Minute }, "policy.health.timeout_ms", CodeOutOfRange},
		{"timeout not below interval", func(r *model.Route, _ *Options) { r.Policy.Health.Interval = time.Second }, "policy.health.timeout_ms", CodeInvalidRelation},
		{"failure threshold", func(r *model.Route, _ *Options) { r.Policy.CircuitBreaker.FailureThreshold = MaxFailureThreshold + 1 }, "policy.circuit_breaker.failure_threshold", CodeOutOfRange},
		{"breaker open too short", func(r *model.Route, _ *Options) { r.Policy.CircuitBreaker.OpenFor = time.Millisecond }, "policy.circuit_breaker.open_ms", CodeOutOfRange},

		// Limits.
		{"negative expiry", func(r *model.Route, _ *Options) { r.Limits.ExpiresAt = time.UnixMilli(-5) }, "limits.expires_at_unix_ms", CodeOutOfRange},
		{"expired on create", func(r *model.Route, o *Options) {
			o.OnCreate, o.Now = true, r.Limits.ExpiresAt
		}, "limits.expires_at_unix_ms", CodeExpired},

		// Labels.
		{"too many labels", func(r *model.Route, _ *Options) {
			for i := 0; i <= MaxLabels; i++ {
				r.Labels["k"+string(rune('a'+i))] = "v"
			}
		}, "labels", CodeTooMany},
		{"bad label key", func(r *model.Route, _ *Options) { r.Labels["-k"] = "v" }, `labels["-k"]`, CodeInvalidFormat},
		{"empty label key", func(r *model.Route, _ *Options) { r.Labels[""] = "v" }, `labels[""]`, CodeInvalidFormat},
		{"long label key", func(r *model.Route, _ *Options) { r.Labels[strings.Repeat("k", MaxLabelKeyBytes+1)] = "v" }, `labels["` + strings.Repeat("k", MaxLabelKeyBytes+1) + `"]`, CodeTooLong},
		{"long label value", func(r *model.Route, _ *Options) { r.Labels["k"] = strings.Repeat("v", MaxLabelValueBytes+1) }, `labels["k"]`, CodeTooLong},
		{"control in label value", func(r *model.Route, _ *Options) { r.Labels["k"] = "a\x00" }, `labels["k"]`, CodeInvalidFormat},

		// Ports.
		{"reserved listen port", func(r *model.Route, o *Options) { o.ReservedPorts = map[string][]uint32{"forward-21": {22, 40001}} }, "listen.port", CodePortReserved},
		{"reserved hop port", func(r *model.Route, o *Options) {
			r.Hops[2].Port = 20022
			o.ReservedPorts = map[string][]uint32{"forward-42": {20022}}
		}, "hops[2].port", CodePortReserved},

		// Inventory.
		{"unknown node", func(r *model.Route, o *Options) { withNodes(o); r.Hops[1].NodeRefs = []string{"forward-99"} }, "hops[1].node_refs[0]", CodeUnknownNode},
		{"engine not advertised", func(r *model.Route, o *Options) {
			withNodes(o)
			o.EnableAnixOps = true
			r.Hops = r.Hops[:1]
			r.Hops[0].Engine = model.EngineAnixOps
			r.Policy.NextHop = 0
		}, "hops[0].engine", CodeEngineNotAdvertised},
		{"engine unavailable", func(r *model.Route, o *Options) {
			withNodes(o)
			o.Nodes[0].Engines[0].Available = false
			o.Nodes[0].Engines[0].UnavailableReason = "nft missing"
		}, "hops[0].engine", CodeEngineUnavailable},
		{"node cannot terminate", func(r *model.Route, o *Options) {
			withNodes(o)
			o.Nodes[3].Engines[0].LinkSecurities = []model.LinkSecurity{model.LinkSecurityRaw}
		}, "hops[2].ingress.security", CodeCapabilityMissing},
		{"node cannot dial", func(r *model.Route, o *Options) {
			withNodes(o)
			o.Nodes[1].Engines[0].LinkSecurities = []model.LinkSecurity{model.LinkSecurityRaw}
		}, "hops[2].ingress.security", CodeCapabilityMissing},
		{"next_hop not offered", func(r *model.Route, o *Options) {
			withNodes(o)
			o.Nodes[1].Engines[0].Strategies = []model.BalanceStrategy{model.BalanceRoundRobin}
		}, "policy.next_hop", CodeCapabilityMissing},
		{"default next_hop not offered", func(r *model.Route, o *Options) {
			withNodes(o)
			r.Policy.NextHop = 0
			o.Nodes[0].Engines[0].Strategies = []model.BalanceStrategy{model.BalanceFailover}
		}, "policy.next_hop", CodeCapabilityMissing},
		{"target strategy not offered", func(r *model.Route, o *Options) {
			withNodes(o)
			r.Policy.Target = model.BalanceLeastConn
			o.Nodes[2].Engines[0].Strategies = []model.BalanceStrategy{model.BalanceFailover}
		}, "policy.target", CodeCapabilityMissing},
		{"preferred direct needs target strategy on entry", func(r *model.Route, o *Options) {
			withNodes(o)
			r.Policy.Direct = model.DirectPreferred
			r.Policy.Target = model.BalanceIPHash
			o.Nodes[0].Engines[0].Strategies = []model.BalanceStrategy{model.BalanceFailover}
		}, "policy.target", CodeCapabilityMissing},
		{"no UDP", func(r *model.Route, o *Options) {
			withNodes(o)
			r.Listen.Protocol = model.L4ProtocolUDP
			o.Nodes[1].Engines[0].UDP = false
		}, "listen.protocol", CodeCapabilityMissing},
		{"no IPv6 for target", func(r *model.Route, o *Options) {
			withNodes(o)
			r.Targets[0].Host = "2001:db8::7"
			o.Nodes[3].Engines[0].IPv6 = false
		}, "targets[0].host", CodeCapabilityMissing},
		{"no IPv6 for listen", func(r *model.Route, o *Options) { withNodes(o); r.Listen.Address = "2001:db8::21" }, "listen.address", CodeCapabilityMissing},
		{"no bandwidth limit", func(r *model.Route, o *Options) { withNodes(o); o.Nodes[0].Engines[0].BandwidthLimit = false }, "limits.bandwidth_bps", CodeCapabilityMissing},
		{"no quota on gost entry", func(r *model.Route, o *Options) {
			withNodes(o)
			r.Hops[0].Engine = model.EngineGost
			r.Hops[0].NodeRefs = []string{"forward-31"}
			r.Hops[1].NodeRefs = []string{"forward-21"}
			r.Hops[1].Engine = model.EngineNFTables
			r.Hops[2].Ingress = model.LinkTransport{Security: model.LinkSecurityRaw}
			r.Hops[1].DialAddress = ""
			r.Limits.QuotaBytes = 1 << 30
		}, "limits.quota_bytes", CodeCapabilityMissing},
		{"no connection limit", func(r *model.Route, o *Options) { withNodes(o); o.Nodes[0].Engines[0].MaxConns = false }, "limits.max_conns", CodeCapabilityMissing},
		{"listen port outside range", func(r *model.Route, o *Options) { withNodes(o); r.Listen.Port = 30001 }, "listen.port", CodePortOutOfRange},
		{"hop port outside range", func(r *model.Route, o *Options) { withNodes(o); r.Hops[2].Port = 21000 }, "hops[2].port", CodePortOutOfRange},
	}
}

func adminPrivate(r *model.Route) {
	r.Owner = "admin"
	r.Policy.TargetPolicy = model.TargetPolicyAllowPrivate
}

func withNodes(o *Options) { o.Nodes = testNodes() }

func TestRules(t *testing.T) {
	seen := map[string]bool{}
	for _, tc := range ruleCases() {
		if seen[tc.name] {
			t.Fatalf("duplicate case %q", tc.name)
		}
		seen[tc.name] = true
		t.Run(tc.name, func(t *testing.T) {
			r := validRoute()
			var opts Options
			tc.edit(&r, &opts)
			vs := Route(&r, opts)
			if !vs.Has(tc.field, tc.code) {
				t.Fatalf("want %s at %q, got %v", tc.code, tc.field, vs)
			}
			for _, v := range vs {
				if v.Message == "" {
					t.Errorf("violation without message: %+v", v)
				}
			}
			// The same route through the contract gives the same answer.
			if pv := Proto(r.ToProto(), opts); len(pv) != len(vs) {
				t.Fatalf("Proto disagrees with Route:\n%v\n%v", pv, vs)
			}
		})
	}
}

// TestEveryCodeIsTested keeps the rule table complete: every code some
// rule can answer has a case.
func TestEveryCodeIsTested(t *testing.T) {
	tested := map[Code]bool{}
	for _, tc := range ruleCases() {
		tested[tc.code] = true
	}
	for _, code := range []Code{
		CodeRequired, CodeInvalidFormat, CodeInvalidEnum, CodeOutOfRange, CodeTooMany, CodeTooLong, CodeTooLarge,
		CodeDuplicate, CodeInvalidRole, CodeEngineNotEnabled, CodeLinkUnsupported, CodeNotApplicable,
		CodeRequiresSingleNode, CodeForbidden, CodeTargetNotAllowed, CodeInvalidRelation, CodeUnusedHops, CodeExpired,
		CodeUnknownNode, CodeEngineNotAdvertised, CodeEngineUnavailable, CodeCapabilityMissing, CodePortOutOfRange,
		CodePortReserved,
	} {
		if !tested[code] {
			t.Errorf("no case for %s", code)
		}
	}
}

func TestNilRoute(t *testing.T) {
	if vs := Route(nil, Options{}); !vs.Has("", CodeRequired) {
		t.Fatalf("nil route: %v", vs)
	}
	if vs := Proto(nil, Options{}); !vs.Has("", CodeRequired) {
		t.Fatalf("nil proto route: %v", vs)
	}
	big := validRoute()
	big.Name = strings.Repeat("x", MaxRouteBytes)
	if vs := Proto(big.ToProto(), Options{}); !vs.Has("", CodeTooLarge) || len(vs) != 1 {
		t.Fatalf("large proto route: %v", vs)
	}
}

func TestLinkMessageMatchesFixture(t *testing.T) {
	r := validRoute()
	r.Hops = []model.Hop{r.Hops[0], r.Hops[2]}
	r.Hops[1].Ingress.Mux = false
	vs := Route(&r, Options{})
	if len(vs) != 1 || vs[0].Message != "hop 0 runs ENGINE_NFTABLES, which only dials LINK_SECURITY_RAW" {
		t.Fatalf("got %v", vs)
	}
	if got := vs.ToProto(); len(got) != 1 || got[0].GetField() != "hops[1].ingress.security" ||
		!strings.HasPrefix(got[0].GetMessage(), "link_unsupported: ") {
		t.Fatalf("proto violations: %v", got)
	}
	if !strings.Contains(vs.Error(), "hops[1].ingress.security") || vs.Err() == nil {
		t.Fatalf("error text: %v", vs)
	}
	if (Violations{}).Err() != nil {
		t.Fatal("no violations is no error")
	}
}

func TestEngineLinks(t *testing.T) {
	if !CanCarry(model.EngineGost, model.LinkSecurityQUIC) || CanCarry(model.EngineNFTables, model.LinkSecurityTLS) ||
		!CanCarry(model.EngineAnixOps, model.LinkSecurityAnixOps) || CanCarry(model.EngineAnixOps, model.LinkSecurityRaw) ||
		CanCarry(model.EngineUnspecified, model.LinkSecurityRaw) {
		t.Fatal("link table")
	}
	links := EngineLinks(model.EngineNFTables)
	links[0] = model.LinkSecurityTLS
	if !CanCarry(model.EngineNFTables, model.LinkSecurityRaw) {
		t.Fatal("EngineLinks must answer a copy")
	}
	if CanMux(model.EngineNFTables) || !CanMux(model.EngineGost) {
		t.Fatal("mux table")
	}
}

func TestCheckTargetAddress(t *testing.T) {
	public, private := model.TargetPolicyPublicOnly, model.TargetPolicyAllowPrivate
	for _, tc := range []struct {
		addr          string
		publicAllowed bool
		privateAllows bool
	}{
		{"198.51.100.7", true, true},
		{"203.0.113.1", true, true},
		{"2001:db8::1", true, true},
		{"8.8.8.8", true, true},
		{"10.1.2.3", false, true},
		{"172.16.5.31", false, true},
		{"192.168.1.1", false, true},
		{"100.64.0.1", false, true},
		{"fd12::1", false, true},
		{"::ffff:10.0.0.1", false, true},
		{"127.0.0.1", false, false},
		{"::1", false, false},
		{"0.0.0.0", false, false},
		{"::", false, false},
		{"169.254.169.254", false, false},
		{"fe80::1", false, false},
		{"224.0.0.251", false, false},
		{"ff02::1", false, false},
		{"192.0.0.8", false, false},
		{"198.18.0.1", false, false},
		{"240.0.0.1", false, false},
		{"2002:7f00:1::", false, false},
		{"64:ff9b:1::1", false, false},
	} {
		addr := netip.MustParseAddr(tc.addr)
		if got := CheckTargetAddress(addr, public) == nil; got != tc.publicAllowed {
			t.Errorf("%s under PUBLIC_ONLY: allowed=%v", tc.addr, got)
		}
		if got := CheckTargetAddress(addr, private) == nil; got != tc.privateAllows {
			t.Errorf("%s under ALLOW_PRIVATE: allowed=%v", tc.addr, got)
		}
		if got := CheckTargetAddress(addr, model.TargetPolicyUnspecified) == nil; got != tc.publicAllowed {
			t.Errorf("%s under the default policy: allowed=%v", tc.addr, got)
		}
	}
	if err := CheckTargetAddress(netip.MustParseAddr("127.0.0.1"), private); err != ErrLoopbackTarget {
		t.Errorf("loopback error: %v", err)
	}
	if err := CheckTargetAddress(netip.Addr{}, private); err == nil {
		t.Error("the zero address must be refused")
	}
	if err := CheckTargetAddress(netip.MustParseAddr("fe80::1%eth0"), private); err != ErrZonedTarget {
		t.Errorf("zoned error: %v", err)
	}
}

func TestPlanRequestMergesNodes(t *testing.T) {
	r := validRoute()
	req := &forwardv1.PlanRouteRequest{Route: r.ToProto()}
	for _, n := range testNodes()[:2] {
		req.Nodes = append(req.Nodes, n.ToProto())
	}
	// forward-21 from the request replaces the inventory's version, which
	// lacks UDP; forward-41 and -42 come from the inventory.
	inventory := testNodes()
	inventory[0].Engines[0].BandwidthLimit = false
	if vs := PlanRequest(req, Options{Nodes: inventory}); len(vs) != 0 {
		t.Fatalf("%v", vs)
	}
	if vs := PlanRequest(req, Options{Nodes: inventory[2:3]}); !vs.Has("hops[2].node_refs[1]", CodeUnknownNode) {
		t.Fatalf("forward-42 is in neither: %v", vs)
	}
}
