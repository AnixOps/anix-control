package networkmodel

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	networkv1 "github.com/AnixOps/anix-control/sdk/api/network/v1"
	"github.com/AnixOps/anix-control/sdk/network"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const testKEK = "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8="

var linkKey = strings.Repeat("5f", 32)

// twoHops is the ANX-Line case: SS-2022 users enter proxy-1 (domestic
// IX), cross to proxy-2 (Japan) over transport tr-jp and leave there.
func twoHops() *Document {
	return &Document{
		Lines: []Line{
			{ID: "ln-ix", Node: "proxy-1", Interface: "eth0", Carrier: "ix", Families: []string{"v4"}, GatewayV4: "10.0.0.1"},
			{ID: "ln-jp", Node: "proxy-2", Interface: "ens3", Families: []string{"v4"}, GatewayV4: "203.0.113.1"},
		},
		Ingresses: []Ingress{
			{ID: "in-ss", Line: "ln-ix", Kind: "direct", Audience: "user", Family: "v4", Listen: Endpoint{Address: "0.0.0.0", Port: 8388},
				Protocols: []string{"ss2022"}, Pairing: &Pairing{Mode: "failover", Egresses: []string{"eg-jp"}}},
			{ID: "in-hop", Line: "ln-jp", Kind: "direct", Audience: "hop", Family: "v4", Listen: Endpoint{Address: "203.0.113.20", Port: 443}},
		},
		Egresses: []Egress{
			{ID: "eg-ix", Line: "ln-ix", Kind: "hop", Audience: "hop", Family: "v4", Source: "10.0.0.10"},
			{ID: "eg-jp", Line: "ln-jp", Kind: "nat_direct", Audience: "internet", Family: "v4", Source: "203.0.113.20"},
		},
		Transports: []Transport{
			{ID: "tr-jp", Kind: "anx_line", FromEgress: "eg-ix", ToIngress: "in-hop", Trusted: true, AnxLine: &AnxLine{QUICPort: 8443, CarrierOrder: []string{"c1", "c2"}}},
		},
		Paths: []Path{{ID: "pa-jp", Ingress: "in-ss", Segments: []string{"tr-jp"}, Egress: "eg-jp", Labels: []string{"jp"}}},
	}
}

func japanKey() map[string]Device {
	return map[string]Device{"proxy-2": {PublicKeys: map[string]string{"anx_line": "anx1-japan"}}}
}

func TestPlanTwoHops(t *testing.T) {
	result, err := Plan(twoHops(), map[string]string{"tr-jp": linkKey}, japanKey())
	require.NoError(t, err)
	require.Empty(t, result.Pending)
	require.Len(t, result.Plans, 2)

	ingress := result.Plans["proxy-1"]
	require.Len(t, ingress.GetTransports(), 1)
	dial := ingress.GetTransports()[0]
	require.Equal(t, networkv1.TransportRole_TRANSPORT_ROLE_DIAL, dial.GetRole())
	require.Equal(t, "proxy-2", dial.GetPeerDeviceRef())
	require.Equal(t, "tr-jp", dial.GetAnxLine().GetLinkId())
	require.Equal(t, linkKey, dial.GetAnxLine().GetLinkKey())
	require.Equal(t, "anx1-japan", dial.GetAnxLine().GetServerPublic())
	require.Equal(t, "203.0.113.20:443", dial.GetAnxLine().GetTcpAddr())
	require.Equal(t, "203.0.113.20:8443", dial.GetAnxLine().GetQuicAddr())
	require.Len(t, ingress.GetPaths(), 1)
	require.Equal(t, "tr-jp", ingress.GetPaths()[0].GetTransportId())
	require.Equal(t, "eg-jp", ingress.GetPaths()[0].GetFinalEgressId())
	require.Equal(t, []string{"eg-jp"}, ingress.GetIngresses()[0].GetPairing().GetEgressIds())

	egress := result.Plans["proxy-2"]
	accept := egress.GetTransports()[0]
	require.Equal(t, networkv1.TransportRole_TRANSPORT_ROLE_ACCEPT, accept.GetRole())
	require.Empty(t, accept.GetAnxLine().GetServerPublic())
	require.Equal(t, linkKey, accept.GetAnxLine().GetLinkKey())
	require.Equal(t, "eg-jp", egress.GetForwarding()[0].GetEgressId())
	require.Equal(t, []string{"10.0.0.10/32"}, egress.GetIngresses()[0].GetAllowedPeers(), "the hop ingress admits the dialing source")
	require.Empty(t, egress.GetPaths())
}

