package l2standby

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/egressroute"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/networkmodel"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// hk (proxy-29) dials jp (proxy-50) over the cross-border transport t-a2;
// t-line is a trusted ANX-Line link that never gets a standby.
func testDocument() *networkmodel.Document {
	return &networkmodel.Document{
		Lines: []networkmodel.Line{
			{ID: "hk-l", Node: "proxy-29", Interface: "eth0"},
			{ID: "jp-l", Node: "proxy-50", Interface: "eth0"},
			{ID: "fw-l", Node: "forward-3", Interface: "eth0"},
		},
		Ingresses: []networkmodel.Ingress{
			{ID: "hk-in", Line: "hk-l"}, {ID: "jp-hop", Line: "jp-l"}, {ID: "fw-in", Line: "fw-l"},
		},
		Egresses: []networkmodel.Egress{
			{ID: "hk-out", Line: "hk-l"}, {ID: "jp-out", Line: "jp-l"},
		},
		Transports: []networkmodel.Transport{
			{ID: "t-a2", Kind: networkmodel.KindAnx, FromEgress: "hk-out", ToIngress: "jp-hop"},
			{ID: "t-fw", Kind: networkmodel.KindAnx, FromEgress: "hk-out", ToIngress: "jp-hop"},
			{ID: "t-line", Kind: "anx_line", FromEgress: "hk-out", ToIngress: "jp-hop", Trusted: true},
		},
		Paths: []networkmodel.Path{
			{ID: "p1", Ingress: "hk-in", Segments: []string{"t-a2"}, Egress: "jp-out"},
			{ID: "p2", Ingress: "fw-in", Segments: []string{"t-fw"}, Egress: "jp-out"},
		},
	}
}

func newService(t *testing.T) *Service {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "standby.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&model.Node{}, &model.NodeProtocol{}, &model.EgressRoute{}, &model.L2Standby{}, &model.OperationLog{}))
	require.NoError(t, db.Create(&[]model.Node{
		{ID: 29, Name: "hk", Host: "hk.example.com"},
		{ID: 50, Name: "jp", Host: "jp.example.com"},
	}).Error)
	// The first port of the range is taken on jp.
	require.NoError(t, db.Create(&model.NodeProtocol{ID: 1, NodeID: 50, Name: "vless", Type: model.ProtocolVLESS, Port: PortFirst, Enable: 1}).Error)
	s := New(db)
	s.Document = func(context.Context) (*networkmodel.Document, error) { return testDocument(), nil }
	return s
}

func TestLinksPicksCrossBorderPairs(t *testing.T) {
	links := Links(testDocument())
	require.Len(t, links, 2)
	require.Equal(t, "t-a2", links[0].TransportID)
	require.Equal(t, []Pair{{SourceNodeID: 29, ExitNodeID: 50}}, links[0].Pairs)
	require.Equal(t, "t-fw", links[1].TransportID)
	require.Empty(t, links[1].Pairs)
	require.Len(t, links[1].Skipped, 1)
}

