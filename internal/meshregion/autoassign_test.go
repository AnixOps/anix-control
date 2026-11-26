package meshregion

import (
	"context"
	"testing"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/require"
)

func probeStatus(regions string) []byte {
	return []byte(`{"networks":[],"region_probe":{"measured_at":"2026-11-25T08:00:00Z","regions":` + regions + `}}`)
}

func TestAutoNodeIsPlacedByItsLatencies(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	cn := region(t, db, "cn", "10.150.0.0/16")
	eu := region(t, db, "eu", "10.152.0.0/16")
	off := region(t, db, "na", "10.154.0.0/16")
	_, err := UpdateRegion(ctx, db, off.ID, RegionInput{Code: "na", Name: "na", CIDR: off.CIDR}, now)
	require.NoError(t, err)
	_, err = AddNode(ctx, db, cn.ID, proxy(1), false, now)
	require.NoError(t, err)
	var pushed []string
	cancel := OnChange(func(nodes []agentcontrol.AgentNode) {
		for _, node := range nodes {
			pushed = append(pushed, node.String())
		}
	})
	defer cancel()

	_, err = AddAutoNode(ctx, db, proxy(99), nil, now)
	require.ErrorIs(t, err, ErrNotFound)
	_, err = AddAutoNode(ctx, db, proxy(1), nil, now)
	require.ErrorIs(t, err, ErrConflict, "already in a region")
	require.ErrorIs(t, RecordStatus(ctx, db, proxy(2), probeStatus(`[]`), now, now), ErrNotMember, "neither a member nor waiting")
	pushed = nil
	admin := uint(7)
	waiting, err := AddAutoNode(ctx, db, proxy(2), &admin, now)
	require.NoError(t, err)
	require.Equal(t, "proxy-2", waiting.NodeRef)
	require.Contains(t, pushed, "proxy-2", "the waiting node gets its configuration")
	_, err = AddAutoNode(ctx, db, proxy(2), nil, now)
	require.ErrorIs(t, err, ErrConflict, "already waiting")
	list, err := AutoNodes(ctx, db)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, "b", list[0].NodeName)

	// No network, a region object without a current region.
	doc, err := NodeMember(db, proxy(2))
	require.NoError(t, err)
	require.Empty(t, doc["networks"])
	require.NotNil(t, doc["networks"], "an empty list, not null")
	object := regionOf(t, doc)
	require.Nil(t, object["current"])
	require.Nil(t, object["basis"])
	require.NotNil(t, object["directory"])

	// Reports without a usable latency leave it waiting: none, only
	// disabled or unknown regions, negative round trips.
	for _, regions := range []string{`[]`, `[{"region":"na","rtt_ms":1}]`, `[{"region":"xx","rtt_ms":1}]`, `[{"region":"cn","rtt_ms":-1}]`, `[{"region":"cn"}]`} {
		require.NoError(t, RecordStatus(ctx, db, proxy(2), probeStatus(regions), now, now), regions)
	}
	require.NoError(t, RecordStatus(ctx, db, proxy(2), []byte(`{"networks":[]}`), now, now))
	var marks int64
	require.NoError(t, db.Model(&model.MeshAutoNode{}).Count(&marks).Error)
	require.EqualValues(t, 1, marks)
	var stored model.MeshStatus
	require.NoError(t, db.First(&stored, "node_ref = ?", "proxy-2").Error, "the report is kept")

	// The default preferred region cn wins 40 against 35 (0.8 x 40 = 32).
	_, err = SaveSettings(ctx, db, SettingsInput{NameSuffix: DefaultNameSuffix, HostGroupSupernet: DefaultHostGroupSupernet, HostGroupPrefix: DefaultHostGroupPrefix, PreferredRegion: "cn"}, now)
	require.NoError(t, err)
	pushed = nil
	require.NoError(t, RecordStatus(ctx, db, proxy(2), probeStatus(`[{"region":"eu","rtt_ms":35},{"region":"cn","rtt_ms":40},{"region":"na","rtt_ms":1}]`), now, now))
	var home model.MeshMember
	require.NoError(t, db.Where("node_ref = ? AND home = ?", "proxy-2", true).First(&home).Error)
	require.Equal(t, cn.ID, home.RegionID)
	require.Equal(t, "10.150.0.2", home.Address, "the lowest free address of the server segment")
	require.False(t, home.Boundary)
	require.Equal(t, "latency", *home.RegionBasis)
	require.NoError(t, db.Model(&model.MeshAutoNode{}).Count(&marks).Error)
	require.EqualValues(t, 0, marks)
	require.Contains(t, pushed, "proxy-2")
	doc, err = NodeMember(db, proxy(2))
	require.NoError(t, err)
	require.Len(t, doc["networks"], 1)
	object = regionOf(t, doc)
	require.Equal(t, "cn", object["current"])
	require.Equal(t, "latency", object["basis"])
	// Reporting as a member works as before.
	require.NoError(t, RecordStatus(ctx, db, proxy(2), probeStatus(`[{"region":"eu","rtt_ms":1}]`), now, now))
	require.NoError(t, db.Model(&model.MeshMember{}).Where("node_ref = ?", "proxy-2").Count(&marks).Error)
	require.EqualValues(t, 1, marks, "a member is not placed again")

	// Without a default preference the lowest latency wins; the
	// administrator's own choice replaces waiting.
	_, err = SaveSettings(ctx, db, SettingsInput{NameSuffix: DefaultNameSuffix, HostGroupSupernet: DefaultHostGroupSupernet, HostGroupPrefix: DefaultHostGroupPrefix}, now)
	require.NoError(t, err)
	_, err = AddAutoNode(ctx, db, proxy(3), nil, now)
	require.NoError(t, err)
	require.NoError(t, RecordStatus(ctx, db, proxy(3), probeStatus(`[{"region":"eu","rtt_ms":35},{"region":"cn","rtt_ms":40}]`), now, now))
	require.NoError(t, db.Where("node_ref = ? AND home = ?", "proxy-3", true).First(&home).Error)
	require.Equal(t, eu.ID, home.RegionID)
	_, err = AddAutoNode(ctx, db, proxy(4), nil, now)
	require.NoError(t, err)
	_, err = AddNode(ctx, db, cn.ID, proxy(4), false, now)
	require.NoError(t, err)
	require.NoError(t, db.Model(&model.MeshAutoNode{}).Count(&marks).Error)
	require.EqualValues(t, 0, marks)
	require.NoError(t, db.Where("node_ref = ? AND home = ?", "proxy-4", true).First(&home).Error)
	require.Equal(t, "admin", *home.RegionBasis)

	// Stop waiting.
	require.NoError(t, db.Create(&model.Node{ID: 5, Name: "e", Host: "e.example.com"}).Error)
	waitingAgain, err := AddAutoNode(ctx, db, proxy(5), nil, now)
	require.NoError(t, err)
	_, err = RemoveAutoNode(ctx, db, waitingAgain.ID)
	require.NoError(t, err)
	_, err = RemoveAutoNode(ctx, db, waitingAgain.ID)
	require.ErrorIs(t, err, ErrNotFound)
	doc, err = NodeMember(db, proxy(5))
	require.NoError(t, err)
	require.Nil(t, doc)
}
