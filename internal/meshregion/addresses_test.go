package meshregion

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/netip"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
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
		{NameSuffix: "corp.example", HostGroupSupernet: "10.127.0.0/16", HostGroupPrefix: 24},
		{NameSuffix: "com", HostGroupSupernet: "10.127.0.0/16", HostGroupPrefix: 24},
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
	_, err = SaveSettings(ctx, db, SettingsInput{NameSuffix: "mesh.internal", HostGroupSupernet: "10.127.0.0/16", HostGroupPrefix: 24, ReservedRanges: []string{"10.126.0.0/24"}}, now)
	require.ErrorIs(t, err, ErrConflict, "an old network has the regions' prefix length")
	saved, err := SaveSettings(ctx, db, SettingsInput{NameSuffix: ".Corp.Internal.", HostGroupSupernet: "10.127.0.0/16", HostGroupPrefix: 26, ReservedRanges: []string{"10.126.0.0/16", " "}}, now)
	require.NoError(t, err)
	require.Equal(t, "corp.internal", saved.NameSuffix)
	require.Equal(t, []string{"10.126.0.0/16"}, saved.ReservedRanges)
	require.NotNil(t, saved.UpdatedAt)

	_, err = CreateRegion(ctx, db, RegionInput{Code: "eu", Name: "eu", CIDR: "10.126.0.0/16"}, now)
	require.ErrorIs(t, err, ErrConflict, "a region may not overlap a reserved range")
	_, err = CreateRegion(ctx, db, RegionInput{Code: "eu", Name: "eu", CIDR: "10.127.0.0/16"}, now)
	require.ErrorIs(t, err, ErrConflict, "a region may not overlap the host group supernet")
	require.NoError(t, DeleteRegion(ctx, db, firstRegion(t, db).ID))
	_, err = CreateRegion(ctx, db, RegionInput{Code: "eu", Name: "eu", CIDR: "10.152.0.0/24"}, now)
	require.ErrorIs(t, err, ErrConflict, "a region has the reserved ranges' prefix length")
}

func firstRegion(t *testing.T, db *gorm.DB) model.MeshRegion {
	t.Helper()
	var row model.MeshRegion
	require.NoError(t, db.Order("id ASC").First(&row).Error)
	return row
}