func TestPrepareSwitchAndSwitchBack(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	actor := Actor{UserID: 1, Name: "admin"}

	// Reality only: no domain yet.
	res, err := s.Prepare(ctx, PrepareInput{Transports: []string{"t-a2"}, RealityServerName: "www.example.org"}, actor)
	require.NoError(t, err)
	require.Equal(t, 1, res.Created)
	link := res.Overview.Links[0]
	require.Equal(t, TierOff, link.ActiveTier)
	require.Equal(t, []string{model.L2StandbyTierReality}, link.Ready)
	require.Len(t, link.Channels, 1)
	require.Equal(t, PortFirst+1, link.Channels[0].RelayPort)
	require.Equal(t, "www.example.org:443", link.Channels[0].RealityDest)

	var row model.L2Standby
	require.NoError(t, s.DB.First(&row).Error)
	encoded, err := json.Marshal(res)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), row.UUID)
	require.NotContains(t, string(encoded), row.RealityPrivateKey)

	// xhttp is not ready without a domain: nothing changes.
	_, err = s.Switch(ctx, SwitchInput{Transports: []string{"t-a2"}, Tier: model.L2StandbyTierXHTTP}, actor)
	require.ErrorIs(t, err, ErrNotReady)

	// A transport without pairs can never switch.
	_, err = s.Switch(ctx, SwitchInput{All: true, Tier: model.L2StandbyTierReality}, actor)
	require.ErrorIs(t, err, ErrNotReady)

	res, err = s.Prepare(ctx, PrepareInput{XHTTPDomains: map[string]string{"proxy-50": "Edge.Example.NET"}}, actor)
	require.NoError(t, err)
	require.Equal(t, 1, res.Created)
	require.ElementsMatch(t, []string{model.L2StandbyTierReality, model.L2StandbyTierXHTTP}, res.Overview.Links[0].Ready)

	sw, err := s.Switch(ctx, SwitchInput{Transports: []string{"t-a2"}, Tier: model.L2StandbyTierReality}, actor)
	require.NoError(t, err)
	require.Equal(t, []string{"t-a2"}, sw.Transports)
	require.Equal(t, model.L2StandbyTierReality, sw.Overview.Links[0].ActiveTier)
	tiers, err := ActiveTiers(s.DB)
	require.NoError(t, err)
	require.Equal(t, map[string]string{"t-a2": model.L2StandbyTierReality}, tiers)

	hk, err := egressroute.NodeMember(s.DB, 29)
	require.NoError(t, err)
	routes := hk["routes"].([]map[string]any)
	require.Len(t, routes, 1)
	require.Equal(t, []string{egressroute.StandbyCatchAll}, routes[0]["domains"])
	server := routes[0]["server"].(map[string]any)
	require.Equal(t, "vless", server["protocol"])
	require.Equal(t, "jp.example.com", server["host"])
	require.Equal(t, PortFirst+1, server["port"])
	require.Empty(t, hk["relays"])

	jp, err := egressroute.NodeMember(s.DB, 50)
	require.NoError(t, err)
	relays := jp["relays"].([]map[string]any)
	require.Len(t, relays, 1)
	require.Equal(t, egressroute.StandbyIDBase+row.ID, relays[0]["id"])
	require.NotContains(t, relays[0], "listen")

	// Tier 2: the relay listens on loopback behind the TLS front.
	_, err = s.Switch(ctx, SwitchInput{Transports: []string{"t-a2"}, Tier: model.L2StandbyTierXHTTP}, actor)
	require.NoError(t, err)
	jp, err = egressroute.NodeMember(s.DB, 50)
	require.NoError(t, err)
	relays = jp["relays"].([]map[string]any)
	require.Len(t, relays, 1)
	require.Equal(t, "127.0.0.1", relays[0]["listen"])
	hk, err = egressroute.NodeMember(s.DB, 29)
	require.NoError(t, err)
	server = hk["routes"].([]map[string]any)[0]["server"].(map[string]any)
	require.Equal(t, "edge.example.net", server["host"])
	require.Equal(t, 443, server["port"])

	// Switch back.
	sw, err = s.Switch(ctx, SwitchInput{Transports: []string{"t-a2"}, Tier: TierOff}, actor)
	require.NoError(t, err)
	require.Equal(t, TierOff, sw.Overview.Links[0].ActiveTier)
	hk, err = egressroute.NodeMember(s.DB, 29)
	require.NoError(t, err)
	require.Nil(t, hk)

	var logs []model.OperationLog
	require.NoError(t, s.DB.Where("module = ?", auditModule).Find(&logs).Error)
	require.Len(t, logs, 5)
	for _, log := range logs {
		require.NotContains(t, log.Content, row.UUID)
	}
}

func TestPrepareRefusesBadInput(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	cases := map[string]PrepareInput{
		"bad server name": {RealityServerName: "not a host"},
		"dest alone":      {RealityDest: "a.example.com:443"},
		"bad dest":        {RealityServerName: "a.example.com", RealityDest: "a.example.com:99999"},
		"bad node key":    {XHTTPDomains: map[string]string{"forward-3": "a.example.com"}},
		"bad domain":      {XHTTPDomains: map[string]string{"50": "a b"}},
		"unknown":         {Transports: []string{"t-line"}},
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := s.Prepare(ctx, input, Actor{})
			require.ErrorIs(t, err, ErrInvalid)
		})
	}
	for name, input := range map[string]SwitchInput{
		"bad tier":    {All: true, Tier: "ss"},
		"both":        {All: true, Transports: []string{"t-a2"}, Tier: TierOff},
		"neither":     {Tier: TierOff},
		"not standby": {Transports: []string{"t-line"}, Tier: TierOff},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := s.Switch(ctx, input, Actor{})
			require.ErrorIs(t, err, ErrInvalid)
		})
	}
}

func TestPrepareRemovesStaleInactiveChannels(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	_, err := s.Prepare(ctx, PrepareInput{RealityServerName: "www.example.org"}, Actor{})
	require.NoError(t, err)
	s.Document = func(context.Context) (*networkmodel.Document, error) {
		doc := testDocument()
		doc.Transports = doc.Transports[2:]
		return doc, nil
	}
	res, err := s.Prepare(ctx, PrepareInput{}, Actor{})
	require.NoError(t, err)
	require.Equal(t, 1, res.Removed)
	require.Empty(t, res.Overview.Stale)
}
