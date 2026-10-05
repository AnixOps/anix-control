package validate

import (
	"testing"

	"github.com/AnixOps/anix-control/sdk/forward/model"
)

func anixopsNode(ref, addr string, labels map[string]string, carriers ...model.AnixOpsCarrier) model.NodeInfo {
	return model.NodeInfo{
		NodeRef: ref, Addresses: []string{addr}, PortRange: model.PortRange{First: 40000, Last: 49999}, Labels: labels,
		Engines: []model.EngineCapabilities{{
			Engine: model.EngineAnixOps, Available: true, IPv6: true, UDP: true, Strategies: allStrategies,
			LinkSecurities: []model.LinkSecurity{model.LinkSecurityAnixOps}, BandwidthLimit: true, Quota: true, MaxConns: true,
			Carriers: carriers, ProxyProtocol: true, ProtocolVersions: []uint32{1},
		}},
	}
}

func anixopsNodes() []model.NodeInfo {
	both := []model.AnixOpsCarrier{model.CarrierTLSTCP, model.CarrierQUIC, model.CarrierPlain}
	iepl := map[string]string{"link": "iepl"}
	return []model.NodeInfo{
		anixopsNode("forward-71", "192.0.2.71", iepl, both...),
		anixopsNode("forward-72", "192.0.2.72", iepl, both...),
		anixopsNode("forward-73", "192.0.2.73", iepl, both...),
	}
}

// anixopsRoute is plan-anixops-experimental.json's route.
func anixopsRoute() model.Route {
	return model.Route{
		ID: "01JF1D000000000000000000D1", Owner: "admin", Name: "anixops",
		Listen: model.Listen{Port: 40001, Protocol: model.L4ProtocolTCP},
		Hops: []model.Hop{
			{Role: model.HopRoleEntry, Engine: model.EngineAnixOps, NodeRefs: []string{"forward-71"}},
			{
				Role: model.HopRoleRelay, Engine: model.EngineAnixOps, NodeRefs: []string{"forward-72"},
				Ingress: model.LinkTransport{Security: model.LinkSecurityAnixOps, Carrier: model.CarrierQUIC},
			},
			{
				Role: model.HopRoleExit, Engine: model.EngineAnixOps, NodeRefs: []string{"forward-73"},
				Ingress: model.LinkTransport{Security: model.LinkSecurityAnixOps, Mux: true},
			},
		},
		Targets: []model.Target{{Host: "198.51.100.20", Port: 8443}},
		Policy:  model.Policy{ProxyProtocol: model.ProxyProtocolV2},
	}
}