func TestPlanPendingWithoutServerKey(t *testing.T) {
	result, err := Plan(twoHops(), map[string]string{"tr-jp": linkKey}, nil)
	require.NoError(t, err)
	require.Equal(t, []Pending{{Path: "pa-jp", Reason: "proxy-2 has not reported its ANX-Line server key"}}, result.Pending)
	ingress := result.Plans["proxy-1"]
	require.Empty(t, ingress.GetTransports())
	require.Empty(t, ingress.GetPaths())
	pairing := ingress.GetIngresses()[0].GetPairing()
	require.Equal(t, networkv1.PairingMode_PAIRING_MODE_SAME_LINE, pairing.GetMode(), "failover without planned paths waits as same_line")
	require.Empty(t, result.Plans["proxy-2"].GetForwarding())

	result, err = Plan(twoHops(), nil, japanKey())
	require.NoError(t, err)
	require.Equal(t, "transport tr-jp has no link key", result.Pending[0].Reason)
}

func TestPlanDisabledPath(t *testing.T) {
	doc := twoHops()
	doc.Paths[0].Disabled = true
	result, err := Plan(doc, map[string]string{"tr-jp": linkKey}, japanKey())
	require.NoError(t, err)
	require.Empty(t, result.Pending)
	require.Empty(t, result.Plans["proxy-1"].GetPaths())
	require.Empty(t, result.Plans["proxy-2"].GetTransports())
}

// A cross-border segment takes kind anx: planned as an anx_line link
// under the A2 jacket (l2), on c1.
func TestPlanJacketedTransport(t *testing.T) {
	doc := twoHops()
	doc.Transports[0] = Transport{ID: "tr-jp", Kind: KindAnx, FromEgress: "eg-ix", ToIngress: "in-hop",
		AnxLine: &AnxLine{ServerName: "www.example.org"}}
	result, err := Plan(doc, map[string]string{"tr-jp": linkKey}, japanKey())
	require.NoError(t, err)
	for _, node := range []string{"proxy-1", "proxy-2"} {
		transports := result.Plans[node].GetTransports()
		require.Len(t, transports, 1, node)
		require.Equal(t, network.TransportAnxLine, transports[0].GetKind())
		require.Equal(t, "l2", transports[0].GetAnxLine().GetEgress())
		require.Equal(t, "www.example.org", transports[0].GetAnxLine().GetServerName())
		require.Empty(t, transports[0].GetAnxLine().GetQuicAddr())
	}
}

func TestPlanRefuses(t *testing.T) {
	cases := map[string]struct {
		edit func(d *Document)
		want string
	}{
		"bad node":          {func(d *Document) { d.Lines[0].Node = "node-1" }, "lines[0].node"},
		"duplicate id":      {func(d *Document) { d.Lines[1].ID = "ln-ix" }, "listed twice"},
		"unknown line":      {func(d *Document) { d.Ingresses[0].Line = "x" }, "is not a line"},
		"other kind":        {func(d *Document) { d.Transports[0].Kind = "wireguard" }, "must be anx_line or anx"},
		"bare untrusted":    {func(d *Document) { d.Transports[0].Trusted = false }, "only for trusted segments"},
		"anx needs a name":  {func(d *Document) { d.Transports[0].Kind = KindAnx }, "server name"},
		"anx on quic": {func(d *Document) {
			d.Transports[0].Kind = KindAnx
			d.Transports[0].AnxLine.ServerName = "www.example.org"
		}, "c1 (TCP) only"},
		"self transport":    {func(d *Document) { d.Lines[1].Node = "proxy-1" }, "joins proxy-1 to itself"},
		"transport to user": {func(d *Document) { d.Transports[0].ToIngress = "in-ss" }, "not a hop ingress"},
		"broken chain": {func(d *Document) {
			d.Paths[0].Segments = nil
		}, "is on proxy-2, but the path ends at proxy-1"},
		"shared transport": {func(d *Document) {
			d.Paths = append(d.Paths, Path{ID: "pa-2", Ingress: "in-ss", Segments: []string{"tr-jp"}, Egress: "eg-jp"})
		}, "one link per path"},
		"plan check":   {func(d *Document) { d.Egresses[1].Source = "2001:db8::1" }, "plans[proxy-2]"},
		"pairing mode": {func(d *Document) { d.Ingresses[0].Pairing.Mode = "random" }, "pairing mode"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			doc := twoHops()
			c.edit(doc)
			_, err := Plan(doc, map[string]string{"tr-jp": linkKey}, japanKey())
			var invalid *Invalid
			require.True(t, errors.As(err, &invalid), "%v", err)
			require.ErrorIs(t, err, ErrInvalid)
			require.Contains(t, err.Error()+"|"+fieldsOf(invalid), c.want)
		})
	}
}

func fieldsOf(invalid *Invalid) string {
	var fields []string
	for _, v := range invalid.Violations {
		fields = append(fields, v.Field+": "+v.Message)
	}
	return strings.Join(fields, "|")
}

func newService(t *testing.T) *Service {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "network.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&model.Node{}, &model.ForwardNode{}, &model.OperationLog{},
		&model.NetworkModel{}, &model.NetworkTransportKey{}, &model.NetworkDevice{}))
	for _, id := range []uint{1, 2} {
		require.NoError(t, db.Create(&model.Node{ID: id, Name: "n", Host: "h.example.com", APIKey: "k" + string(rune('0'+id))}).Error)
	}
	now := time.Date(2026, 10, 28, 8, 0, 0, 0, time.UTC)
	return &Service{DB: db, KEK: func() string { return testKEK }, Now: func() time.Time { return now }}
}

