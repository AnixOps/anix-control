package meshregion

import (
	"context"
	"fmt"
	"net/netip"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSettings(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	settings, err := LoadSettings(ctx, db)
	require.NoError(t, err)
	require.Equal(t, DefaultNameSuffix, settings.NameSuffix)
	require.Nil(t, settings.UpdatedAt, "defaults until saved")

	for _, input := range []SettingsInput{
		{NameSuffix: "Mesh..internal", HostGroupSupernet: "10.127.0.0/16", HostGroupPrefix: 24},
		{NameSuffix: "mesh.internal", HostGroupSupernet: "8.8.0.0/16", HostGroupPrefix: 24},
		{NameSuffix: "mesh.internal", HostGroupSupernet: "10.127.0.0/16", HostGroupPrefix: 12},
		{NameSuffix: "mesh.internal", HostGroupSupernet: "10.127.0.0/16", HostGroupPrefix: 24, ReservedRanges: []string{"10.127.5.0/24"}},
	} {
		_, err := SaveSettings(ctx, db, input, now)
		require.ErrorIs(t, err, ErrInvalid, "%+v", input)
	}
	region(t, db, "cn", "10.150.0.0/16")
	_, err = SaveSettings(ctx, db, SettingsInput{NameSuffix: "mesh.internal", HostGroupSupernet: "10.150.0.0/16", HostGroupPrefix: 24}, now)
	require.ErrorIs(t, err, ErrConflict, "the supernet may not overlap a region")
	saved, err := SaveSettings(ctx, db, SettingsInput{NameSuffix: ".Corp.Example.", HostGroupSupernet: "10.127.0.0/16", HostGroupPrefix: 26, ReservedRanges: []string{"10.126.0.0/16", " "}}, now)
	require.NoError(t, err)
	require.Equal(t, "corp.example", saved.NameSuffix)
	require.Equal(t, []string{"10.126.0.0/16"}, saved.ReservedRanges)
	require.NotNil(t, saved.UpdatedAt)

	_, err = CreateRegion(ctx, db, RegionInput{Code: "eu", Name: "eu", CIDR: "10.126.0.0/16"}, now)
	require.ErrorIs(t, err, ErrConflict, "a region may not overlap a reserved range")
	_, err = CreateRegion(ctx, db, RegionInput{Code: "eu", Name: "eu", CIDR: "10.127.0.0/16"}, now)
	require.ErrorIs(t, err, ErrConflict, "a region may not overlap the host group supernet")
}

func TestDevicePool(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	cn := region(t, db, "cn", "10.150.0.0/24")
	eu := region(t, db, "eu", "10.152.0.0/16")

	first, err := CreateDevice(ctx, db, cn.ID, DeviceInput{Hostname: "Phone-1"}, now)
	require.NoError(t, err)
	require.Equal(t, "phone-1", first.Hostname)
	require.Equal(t, "10.150.0.128", *first.Address, "the pool is the upper half")
	second, err := CreateDevice(ctx, db, cn.ID, DeviceInput{Hostname: "laptop", DeviceKey: "AAAAAAAAAAAAAAAAAAAAAA=="}, now)
	require.NoError(t, err)
	require.Equal(t, "10.150.0.129", *second.Address)
	_, err = CreateDevice(ctx, db, cn.ID, DeviceInput{Hostname: "tablet", DeviceKey: "AAAAAAAAAAAAAAAAAAAAAA=="}, now)
	require.ErrorIs(t, err, ErrConflict, "a device key is one device's")
	_, err = CreateDevice(ctx, db, cn.ID, DeviceInput{Hostname: "bad_name"}, now)
	require.ErrorIs(t, err, ErrInvalid)

	require.NoError(t, DeleteDevice(ctx, db, first.ID, now))
	third, err := CreateDevice(ctx, db, cn.ID, DeviceInput{Hostname: "tablet"}, now)
	require.NoError(t, err)
	require.Equal(t, "10.150.0.128", *third.Address, "a freed address is free again")

	summary, err := LoadSummary(ctx, db)
	require.NoError(t, err)
	pools := map[uint]RegionPool{}
	for _, pool := range summary.Pools {
		pools[pool.RegionID] = pool
	}
	require.Equal(t, RegionPool{RegionID: cn.ID, Size: 127, Used: 2}, pools[cn.ID])
	require.Equal(t, 32767, pools[eu.ID].Size, "a /16 pool is .128.0 to .255.254")
}

func TestIdlePoolAddressGoesToTheNextDevice(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	lab := region(t, db, "lab", "192.168.9.0/24")
	old := now.Add(-PoolReclaimAfter - 1)
	idle, err := CreateDevice(ctx, db, lab.ID, DeviceInput{Hostname: "idle"}, old)
	require.NoError(t, err)
	for i := 1; i < PoolSize(netip.MustParsePrefix(lab.CIDR)); i++ {
		_, err := CreateDevice(ctx, db, lab.ID, DeviceInput{Hostname: fmt.Sprintf("d%d", i)}, now)
		require.NoError(t, err)
	}
	// The pool (.128 to .254) is full: the address unused for 30 days moves.
	next, err := CreateDevice(ctx, db, lab.ID, DeviceInput{Hostname: "next"}, now)
	require.NoError(t, err)
	require.Equal(t, *idle.Address, *next.Address)
	devices, err := Devices(ctx, db, lab.ID, now)
	require.NoError(t, err)
	byName := map[string]DeviceView{}
	for _, device := range devices {
		byName[device.Hostname] = device
	}
	require.Nil(t, byName["idle"].Address)
	require.Equal(t, []string{WarningNoAddress}, byName["idle"].Warnings)
	require.Equal(t, "next.lab.mesh.internal", byName["next"].FQDN)
	// No address is unused that long now.
	_, err = CreateDevice(ctx, db, lab.ID, DeviceInput{Hostname: "late"}, now)
	require.ErrorIs(t, err, ErrPoolFull)

	_, err = UpdateRegion(ctx, db, lab.ID, RegionInput{Code: "lab", Name: "lab", CIDR: "192.168.10.0/24", Enabled: true}, now)
	require.ErrorIs(t, err, ErrConflict, "the range of a region with devices stays")
	require.ErrorIs(t, DeleteRegion(ctx, db, lab.ID), ErrConflict)
}

func TestRegionChanges(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	cn := region(t, db, "cn", "10.150.0.0/16")
	eu := region(t, db, "eu", "10.152.0.0/16")
	member, err := AddNode(ctx, db, cn.ID, proxy(1), false, now)
	require.NoError(t, err)
	device, err := CreateDevice(ctx, db, cn.ID, DeviceInput{Hostname: "phone"}, now)
	require.NoError(t, err)
	admin := uint(7)

	_, err = RequestChange(ctx, db, ChangeInput{Node: "proxy-1", DeviceID: device.ID, ToRegionID: eu.ID}, model.MeshChangeAdmin, nil, now)
	require.ErrorIs(t, err, ErrInvalid, "one subject")
	_, err = RequestChange(ctx, db, ChangeInput{Node: "proxy-1", ToRegionID: cn.ID}, model.MeshChangeAdmin, nil, now)
	require.ErrorIs(t, err, ErrInvalid, "already the region")
	request, err := RequestChange(ctx, db, ChangeInput{Node: "proxy-1", ToRegionID: eu.ID, Reason: "moved to Frankfurt"}, model.MeshChangeAdmin, &admin, now)
	require.NoError(t, err)
	require.Equal(t, member.Address, request.OldAddress)
	_, err = RequestChange(ctx, db, ChangeInput{Node: "proxy-1", ToRegionID: eu.ID}, model.MeshChangeAdmin, nil, now)
	require.ErrorIs(t, err, ErrConflict, "one pending change")
	require.ErrorIs(t, DeleteRegion(ctx, db, eu.ID), ErrConflict, "a pending change asks for the region")

	approved, err := ApproveChange(ctx, db, request.ID, "ok", &admin, now)
	require.NoError(t, err)
	require.Equal(t, model.MeshChangeApproved, approved.Status)
	require.Equal(t, "10.152.0.1", approved.NewAddress)
	home, err := homeOf(db, "proxy-1")
	require.NoError(t, err)
	require.Equal(t, eu.ID, home.RegionID)
	_, err = ApproveChange(ctx, db, request.ID, "", &admin, now)
	require.ErrorIs(t, err, ErrConflict, "decided once")

	pending, err := RequestChange(ctx, db, ChangeInput{DeviceID: device.ID, ToRegionID: eu.ID}, model.MeshChangeMember, nil, now)
	require.NoError(t, err)
	rejected, err := RejectChange(ctx, db, pending.ID, "stay", &admin, now)
	require.NoError(t, err)
	require.Equal(t, model.MeshChangeRejected, rejected.Status)

	// A direct move settles the pending change.
	pending, err = RequestChange(ctx, db, ChangeInput{DeviceID: device.ID, ToRegionID: eu.ID}, model.MeshChangeMember, nil, now)
	require.NoError(t, err)
	moved, err := MoveDevice(ctx, db, device.ID, eu.ID, &admin, now)
	require.NoError(t, err)
	require.Equal(t, "10.150.128.0", moved.OldAddress)
	require.Equal(t, "10.152.128.0", moved.NewAddress)
	changes, err := Changes(ctx, db, model.MeshChangePending)
	require.NoError(t, err)
	require.Empty(t, changes)
	all, err := Changes(ctx, db, "")
	require.NoError(t, err)
	require.Len(t, all, 4)
	require.Equal(t, "eu", all[0].ToRegionCode)
	_, err = Changes(ctx, db, "lost")
	require.ErrorIs(t, err, ErrInvalid)

	// A move through the membership is recorded and settles a pending change.
	_, err = RequestChange(ctx, db, ChangeInput{Node: "proxy-1", ToRegionID: cn.ID}, model.MeshChangeMember, nil, now)
	require.NoError(t, err)
	_, err = UpdateMember(ctx, db, home.ID, cn.ID, false, now)
	require.NoError(t, err)
	changes, err = Changes(ctx, db, model.MeshChangePending)
	require.NoError(t, err)
	require.Empty(t, changes)
	_ = pending
}

func TestHostGroupsAndNames(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	cn := region(t, db, "cn", "10.150.0.0/16")
	eu := region(t, db, "eu", "10.152.0.0/16")
	gateway, err := AddNode(ctx, db, cn.ID, proxy(1), false, now)
	require.NoError(t, err)
	_, err = AddNode(ctx, db, cn.ID, proxy(2), false, now)
	require.NoError(t, err)
	_, err = AddNode(ctx, db, eu.ID, proxy(3), false, now)
	require.NoError(t, err)

	_, err = CreateHostGroup(ctx, db, HostGroupInput{Name: "vms", Gateway: "proxy-4"}, now)
	require.ErrorIs(t, err, ErrInvalid, "the gateway is in the mesh")
	_, err = CreateHostGroup(ctx, db, HostGroupInput{Name: "vms", Gateway: "proxy-1", Standby: "proxy-3"}, now)
	require.ErrorIs(t, err, ErrInvalid, "the standby has the gateway's region")
	group, err := CreateHostGroup(ctx, db, HostGroupInput{Name: "vms", Gateway: "proxy-1", Standby: "proxy-2"}, now)
	require.NoError(t, err)
	require.Equal(t, "10.127.0.0/24", group.Segment)
	second, err := CreateHostGroup(ctx, db, HostGroupInput{Name: "lab", Gateway: "proxy-2"}, now)
	require.NoError(t, err)
	require.Equal(t, "10.127.1.0/24", second.Segment)
	_, err = CreateHostGroup(ctx, db, HostGroupInput{Name: "x", Gateway: "proxy-2", Segment: "10.127.1.0/24"}, now)
	require.ErrorIs(t, err, ErrConflict)

	_, err = CreateDeviceName(ctx, db, DeviceNameInput{HostGroupID: group.ID, Name: "db", Address: "10.127.1.5"}, now)
	require.ErrorIs(t, err, ErrInvalid, "inside the group's segment")
	_, err = CreateDeviceName(ctx, db, DeviceNameInput{HostGroupID: group.ID, Name: "db", Address: "10.127.0.5"}, now)
	require.NoError(t, err)
	_, err = CreateDeviceName(ctx, db, DeviceNameInput{Gateway: "proxy-1", Name: "printer", Address: "10.150.9.9"}, now)
	require.ErrorIs(t, err, ErrInvalid, "a local device is outside the regions")
	_, err = CreateDeviceName(ctx, db, DeviceNameInput{Gateway: "proxy-1", Name: "printer", Address: "192.168.1.20"}, now)
	require.NoError(t, err)
	// A second "a" in cn: node 1 is named "a" too.
	_, err = CreateDeviceName(ctx, db, DeviceNameInput{Gateway: "proxy-1", Name: "a", Address: "192.168.1.21"}, now)
	require.NoError(t, err)

	_, err = UpdateMember(ctx, db, gateway.ID, eu.ID, false, now)
	require.ErrorIs(t, err, ErrConflict, "a gateway stays while it has host groups")
	require.ErrorIs(t, RemoveNode(ctx, db, gateway.ID), ErrConflict)

	moved, err := UpdateHostGroup(ctx, db, group.ID, HostGroupInput{Name: "vms", Gateway: "proxy-2"}, now)
	require.NoError(t, err)
	require.Equal(t, "proxy-2", moved.GatewayRef)
	names, err := DeviceNames(ctx, db)
	require.NoError(t, err)
	for _, name := range names {
		if name.HostGroupID != nil {
			require.Equal(t, "proxy-2", name.GatewayRef, "the names follow the new gateway")
		}
	}

	list, err := Names(ctx, db)
	require.NoError(t, err)
	require.Equal(t, DefaultNameSuffix, list.Suffix)
	fqdns := map[string]int{}
	for _, entry := range list.Entries {
		fqdns[entry.FQDN]++
		if entry.FQDN == "a.cn.mesh.internal" {
			require.True(t, entry.Duplicate)
		}
	}
	require.Equal(t, 2, fqdns["a.cn.mesh.internal"])
	require.Equal(t, 1, fqdns["db.cn.mesh.internal"])
	require.Equal(t, 1, fqdns["printer.cn.mesh.internal"])
	require.Equal(t, 1, fqdns["c.eu.mesh.internal"])

	_, err = SaveSettings(ctx, db, SettingsInput{NameSuffix: "mesh.internal", HostGroupSupernet: "10.128.0.0/16", HostGroupPrefix: 24}, now)
	require.ErrorIs(t, err, ErrConflict, "the supernet stays while host groups exist")

	require.NoError(t, DeleteHostGroup(ctx, db, group.ID))
	var left int64
	require.NoError(t, db.Model(&model.MeshDeviceName{}).Where("host_group_id IS NOT NULL").Count(&left).Error)
	require.Zero(t, left)
	requireNoNames(t, db, "proxy-2")
}

func requireNoNames(t *testing.T, db *gorm.DB, gateway string) {
	t.Helper()
	var count int64
	require.NoError(t, db.Model(&model.MeshDeviceName{}).Where("gateway_ref = ?", gateway).Count(&count).Error)
	require.Zero(t, count)
}
