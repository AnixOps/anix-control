package networkmodel

import (
	"context"
	"testing"

	networkv1 "github.com/AnixOps/anix-control/sdk/api/network/v1"
	"github.com/AnixOps/anix-control/sdk/network"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// seenAgents records proxy nodes' Agents on the mTLS stream.
func seenAgents(t *testing.T, s *Service, ids ...uint) {
	t.Helper()
	for _, id := range ids {
		require.NoError(t, s.DB.Create(&model.AgentTransport{NodeKind: "proxy", NodeID: id, Transport: model.AgentTransportMTLSStream,
			FirstSeenAt: s.now(), LastSeenAt: s.now()}).Error)
	}
}

func autoForward() Forward {
	return Forward{
		ID: "01HXAUTO000000000000000000", Entries: []string{"proxy-1"}, Listen: Endpoint{Address: "::", Port: 20022},
		Protocols: []string{"tcp"}, Targets: []*networkv1.ForwardTarget{{Host: "10.1.0.5", Port: 22}},
		Segments: []string{""}, Via: []string{"proxy-2"},
	}
}

// A route that names only its devices runs on Control's own line,
// egresses, hop ingress and ANX-A transport, with a sealed link key that
// goes away with the route.
func TestServiceCompletesRoutes(t *testing.T) {
	s := newService(t)
	seenAgents(t, s, 1, 2)
	ctx := context.Background()
	forwards := []Forward{autoForward()}
	s.Forwards = func(*gorm.DB) ([]Forward, error) { return forwards, nil }

	view, err := s.Replace(ctx, &Document{}, nil, Actor{})
	require.NoError(t, err)
	require.Equal(t, []AgentNode{{Node: "proxy-1", Name: "n"}, {Node: "proxy-2", Name: "n"}}, view.Nodes)
	require.Len(t, view.Pending, 1)
	require.Contains(t, view.Pending[0].Reason, "proxy-2 has not reported its ANX-Line server key")
	var key model.NetworkTransportKey
	require.NoError(t, s.DB.First(&key, "transport_id = ?", "auto-proxy-1-proxy-2").Error)

	require.NoError(t, s.SetPublicKeys(ctx, "proxy-2", map[string]string{"anx_line": "anx1-japan"}))
	entry, found, err := s.DevicePlan(ctx, "proxy-1")
	require.NoError(t, err)
	require.True(t, found)
	require.NoError(t, network.Check(entry))
	require.Equal(t, "auto-proxy-1-proxy-2", entry.GetPortForwards()[0].GetTransportId())
	transport := entry.GetTransports()[0]
	require.Equal(t, "auto-proxy-1-hop", transport.GetFromEgressId())
	require.Equal(t, "l2", transport.GetAnxLine().GetEgress(), "ANX-A: the jacket")
	require.Equal(t, AutoServerName, transport.GetAnxLine().GetServerName())
	require.Equal(t, "h.example.com:24443", transport.GetAnxLine().GetTcpAddr())
	require.Len(t, transport.GetAnxLine().GetLinkKey(), 64)

	exit, _, err := s.DevicePlan(ctx, "proxy-2")
	require.NoError(t, err)
	require.NoError(t, network.Check(exit))
	require.Equal(t, "auto-proxy-2-hop", exit.GetIngresses()[0].GetId())
	require.Equal(t, uint32(AutoHopPort), exit.GetIngresses()[0].GetListen().GetPort())
	require.Equal(t, "auto-proxy-2-internet", exit.GetForwarding()[0].GetEgressId())
	require.Equal(t, transport.GetAnxLine().GetLinkKey(), exit.GetTransports()[0].GetAnxLine().GetLinkKey())

	// Declared ids may not start with auto-.
	_, err = s.Replace(ctx, &Document{Lines: []Line{{ID: "auto-x", Node: "proxy-1"}}}, nil, Actor{})
	var invalid *Invalid
	require.ErrorAs(t, err, &invalid)

	// The route is gone: so are its components and its key.
	forwards = nil
	_, err = s.Replace(ctx, &Document{}, nil, Actor{})
	require.NoError(t, err)
	var count int64
	require.NoError(t, s.DB.Model(&model.NetworkTransportKey{}).Count(&count).Error)
	require.Zero(t, count)
	empty, _, err := s.DevicePlan(ctx, "proxy-1")
	require.NoError(t, err)
	require.Empty(t, empty.GetLines())
}

// complete leaves declared parts alone and links extra entries.
func TestCompleteUsesDeclaredParts(t *testing.T) {
	hosts := map[string]string{"proxy-1": "a.example.com", "proxy-2": "2001:db8::2", "proxy-3": "c.example.com"}
	forward := sshForward()
	forward.Entries = []string{"proxy-1", "proxy-3"}
	forward.Segments, forward.Egress = []string{"tr-jp"}, ""
	full, forwards := complete(twoHops(), []Forward{forward}, hosts)
	require.Equal(t, []string{"tr-jp"}, forwards[0].Segments)
	require.Equal(t, "eg-jp", forwards[0].Egress, "the exit device's declared internet egress")
	require.Len(t, full.Lines, 3, "proxy-3 gets a line")
	require.Len(t, full.Transports, 1, "an extra entry on a declared transport gets none")

	// Over an auto segment, the extra entry gets its own transport; a v6
	// device is reached on v6.
	forward.Segments, forward.Via = []string{""}, []string{"proxy-2"}
	full, forwards = complete(twoHops(), []Forward{forward}, hosts)
	require.Equal(t, []string{"auto-proxy-1-proxy-2"}, forwards[0].Segments)
	ids := map[string]bool{}
	for _, transport := range full.Transports {
		ids[transport.ID] = true
	}
	require.True(t, ids["auto-proxy-3-proxy-2"])
	for _, ingress := range full.Ingresses {
		if ingress.ID == "auto-proxy-2-hop" {
			require.Equal(t, "v6", ingress.Family)
			require.Equal(t, "::", ingress.Listen.Address)
		}
	}

	// A device without an address cannot be linked: the route waits.
	delete(hosts, "proxy-2")
	full, forwards = complete(twoHops(), []Forward{forward}, hosts)
	result, err := Plan(full, map[string]string{"tr-jp": linkKey}, japanKey(), forwards)
	require.NoError(t, err)
	require.Len(t, result.Pending, 2)
	require.Contains(t, result.Pending[0].Reason, "cannot link to proxy-2")
}
