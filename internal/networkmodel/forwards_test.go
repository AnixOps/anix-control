package networkmodel

import (
	"testing"

	networkv1 "github.com/AnixOps/anix-control/sdk/api/network/v1"
	"github.com/stretchr/testify/require"
)

func sshForward() Forward {
	return Forward{
		ID: "01HXROUTE0000000000000000A", Entry: "proxy-1", Listen: Endpoint{Address: "0.0.0.0", Port: 20022},
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
	local.Entry, local.Segments = "proxy-2", nil
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
		"unknown entry": {edit: func(f *Forward, _ *Document) { f.Entry = "proxy-9" }, want: "no line"},
		"wrong start":   {edit: func(f *Forward, _ *Document) { f.Entry = "proxy-2" }, want: "leaves proxy-1"},
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
