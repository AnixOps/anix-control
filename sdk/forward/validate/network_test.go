package validate

import (
	"testing"

	"github.com/AnixOps/anix-control/sdk/forward/model"
)

// networkRoute is an ENGINE_NETWORK route: an entry on an ingress device,
// one network model transport to the exit, which dials the targets by the
// internet egress eg-hk.
func networkRoute() model.Route {
	return model.Route{
		ID: "01JF1B000000000000000000N1", Owner: "admin", Name: "sh-to-hk",
		Listen: model.Listen{Port: 40001, Protocol: model.L4ProtocolTCPUDP},
		Hops: []model.Hop{
			{Role: model.HopRoleEntry, Engine: model.EngineNetwork, NodeRefs: []string{"proxy-21"}},
			{
				Role: model.HopRoleExit, Engine: model.EngineNetwork, NodeRefs: []string{"proxy-41"},
				Ingress:         model.LinkTransport{Security: model.LinkSecurityNetwork, NetworkTransportID: "tr-sh-hk"},
				NetworkEgressID: "eg-hk",
			},
		},
		Targets: []model.Target{{Host: "198.51.100.7", Port: 8443, Weight: 2}, {Host: "origin.example.net", Port: 8443}},
		Policy:  model.Policy{Target: model.BalanceRoundRobin, ProxyProtocol: model.ProxyProtocolV1},
		Limits:  model.Limits{BandwidthBPS: 50_000_000, MaxConns: 500, QuotaBytes: 1 << 40},
	}
}

func onNetwork(edit func(r *model.Route)) func(r *model.Route, o *Options) {
	return func(r *model.Route, o *Options) {
		*r = networkRoute()
		o.EnableNetwork = true
		edit(r)
	}
}

func networkRuleCases() []ruleCase {
	return []ruleCase{
		{"network: listen port allocated", onNetwork(func(r *model.Route) { r.Listen.Port = 0 }), "listen.port", CodeRequired},
		{"network: direct", onNetwork(func(r *model.Route) { r.Policy.Direct = model.DirectPreferred }), "policy.direct", CodeNotApplicable},
		{"network: mixed engines", onNetwork(func(r *model.Route) { r.Hops[0].Engine = model.EngineNFTables }), "hops[0].engine", CodeInvalidRelation},
		{"network: several nodes", onNetwork(func(r *model.Route) { r.Hops[1].NodeRefs = []string{"proxy-41", "proxy-42"} }),
			"hops[1].node_refs", CodeRequiresSingleNode},
		{"network: no egress", onNetwork(func(r *model.Route) { r.Hops[1].NetworkEgressID = "" }), "hops[1].network_egress_id", CodeRequired},
		{"network: bad egress", onNetwork(func(r *model.Route) { r.Hops[1].NetworkEgressID = "eg hk" }), "hops[1].network_egress_id", CodeInvalidFormat},
		{"network: egress on the entry", onNetwork(func(r *model.Route) { r.Hops[0].NetworkEgressID = "eg-sh" }), "hops[0].network_egress_id", CodeNotApplicable},
		{"network: hop port", onNetwork(func(r *model.Route) { r.Hops[1].Port = 20000 }), "hops[1].port", CodeNotApplicable},
		{"network: dial address", onNetwork(func(r *model.Route) { r.Hops[1].DialAddress = "10.0.0.41" }), "hops[1].dial_address", CodeNotApplicable},
		{"network: no transport", onNetwork(func(r *model.Route) { r.Hops[1].Ingress.NetworkTransportID = "" }),
			"hops[1].ingress.network_transport_id", CodeRequired},
		{"network: bad transport", onNetwork(func(r *model.Route) { r.Hops[1].Ingress.NetworkTransportID = "tr/1" }),
			"hops[1].ingress.network_transport_id", CodeInvalidFormat},
		{"network: mux", onNetwork(func(r *model.Route) { r.Hops[1].Ingress.Mux = true }), "hops[1].ingress.security", CodeNotApplicable},
		{"network: raw link", onNetwork(func(r *model.Route) { r.Hops[1].Ingress.Security = model.LinkSecurityRaw }),
			"hops[1].ingress.security", CodeLinkUnsupported},
		{"network: proxy protocol on UDP", onNetwork(func(r *model.Route) { r.Listen.Protocol = model.L4ProtocolUDP }),
			"policy.proxy_protocol", CodeProxyProtocolUnsupported},
		{"egress on a gost exit", func(r *model.Route, _ *Options) { r.Hops[2].NetworkEgressID = "eg-hk" }, "hops[2].network_egress_id", CodeNotApplicable},
		{"transport on a TLS link", func(r *model.Route, _ *Options) { r.Hops[2].Ingress.NetworkTransportID = "tr-1" },
			"hops[2].ingress.network_transport_id", CodeNotApplicable},
	}
}

func TestNetworkRoutes(t *testing.T) {
	r := networkRoute()
	if vs := Route(&r, Options{EnableNetwork: true, Nodes: testNodes()}); len(vs) != 0 {
		t.Fatalf("network route: %v", vs)
	}
	// A single hop dials the targets by its own egress.
	r.Hops = []model.Hop{{Role: model.HopRoleEntry, Engine: model.EngineNetwork, NodeRefs: []string{"proxy-21"}, NetworkEgressID: "eg-sh"}}
	if vs := Route(&r, Options{EnableNetwork: true}); len(vs) != 0 {
		t.Fatalf("single hop: %v", vs)
	}
	if vs := Route(&r, Options{}); !vs.Has("hops[0].engine", CodeEngineNotEnabled) {
		t.Fatalf("not enabled: %v", vs)
	}
	// Several entry devices are entry HA, with the DNS name clients use.
	r = networkRoute()
	r.Hops[0].NodeRefs = []string{"proxy-21", "proxy-22"}
	if vs := Route(&r, Options{EnableNetwork: true}); !vs.Has("listen.entry_hostname", CodeRequired) || len(vs) != 1 {
		t.Fatalf("several entries without a name: %v", vs)
	}
	r.Listen.EntryHostname = "sh.example.net"
	if vs := Route(&r, Options{EnableNetwork: true}); len(vs) != 0 {
		t.Fatalf("several entries: %v", vs)
	}
}
