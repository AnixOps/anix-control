package meshregion

import (
	"context"
	"encoding/json"
	"errors"
	"net/netip"
	"path/filepath"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v5/internal/features"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var now = time.Date(2026, 11, 25, 8, 0, 0, 0, time.UTC)

func openDB(t *testing.T) *gorm.DB {
	t.Helper()
	features.SetVersion("5.0.13.dev.1")
	t.Cleanup(func() { features.SetVersion("") })
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "mesh.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.MeshRegion{}, &model.MeshMember{}, &model.MeshStatus{}, &model.MeshEnrollment{}, &model.MeshDevice{}, &model.MeshRegionChange{}, &model.Node{}, &model.ForwardNode{}))
	for i, host := range []string{"a.example.com", "b.example.com", "c.example.com", "d.example.com"} {
		require.NoError(t, db.Create(&model.Node{ID: uint(i + 1), Name: host[:1], Host: host}).Error)
	}
	return db
}

func proxy(id uint32) agentcontrol.AgentNode {
	return agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: id}
}

func region(t *testing.T, db *gorm.DB, code, cidr string) model.MeshRegion {
	t.Helper()
	row, err := CreateRegion(context.Background(), db, RegionInput{Code: code, Name: code, CIDR: cidr, Enabled: true}, now)
	require.NoError(t, err)
	return row
}

func TestRegionValidation(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	for _, input := range []RegionInput{
		{Code: "C", Name: "x", CIDR: "10.1.0.0/16"},
		{Code: "cn", Name: "", CIDR: "10.1.0.0/16"},
		{Code: "cn", Name: "x", CIDR: "10.1.0.1/16"},
		{Code: "cn", Name: "x", CIDR: "8.8.0.0/16"},
		{Code: "cn", Name: "x", CIDR: "10.1.0.0/25"},
		{Code: "cn", Name: "x", CIDR: "10.0.0.0/7"},
		{Code: "cn", Name: "x", CIDR: "fd00::/64"},
	} {
		_, err := CreateRegion(ctx, db, input, now)
		require.ErrorIs(t, err, ErrInvalid, "%+v", input)
	}
	first := region(t, db, "cn", "10.150.0.0/16")
	require.NotEmpty(t, first.Secret)
	_, err := CreateRegion(ctx, db, RegionInput{Code: "cn", Name: "x", CIDR: "10.151.0.0/16"}, now)
	require.ErrorIs(t, err, ErrConflict)
	_, err = CreateRegion(ctx, db, RegionInput{Code: "eu", Name: "x", CIDR: "10.150.128.0/17"}, now)
	require.ErrorIs(t, err, ErrConflict)
}

func TestAddressesComeFromTheServerSegment(t *testing.T) {
	address, ok := freeAddress(netip.MustParsePrefix("10.150.0.0/16"), []string{"10.150.0.1", "10.150.0.3"}, false)
	require.True(t, ok)
	require.Equal(t, "10.150.0.2", address.String())
	// A /24's server segment is .1 to .127.
	used := []string{}
	for i := 1; i < 128; i++ {
		used = append(used, netip.AddrFrom4([4]byte{192, 168, 9, byte(i)}).String())
	}
	_, ok = freeAddress(netip.MustParsePrefix("192.168.9.0/24"), used, false)
	require.False(t, ok, "the upper half is the device pool")
	address, ok = freeAddress(netip.MustParsePrefix("192.168.9.0/24"), nil, true)
	require.True(t, ok)
	require.Equal(t, "192.168.9.128", address.String())
	for i := 128; i < 255; i++ {
		used = append(used, netip.AddrFrom4([4]byte{192, 168, 9, byte(i)}).String())
	}
	_, ok = freeAddress(netip.MustParsePrefix("192.168.9.0/24"), used, true)
	require.False(t, ok, "never the broadcast address")
}

