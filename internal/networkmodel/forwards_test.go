package networkmodel

import (
	"testing"

	networkv1 "github.com/AnixOps/anix-control/sdk/api/network/v1"
	"github.com/stretchr/testify/require"
)

func sshForward() Forward {
	return Forward{
		ID: "01HXROUTE0000000000000000A", Entries: []string{"proxy-1"}, Listen: Endpoint{Address: "0.0.0.0", Port: 20022},
		Protocols: []string{"tcp"}, Targets: []*networkv1.ForwardTarget{{Host: "10.1.0.5", Port: 22}},
		Policy:   &networkv1.ForwardPolicy{Balance: "failover", MaxConns: 10},
		Segments: []string{"tr-jp"}, Egress: "eg-jp",
	}
}

// A forward over the path's transport shares it: the entry gets a port
// forward into tr-jp, the transport is planned once.
func TestPlanForwardSharesTheTransport(t *testing.T) {
	result, err := Plan(twoHops(), map[string]string{"tr-jp": linkKey}, japanKey(), []Forward{sshForward()})
	require.NoError(t, err)
	require.Empty(t, result.Pending)
	entry := result.Plans["proxy-1"]
	require.Len(t, entry.GetPortForwards(), 1)
	forward := entry.GetPortForwards()[0]
	require.Equal(t, "tr-jp", forward.GetTransportId())
	require.Equal(t, uint32(20022), forward.GetListen().GetPort())
	require.Equal(t, "failover", forward.GetPolicy().GetBalance())
	require.Len(t, entry.GetTransports(), 1)
	require.Len(t, result.Plans["proxy-2"].GetForwarding(), 1)

	// Without the path the forward plans the transport itself.
	doc := twoHops()
	doc.Paths = nil
	doc.Ingresses[0].Pairing = nil
	doc.Ingresses = doc.Ingresses[1:]
	result, err = Plan(doc, map[string]string{"tr-jp": linkKey}, japanKey(), []Forward{sshForward()})
	require.NoError(t, err)
	require.Empty(t, result.Pending)
	require.Len(t, result.Plans["proxy-1"].GetTransports(), 1)
	require.Equal(t, "eg-jp", result.Plans["proxy-2"].GetForwarding()[0].GetEgressId())

	// A forward on the exit device leaves by the egress directly.
	local := sshForward()
	local.Entries, local.Segments = []string{"proxy-2"}, nil
	result, err = Plan(twoHops(), map[string]string{"tr-jp": linkKey}, japanKey(), []Forward{local})
	require.NoError(t, err)
	require.Empty(t, result.Pending)
	require.Equal(t, "eg-jp", result.Plans["proxy-2"].GetPortForwards()[0].GetEgressId())
}

// A forward that cannot be planned waits; the document stays valid.
func TestPlanForwardWaits(t *testing.T) {
	cases := map[string]struct {
		edit func(f *Forward, d *Document)
		keys map[string]string
		want string
	}{
		"unknown entry": {edit: func(f *Forward, _ *Document) { f.Entries = []string{"proxy-9"} }, want: "no line"},
		"wrong start":   {edit: func(f *Forward, _ *Document) { f.Entries = []string{"proxy-2"} }, want: "leaves proxy-1"},
		"egress":        {edit: func(f *Forward, _ *Document) { f.Egress = "eg-ix" }, want: "not an internet egress"},
		"no key":        {edit: func(*Forward, *Document) {}, keys: map[string]string{}, want: "no link key"},
		"listen taken":  {edit: func(f *Forward, _ *Document) { f.Listen.Port = 8388 }, want: "taken by ingress in-ss"},
		"wrong hop":     {edit: func(f *Forward, _ *Document) { f.Via = []string{"proxy-3"} }, want: "hop 1 is on proxy-3"},
		"bad policy":    {edit: func(f *Forward, _ *Document) { f.Policy.Balance = "fastest" }, want: "balance"},
		"continues elsewhere": {edit: func(f *Forward, d *Document) {
			d.Egresses = append(d.Egresses, Egress{ID: "eg-jp2", Line: "ln-jp", Kind: "nat_direct", Audience: "internet", Family: "v4", Source: "auto"})
			f.Egress = "eg-jp2"
		}, want: "continues elsewhere"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			doc, forward := twoHops(), sshForward()
			c.edit(&forward, doc)
			keys := c.keys
			if keys == nil {
				keys = map[string]string{"tr-jp": linkKey}
			}
			result, err := Plan(doc, keys, japanKey(), []Forward{forward})
			require.NoError(t, err)
			var reasons []string
			for _, pending := range result.Pending {
				if pending.Forward == forward.ID {
					reasons = append(reasons, pending.Reason)
				}
			}
			require.Len(t, reasons, 1, "%v", result.Pending)
			require.Contains(t, reasons[0], c.want)
			require.Empty(t, result.Plans["proxy-1"].GetPortForwards())
		})
	}

	paused := sshForward()
	paused.Paused = true
	result, err := Plan(twoHops(), map[string]string{"tr-jp": linkKey}, japanKey(), []Forward{paused})
	require.NoError(t, err)
	require.Empty(t, result.Pending)
	require.Empty(t, result.Plans["proxy-1"].GetPortForwards())
}

