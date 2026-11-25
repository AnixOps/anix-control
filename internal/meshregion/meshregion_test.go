package meshregion

import (
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type clock struct{ now time.Time }

func (c *clock) Now() time.Time { return c.now }

func newService(t *testing.T) (*Service, *clock) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "mesh.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(model.MeshModels()...))
	c := &clock{now: time.Date(2026, 11, 25, 8, 0, 0, 0, time.UTC)}
	return &Service{DB: db, Now: c.Now}, c
}

func regionID(t *testing.T, s *Service, code string) uint {
	t.Helper()
	regions, err := s.Regions()
	require.NoError(t, err)
	for _, region := range regions {
		if region.Code == code {
			return region.ID
		}
	}
	t.Fatalf("no region %s", code)
	return 0
}

func TestDefaultRegions(t *testing.T) {
	s, _ := newService(t)
	regions, err := s.Regions()
	require.NoError(t, err)
	require.Len(t, regions, 4)
	var codes []string
	for _, region := range regions {
		codes = append(codes, region.Code)
		assert.Equal(t, SecurityForwardSecrecy, region.Security)
		assert.Contains(t, region.Warnings, WarningFewBorders)
		assert.Regexp(t, `^mesh-`+region.Code+`-[0-9a-f]{12}$`, region.NetworkName)
		assert.Empty(t, region.NetworkKeySealed)
		assert.Equal(t, 16384, region.PoolSize)
	}
	assert.Equal(t, []string{"cn", "ap", "eu", "na"}, codes)

	settings, err := s.Settings()
	require.NoError(t, err)
	assert.Equal(t, DefaultNameSuffix, settings.NameSuffix)
	assert.Equal(t, DefaultHostGroupSupernet, settings.HostGroupSupernet)

	// The defaults are created once.
	again, err := s.Regions()
	require.NoError(t, err)
	assert.Len(t, again, 4)
}

func TestRegionValidation(t *testing.T) {
	s, _ := newService(t)
	valid := RegionInput{Code: "me", Name: "中东", Subnet: "10.125.0.0/16", ServerRange: "10.125.0.0/20", DevicePool: "10.125.64.0/18"}
	for name, change := range map[string]func(*RegionInput){
		"code":           func(r *RegionInput) { r.Code = "Me!" },
		"name":           func(r *RegionInput) { r.Name = "" },
		"public subnet":  func(r *RegionInput) { r.Subnet = "8.8.0.0/16" },
		"not masked":     func(r *RegionInput) { r.Subnet = "10.125.1.0/16" },
		"too small":      func(r *RegionInput) { r.Subnet = "10.125.0.0/25" },
		"server outside": func(r *RegionInput) { r.ServerRange = "10.126.0.0/20" },
		"pool outside":   func(r *RegionInput) { r.DevicePool = "10.0.0.0/8" },
		"ranges overlap": func(r *RegionInput) { r.DevicePool = "10.125.0.0/18" },
		"host supernet": func(r *RegionInput) {
			r.Subnet, r.ServerRange, r.DevicePool = "10.127.0.0/16", "10.127.0.0/20", "10.127.64.0/18"
		},
		"ipv6": func(r *RegionInput) { r.Subnet = "fd00::/64" },
	} {
		input := valid
		change(&input)
		_, err := s.CreateRegion(input)
		assert.ErrorIs(t, err, ErrInvalid, name)
	}
	overlap := valid
	overlap.Subnet, overlap.ServerRange, overlap.DevicePool = "10.121.0.0/17", "10.121.0.0/20", "10.121.64.0/18"
	_, err := s.CreateRegion(overlap)
	assert.ErrorIs(t, err, ErrConflict)
	same := valid
	same.Code = "cn"
	_, err = s.CreateRegion(same)
	assert.ErrorIs(t, err, ErrConflict)

	created, err := s.CreateRegion(valid)
	require.NoError(t, err)
	assert.Equal(t, "me", created.Code)

	_, err = s.UpdateSettings(SettingsUpdate{NameSuffix: "mesh.internal", HostGroupSupernet: "10.127.0.0/16", HostGroupPrefix: 24, ReservedSubnets: []string{"10.125.0.0/24"}})
	assert.ErrorIs(t, err, ErrInvalid, "a reserved range may not overlap a region")
	settings, err := s.UpdateSettings(SettingsUpdate{NameSuffix: ".Corp.Internal.", HostGroupSupernet: "10.127.0.0/16", HostGroupPrefix: 24, ReservedSubnets: []string{"10.126.0.0/16", " "}})
	require.NoError(t, err)
	assert.Equal(t, "corp.internal", settings.NameSuffix)
	assert.Equal(t, "10.126.0.0/16", settings.ReservedSubnets)
	legacy := valid
	legacy.Code, legacy.Subnet, legacy.ServerRange, legacy.DevicePool = "old", "10.126.0.0/16", "10.126.0.0/20", "10.126.64.0/18"
	_, err = s.CreateRegion(legacy)
	assert.ErrorIs(t, err, ErrInvalid, "a region may not overlap a reserved range")

	require.NoError(t, s.DeleteRegion(created.ID))
	assert.ErrorIs(t, s.DeleteRegion(created.ID), ErrNotFound)
}