func TestServiceReplaceSealsKeysAndStampsPlans(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	view, err := s.Replace(ctx, twoHops(), nil, Actor{UserID: 1, Name: "admin"})
	require.NoError(t, err)
	require.Equal(t, uint64(1), view.Revision)
	require.Equal(t, []Pending{{Path: "pa-jp", Reason: "proxy-2 has not reported its ANX-Line server key"}}, view.Pending)

	var keyRow model.NetworkTransportKey
	require.NoError(t, s.DB.First(&keyRow, "transport_id = ?", "tr-jp").Error)
	require.NotContains(t, keyRow.SealedKey, linkKey)

	// Japan reports its server key: the path is planned, both devices
	// get the same link key.
	require.NoError(t, s.SetPublicKeys(ctx, "proxy-2", map[string]string{"anx_line": "anx1-japan"}))
	ingress, found, err := s.DevicePlan(ctx, "proxy-1")
	require.NoError(t, err)
	require.True(t, found)
	egress, _, err := s.DevicePlan(ctx, "proxy-2")
	require.NoError(t, err)
	key := ingress.GetTransports()[0].GetAnxLine().GetLinkKey()
	require.Len(t, key, 64)
	require.Equal(t, key, egress.GetTransports()[0].GetAnxLine().GetLinkKey())
	require.NoError(t, network.Check(ingress))
	require.Equal(t, "redacted", Redacted(ingress).GetTransports()[0].GetAnxLine().GetLinkKey())
	require.Equal(t, key, ingress.GetTransports()[0].GetAnxLine().GetLinkKey(), "Redacted copies")

	// A stale revision is refused; the current one replans.
	stale := uint64(0)
	_, err = s.Replace(ctx, twoHops(), &stale, Actor{})
	require.ErrorIs(t, err, ErrConflict)
	current := uint64(1)
	view, err = s.Replace(ctx, twoHops(), &current, Actor{})
	require.NoError(t, err)
	require.Empty(t, view.Pending)
	var device model.NetworkDevice
	require.NoError(t, s.DB.First(&device, "node_ref = ?", "proxy-1").Error)
	require.Equal(t, uint64(2), device.Generation, "the server key planned the path: proxy-1's plan changed once")
	stamped, _, err := s.DevicePlan(ctx, "proxy-1")
	require.NoError(t, err)
	require.Equal(t, device.PlanHash, stamped.GetHash())
	require.Equal(t, uint64(2), stamped.GetGeneration())

	// Rotation changes the key on both ends.
	_, err = s.RotateLinkKey(ctx, "tr-jp", Actor{})
	require.NoError(t, err)
	rotated, _, err := s.DevicePlan(ctx, "proxy-1")
	require.NoError(t, err)
	require.NotEqual(t, key, rotated.GetTransports()[0].GetAnxLine().GetLinkKey())
	require.Equal(t, uint64(3), rotated.GetGeneration())
	_, err = s.RotateLinkKey(ctx, "nope", Actor{})
	require.ErrorIs(t, err, ErrNotFound)

	// Removing everything empties the plans and deletes the key.
	_, err = s.Replace(ctx, &Document{}, nil, Actor{})
	require.NoError(t, err)
	empty, found, err := s.DevicePlan(ctx, "proxy-1")
	require.NoError(t, err)
	require.True(t, found, "a device that had a plan gets an empty one")
	require.Empty(t, empty.GetLines())
	var count int64
	require.NoError(t, s.DB.Model(&model.NetworkTransportKey{}).Count(&count).Error)
	require.Zero(t, count)

	var logs []model.OperationLog
	require.NoError(t, s.DB.Order("id").Find(&logs).Error)
	require.Len(t, logs, 4)
	require.Equal(t, AuditActionRotate, logs[2].Action)
	for _, log := range logs {
		require.NotContains(t, log.Content, key)
	}
}

func TestServiceRefusals(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	doc := twoHops()
	doc.Lines[1].Node = "proxy-3"
	_, err := s.Replace(ctx, doc, nil, Actor{})
	require.ErrorIs(t, err, ErrUnknownNode)

	s.KEK = func() string { return "" }
	_, err = s.Replace(ctx, twoHops(), nil, Actor{})
	require.ErrorIs(t, err, ErrNoKEK)

	doc = twoHops()
	doc.Transports = nil
	doc.Paths = nil
	view, err := s.Replace(ctx, doc, nil, Actor{})
	require.NoError(t, err, "a model without anx_line transports needs no key-encryption key")
	require.Equal(t, uint64(1), view.Revision)

	_, found, err := s.DevicePlan(ctx, "proxy-9")
	require.NoError(t, err)
	require.False(t, found)
}
