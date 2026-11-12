package netline

import (
	"encoding/json"
	"path/filepath"
	"testing"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	_ "github.com/AnixOps/anix-control/v5/internal/kernelforward"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "control.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Node{}, &model.NodeProtocol{}, &model.NodeEntry{}, &model.NodeExitPolicy{}, &model.NodeNetwork{},
		&model.NodeUplinks{}, &model.KernelForwardRoute{}, &model.ForwardRouteSubscription{}, &model.EgressRoute{}, &LineRow{}, &TransportRow{}))
	return db
}

func addRoute(t *testing.T, db *gorm.DB, route *forwardv1.Route, enforced string) {
	t.Helper()
	encoded, err := protojson.Marshal(route)
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.KernelForwardRoute{ID: route.GetId(), Owner: "test", Revision: 1, RouteJSON: string(encoded), Enforced: enforced}).Error)
}

func hop(engine forwardv1.Engine, nodes ...string) *forwardv1.Hop {
	return &forwardv1.Hop{Engine: engine, NodeRefs: nodes}
}

const (
	network = forwardv1.Engine_ENGINE_NETWORK
	kernel  = forwardv1.Engine_ENGINE_NFTABLES
)

// seed is three nodes (1 Shanghai entry, 2 Tencent entry, 3 Japan exit
// with a VLESS protocol on 443) and one route of every shape.
func seed(t *testing.T, db *gorm.DB) {
	t.Helper()
	for id, name := range map[uint]string{1: "sh", 2: "tencent", 3: "jp"} {
		require.NoError(t, db.Create(&model.Node{ID: id, Name: name, Host: name + ".example.com", Rate: 1.5}).Error)
	}
	require.NoError(t, db.Create(&model.NodeProtocol{ID: 7, NodeID: 3, Name: "JP VLESS", Type: model.ProtocolVLESS, Port: 443, Enable: 1, Show: 1}).Error)
	target := []*forwardv1.Target{{Host: "127.0.0.1", Port: 443}}
	// Userspace entry over the jacket to the exit, published.
	addRoute(t, db, &forwardv1.Route{
		Id: "relay", Name: "sh-jp", Listen: &forwardv1.Listen{Port: 31111, Protocol: forwardv1.L4Protocol_L4_PROTOCOL_TCP},
		Hops: []*forwardv1.Hop{hop(network, "proxy-1"), hop(network, "proxy-3")}, Targets: target,
	}, "")
	require.NoError(t, db.Create(&model.ForwardRouteSubscription{RouteID: "relay", DisplayName: "上海"}).Error)
	// Kernel entry to the userspace exit, over its quota.
	addRoute(t, db, &forwardv1.Route{
		Id: "kernel", Listen: &forwardv1.Listen{Port: 32222, Protocol: forwardv1.L4Protocol_L4_PROTOCOL_TCP_UDP},
		Hops:    []*forwardv1.Hop{hop(kernel, "proxy-2"), {Engine: network, NodeRefs: []string{"proxy-3"}, Port: 40000, DialAddress: "198.51.100.3"}},
		Targets: target, Policy: &forwardv1.Policy{NextHop: forwardv1.BalanceStrategy_BALANCE_STRATEGY_IP_HASH, Health: &forwardv1.HealthCheck{IntervalMs: 3000}},
	}, "quota")
	// Two entries, a bare line with QUIC, a declared exit, fixed targets.
	addRoute(t, db, &forwardv1.Route{
		Id: "fixed", Listen: &forwardv1.Listen{Address: "0.0.0.0", Port: 33333, Protocol: forwardv1.L4Protocol_L4_PROTOCOL_UDP, EntryHostname: "in.example.com"},
		Hops: []*forwardv1.Hop{hop(network, "proxy-1", "proxy-4"), {
			Engine: network, NodeRefs: []string{"proxy-3"}, NetworkEgressId: "jp-dedicated",
			Ingress: &forwardv1.LinkTransport{NetworkMode: forwardv1.NetworkTransportMode_NETWORK_TRANSPORT_MODE_ANX_LINE, NetworkCarriers: []forwardv1.NetworkCarrier{forwardv1.NetworkCarrier_NETWORK_CARRIER_QUIC}, NetworkFec: true},
		}},
		Targets: []*forwardv1.Target{{Host: "203.0.113.9", Port: 25565, Weight: 2}},
		Limits:  &forwardv1.Limits{BandwidthBps: 1000, MaxConns: 5}, Paused: true,
	}, "")
	require.NoError(t, db.Create(&model.Node{ID: 4, Name: "sh2", Host: "sh2.example.com", Rate: 1}).Error)
	// A declared transport, dialed back; a kernel-only route.
	addRoute(t, db, &forwardv1.Route{
		Id: "declared", Listen: &forwardv1.Listen{Port: 34444},
		Hops: []*forwardv1.Hop{hop(network, "proxy-4"), {Engine: network, NodeRefs: []string{"proxy-1"}, Ingress: &forwardv1.LinkTransport{NetworkTransportId: "t-ix"}},
			{Engine: network, NodeRefs: []string{"proxy-3"}, Ingress: &forwardv1.LinkTransport{NetworkMode: forwardv1.NetworkTransportMode_NETWORK_TRANSPORT_MODE_ANX_LINE_REVERSE}}},
		Targets: target,
	}, "")
	addRoute(t, db, &forwardv1.Route{
		Id: "nat-only", Listen: &forwardv1.Listen{Port: 35555}, Hops: []*forwardv1.Hop{hop(kernel, "proxy-2")},
		Targets: []*forwardv1.Target{{Host: "jp.example.com", Port: 443}},
	}, "")
	addRoute(t, db, &forwardv1.Route{Id: "gost", Hops: []*forwardv1.Hop{hop(forwardv1.Engine_ENGINE_GOST, "proxy-1")}}, "")
}