func TestDevicePool(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	cn := region(t, db, "cn", "10.150.0.0/24")
	eu := region(t, db, "eu", "10.152.0.0/24")

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
	require.Equal(t, 127, pools[eu.ID].Size)
	require.Equal(t, 32767, PoolSize(netip.MustParsePrefix("10.153.0.0/16")), "a /16 pool is .128.0 to .255.254")
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
	_, err = UpdateMember(ctx, db, home.ID, cn.ID, false, nil, now)
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

	_, err = UpdateMember(ctx, db, gateway.ID, eu.ID, false, nil, now)
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

func TestNodeConfigNamesAndHostGroups(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	cn := region(t, db, "cn", "10.150.0.0/16")
	eu := region(t, db, "eu", "10.152.0.0/16")
	for _, id := range []uint32{1, 2} {
		_, err := AddNode(ctx, db, cn.ID, proxy(id), false, now)
		require.NoError(t, err)
	}
	_, err := AddNode(ctx, db, eu.ID, proxy(3), false, now)
	require.NoError(t, err)
	_, err = CreateDevice(ctx, db, cn.ID, DeviceInput{Hostname: "phone"}, now)
	require.NoError(t, err)
	_, err = CreateDevice(ctx, db, cn.ID, DeviceInput{Hostname: "a"}, now)
	require.NoError(t, err)
	_, err = CreateHostGroup(ctx, db, HostGroupInput{Name: "vms", Gateway: "proxy-1", Standby: "proxy-2"}, now)
	require.NoError(t, err)

	network := func(id uint32) map[string]any {
		doc, err := NodeMember(db, proxy(id))
		require.NoError(t, err)
		networks := doc["networks"].([]map[string]any)
		require.Len(t, networks, 1)
		return networks[0]
	}
	// Every name as <name>.<region code>, the duplicate "a.cn" (node a and
	// the device a) with both addresses (R28: the kernel answers both and
	// marks the duplicate); no host group before its gateway reports.
	cnNet := network(1)
	dns := cnNet["dns"].(map[string]any)
	require.Equal(t, true, dns["enabled"])
	require.Equal(t, "mesh.internal", dns["suffix"])
	require.ElementsMatch(t, []map[string]any{
		{"name": "a.cn", "address": "10.150.0.1"},
		{"name": "a.cn", "address": "10.150.128.1"},
		{"name": "b.cn", "address": "10.150.0.2"},
		{"name": "c.eu", "address": "10.152.0.1"},
		{"name": "phone.cn", "address": "10.150.128.0"},
	}, dns["names"])
	require.Equal(t, []map[string]any{}, cnNet["host_groups"])

	var pushed int
	cancel := OnChange(func([]agentcontrol.AgentNode) { pushed++ })
	defer cancel()
	status := func(id uint32, instance string) {
		payload := fmt.Sprintf(`{"networks":[],"instance_id":%q}`, instance)
		require.NoError(t, RecordStatus(ctx, db, proxy(id), []byte(payload), now, now))
	}
	status(1, "2f6d7c1e-0b8a-4c55-9e0e-6a1f3b9d2c10")
	require.Equal(t, 1, pushed, "a gateway's new instance id pushes")
	status(1, "2f6d7c1e-0b8a-4c55-9e0e-6a1f3b9d2c10")
	status(3, "7a0c9e42-51d3-4b1e-8f7a-3c2d1e0f9b88")
	require.Equal(t, 1, pushed, "an unchanged id or a node that is no gateway does not")
	require.Equal(t, []map[string]any{{"cidr": "10.127.0.0/24", "gateway": "2f6d7c1e-0b8a-4c55-9e0e-6a1f3b9d2c10"}}, network(2)["host_groups"])
	require.Equal(t, []map[string]any{}, network(3)["host_groups"], "only the gateway's region")

	status(2, "bad id")
	require.NotContains(t, network(1)["host_groups"].([]map[string]any)[0], "backup_gateway", "an invalid id is ignored")
	status(2, "c41b7d0e-9a2f-4e63-8d15-0f7e6b5a4c39")
	require.Equal(t, "c41b7d0e-9a2f-4e63-8d15-0f7e6b5a4c39", network(1)["host_groups"].([]map[string]any)[0]["backup_gateway"])
}

// The vectors of the node kernel's name rule (spec §22.5), which the
// panel must match exactly.
func TestNormalizeName(t *testing.T) {
	for raw, want := range map[string]string{
		"中国":     "xn--fiqs8s",
		"Bücher": "xn--bcher-kva",
		"香港 01":  "xn---01-by5gj66p",
		"北京上海广州深圳杭州南京成都重庆武汉西安天津苏州长沙郑州青岛": "xn--fhq3ob536bhjr1rcwim2eba12nmcw58c4qr78mkhd0rglncb29td03a4nd",
		"北京上海广州深圳杭州南京成都重庆武汉西 安天津苏州":      "xn--fhq3ob536bhjr1rnqnba06lfc366cwwq5fm24ce3f36brv1ukg2a6hd",
		"e\u0301x":                     "e-x",
		"①号机":                          "xn--rpr216b",
		"٣":                            "xn--cib",
		"İstanbul":                     "istanbul",
		"ΟΔΟΣ":                         "xn--pxavbq",
		strings.Repeat("a", 62) + "-b": strings.Repeat("a", 62),
		strings.Repeat("a", 60) + "é":  strings.Repeat("a", 60),
		"！！":                           "",
		"a\U000105C0b":                 "a-b",
		"cafe\u0301":                   "cafe",
		"DB-01":                        "db-01",
		" web_1 ":                      "web-1",
		"--edge--":                     "edge",
		"!!!":                          "",
		"":                             "",
	} {
		require.Equal(t, want, normalizeName(raw), raw)
	}
	// A long name loses whole characters until its encoding fits.
	require.Equal(t, "xn--fiq"+strings.Repeat("a", 56), normalizeName(strings.Repeat("中", 85)))
}

// nameTableSHA256 is the SHA-256 of the generated name table. The node
// kernel is to check its copy against the same value (TODO.md), so that
// the two cannot drift apart; a new table changes it in both together.
const nameTableSHA256 = "f1f486fd79d9fd8b15183a5d4d1a6efa801af64c6744973d0c65ba958010cd7c"

func TestNameTable(t *testing.T) {
	sum := sha256.Sum256([]byte(nameTableText))
	require.Equal(t, nameTableSHA256, hex.EncodeToString(sum[:]), "the table changed: update the kernel's copy and this value together")
	table := loadNameTable()
	require.True(t, table.kept('中'))
	require.True(t, table.kept(0x0663), "Arabic-Indic digit three is Nd")
	require.False(t, table.kept(0x0301), "a combining mark is not kept")
	require.False(t, table.kept(0x105C0), "unassigned in Unicode 15.1.0")
	require.Equal(t, rune('i'), table.lower[0x0130])
	require.Equal(t, rune(0x03C3), table.lower[0x03A3], "final sigma is not context dependent")
	for _, bad := range []string{"keep 0100 00FF\n", "keep 0100 1100000\n", "lower 01g0 0069\n", "lower 110000 0069\n", "keep 0100\n"} {
		_, err := parseNameTable(bad)
		require.Error(t, err, bad)
	}
}

func TestUsedPoolAddressStays(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	lab := region(t, db, "lab", "192.168.9.0/24")
	_, err := AddNode(ctx, db, lab.ID, proxy(1), false, now)
	require.NoError(t, err)
	old := now.Add(-PoolReclaimAfter - 1)
	seen, err := CreateDevice(ctx, db, lab.ID, DeviceInput{Hostname: "seen"}, old)
	require.NoError(t, err)
	for i := 1; i < PoolSize(netip.MustParsePrefix(lab.CIDR)); i++ {
		_, err := CreateDevice(ctx, db, lab.ID, DeviceInput{Hostname: fmt.Sprintf("d%d", i)}, now)
		require.NoError(t, err)
	}
	// A node sees the device: its address is in use, so it stays its own.
	report := fmt.Sprintf(`{"networks":[{"id":"lab","peers":[{"address":%q},{"address":"bad"}]}]}`, *seen.Address)
	require.NoError(t, RecordStatus(ctx, db, proxy(1), []byte(report), now, now))
	_, err = CreateDevice(ctx, db, lab.ID, DeviceInput{Hostname: "next"}, now)
	require.ErrorIs(t, err, ErrPoolFull, "a seen address is in use")
	require.ErrorIs(t, RecordStatus(ctx, db, proxy(2), []byte(report), now, now), ErrNotMember)

	// A device that lost its address gets a free one again.
	later := now.Add(PoolReclaimAfter + 1)
	next, err := CreateDevice(ctx, db, lab.ID, DeviceInput{Hostname: "next"}, later)
	require.NoError(t, err)
	var lost model.MeshDevice
	require.NoError(t, db.Where("address IS NULL").First(&lost).Error)
	_, err = AssignDeviceAddress(ctx, db, lost.ID, now)
	require.ErrorIs(t, err, ErrPoolFull, "no address is unused for 30 days")
	require.NoError(t, DeleteDevice(ctx, db, next.ID, now))
	again, err := AssignDeviceAddress(ctx, db, lost.ID, now)
	require.NoError(t, err)
	require.Equal(t, *next.Address, *again.Address)
	_, err = AssignDeviceAddress(ctx, db, lost.ID, later)
	require.ErrorIs(t, err, ErrConflict, "a device with an address keeps it")
}

func TestRegionChangeToADisabledRegion(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	cn := region(t, db, "cn", "10.150.0.0/16")
	eu := region(t, db, "eu", "10.152.0.0/16")
	na, err := CreateRegion(ctx, db, RegionInput{Code: "na", Name: "na", CIDR: "10.153.0.0/16"}, now)
	require.NoError(t, err)
	device, err := CreateDevice(ctx, db, cn.ID, DeviceInput{Hostname: "phone"}, now)
	require.NoError(t, err)
	_, err = RequestChange(ctx, db, ChangeInput{DeviceID: device.ID, ToRegionID: na.ID}, model.MeshChangeMember, nil, now)
	require.ErrorIs(t, err, ErrInvalid, "a disabled region takes no request")

	// A region disabled after the request: the approval is refused.
	request, err := RequestChange(ctx, db, ChangeInput{DeviceID: device.ID, ToRegionID: eu.ID}, model.MeshChangeMember, nil, now)
	require.NoError(t, err)
	_, err = UpdateRegion(ctx, db, eu.ID, RegionInput{Code: "eu", Name: "eu", CIDR: eu.CIDR}, now)
	require.NoError(t, err)
	_, err = ApproveChange(ctx, db, request.ID, "", nil, now)
	require.ErrorIs(t, err, ErrConflict)
	rejected, err := RejectChange(ctx, db, request.ID, "the region is closed", nil, now)
	require.NoError(t, err)
	require.Equal(t, model.MeshChangeRejected, rejected.Status)
}
