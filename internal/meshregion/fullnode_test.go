package meshregion

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestFullNodeCredentials(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	cn := region(t, db, "cn", "10.150.0.0/16")
	_, err := AddNode(ctx, db, cn.ID, proxy(1), true, now)
	require.NoError(t, err)

	for _, input := range []EnrollmentInput{
		{Hostname: "router", Role: "relay"},
		{Hostname: "router", Role: RoleFull},
		{Hostname: "router", RegionID: cn.ID, Address: "10.150.0.9"},
		{Hostname: "router", RegionID: cn.ID, Role: RoleFull, Address: "10.150.128.9"},
		{Hostname: "router", RegionID: cn.ID, Role: RoleFull, Address: "10.150.0.0"},
		{Hostname: "router", RegionID: cn.ID, Role: RoleFull, Address: "10.151.0.9"},
		{Hostname: "router", RegionID: cn.ID, Role: RoleFull, Address: "fd00::1"},
	} {
		_, err := CreateEnrollment(ctx, db, input, nil, now)
		require.ErrorIs(t, err, ErrInvalid, "%+v", input)
	}
	_, err = CreateEnrollment(ctx, db, EnrollmentInput{Hostname: "router", RegionID: cn.ID, Role: RoleFull, Address: "10.150.0.1"}, nil, now)
	require.ErrorIs(t, err, ErrConflict, "the node holds it")

	fixed, err := CreateEnrollment(ctx, db, EnrollmentInput{Hostname: "router", RegionID: cn.ID, Role: RoleFull, Address: " 10.150.0.9 "}, nil, now)
	require.NoError(t, err)
	require.Equal(t, RoleFull, fixed.Role)
	require.Equal(t, "10.150.0.9", fixed.Address)
	_, err = CreateEnrollment(ctx, db, EnrollmentInput{Hostname: "nas", RegionID: cn.ID, Role: RoleFull, Address: "10.150.0.9"}, nil, now)
	require.ErrorIs(t, err, ErrConflict, "the unused credential keeps it")
	auto, err := CreateEnrollment(ctx, db, EnrollmentInput{Hostname: "server", RegionID: cn.ID, Role: RoleFull}, nil, now)
	require.NoError(t, err)
	require.Empty(t, auto.Address)
	rows, err := Enrollments(ctx, db)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, RoleFull, rows[1].Role)
	require.Equal(t, "10.150.0.9", rows[1].Address)

	// The node joining later avoids the kept address.
	member, err := AddNode(ctx, db, cn.ID, proxy(2), false, now)
	require.NoError(t, err)
	require.Equal(t, "10.150.0.2", member.Address)

	enrolled, err := Enroll(ctx, db, EnrollRequest{Token: fixed.Token, DeviceKey: laptopKey}, now)
	require.NoError(t, err)
	network := enrolled.Config["networks"].([]map[string]any)[0]
	require.Equal(t, RoleFull, network["role"])
	require.Equal(t, "10.150.0.9/16", network["address"])
	require.Equal(t, []map[string]any{{"transport": "tcp", "port": Port}, {"transport": "udp", "port": Port}}, network["listen"])
	require.Equal(t, map[string]any{"accept": []string{}, "advertise": []string{}}, network["subnets"], "nothing confirmed yet")

	// The lowest free address of the server segment, past the members and
	// the full node.
	server, err := Enroll(ctx, db, EnrollRequest{Token: auto.Token, DeviceKey: strings.Repeat("s", 32)}, now)
	require.NoError(t, err)
	require.Equal(t, "10.150.0.3/16", server.Config["networks"].([]map[string]any)[0]["address"])

	views, err := Devices(ctx, db, cn.ID, now.Add(PoolReclaimAfter+time.Hour))
	require.NoError(t, err)
	require.Len(t, views, 2)
	for _, view := range views {
		require.Equal(t, RoleFull, view.Role)
		require.Empty(t, view.Warnings, "a fixed address is never idle")
	}

	// A fixed address never goes back to the pool, however long unused.
	later := now.Add(PoolReclaimAfter + time.Hour)
	phone, err := CreateDevice(ctx, db, cn.ID, DeviceInput{Hostname: "phone"}, later)
	require.NoError(t, err)
	require.Equal(t, "10.150.128.0", *phone.Address)
	var router model.MeshDevice
	require.NoError(t, db.First(&router, enrolled.DeviceID).Error)
	require.Equal(t, "10.150.0.9", *router.Address)

	// Enrolling again keeps the role and the address.
	_, err = CreateEnrollment(ctx, db, EnrollmentInput{DeviceID: router.ID, Role: RoleAccess}, nil, now)
	require.ErrorIs(t, err, ErrInvalid)
	_, err = CreateEnrollment(ctx, db, EnrollmentInput{DeviceID: router.ID, Address: "10.150.0.20"}, nil, now)
	require.ErrorIs(t, err, ErrInvalid)
	again, err := CreateEnrollment(ctx, db, EnrollmentInput{DeviceID: router.ID}, nil, now)
	require.NoError(t, err)
	require.Equal(t, RoleFull, again.Role)
	require.Equal(t, "10.150.0.9", again.Address)
	second, err := Enroll(ctx, db, EnrollRequest{Token: again.Token, DeviceKey: laptopKey}, now)
	require.NoError(t, err)
	require.Equal(t, router.ID, second.DeviceID)
	require.Equal(t, "10.150.0.9/16", second.Config["networks"].([]map[string]any)[0]["address"])

	// Moving to another region gives a fixed address there.
	eu := region(t, db, "eu", "10.152.0.0/16")
	change, err := RequestChange(ctx, db, ChangeInput{DeviceID: router.ID, ToRegionID: eu.ID}, model.MeshChangeAdmin, nil, now)
	require.NoError(t, err)
	_, err = ApproveChange(ctx, db, change.ID, "", nil, now)
	require.NoError(t, err)
	require.NoError(t, db.First(&router, router.ID).Error)
	require.Equal(t, "10.152.0.2", *router.Address, "after the boundary node's address")

	// Deleting the device keeps the credential as its record.
	require.NoError(t, DeleteDevice(ctx, db, router.ID, now))
	rows, err = Enrollments(ctx, db)
	require.NoError(t, err)
	for _, row := range rows {
		if row.ID == fixed.ID {
			require.Equal(t, RoleFull, row.Role)
		}
	}
}