// threeEntries adds a second entry device, proxy-3, with its own
// transport into proxy-2's hop ingress, and an internet egress of its own.
func threeEntries() *Document {
	doc := twoHops()
	doc.Lines = append(doc.Lines, Line{ID: "ln-hk", Node: "proxy-3", Interface: "eth0", Families: []string{"v4"}, GatewayV4: "198.51.100.1"})
	doc.Egresses = append(doc.Egresses,
		Egress{ID: "eg-hk", Line: "ln-hk", Kind: "hop", Audience: "hop", Family: "v4", Source: "198.51.100.10"},
		Egress{ID: "eg-hk-net", Line: "ln-hk", Kind: "nat_direct", Audience: "internet", Family: "v4", Source: "198.51.100.10"})
	doc.Transports = append(doc.Transports,
		Transport{ID: "tr-hk", Kind: "anx_line", FromEgress: "eg-hk", ToIngress: "in-hop", Trusted: true, AnxLine: &AnxLine{QUICPort: 8443}})
	return doc
}

// Several entries each run the port forward: the first on the named
// transport, another on its own transport into the same ingress, which
// continues the same way.
func TestPlanForwardSeveralEntries(t *testing.T) {
	keys := map[string]string{"tr-jp": linkKey, "tr-hk": linkKey}
	forward := sshForward()
	forward.Entries = []string{"proxy-1", "proxy-3"}
	forward.Via = []string{"proxy-2"}
	result, err := Plan(threeEntries(), keys, japanKey(), []Forward{forward})
	require.NoError(t, err)
	require.Empty(t, result.Pending)
	require.Equal(t, "tr-jp", result.Plans["proxy-1"].GetPortForwards()[0].GetTransportId())
	second := result.Plans["proxy-3"]
	require.Len(t, second.GetPortForwards(), 1)
	require.Equal(t, "tr-hk", second.GetPortForwards()[0].GetTransportId())
	require.Equal(t, uint32(20022), second.GetPortForwards()[0].GetListen().GetPort())
	require.Len(t, second.GetTransports(), 1)
	var forwarding []string
	for _, f := range result.Plans["proxy-2"].GetForwarding() {
		forwarding = append(forwarding, f.GetFromTransportId()+">"+f.GetEgressId())
	}
	require.ElementsMatch(t, []string{"tr-jp>eg-jp", "tr-hk>eg-jp"}, forwarding)

	// Without a key for its transport the second entry waits, named, and
	// the first runs.
	result, err = Plan(threeEntries(), map[string]string{"tr-jp": linkKey}, japanKey(), []Forward{forward})
	require.NoError(t, err)
	require.Len(t, result.Pending, 1)
	require.Contains(t, result.Pending[0].Reason, "entry proxy-3: no enabled transport")
	require.Len(t, result.Plans["proxy-1"].GetPortForwards(), 1)
	require.Empty(t, result.Plans["proxy-3"].GetPortForwards())

	// A transport that continues elsewhere (for a forward planned
	// earlier) is not chosen.
	doc := threeEntries()
	doc.Egresses = append(doc.Egresses, Egress{ID: "eg-jp2", Line: "ln-jp", Kind: "nat_direct", Audience: "internet", Family: "v4", Source: "auto"})
	other := sshForward()
	other.ID, other.Entries, other.Segments, other.Egress, other.Listen.Port = "01HXROUTE00000000000000000", []string{"proxy-3"}, []string{"tr-hk"}, "eg-jp2", 20023
	result, err = Plan(doc, keys, japanKey(), []Forward{forward, other})
	require.NoError(t, err)
	require.Len(t, result.Pending, 1)
	require.Contains(t, result.Pending[0].Reason, "entry proxy-3: no enabled transport")
	require.Equal(t, "01HXROUTE00000000000000000", result.Plans["proxy-3"].GetPortForwards()[0].GetId())

	// Without segments each entry leaves by an internet egress of its own.
	local := sshForward()
	local.Entries, local.Segments, local.Egress = []string{"proxy-2", "proxy-3"}, nil, "eg-jp"
	result, err = Plan(threeEntries(), keys, japanKey(), []Forward{local})
	require.NoError(t, err)
	require.Empty(t, result.Pending)
	require.Equal(t, "eg-jp", result.Plans["proxy-2"].GetPortForwards()[0].GetEgressId())
	require.Equal(t, "eg-hk-net", result.Plans["proxy-3"].GetPortForwards()[0].GetEgressId())
}
