package networkmodel

import (
	"context"
	"strings"
	"testing"

	networkv1 "github.com/AnixOps/anix-control/sdk/api/network/v1"
	"github.com/AnixOps/anix-control/sdk/network"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/nodeentry"
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
	full, forwards := complete(twoHops(), []Forward{forward}, hostsOnly(hosts))
	require.Equal(t, []string{"tr-jp"}, forwards[0].Segments)
	require.Equal(t, "eg-jp", forwards[0].Egress, "the exit device's declared internet egress")
	require.Len(t, full.Lines, 3, "proxy-3 gets a line")
	require.Len(t, full.Transports, 1, "an extra entry on a declared transport gets none")

	// Over an auto segment, the extra entry gets its own transport; a v6
	// device is reached on v6.
	forward.Segments, forward.Via = []string{""}, []string{"proxy-2"}
	full, forwards = complete(twoHops(), []Forward{forward}, hostsOnly(hosts))
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
	full, forwards = complete(twoHops(), []Forward{forward}, hostsOnly(hosts))
	result, err := Plan(full, map[string]string{"tr-jp": linkKey}, japanKey(), forwards)
	require.NoError(t, err)
	require.Len(t, result.Pending, 2)
	require.Contains(t, result.Pending[0].Reason, "cannot link to proxy-2")
}

// hostsOnly answers devices that are only reached at their hosts.
func hostsOnly(hosts map[string]string) map[string]device {
	out := map[string]device{}
	for ref, host := range hosts {
		out[ref] = device{Host: host}
	}
	return out
}

// A device's network settings and NAT entries shape its auto parts.
func TestCompleteFollowsDeviceSettings(t *testing.T) {
	forward := sshForward()
	forward.Entries, forward.Segments, forward.Via, forward.Egress = []string{"proxy-7"}, []string{""}, []string{"proxy-8"}, ""
	devices := map[string]device{
		"proxy-7": {Host: "sh.example.com", Network: model.NodeNetwork{
			Interface: "eth1", SourceV4: "10.0.0.7", GatewayV4: "10.0.0.1",
		}},
		"proxy-8": {Host: "203.0.113.8", Network: model.NodeNetwork{HopPort: 30001},
			NAT: []natEntry{{Host: "198.51.100.8", Ranges: []nodeentry.PortRange{{From: 30000, To: 30010, External: 40000}}}}},
	}
	full, forwards := complete(&Document{}, []Forward{forward}, devices)
	require.Equal(t, []string{"auto-proxy-7-proxy-8"}, forwards[0].Segments)
	require.Equal(t, "auto-proxy-8-internet", forwards[0].Egress)
	lines := map[string]Line{}
	for _, line := range full.Lines {
		lines[line.Node] = line
	}
	require.Equal(t, "eth1", lines["proxy-7"].Interface)
	require.Equal(t, []string{"v4"}, lines["proxy-7"].Families)
	require.Equal(t, "10.0.0.1", lines["proxy-7"].GatewayV4)
	for _, egress := range full.Egresses {
		switch egress.ID {
		case "auto-proxy-7-hop":
			require.Equal(t, "10.0.0.7", egress.Source, "the hop egress leaves from the settings' source")
		case "auto-proxy-8-internet":
			require.Equal(t, network.SourceAuto, egress.Source, "no settings: the default route")
		}
	}
	require.Len(t, full.Ingresses, 1)
	ingress := full.Ingresses[0]
	require.Equal(t, network.IngressNAT, ingress.Kind)
	require.Equal(t, "198.51.100.8", ingress.PublicHost)
	require.Equal(t, uint32(30001), ingress.Listen.Port)
	require.Equal(t, []PortMap{{Public: PortRange{First: 40001, Last: 40001}, Local: PortRange{First: 30001, Last: 30001}}}, ingress.PortMap)

	// Without a NAT entry that maps the hop port, the ingress is direct at
	// the node's host.
	devices["proxy-8"] = device{Host: "203.0.113.8", Network: model.NodeNetwork{HopPort: 31000}, NAT: devices["proxy-8"].NAT}
	full, _ = complete(&Document{}, []Forward{forward}, devices)
	require.Equal(t, network.IngressDirect, full.Ingresses[0].Kind)
	require.Equal(t, "203.0.113.8", full.Ingresses[0].PublicHost)
}

