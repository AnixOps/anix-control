package meshregion

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"strconv"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// admitted gives a device a static key, which an enrolled device names, so
// that an allow list expands it.
func admitted(t *testing.T, db *gorm.DB, device model.MeshDevice, b byte) {
	t.Helper()
	require.NoError(t, db.Model(&model.MeshDevice{}).Where("id = ?", device.ID).Update("static_key", testStaticKey(b)).Error)
}

// bridgeStatus is a bridge node's report with the allow list it applied in
// network code.
func bridgeStatus(code string, allow ...string) []byte {
	if allow == nil {
		allow = []string{}
	}
	list, _ := json.Marshal(allow)
	return fmt.Appendf(nil, `{"features":["mesh","mesh_native_keys","mesh_external_bridge"],"networks":[{"id":%q,"running":true,"external_bridge":{"allow":%s}}]}`, code, list)
}

func bridgeView(t *testing.T, db *gorm.DB, nodeRef string) BridgeView {
	t.Helper()
	views, err := Bridges(context.Background(), db)
	require.NoError(t, err)
	for _, view := range views {
		if view.NodeRef == nodeRef {
			return view
		}
	}
	t.Fatalf("no bridge %s", nodeRef)
	return BridgeView{}
}

func TestBridgeRoutes(t *testing.T) {
	for _, raw := range [][]string{
		{"8.8.8.8"},
		{"0.0.0.0/0"},
		{"100.64.0.0/10"},
		{"100.64.0.0/24"},
		{"100.100.0.0/32", "100.100.0.0/31"},
		{"fd7a:115c:a1e0::/48"},
		{"fd7a:115c:a1e0::/64"},
		{"fd7a::/16"},
		{"2001:db8::1"},
		{"192.168.50.1/24"},
		{"::ffff:10.0.0.1"},
		{"fe80::1%eth0"},
		{"not an address"},
	} {
		_, err := checkRoutes(raw)
		require.ErrorIs(t, err, ErrInvalid, "%v", raw)
	}
	routes, err := checkRoutes([]string{" 100.100.1.2 ", "192.168.50.0/24", "fd7a:115c:a1e0::5", "fd00:1::/64", "100.100.1.2/32", ""})
	require.NoError(t, err)
	require.Equal(t, []netip.Prefix{
		netip.MustParsePrefix("100.100.1.2/32"), netip.MustParsePrefix("192.168.50.0/24"),
		netip.MustParsePrefix("fd7a:115c:a1e0::5/128"), netip.MustParsePrefix("fd00:1::/64"),
	}, routes, "single addresses become /32 and /128, duplicates go")
	many := make([]string, maxBridgeRoutes+1)
	for i := range many {
		many[i] = fmt.Sprintf("192.168.%d.0/24", i)
	}
	_, err = checkRoutes(many)
	require.ErrorIs(t, err, ErrInvalid)
}