func TestWithdrawnFullNodeCredentialFreesItsAddress(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	cn := region(t, db, "cn", "10.150.0.0/16")
	issued, err := CreateEnrollment(ctx, db, EnrollmentInput{Hostname: "router", RegionID: cn.ID, Role: RoleFull, Address: "10.150.0.1"}, nil, now)
	require.NoError(t, err)
	member, err := AddNode(ctx, db, cn.ID, proxy(1), true, now)
	require.NoError(t, err)
	require.Equal(t, "10.150.0.2", member.Address)
	require.NoError(t, DeleteEnrollment(ctx, db, issued.ID))
	var count int64
	require.NoError(t, db.Model(&model.MeshFullNode{}).Count(&count).Error)
	require.Zero(t, count)
	_, err = CreateEnrollment(ctx, db, EnrollmentInput{Hostname: "router", RegionID: cn.ID, Role: RoleFull, Address: "10.150.0.1", Hours: 1}, nil, now)
	require.NoError(t, err)

	// An expired credential keeps no address.
	_, err = CreateEnrollment(ctx, db, EnrollmentInput{Hostname: "nas", RegionID: cn.ID, Role: RoleFull, Address: "10.150.0.1"}, nil, now)
	require.ErrorIs(t, err, ErrConflict)
	_, err = CreateEnrollment(ctx, db, EnrollmentInput{Hostname: "nas", RegionID: cn.ID, Role: RoleFull, Address: "10.150.0.1"}, nil, now.Add(2*time.Hour))
	require.NoError(t, err)
}

// A new range clears the kept addresses; deleting the region removes its
// unused credentials.
func TestFullNodeCredentialsFollowTheirRegion(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	other := region(t, db, "eu", "10.152.0.0/16")
	issued, err := CreateEnrollment(ctx, db, EnrollmentInput{Hostname: "edge", RegionID: other.ID, Role: RoleFull, Address: "10.152.0.5"}, nil, now)
	require.NoError(t, err)
	var node model.MeshFullNode
	_, err = UpdateRegion(ctx, db, other.ID, RegionInput{Code: "eu2", Name: "eu", CIDR: "10.152.0.0/16", Enabled: true}, now)
	require.NoError(t, err)
	require.NoError(t, db.Where("enrollment_id = ?", issued.ID).First(&node).Error)
	require.Equal(t, "10.152.0.5", node.Address, "a new code keeps the address")
	_, err = UpdateRegion(ctx, db, other.ID, RegionInput{Code: "eu", Name: "eu", CIDR: "10.154.0.0/16", Enabled: true}, now)
	require.NoError(t, err)
	require.NoError(t, db.Where("enrollment_id = ?", issued.ID).First(&node).Error)
	require.Empty(t, node.Address)
	require.NoError(t, DeleteRegion(ctx, db, other.ID))
	require.ErrorIs(t, db.Where("enrollment_id = ?", issued.ID).First(&node).Error, gorm.ErrRecordNotFound)
	require.ErrorIs(t, DeleteEnrollment(ctx, db, issued.ID), ErrNotFound)
}