func TestBoundaryNodesJoinEveryRegion(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	cn := region(t, db, "cn", "10.150.0.0/16")
	eu := region(t, db, "eu", "10.152.0.0/16")
	var changed [][]agentcontrol.AgentNode
	cancel := OnChange(func(nodes []agentcontrol.AgentNode) { changed = append(changed, nodes) })
	defer cancel()

	b1, err := AddNode(ctx, db, cn.ID, proxy(1), true, now)
	require.NoError(t, err)
	require.Equal(t, "10.150.0.1", b1.Address)
	_, err = AddNode(ctx, db, eu.ID, proxy(2), true, now)
	require.NoError(t, err)
	member, err := AddNode(ctx, db, cn.ID, proxy(3), false, now)
	require.NoError(t, err)
	require.Equal(t, "10.150.0.3", member.Address, "boundary node 2 took .2 in cn")
	_, err = AddNode(ctx, db, eu.ID, proxy(3), false, now)
	require.ErrorIs(t, err, ErrConflict)
	_, err = AddNode(ctx, db, eu.ID, proxy(9), false, now)
	require.ErrorIs(t, err, ErrNotFound)
	require.Len(t, changed, 3)

	// An ordinary member sees the boundary nodes of its region as peers.
	doc, err := NodeMember(db, proxy(3))
	require.NoError(t, err)
	networks := doc["networks"].([]map[string]any)
	require.Len(t, networks, 1)
	require.Equal(t, "cn", networks[0]["id"])
	require.Equal(t, "10.150.0.3/16", networks[0]["address"])
	require.Equal(t, true, networks[0]["secure_mode"])
	require.Len(t, networks[0]["peers"], 2)
	require.Empty(t, networks[0]["subnets"].(map[string]any)["advertise"])

	// A boundary node is in both, peers with the other boundary node and
	// advertises the other region's range.
	doc, err = NodeMember(db, proxy(1))
	require.NoError(t, err)
	networks = doc["networks"].([]map[string]any)
	require.Len(t, networks, 2)
	require.Equal(t, []map[string]any{{"server": "b.example.com", "port": Port, "transport": "tcp"}}, networks[0]["peers"])
	require.Equal(t, []string{"10.152.0.0/16"}, networks[0]["subnets"].(map[string]any)["advertise"])

	// A new region gets both boundary nodes.
	na := region(t, db, "na", "10.153.0.0/16")
	var count int64
	require.NoError(t, db.Model(&model.MeshMember{}).Where("region_id = ?", na.ID).Count(&count).Error)
	require.EqualValues(t, 2, count)

	// A disabled region leaves the documents; no enabled region, no member.
	_, err = UpdateRegion(ctx, db, cn.ID, RegionInput{Code: "cn", Name: "cn", CIDR: cn.CIDR}, now)
	require.NoError(t, err)
	doc, err = NodeMember(db, proxy(3))
	require.NoError(t, err)
	require.Nil(t, doc)

	// The range of a region with members cannot change; a home region
	// cannot be deleted; another region can, and its boundary rows go.
	_, err = UpdateRegion(ctx, db, cn.ID, RegionInput{Code: "cn", Name: "cn", CIDR: "10.160.0.0/16"}, now)
	require.ErrorIs(t, err, ErrConflict)
	require.ErrorIs(t, DeleteRegion(ctx, db, cn.ID), ErrConflict)
	require.NoError(t, DeleteRegion(ctx, db, na.ID))
	require.NoError(t, db.Model(&model.MeshMember{}).Where("region_id = ?", na.ID).Count(&count).Error)
	require.Zero(t, count)
}

func TestUpdateAndRemoveMember(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	cn := region(t, db, "cn", "10.150.0.0/16")
	eu := region(t, db, "eu", "10.152.0.0/16")
	home, err := AddNode(ctx, db, cn.ID, proxy(1), false, now)
	require.NoError(t, err)

	home, err = UpdateMember(ctx, db, home.ID, 0, true, now)
	require.NoError(t, err)
	var rows []model.MeshMember
	require.NoError(t, db.Where("node_ref = ?", "proxy-1").Order("region_id").Find(&rows).Error)
	require.Len(t, rows, 2)

	// Moving keeps it a boundary node: home in eu, an address in cn.
	home, err = UpdateMember(ctx, db, home.ID, eu.ID, true, now)
	require.NoError(t, err)
	require.Equal(t, eu.ID, home.RegionID)
	require.True(t, home.Home && home.Boundary)
	require.NoError(t, db.Where("node_ref = ?", "proxy-1").Find(&rows).Error)
	require.Len(t, rows, 2)

	home, err = UpdateMember(ctx, db, home.ID, 0, false, now)
	require.NoError(t, err)
	require.NoError(t, db.Where("node_ref = ?", "proxy-1").Find(&rows).Error)
	require.Len(t, rows, 1)

	_, err = UpdateMember(ctx, db, home.ID+100, 0, false, now)
	require.ErrorIs(t, err, ErrNotFound)
	require.NoError(t, RemoveNode(ctx, db, home.ID))
	require.NoError(t, db.Where("node_ref = ?", "proxy-1").Find(&rows).Error)
	require.Empty(t, rows)
}

func TestFeatureOffHasNoMember(t *testing.T) {
	db := openDB(t)
	cn := region(t, db, "cn", "10.150.0.0/16")
	_, err := AddNode(context.Background(), db, cn.ID, proxy(1), false, now)
	require.NoError(t, err)
	features.SetVersion("5.0.13")
	doc, err := NodeMember(db, proxy(1))
	require.NoError(t, err)
	require.Nil(t, doc)
}

func TestRecordStatusAndList(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	cn := region(t, db, "cn", "10.150.0.0/16")
	_, err := AddNode(ctx, db, cn.ID, proxy(1), false, now)
	require.NoError(t, err)

	require.ErrorIs(t, RecordStatus(ctx, db, "proxy-1", []byte(`{"x":1}`), now, now), ErrInvalid)
	require.True(t, errors.Is(RecordStatus(ctx, db, "proxy-1", []byte(`[`), now, now), ErrInvalid))
	payload := `{"networks":[{"id":"cn","running":true,"alerts":[{"code":"address_unmanaged","address":"10.150.0.9"}]}]}`
	require.NoError(t, RecordStatus(ctx, db, "proxy-1", []byte(`{"networks":[]}`), now, now))
	require.NoError(t, RecordStatus(ctx, db, "proxy-1", []byte(payload), now.Add(time.Minute), now.Add(time.Minute)))

	regions, err := List(ctx, db)
	require.NoError(t, err)
	require.Len(t, regions, 1)
	require.Len(t, regions[0].Members, 1)
	view := regions[0].Members[0]
	require.Equal(t, "a", view.NodeName)
	require.JSONEq(t, payload, string(view.Status))
	require.True(t, view.StatusReceivedAt.Equal(now.Add(time.Minute)))
	encoded, err := json.Marshal(regions)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), cn.Secret, "the network key is never answered")
}
