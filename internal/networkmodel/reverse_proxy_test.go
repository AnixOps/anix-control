package networkmodel

import (
	"testing"

	networkv1 "github.com/AnixOps/anix-control/sdk/api/network/v1"
	"github.com/AnixOps/anix-control/sdk/network"
	"github.com/stretchr/testify/require"
)

func webProxy() Forward {
	return Forward{
		ID: "rp-1", Entries: []string{"proxy-1"}, Listen: Endpoint{Address: "0.0.0.0", Port: 443},
		Segments: []string{"tr-jp"}, Egress: "eg-jp",
		ReverseProxy: &ReverseProxySpec{Sites: []*networkv1.ReverseProxySite{{
			Hosts:  []string{"app.example.com"},
			Routes: []*networkv1.ReverseProxyRoute{{Upstreams: []*networkv1.ForwardTarget{{Host: "10.1.0.5", Port: 8080}}}},
		}}},
	}
}

// A reverse proxy rides a forward's chain: the entry runs it into tr-jp,
// the transport is planned once, and its listen is its own.
func TestPlanReverseProxy(t *testing.T) {
	result, err := Plan(twoHops(), map[string]string{"tr-jp": linkKey}, japanKey(), []Forward{webProxy(), sshForward()})
	require.NoError(t, err)
	require.Empty(t, result.Pending)
	entry := result.Plans["proxy-1"]
	require.NoError(t, network.Check(entry))
	require.Len(t, entry.GetReverseProxies(), 1)
	proxy := entry.GetReverseProxies()[0]
	require.Equal(t, "tr-jp", proxy.GetTransportId())
	require.Equal(t, "app.example.com", proxy.GetSites()[0].GetHosts()[0])
	require.Len(t, entry.GetPortForwards(), 1)
	require.Len(t, entry.GetTransports(), 1)

	// On the exit device it leaves by the egress.
	local := webProxy()
	local.Entries, local.Segments = []string{"proxy-2"}, nil
	result, err = Plan(twoHops(), map[string]string{"tr-jp": linkKey}, japanKey(), []Forward{local})
	require.NoError(t, err)
	require.Empty(t, result.Pending)
	require.Equal(t, "eg-jp", result.Plans["proxy-2"].GetReverseProxies()[0].GetEgressId())

	// A taken listen or a bad site waits, named as a reverse proxy.
	clash := sshForward()
	clash.Listen.Port = 443
	result, err = Plan(twoHops(), map[string]string{"tr-jp": linkKey}, japanKey(), []Forward{webProxy(), clash})
	require.NoError(t, err)
	require.Len(t, result.Pending, 1)
	require.Contains(t, result.Pending[0].Reason, "taken")

	bad := webProxy()
	bad.ReverseProxy.Sites[0].Hosts = []string{"a..b"}
	result, err = Plan(twoHops(), map[string]string{"tr-jp": linkKey}, japanKey(), []Forward{bad})
	require.NoError(t, err)
	require.Len(t, result.Pending, 1)
	require.Equal(t, "rp-1", result.Pending[0].ReverseProxy)
	require.Empty(t, result.Plans["proxy-1"].GetReverseProxies())
}