func TestExternalBridge(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	cn := region(t, db, "cn", "10.150.0.0/24")
	eu := region(t, db, "eu", "10.152.0.0/24")
	nodeHome, err := AddNode(ctx, db, cn.ID, proxy(1), false, now)
	require.NoError(t, err)
	_, err = AddNode(ctx, db, cn.ID, proxy(2), false, now)
	require.NoError(t, err)
	_, err = AddNode(ctx, db, eu.ID, proxy(3), false, now)
	require.NoError(t, err)
	phone, err := CreateDevice(ctx, db, cn.ID, DeviceInput{Hostname: "phone"}, now)
	require.NoError(t, err)
	admitted(t, db, phone, 1)
	laptop, err := CreateDevice(ctx, db, eu.ID, DeviceInput{Hostname: "laptop"}, now)
	require.NoError(t, err)
	group, err := CreateHostGroup(ctx, db, HostGroupInput{Name: "vms", Gateway: "proxy-2"}, now)
	require.NoError(t, err)

	device := func(row model.MeshDevice) string { return "device:" + strconv.FormatUint(uint64(row.ID), 10) }
	hostGroup := "host_group:" + strconv.FormatUint(uint64(group.ID), 10)
	input := BridgeInput{Node: "proxy-1", Interface: "ext0", Routes: []string{"100.100.1.2", "192.168.50.0/24", "fd7a:115c:a1e0::5"}, Allow: []string{"node:proxy-2", device(phone), hostGroup}}
	for _, bad := range []BridgeInput{
		{Node: "proxy-1", Interface: "bad name", Routes: input.Routes},
		{Node: "proxy-1", Interface: "a-name-of-16-byte", Routes: input.Routes},
		{Node: "proxy-1", Interface: "eth1"},
		{Node: "proxy-1", Interface: "eth1", Routes: []string{"8.8.8.8"}},
		{Node: "proxy-1", Interface: "eth1", Routes: input.Routes, Allow: []string{"node:proxy-3"}},
		{Node: "proxy-1", Interface: "eth1", Routes: input.Routes, Allow: []string{device(laptop)}},
		{Node: "proxy-1", Interface: "eth1", Routes: input.Routes, Allow: []string{"device:999"}},
		{Node: "proxy-1", Interface: "eth1", Routes: input.Routes, Allow: []string{"user:1"}},
		{Node: "proxy-4", Interface: "eth1", Routes: input.Routes},
	} {
		_, err := CreateBridge(ctx, db, bad, now)
		require.ErrorIs(t, err, ErrInvalid, "%+v", bad)
	}
	_, err = CreateBridge(ctx, db, BridgeInput{Node: "proxy-1", Interface: "eth1", Routes: []string{"10.150.0.0/25"}}, now)
	require.ErrorIs(t, err, ErrConflict, "a route of a region's range")
	_, err = CreateBridge(ctx, db, BridgeInput{Node: "proxy-1", Interface: "eth1", Routes: []string{"10.127.9.0/24"}}, now)
	require.ErrorIs(t, err, ErrConflict, "a route of the host group supernet")

	bridge, err := CreateBridge(ctx, db, input, now)
	require.NoError(t, err)
	_, err = CreateBridge(ctx, db, input, now)
	require.ErrorIs(t, err, ErrConflict, "a node is one bridge")

	network := nodeNetwork(t, db, proxy(1), "cn")
	require.Equal(t, map[string]any{
		"interface": "ext0",
		"routes":    []string{"100.100.1.2/32", "192.168.50.0/24", "fd7a:115c:a1e0::5/128"},
		"allow":     []string{"10.127.0.0/24", "10.150.0.2/32", *phone.Address + "/32"},
	}, network["external_bridge"])
	require.NotContains(t, nodeNetwork(t, db, proxy(2), "cn"), "external_bridge", "only the bridge node bridges")

	regions, err := List(ctx, db)
	require.NoError(t, err)
	for _, member := range regions[0].Members {
		require.Equal(t, member.NodeRef == "proxy-1", member.ExternalBridge)
	}
	view := bridgeView(t, db, "proxy-1")
	require.Equal(t, cn.ID, view.RegionID)
	require.Nil(t, view.Applied, "not reported yet")
	require.Equal(t, []string{WarningBridgeUnconfirmed}, view.Warnings)
	require.NoError(t, RecordStatus(ctx, db, proxy(1), bridgeStatus("cn", network["external_bridge"].(map[string]any)["allow"].([]string)...), now, now))
	view = bridgeView(t, db, "proxy-1")
	require.Equal(t, view.Expanded, view.Applied)
	require.Empty(t, view.Warnings, "confirmed")

	// Our ranges stay clear of the bridge's routes, and the bridge node
	// stays while it bridges.
	_, err = CreateRegion(ctx, db, RegionInput{Code: "lab", Name: "lab", CIDR: "192.168.50.0/24"}, now)
	require.ErrorIs(t, err, ErrConflict)
	require.ErrorIs(t, RemoveNode(ctx, db, nodeHome.ID), ErrConflict)
	_, err = UpdateMember(ctx, db, nodeHome.ID, eu.ID, false, nil, now)
	require.ErrorIs(t, err, ErrConflict)

	// An entry that leaves the region is left out and marked.
	_, err = UpdateHostGroup(ctx, db, group.ID, HostGroupInput{Name: "vms", Gateway: "proxy-1"}, now)
	require.NoError(t, err)
	require.NoError(t, RemoveNode(ctx, db, mustHome(t, db, "proxy-2")))
	view = bridgeView(t, db, "proxy-1")
	require.Contains(t, view.Warnings, WarningAllowOutsideRegion)
	require.Equal(t, []string{"10.150.0.2/32"}, view.Held, "the former member's address is held")

	updated, err := UpdateBridge(ctx, db, bridge.ID, BridgeInput{Node: "proxy-9", Interface: "eth1", Routes: []string{"192.168.60.0/24"}, Allow: []string{device(phone)}}, now)
	require.NoError(t, err)
	require.Equal(t, "proxy-1", updated.NodeRef, "the node never changes")
	require.NoError(t, DeleteBridge(ctx, db, bridge.ID))
	require.NotContains(t, nodeNetwork(t, db, proxy(1), "cn"), "external_bridge")
	require.ErrorIs(t, DeleteBridge(ctx, db, bridge.ID), ErrNotFound)
	var holds int64
	require.NoError(t, db.Model(&model.MeshBridgeHold{}).Where("node_ref = ?", "proxy-1").Count(&holds).Error)
	require.EqualValues(t, 3, holds, "the phone, the host group and the former member wait for the node")
	require.NoError(t, DeleteHostGroup(ctx, db, group.ID))
	require.NoError(t, RemoveNode(ctx, db, nodeHome.ID))
	require.NoError(t, db.Model(&model.MeshBridgeHold{}).Count(&holds).Error)
	require.Zero(t, holds, "a node that left the mesh holds nothing")
}

func mustHome(t *testing.T, db *gorm.DB, nodeRef string) uint {
	t.Helper()
	home, err := homeOf(db, nodeRef)
	require.NoError(t, err)
	return home.ID
}

