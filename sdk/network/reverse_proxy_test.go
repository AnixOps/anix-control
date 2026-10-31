package network

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	networkv1 "github.com/AnixOps/anix-control/sdk/api/network/v1"
)

const testCert = "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n"
const testKey = "-----BEGIN PRIVATE KEY-----\nMIIB\n-----END PRIVATE KEY-----\n"

// withReverseProxy adds a TLS reverse proxy over transport tr-jp to the
// ingress golden plan.
func withReverseProxy(t *testing.T) *networkv1.NetworkPlan {
	t.Helper()
	plan := readPlan(t, "plan-anx-line-ingress.json")
	plan.ReverseProxies = []*networkv1.ReverseProxy{{
		Id: "rp-1", Listen: &networkv1.Endpoint{Address: "0.0.0.0", Port: 443},
		CertificatePem: testCert, PrivateKeyPem: testKey,
		Sites: []*networkv1.ReverseProxySite{
			{Hosts: []string{"app.example.com", "*.app.example.com"}, Routes: []*networkv1.ReverseProxyRoute{
				{Path: "/api", StripPrefix: true, HostHeader: "upstream", Headers: map[string]string{"X-Route": "api"},
					Upstreams: []*networkv1.ForwardTarget{{Host: "10.0.0.5", Port: 8080}, {Host: "api.internal", Port: 8080}},
					Policy:    &networkv1.ForwardPolicy{Balance: "least_conn", Health: &networkv1.ForwardHealth{IntervalMs: 5000, TimeoutMs: 2000}}},
				{Path: "/", Host: "origin.example.com:8443", Upstreams: []*networkv1.ForwardTarget{{Host: "10.0.0.6", Port: 80}}},
			}},
			{Hosts: []string{"*"}, Routes: []*networkv1.ReverseProxyRoute{{Upstreams: []*networkv1.ForwardTarget{{Host: "10.0.0.7", Port: 80}}}}},
		},
		Next: &networkv1.ReverseProxy_TransportId{TransportId: "tr-jp"},
	}, {
		Id: "rp-2", Listen: &networkv1.Endpoint{Address: "0.0.0.0", Port: 80},
		Sites: []*networkv1.ReverseProxySite{{Hosts: []string{"app.example.com"}, RedirectHttps: true}},
		Next:  &networkv1.ReverseProxy_TransportId{TransportId: "tr-jp"},
	}}
	return plan
}