// A dialer reaches a NAT ingress at the public port it maps.
func TestPublicPortFollowsThePortMap(t *testing.T) {
	ingress := &Ingress{PortMap: []PortMap{{Public: PortRange{First: 40000, Last: 40010}, Local: PortRange{First: 30000, Last: 30010}}}}
	require.Equal(t, uint32(40003), publicPort(ingress, 30003))
	require.Equal(t, uint32(24443), publicPort(ingress, 24443))
	require.Equal(t, uint32(24443), publicPort(&Ingress{}, 24443))
}

// A route that picks the bare ANX-Line runs it trusted on its own hop
// ingress (TCP and QUIC at AutoLinePort), next to the A2 jacket's; a
// reversed one dials back to the dialing device.
func TestServiceCompletesLinkModes(t *testing.T) {
	s := newService(t)
	seenAgents(t, s, 1, 2)
	ctx := context.Background()
	jacket := autoForward()
	line := autoForward()
	line.ID, line.Listen.Port = "01HXAUTO000000000000000001", 20023
	line.Links = []Link{{Mode: LinkAnxLine, Carriers: []string{"c2", "c1"}, Multipath: true}}
	reverse := autoForward()
	reverse.ID, reverse.Listen.Port = "01HXAUTO000000000000000002", 20024
	reverse.Links = []Link{{Mode: LinkAnxLineReverse}}
	s.Forwards = func(*gorm.DB) ([]Forward, error) { return []Forward{jacket, line, reverse}, nil }
	require.NoError(t, s.SetPublicKeys(ctx, "proxy-2", map[string]string{"anx_line": "anx1-japan"}))

	view, err := s.Replace(ctx, &Document{}, nil, Actor{})
	require.NoError(t, err)
	require.Empty(t, view.Pending)
	entry, _, err := s.DevicePlan(ctx, "proxy-1")
	require.NoError(t, err)
	require.NoError(t, network.Check(entry))
	transports := map[string]*networkv1.Transport{}
	for _, transport := range entry.GetTransports() {
		transports[transport.GetId()] = transport
	}
	require.Len(t, transports, 3)
	require.Equal(t, "l2", transports["auto-proxy-1-proxy-2"].GetAnxLine().GetEgress())

	var lineID string
	for id := range transports {
		if strings.HasPrefix(id, "auto-proxy-1-proxy-2-line-") {
			lineID = id
		}
	}
	require.NotEmpty(t, lineID, "a shaped bare ANX-Line gets its own transport")
	bare := transports[lineID].GetAnxLine()
	require.Empty(t, bare.GetEgress(), "bare: l1")
	require.Equal(t, []string{"c2", "c1"}, bare.GetCarrierOrder())
	require.True(t, bare.GetMultipath())
	require.Equal(t, "h.example.com:24445", bare.GetTcpAddr())
	require.Equal(t, "h.example.com:24445", bare.GetQuicAddr())

	reversed := transports["auto-proxy-1-proxy-2-rev"].GetAnxLine()
	require.True(t, reversed.GetReverse())
	require.Empty(t, reversed.GetTcpAddr())
	require.Equal(t, "0.0.0.0:24444", reversed.GetReverseListen())

	exit, _, err := s.DevicePlan(ctx, "proxy-2")
	require.NoError(t, err)
	require.NoError(t, network.Check(exit))
	ingresses := map[string]uint32{}
	for _, ingress := range exit.GetIngresses() {
		ingresses[ingress.GetId()] = ingress.GetListen().GetPort()
	}
	require.Equal(t, map[string]uint32{"auto-proxy-2-hop": AutoHopPort, "auto-proxy-2-hop-line": AutoLinePort}, ingresses)
	for _, transport := range exit.GetTransports() {
		if transport.GetId() == "auto-proxy-1-proxy-2-rev" {
			require.Equal(t, "h.example.com:24444", transport.GetAnxLine().GetReverseDial())
		}
	}
}