func TestLinesPlanWhatTheRoutesPlan(t *testing.T) {
	db := openDB(t)
	seed(t, db)
	report, err := Shadow(db)
	require.NoError(t, err)
	require.Equal(t, 5, report.Forwards)
	require.Empty(t, report.Differences)
	require.Equal(t, []Skipped{{ID: "gost", Reason: "hop 0 runs on ENGINE_GOST"}}, report.Snapshot.Skipped)
}

func TestImportReadsNodesAndRoutesAsLines(t *testing.T) {
	db := openDB(t)
	seed(t, db)
	require.NoError(t, db.Create(&model.NodeEntry{ID: 9, NodeID: 3, Label: "v6", Kind: model.NodeEntryDirect, Host: "2001:db8::3", Enabled: true}).Error)
	require.NoError(t, db.Create(&model.NodeExitPolicy{NodeID: 3, BlockMainland: true}).Error)
	require.NoError(t, db.Create(&model.EgressRoute{Name: "ai", SourceNodeID: 3, ExitNodeID: 4, RelayPort: 1, Method: "m", RelayKey: "k", Domains: `["openai.com"]`, Enabled: true}).Error)
	s, err := Import(db)
	require.NoError(t, err)
	lines := map[string]Line{}
	for _, line := range s.Lines {
		lines[line.ID] = line
	}
	require.Len(t, lines, 6)

	direct := lines["protocol-7"]
	require.Equal(t, []string{"proxy-3/9"}, direct.Entries)
	require.True(t, direct.direct())
	require.Equal(t, []Rule{{Name: "ai", Domains: []string{"openai.com"}, Exit: "proxy-4", Enabled: true}}, direct.Rules)

	relay := lines["relay"]
	require.Equal(t, "JP VLESS | 上海", relay.Name)
	require.True(t, relay.Show)
	require.Equal(t, 1.5, relay.Rate)
	require.Equal(t, &Protocol{ID: 7, Type: "vless", Device: "proxy-3", Port: 443}, relay.Protocol)
	require.Equal(t, "proxy-1>proxy-3/jacket", relay.Chain[1].Transport)
	require.Len(t, relay.Rules, 1)

	require.False(t, lines["kernel"].Enabled)
	require.Empty(t, lines["kernel"].Chain[1].Transport)
	require.Nil(t, lines["fixed"].Protocol)
	require.Equal(t, "jp-dedicated", lines["fixed"].Exit)

	devices := map[string]Device{}
	for _, device := range s.Devices {
		devices[device.Ref] = device
	}
	require.True(t, devices["proxy-2"].Kernel)
	require.False(t, devices["proxy-1"].Kernel)
	require.Contains(t, s.Exits, Exit{ID: "proxy-3", Device: "proxy-3", BlockMainland: true})
	require.Contains(t, s.Exits, Exit{ID: "jp-dedicated", Device: "proxy-3", Declared: true})
	require.Contains(t, s.Transports, Transport{ID: "t-ix", From: "proxy-4", To: "proxy-1", Kind: TransportJacket, Declared: true})
	require.Contains(t, s.Transports, Transport{ID: "proxy-1>proxy-3/line/c2/false/true", From: "proxy-1", To: "proxy-3", Kind: TransportLine, Carriers: []string{"c2"}, FEC: true})
}

func TestSavedLinesLoadAsImported(t *testing.T) {
	db := openDB(t)
	seed(t, db)
	require.NoError(t, db.Create(&model.EgressRoute{Name: "ai", SourceNodeID: 3, ExitNodeID: 1, RelayPort: 1, Method: "m", RelayKey: "k", Domains: `["openai.com"]`, Enabled: true}).Error)
	imported, err := Import(db)
	require.NoError(t, err)
	imported.Lines[0].ServeAt = "proxy-3"
	require.NoError(t, Save(db, imported))
	require.NoError(t, Save(db, imported), "saving again replaces the rows")

	loaded, err := Load(db)
	require.NoError(t, err)
	require.ElementsMatch(t, imported.Transports, loaded.Transports)
	loaded.Transports, imported.Transports = nil, nil
	loaded.Skipped = imported.Skipped
	want, err := json.Marshal(imported)
	require.NoError(t, err)
	got, err := json.Marshal(loaded)
	require.NoError(t, err)
	require.JSONEq(t, string(want), string(got))
	require.Empty(t, Compare(Forwards(imported), Forwards(loaded)))
}

func TestCompareNamesWhatDiffers(t *testing.T) {
	db := openDB(t)
	seed(t, db)
	s, err := Import(db)
	require.NoError(t, err)
	today := Forwards(s)
	for i := range s.Lines {
		if s.Lines[i].ID == "relay" {
			s.Lines[i].Port = 1
		}
	}
	s.Lines = append(s.Lines[:1], s.Lines[2:]...)
	differences := Compare(today, Forwards(s))
	require.Contains(t, differences, Difference{ID: "relay", What: "[Listen]"})
	require.Len(t, differences, 2)
}