func TestFixedAndPoolAddresses(t *testing.T) {
	s, c := newService(t)
	eu := regionID(t, s, "eu")

	border, err := s.CreateMember(MemberInput{RegionID: eu, Hostname: "Border-1", Role: model.MeshRoleBorder, ProbeAddress: "203.0.113.10:11010", NodeRef: "proxy-5"})
	require.NoError(t, err)
	assert.Equal(t, "border-1", border.Hostname)
	assert.Equal(t, model.MeshAddressFixed, border.AddressMode)
	require.NotNil(t, border.Address)
	assert.Equal(t, "10.123.0.1", *border.Address, "the lowest free address of the server range, never the subnet's first")

	second, err := s.CreateMember(MemberInput{RegionID: eu, Hostname: "border-2", Role: model.MeshRoleBorder, Address: "10.123.1.1"})
	require.NoError(t, err)
	assert.Equal(t, "10.123.1.1", *second.Address)

	for name, input := range map[string]MemberInput{
		"taken address":  {RegionID: eu, Hostname: "srv", Role: model.MeshRoleServer, Address: "10.123.1.1"},
		"taken hostname": {RegionID: eu, Hostname: "border-1", Role: model.MeshRoleServer},
		"taken node":     {RegionID: eu, Hostname: "srv", Role: model.MeshRoleServer, NodeRef: "proxy-5"},
	} {
		_, err := s.CreateMember(input)
		assert.ErrorIs(t, err, ErrConflict, name)
	}
	for name, input := range map[string]MemberInput{
		"pool address as fixed": {RegionID: eu, Hostname: "srv", Role: model.MeshRoleServer, Address: "10.123.64.9"},
		"outside the subnet":    {RegionID: eu, Hostname: "srv", Role: model.MeshRoleServer, Address: "10.121.0.9"},
		"broadcast":             {RegionID: eu, Hostname: "srv", Role: model.MeshRoleServer, Address: "10.123.255.255"},
		"pool server":           {RegionID: eu, Hostname: "srv", Role: model.MeshRoleServer, AddressMode: model.MeshAddressPool},
		"probe on a server":     {RegionID: eu, Hostname: "srv", Role: model.MeshRoleServer, ProbeAddress: "203.0.113.1:1"},
		"bad role":              {RegionID: eu, Hostname: "srv", Role: "exit"},
		"bad hostname":          {RegionID: eu, Hostname: "主机", Role: model.MeshRoleServer},
		"bad node":              {RegionID: eu, Hostname: "srv", Role: model.MeshRoleServer, NodeRef: "proxy-x"},
	} {
		_, err := s.CreateMember(input)
		assert.ErrorIs(t, err, ErrInvalid, name)
	}

	phone, err := s.CreateMember(MemberInput{RegionID: eu, Hostname: "phone", Role: model.MeshRoleDevice})
	require.NoError(t, err)
	assert.Equal(t, model.MeshAddressPool, phone.AddressMode)
	assert.Equal(t, "10.123.64.0", *phone.Address, "the pool's first address is not the subnet's first")
	laptop, err := s.CreateMember(MemberInput{RegionID: eu, Hostname: "laptop", Role: model.MeshRoleDevice})
	require.NoError(t, err)
	assert.Equal(t, "10.123.64.1", *laptop.Address)

	regions, err := s.Regions()
	require.NoError(t, err)
	for _, region := range regions {
		if region.ID == eu {
			assert.Equal(t, 2, region.BorderNodes)
			assert.Equal(t, 4, region.Members)
			assert.Equal(t, 2, region.PoolUsed)
			assert.Equal(t, []string{WarningBorderWithoutProbe}, region.Warnings)
		}
	}

	// A pool address unused for 30 days goes to the next device; until then
	// it stays its device's.
	c.now = c.now.Add(PoolReclaimAfter - time.Hour)
	tablet, err := s.CreateMember(MemberInput{RegionID: eu, Hostname: "tablet", Role: model.MeshRoleDevice})
	require.NoError(t, err)
	assert.Equal(t, "10.123.64.2", *tablet.Address)
	members, err := s.Members(eu)
	require.NoError(t, err)
	for _, member := range members {
		assert.Empty(t, member.Warnings, member.Hostname)
		assert.Equal(t, member.Hostname+".eu.mesh.internal", member.FQDN)
	}
	c.now = c.now.Add(2 * time.Hour)
	members, err = s.Members(eu)
	require.NoError(t, err)
	idle := map[string]bool{}
	for _, member := range members {
		idle[member.Hostname] = len(member.Warnings) > 0 && member.Warnings[0] == WarningPoolAddressIdle
	}
	assert.Equal(t, map[string]bool{"border-1": false, "border-2": false, "phone": true, "laptop": true, "tablet": false}, idle, "fixed addresses are never reclaimed")
	// The pool is not full, so a new device still takes a free address.
	watch, err := s.CreateMember(MemberInput{RegionID: eu, Hostname: "watch", Role: model.MeshRoleDevice})
	require.NoError(t, err)
	assert.Equal(t, "10.123.64.3", *watch.Address)
}