func TestBridgeHoldsAReleasedAddress(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	cn := region(t, db, "cn", "10.150.0.0/24")
	_, err := AddNode(ctx, db, cn.ID, proxy(1), false, now)
	require.NoError(t, err)
	_, err = AddNode(ctx, db, cn.ID, proxy(2), false, now)
	require.NoError(t, err)
	phone, err := CreateDevice(ctx, db, cn.ID, DeviceInput{Hostname: "phone"}, now)
	require.NoError(t, err)
	admitted(t, db, phone, 1)
	group, err := CreateHostGroup(ctx, db, HostGroupInput{Name: "vms", Gateway: "proxy-2"}, now)
	require.NoError(t, err)
	_, err = CreateBridge(ctx, db, BridgeInput{Node: "proxy-1", Interface: "eth1", Routes: []string{"192.168.50.0/24"},
		Allow: []string{"device:" + strconv.FormatUint(uint64(phone.ID), 10), "host_group:" + strconv.FormatUint(uint64(group.ID), 10)}}, now)
	require.NoError(t, err)
	address := *phone.Address
	require.NoError(t, RecordStatus(ctx, db, proxy(1), bridgeStatus("cn", "10.127.0.0/24", address+"/32"), now, now))

	// Removed, the phone's address and the group's segment are held until
	// the bridge node reports a list without them.
	require.NoError(t, DeleteDevice(ctx, db, phone.ID, now))
	require.NoError(t, DeleteHostGroup(ctx, db, group.ID))
	require.Equal(t, []string{"10.127.0.0/24", address + "/32"}, bridgeView(t, db, "proxy-1").Held)
	tablet, err := CreateDevice(ctx, db, cn.ID, DeviceInput{Hostname: "tablet"}, now)
	require.NoError(t, err)
	require.NotEqual(t, address, *tablet.Address)
	next, err := CreateHostGroup(ctx, db, HostGroupInput{Name: "lab", Gateway: "proxy-2"}, now)
	require.NoError(t, err)
	require.Equal(t, "10.127.1.0/24", next.Segment)

	// A report observed before the release does not count; one after
	// that still lets the address through neither.
	require.NoError(t, RecordStatus(ctx, db, proxy(1), bridgeStatus("cn"), now, now))
	require.Len(t, bridgeView(t, db, "proxy-1").Held, 2)
	later := time.Now().UTC().Add(time.Minute)
	require.NoError(t, RecordStatus(ctx, db, proxy(1), bridgeStatus("cn", address+"/32"), later, later))
	require.Equal(t, []string{address + "/32"}, bridgeView(t, db, "proxy-1").Held)
	require.NoError(t, RecordStatus(ctx, db, proxy(1), bridgeStatus("cn"), later, later))
	require.Empty(t, bridgeView(t, db, "proxy-1").Held)
	again, err := CreateDevice(ctx, db, cn.ID, DeviceInput{Hostname: "again"}, now)
	require.NoError(t, err)
	require.Equal(t, address, *again.Address, "a confirmed address is free again")
}

func TestPoolReclaimWaitsForTheBridge(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	lab := region(t, db, "lab", "192.168.9.0/24")
	_, err := AddNode(ctx, db, lab.ID, proxy(1), false, now)
	require.NoError(t, err)
	old := now.Add(-PoolReclaimAfter - 1)
	idle, err := CreateDevice(ctx, db, lab.ID, DeviceInput{Hostname: "idle"}, old)
	require.NoError(t, err)
	admitted(t, db, idle, 1)
	for i := 1; i < PoolSize(netip.MustParsePrefix(lab.CIDR)); i++ {
		_, err := CreateDevice(ctx, db, lab.ID, DeviceInput{Hostname: fmt.Sprintf("d%d", i)}, now)
		require.NoError(t, err)
	}
	_, err = CreateBridge(ctx, db, BridgeInput{Node: "proxy-1", Interface: "eth1", Routes: []string{"10.20.0.0/24"},
		Allow: []string{"device:" + strconv.FormatUint(uint64(idle.ID), 10)}}, now)
	require.NoError(t, err)

	// The only idle address is allowed through the bridge: it is taken
	// from its device first and given out after the bridge confirms.
	_, err = CreateDevice(ctx, db, lab.ID, DeviceInput{Hostname: "next"}, now)
	require.ErrorIs(t, err, ErrPoolFull)
	var row model.MeshDevice
	require.NoError(t, db.First(&row, idle.ID).Error)
	require.Nil(t, row.Address, "released")
	require.Equal(t, []string{*idle.Address + "/32"}, bridgeView(t, db, "proxy-1").Held)
	_, err = CreateDevice(ctx, db, lab.ID, DeviceInput{Hostname: "next"}, now)
	require.ErrorIs(t, err, ErrPoolFull, "still held")

	later := time.Now().UTC().Add(time.Minute)
	require.NoError(t, RecordStatus(ctx, db, proxy(1), bridgeStatus("lab"), later, later))
	next, err := CreateDevice(ctx, db, lab.ID, DeviceInput{Hostname: "next"}, now)
	require.NoError(t, err)
	require.Equal(t, *idle.Address, *next.Address)
}