// anixopsRuleCases run on validRoute(), so each edit rebuilds the route.
func anixopsRuleCases() []ruleCase {
	on := func(r *model.Route, o *Options, edit func(*model.Route, *Options)) {
		*r = anixopsRoute()
		o.EnableAnixOps = true
		o.Nodes = anixopsNodes()
		edit(r, o)
	}
	const carrier = "hops[1].ingress.carrier"
	return []ruleCase{
		{"anixops carrier on a gost link", func(r *model.Route, _ *Options) { r.Hops[2].Ingress.Carrier = model.CarrierQUIC },
			"hops[2].ingress.carrier", CodeNotApplicable},
		{"unknown carrier", func(r *model.Route, _ *Options) { r.Hops[2].Ingress.Carrier = 9 }, "hops[2].ingress.carrier", CodeInvalidEnum},
		{"unknown proxy protocol", func(r *model.Route, _ *Options) { r.Policy.ProxyProtocol = 9 }, "policy.proxy_protocol", CodeInvalidEnum},
		{"proxy protocol on a gost exit", func(r *model.Route, _ *Options) { r.Policy.ProxyProtocol = model.ProxyProtocolV2 },
			"policy.proxy_protocol", CodeProxyProtocolUnsupported},
		{"proxy protocol on an nftables entry", func(r *model.Route, _ *Options) {
			*r = validRoute()
			r.Hops = r.Hops[:1]
			r.Policy.ProxyProtocol = model.ProxyProtocolV2
		}, "policy.proxy_protocol", CodeProxyProtocolUnsupported},
		{"proxy protocol on a UDP route", func(r *model.Route, o *Options) {
			on(r, o, func(r *model.Route, _ *Options) { r.Listen.Protocol = model.L4ProtocolUDP })
		}, "policy.proxy_protocol", CodeProxyProtocolUnsupported},
		{"proxy protocol with a direct entry", func(r *model.Route, o *Options) {
			on(r, o, func(r *model.Route, _ *Options) { r.Policy.Direct = model.DirectPreferred })
		}, "policy.proxy_protocol", CodeInvalidRelation},
		{"exit node without proxy protocol", func(r *model.Route, o *Options) {
			on(r, o, func(_ *model.Route, o *Options) { o.Nodes[2].Engines[0].ProxyProtocol = false })
		}, "policy.proxy_protocol", CodeProxyProtocolUnsupported},
		{"foreign server name", func(r *model.Route, o *Options) {
			on(r, o, func(r *model.Route, _ *Options) { r.Hops[1].Ingress.ServerName = "edge.example.com" })
		}, "hops[1].ingress.server_name", CodeServerNameUnsupported},
		{"server name on a hop with two nodes", func(r *model.Route, o *Options) {
			on(r, o, func(r *model.Route, _ *Options) {
				r.Hops[1].NodeRefs = []string{"forward-72", "forward-73"}
				r.Hops[1].Ingress.ServerName = "forward-72"
			})
		}, "hops[1].ingress.server_name", CodeServerNameUnsupported},
		{"plain carrier on a user's route", func(r *model.Route, o *Options) {
			on(r, o, func(r *model.Route, _ *Options) { r.Owner = "user:7"; r.Hops[1].Ingress.Carrier = model.CarrierPlain })
		}, carrier, CodePlainUntrusted},
		{"plain carrier on an unlabelled node", func(r *model.Route, o *Options) {
			on(r, o, func(r *model.Route, o *Options) {
				r.Hops[1].Ingress.Carrier = model.CarrierPlain
				o.Nodes[1].Labels = nil
			})
		}, carrier, CodePlainUntrusted},
		{"carrier a node lacks", func(r *model.Route, o *Options) {
			on(r, o, func(_ *model.Route, o *Options) {
				o.Nodes[1].Engines[0].Carriers = []model.AnixOpsCarrier{model.CarrierTLSTCP}
			})
		}, carrier, CodeCarrierUnsupported},
		{"auto without tls", func(r *model.Route, o *Options) {
			on(r, o, func(r *model.Route, o *Options) {
				r.Hops[1].Ingress.Carrier = model.CarrierAuto
				o.Nodes[0].Engines[0].Carriers = []model.AnixOpsCarrier{model.CarrierQUIC}
			})
		}, carrier, CodeCarrierUnsupported},
		{"no shared wire version", func(r *model.Route, o *Options) {
			on(r, o, func(_ *model.Route, o *Options) { o.Nodes[1].Engines[0].ProtocolVersions = []uint32{2} })
		}, "hops[1].ingress.security", CodeCapabilityMissing},
		{"no reported wire version", func(r *model.Route, o *Options) {
			on(r, o, func(_ *model.Route, o *Options) { o.Nodes[2].Engines[0].ProtocolVersions = nil })
		}, "hops[2].ingress.security", CodeCapabilityMissing},
	}
}

func TestAnixOpsRouteIsValidOnlyWhenEnabled(t *testing.T) {
	r := anixopsRoute()
	if vs := Route(&r, Options{EnableAnixOps: true, Nodes: anixopsNodes()}); len(vs) != 0 {
		t.Fatalf("enabled: %v", vs)
	}
	if vs := Route(&r, Options{EnableAnixOps: true}); len(vs) != 0 {
		t.Fatalf("enabled, no inventory: %v", vs)
	}
	// Mux either way, a node's own identity name, AUTO and the plain
	// carrier on an administrator's labelled nodes are accepted.
	r.Hops[1].Ingress.Mux = true
	r.Hops[1].Ingress.ServerName = "FORWARD-72"
	r.Hops[1].Ingress.Carrier = model.CarrierPlain
	r.Hops[2].Ingress.Carrier = model.CarrierAuto
	if vs := Route(&r, Options{EnableAnixOps: true, Nodes: anixopsNodes()}); len(vs) != 0 {
		t.Fatalf("variants: %v", vs)
	}
	// Off by default: the engine is refused, and no anixops rule adds noise.
	vs := Route(&r, Options{Nodes: anixopsNodes()})
	for _, v := range vs {
		if v.Code != CodeEngineNotEnabled {
			t.Errorf("unexpected %v with the flag off", v)
		}
	}
	if len(vs) == 0 {
		t.Fatal("anixops is accepted with the flag off")
	}
}