func TestIdlePoolAddressGoesToTheNextDevice(t *testing.T) {
	s, c := newService(t)
	small, err := s.CreateRegion(RegionInput{Code: "lab", Name: "实验", Subnet: "10.130.0.0/24", ServerRange: "10.130.0.0/28", DevicePool: "10.130.0.252/30"})
	require.NoError(t, err)
	// 10.130.0.252/30 holds .252, .253, .254 (.255 is the subnet's last).
	var names []string
	for i := 0; i < 3; i++ {
		member, err := s.CreateMember(MemberInput{RegionID: small.ID, Hostname: fmt.Sprintf("d%d", i), Role: model.MeshRoleDevice})
		require.NoError(t, err)
		names = append(names, *member.Address)
	}
	assert.Equal(t, []string{"10.130.0.252", "10.130.0.253", "10.130.0.254"}, names)
	_, err = s.CreateMember(MemberInput{RegionID: small.ID, Hostname: "d3", Role: model.MeshRoleDevice})
	assert.ErrorIs(t, err, ErrExhausted)

	c.now = c.now.Add(PoolReclaimAfter)
	taker, err := s.CreateMember(MemberInput{RegionID: small.ID, Hostname: "d3", Role: model.MeshRoleDevice})
	require.NoError(t, err)
	assert.Equal(t, "10.130.0.252", *taker.Address)
	members, err := s.Members(small.ID)
	require.NoError(t, err)
	for _, member := range members {
		if member.Hostname == "d0" {
			assert.Nil(t, member.Address)
			assert.Equal(t, []string{WarningNoAddress}, member.Warnings)
		}
	}
}