func TestReverseProxies(t *testing.T) {
	require.NoError(t, Check(withReverseProxy(t)))
	require.NoError(t, CheckReverseProxy(withReverseProxy(t).ReverseProxies[0]))

	route := func(p *networkv1.NetworkPlan) *networkv1.ReverseProxyRoute {
		return p.ReverseProxies[0].Sites[0].Routes[0]
	}
	cases := []struct {
		name      string
		breakPlan func(p *networkv1.NetworkPlan)
		want      string
	}{
		{"bad id", func(p *networkv1.NetworkPlan) { p.ReverseProxies[0].Id = "a b" }, "must be 1 to 64"},
		{"twice", func(p *networkv1.NetworkPlan) { p.ReverseProxies[1].Id = "rp-1" }, "listed twice"},
		{"listen taken by ingress", func(p *networkv1.NetworkPlan) { p.ReverseProxies[0].Listen.Port = 8388 }, "taken"},
		{"listen taken by forward", func(p *networkv1.NetworkPlan) {
			p.PortForwards = []*networkv1.PortForward{{Id: "rt-1", Listen: &networkv1.Endpoint{Address: "0.0.0.0", Port: 443},
				Protocols: []string{"tcp"}, Targets: []*networkv1.ForwardTarget{{Host: "10.0.0.5", Port: 22}},
				Next: &networkv1.PortForward_TransportId{TransportId: "tr-jp"}}}
		}, "taken"},
		{"cert without key", func(p *networkv1.NetworkPlan) { p.ReverseProxies[0].PrivateKeyPem = "" }, "both set"},
		{"not pem", func(p *networkv1.NetworkPlan) { p.ReverseProxies[0].CertificatePem = "hello" }, "PEM"},
		{"no sites", func(p *networkv1.NetworkPlan) { p.ReverseProxies[0].Sites = nil }, "sites are required"},
		{"no hosts", func(p *networkv1.NetworkPlan) { p.ReverseProxies[0].Sites[0].Hosts = nil }, "hosts are required"},
		{"bad host", func(p *networkv1.NetworkPlan) { p.ReverseProxies[0].Sites[0].Hosts[0] = "a..b" }, "not a name"},
		{"bad wildcard", func(p *networkv1.NetworkPlan) { p.ReverseProxies[0].Sites[0].Hosts[0] = "a.*.com" }, "not a name"},
		{"host twice", func(p *networkv1.NetworkPlan) { p.ReverseProxies[0].Sites[1].Hosts = []string{"APP.example.com"} }, "listed twice"},
		{"no routes", func(p *networkv1.NetworkPlan) { p.ReverseProxies[0].Sites[1].Routes = nil }, "routes are required"},
		{"redirect with routes", func(p *networkv1.NetworkPlan) { p.ReverseProxies[0].Sites[0].RedirectHttps = true }, "no routes"},
		{"path twice", func(p *networkv1.NetworkPlan) { p.ReverseProxies[0].Sites[0].Routes[1].Path = "/api" }, "listed twice"},
		{"root twice", func(p *networkv1.NetworkPlan) { route(p).Path = "" }, "listed twice"},
		{"trailing slash", func(p *networkv1.NetworkPlan) { route(p).Path = "/api/" }, "trailing"},
		{"relative path", func(p *networkv1.NetworkPlan) { route(p).Path = "api" }, "start with /"},
		{"host header", func(p *networkv1.NetworkPlan) { route(p).HostHeader = "custom" }, "host_header"},
		{"bad fixed host", func(p *networkv1.NetworkPlan) { route(p).Host = "a b" }, "host[:port]"},
		{"bad header name", func(p *networkv1.NetworkPlan) { route(p).Headers = map[string]string{"X Bad": "1"} }, "header name"},
		{"host header set", func(p *networkv1.NetworkPlan) { route(p).Headers = map[string]string{"Host": "x"} }, "set by the proxy"},
		{"framing header set", func(p *networkv1.NetworkPlan) {
			route(p).Headers = map[string]string{"Transfer-Encoding": "chunked"}
		}, "set by the proxy"},
		{"header injection", func(p *networkv1.NetworkPlan) { route(p).Headers = map[string]string{"X-A": "1\r\nX-B: 2"} }, "bad value"},
		{"no upstreams", func(p *networkv1.NetworkPlan) { route(p).Upstreams = nil }, "upstreams are required"},
		{"bad upstream", func(p *networkv1.NetworkPlan) { route(p).Upstreams[1].Port = 0 }, "upstreams[1]"},
		{"balance", func(p *networkv1.NetworkPlan) { route(p).Policy.Balance = "fastest" }, "balance"},
		{"proxy protocol", func(p *networkv1.NetworkPlan) { route(p).Policy.ProxyProtocol = "v2" }, "not used"},
		{"max conns", func(p *networkv1.NetworkPlan) { route(p).Policy.MaxConns = 10 }, "not used"},
		{"no next", func(p *networkv1.NetworkPlan) { p.ReverseProxies[0].Next = nil }, "next is required"},
		{"hop egress", func(p *networkv1.NetworkPlan) {
			p.ReverseProxies[0].Next = &networkv1.ReverseProxy_EgressId{EgressId: "eg-ix"}
		}, "internet egress"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			plan := withReverseProxy(t)
			c.breakPlan(plan)
			err := Check(plan)
			require.ErrorIs(t, err, ErrInvalid)
			require.Contains(t, err.Error(), c.want)
		})
	}
}

func TestReverseProxyStatus(t *testing.T) {
	status := &networkv1.NetworkStatus{DeviceRef: "proxy-1", ReverseProxies: []*networkv1.ReverseProxyStatus{{
		Id: "rp-1", State: networkv1.ComponentState_COMPONENT_STATE_APPLIED, UpBytes: 10, DownBytes: 20,
		Upstreams: []*networkv1.ForwardTargetHealth{
			{Host: "10.0.0.5", Port: 8080, State: networkv1.TargetHealthState_TARGET_HEALTH_STATE_HEALTHY, CheckedAtUnixMs: 1},
		},
	}}}
	require.NoError(t, CheckStatus(status, "proxy-1"))
	for _, broken := range []func(*networkv1.ReverseProxyStatus){
		func(p *networkv1.ReverseProxyStatus) { p.Id = "a b" },
		func(p *networkv1.ReverseProxyStatus) { p.State = 99 },
		func(p *networkv1.ReverseProxyStatus) { p.Upstreams[0].Port = 0 },
		func(p *networkv1.ReverseProxyStatus) { p.Upstreams[0].State = 99 },
		func(p *networkv1.ReverseProxyStatus) { p.Upstreams[0].CheckedAtUnixMs = -1 },
	} {
		bad := proto.Clone(status).(*networkv1.NetworkStatus)
		broken(bad.ReverseProxies[0])
		require.ErrorIs(t, CheckStatus(bad, "proxy-1"), ErrInvalid)
	}
}