func TestFullNodeSubnets(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	cn := region(t, db, "cn", "10.150.0.0/16")
	access, err := CreateDevice(ctx, db, cn.ID, DeviceInput{Hostname: "phone"}, now)
	require.NoError(t, err)
	_, err = SetDeviceSubnets(ctx, db, access.ID, []string{"192.168.1.0/24"}, now)
	require.ErrorIs(t, err, ErrInvalid, "an access device advertises nothing")
	_, err = SetDeviceSubnets(ctx, db, 99, nil, now)
	require.ErrorIs(t, err, ErrNotFound)

	routers := []uint{}
	for i, name := range []string{"router", "office"} {
		issued, err := CreateEnrollment(ctx, db, EnrollmentInput{Hostname: name, RegionID: cn.ID, Role: RoleFull}, nil, now)
		require.NoError(t, err)
		enrolled, err := Enroll(ctx, db, EnrollRequest{Token: issued.Token, DeviceKey: strings.Repeat(string(rune('a'+i)), 32)}, now)
		require.NoError(t, err)
		routers = append(routers, enrolled.DeviceID)
	}

	for _, bad := range [][]string{{"8.8.8.0/24"}, {"not-a-network"}, {"fd00::/64"}} {
		_, err := SetDeviceSubnets(ctx, db, routers[0], bad, now)
		require.ErrorIs(t, err, ErrInvalid, "%v", bad)
	}
	_, err = SetDeviceSubnets(ctx, db, routers[0], distinctHosts(maxFullNodeSubnets+1), now)
	require.ErrorIs(t, err, ErrInvalid)
	for _, overlap := range [][]string{{"10.150.5.0/24"}, {"10.127.1.0/24"}} {
		_, err := SetDeviceSubnets(ctx, db, routers[0], overlap, now)
		require.ErrorIs(t, err, ErrConflict, "%v", overlap)
	}

	subnets, err := SetDeviceSubnets(ctx, db, routers[0], []string{"192.168.1.0/24", " 192.168.1.0/24", "", "172.16.0.0/16"}, now)
	require.NoError(t, err)
	require.Equal(t, []string{"192.168.1.0/24", "172.16.0.0/16"}, subnets)
	_, err = SetDeviceSubnets(ctx, db, routers[1], []string{"192.168.1.128/25"}, now)
	require.ErrorIs(t, err, ErrConflict, "the other full node advertises it")
	// The address plan keeps out of the confirmed subnets too.
	_, err = CreateRegion(ctx, db, RegionInput{Code: "lan", Name: "lan", CIDR: "192.168.0.0/16"}, now)
	require.ErrorIs(t, err, ErrConflict, "a region over a confirmed subnet")
	settings := SettingsInput{NameSuffix: DefaultNameSuffix, HostGroupSupernet: DefaultHostGroupSupernet, HostGroupPrefix: DefaultHostGroupPrefix}
	_, err = SaveSettings(ctx, db, SettingsInput{NameSuffix: settings.NameSuffix, HostGroupSupernet: "172.16.0.0/16", HostGroupPrefix: 24}, now)
	require.ErrorIs(t, err, ErrConflict, "a host group supernet over a confirmed subnet")
	settings.ReservedRanges = []string{"172.16.0.0/16"}
	_, err = SaveSettings(ctx, db, settings, now)
	require.ErrorIs(t, err, ErrConflict, "a reserved range over a confirmed subnet")

	var router model.MeshDevice
	require.NoError(t, db.First(&router, routers[0]).Error)
	config, err := DeviceConfig(ctx, db, router, now)
	require.NoError(t, err)
	require.Equal(t, []string{"192.168.1.0/24", "172.16.0.0/16"}, config["networks"].([]map[string]any)[0]["subnets"].(map[string]any)["advertise"])
	views, err := Devices(ctx, db, cn.ID, now)
	require.NoError(t, err)
	for _, view := range views {
		if view.ID == router.ID {
			require.Equal(t, []string{"192.168.1.0/24", "172.16.0.0/16"}, view.Subnets)
		}
	}

	subnets, err = SetDeviceSubnets(ctx, db, routers[0], nil, now)
	require.NoError(t, err)
	require.Empty(t, subnets)
}

// distinctHosts answers n distinct private /32 networks.
func distinctHosts(n int) []string {
	hosts := make([]string, 0, n)
	for i := range n {
		hosts = append(hosts, "192.168.200."+strconv.Itoa(i+1)+"/32")
	}
	return hosts
}