func TestRegionChangeApproval(t *testing.T) {
	s, _ := newService(t)
	cn, ap := regionID(t, s, "cn"), regionID(t, s, "ap")
	phone, err := s.CreateMember(MemberInput{RegionID: cn, Hostname: "phone", Role: model.MeshRoleDevice})
	require.NoError(t, err)

	_, err = s.RequestChange(phone.ID, cn, "", nil)
	assert.ErrorIs(t, err, ErrInvalid)
	change, err := s.RequestChange(phone.ID, ap, "ap 40 ms, cn 180 ms", nil)
	require.NoError(t, err)
	assert.Equal(t, model.MeshChangePending, change.Status)
	assert.Equal(t, model.MeshChangeByMember, change.Source)
	_, err = s.RequestChange(phone.ID, ap, "", nil)
	assert.ErrorIs(t, err, ErrConflict, "one pending change per member")

	pending, err := s.Changes(model.MeshChangePending)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	assert.Equal(t, "phone", pending[0].Hostname)
	assert.Equal(t, "cn", pending[0].FromCode)
	assert.Equal(t, "ap", pending[0].ToCode)

	admin := uint(7)
	approved, err := s.Approve(change.ID, Decision{Note: "ok", DecidedBy: &admin})
	require.NoError(t, err)
	assert.Equal(t, model.MeshChangeApproved, approved.Status)
	assert.Equal(t, "10.121.64.0", approved.OldAddress)
	assert.Equal(t, "10.122.64.0", approved.NewAddress)
	_, err = s.Approve(change.ID, Decision{})
	assert.ErrorIs(t, err, ErrConflict, "a decided change stays decided")

	members, err := s.Members(ap)
	require.NoError(t, err)
	require.Len(t, members, 1)
	assert.Equal(t, "phone.ap.mesh.internal", members[0].FQDN)

	rejected, err := s.RequestChange(phone.ID, cn, "", nil)
	require.NoError(t, err)
	rejected, err = s.Reject(rejected.ID, Decision{Note: "no"})
	require.NoError(t, err)
	assert.Equal(t, model.MeshChangeRejected, rejected.Status)

	// An administrator's move grants a pending request for the same region.
	request, err := s.RequestChange(phone.ID, cn, "", nil)
	require.NoError(t, err)
	moved, err := s.Move(phone.ID, cn, Decision{DecidedBy: &admin})
	require.NoError(t, err)
	assert.Equal(t, request.ID, moved.ID)
	assert.Equal(t, model.MeshChangeApproved, moved.Status)
	direct, err := s.Move(phone.ID, ap, Decision{DecidedBy: &admin})
	require.NoError(t, err)
	assert.Equal(t, model.MeshChangeByAdmin, direct.Source)
	all, err := s.Changes("")
	require.NoError(t, err)
	assert.Len(t, all, 4)
	_, err = s.Changes("open")
	assert.ErrorIs(t, err, ErrInvalid)
}

func TestHostGroupsAndNames(t *testing.T) {
	s, _ := newService(t)
	eu, na := regionID(t, s, "eu"), regionID(t, s, "na")
	gateway, err := s.CreateMember(MemberInput{RegionID: eu, Hostname: "host-a", Role: model.MeshRoleGateway})
	require.NoError(t, err)
	standby, err := s.CreateMember(MemberInput{RegionID: eu, Hostname: "host-a2", Role: model.MeshRoleGateway})
	require.NoError(t, err)
	server, err := s.CreateMember(MemberInput{RegionID: eu, Hostname: "db", Role: model.MeshRoleServer})
	require.NoError(t, err)
	far, err := s.CreateMember(MemberInput{RegionID: na, Hostname: "host-b", Role: model.MeshRoleGateway})
	require.NoError(t, err)

	_, err = s.CreateHostGroup(HostGroupInput{Name: "x", GatewayMemberID: server.ID})
	assert.ErrorIs(t, err, ErrInvalid, "a server is not a gateway")
	_, err = s.CreateHostGroup(HostGroupInput{Name: "x", GatewayMemberID: gateway.ID, StandbyMemberID: far.ID})
	assert.ErrorIs(t, err, ErrInvalid, "the standby is in the gateway's region")

	group, err := s.CreateHostGroup(HostGroupInput{Name: "宿主 A", GatewayMemberID: gateway.ID, StandbyMemberID: standby.ID})
	require.NoError(t, err)
	assert.Equal(t, "10.127.0.0/24", group.Segment)
	next, err := s.CreateHostGroup(HostGroupInput{Name: "宿主 B", GatewayMemberID: far.ID})
	require.NoError(t, err)
	assert.Equal(t, "10.127.1.0/24", next.Segment)
	_, err = s.CreateHostGroup(HostGroupInput{Name: "宿主 C", GatewayMemberID: far.ID, Segment: "10.127.1.0/24"})
	assert.ErrorIs(t, err, ErrConflict)
	_, err = s.CreateHostGroup(HostGroupInput{Name: "宿主 C", GatewayMemberID: far.ID, Segment: "10.127.2.0/25"})
	assert.ErrorIs(t, err, ErrInvalid)

	vm, err := s.CreateDeviceName(DeviceNameInput{HostGroupID: group.ID, Name: "VM-1", Address: "10.127.0.10"})
	require.NoError(t, err)
	assert.Equal(t, gateway.ID, vm.MemberID, "a group's names are published by its gateway")
	assert.Equal(t, "vm-1", vm.Name)
	_, err = s.CreateDeviceName(DeviceNameInput{HostGroupID: group.ID, Name: "vm-2", Address: "10.127.1.10"})
	assert.ErrorIs(t, err, ErrInvalid, "outside the group's segment")
	_, err = s.CreateDeviceName(DeviceNameInput{HostGroupID: group.ID, Name: "vm-1", Address: "10.127.0.11"})
	assert.ErrorIs(t, err, ErrConflict, "names are unique in a region")
	_, err = s.CreateDeviceName(DeviceNameInput{HostGroupID: group.ID, Name: "db", Address: "10.127.0.11"})
	assert.ErrorIs(t, err, ErrConflict, "a member's hostname is taken")
	printer, err := s.CreateDeviceName(DeviceNameInput{MemberID: gateway.ID, Name: "printer", Address: "192.168.1.20"})
	require.NoError(t, err)
	assert.Nil(t, printer.HostGroupID)
	for name, input := range map[string]DeviceNameInput{
		"public":        {MemberID: gateway.ID, Name: "nas", Address: "203.0.113.5"},
		"region subnet": {MemberID: gateway.ID, Name: "nas", Address: "10.123.0.50"},
		"host supernet": {MemberID: gateway.ID, Name: "nas", Address: "10.127.9.1"},
		"not a gateway": {MemberID: server.ID, Name: "nas", Address: "192.168.1.21"},
	} {
		_, err := s.CreateDeviceName(input)
		assert.ErrorIs(t, err, ErrInvalid, name)
	}
	_, err = s.CreateMember(MemberInput{RegionID: eu, Hostname: "printer", Role: model.MeshRoleServer})
	assert.ErrorIs(t, err, ErrConflict, "a device name is taken for members too")

	groups, err := s.HostGroups()
	require.NoError(t, err)
	require.Len(t, groups, 2)
	assert.Equal(t, "eu", groups[0].RegionCode)
	assert.Equal(t, "host-a", groups[0].GatewayHostname)
	assert.Equal(t, "host-a2", groups[0].StandbyHostname)
	assert.Equal(t, 1, groups[0].Names)

	list, err := s.Names()
	require.NoError(t, err)
	assert.Equal(t, "mesh.internal", list.Suffix)
	byName := map[string]NameEntry{}
	for _, entry := range list.Entries {
		byName[entry.FQDN] = entry
		assert.False(t, entry.Duplicate)
	}
	assert.Equal(t, NameSourceHostGroup, byName["vm-1.eu.mesh.internal"].Source)
	assert.Equal(t, "10.127.0.10", byName["vm-1.eu.mesh.internal"].Address)
	assert.Equal(t, NameSourceDevice, byName["printer.eu.mesh.internal"].Source)
	assert.Equal(t, NameSourceMember, byName["db.eu.mesh.internal"].Source)
	assert.Len(t, list.Entries, 6)

	// A gateway moves only with the group's other gateway; its names move
	// with it.
	_, err = s.Move(gateway.ID, na, Decision{})
	assert.ErrorIs(t, err, ErrConflict)
	assert.ErrorIs(t, s.DeleteMember(gateway.ID), ErrConflict, "a gateway of a group stays")
	_, err = s.UpdateHostGroup(group.ID, HostGroupInput{Name: "宿主 A", GatewayMemberID: gateway.ID})
	require.NoError(t, err)
	_, err = s.Move(gateway.ID, na, Decision{})
	require.NoError(t, err)
	list, err = s.Names()
	require.NoError(t, err)
	found := false
	for _, entry := range list.Entries {
		if entry.Name == "vm-1" {
			found = true
			assert.Equal(t, "vm-1.na.mesh.internal", entry.FQDN)
		}
	}
	assert.True(t, found)

	require.NoError(t, s.DeleteHostGroup(group.ID))
	rows, err := s.DeviceNames()
	require.NoError(t, err)
	require.Len(t, rows, 1, "the group's names go with it")
	assert.Equal(t, "printer", rows[0].Name)
	_, err = s.UpdateSettings(SettingsUpdate{NameSuffix: "mesh.internal", HostGroupSupernet: "10.128.0.0/16", HostGroupPrefix: 24})
	assert.True(t, errors.Is(err, ErrConflict), "the supernet stays while a host group exists")
}
