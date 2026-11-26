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

// pathMesh is a region cn with the nodes proxy-1 to proxy-3 and proxy-5 to
// proxy-9, a region eu with proxy-4, an access device and a full node
// device in cn.
type pathMesh struct {
	db      *gorm.DB
	cn, eu  model.MeshRegion
	members map[uint32]model.MeshMember
	phone   model.MeshDevice
	router  model.MeshDevice
}

func (m pathMesh) address(id uint32) string { return m.members[id].Address }

func newPathMesh(t *testing.T) pathMesh {
	t.Helper()
	db := openDB(t)
	ctx := context.Background()
	for id := uint(5); id <= 9; id++ {
		require.NoError(t, db.Create(&model.Node{ID: id, Name: "n" + strconv.Itoa(int(id)), Host: "n" + strconv.Itoa(int(id)) + ".example.com"}).Error)
	}
	mesh := pathMesh{db: db, members: map[uint32]model.MeshMember{}}
	mesh.cn = region(t, db, "cn", "10.150.0.0/24")
	mesh.eu = region(t, db, "eu", "10.152.0.0/24")
	for _, id := range []uint32{1, 2, 3, 5, 6, 7, 8, 9} {
		member, err := AddNode(ctx, db, mesh.cn.ID, proxy(id), false, now)
		require.NoError(t, err)
		mesh.members[id] = member
	}
	member, err := AddNode(ctx, db, mesh.eu.ID, proxy(4), false, now)
	require.NoError(t, err)
	mesh.members[4] = member
	mesh.phone, err = CreateDevice(ctx, db, mesh.cn.ID, DeviceInput{Hostname: "phone"}, now)
	require.NoError(t, err)
	issued, err := CreateEnrollment(ctx, db, EnrollmentInput{Hostname: "router", RegionID: mesh.cn.ID, Role: RoleFull}, nil, now)
	require.NoError(t, err)
	enrolled, err := Enroll(ctx, db, EnrollRequest{Features: meshFeatures, Token: issued.Token, DeviceKey: strings.Repeat("r", 32)}, now)
	require.NoError(t, err)
	require.NoError(t, db.First(&mesh.router, enrolled.DeviceID).Error)
	require.NotNil(t, mesh.router.Address)
	return mesh
}

func deviceID(row model.MeshDevice) string { return "device:" + strconv.FormatUint(uint64(row.ID), 10) }

func pathViews(t *testing.T, db *gorm.DB) []PathView {
	t.Helper()
	views, err := Paths(context.Background(), db)
	require.NoError(t, err)
	return views
}

func routingPaths(t *testing.T, network map[string]any) any {
	t.Helper()
	return network["routing"].(map[string]any)["paths"]
}

func TestPathValidation(t *testing.T) {
	mesh := newPathMesh(t)
	db, ctx := mesh.db, context.Background()
	_, err := CreateHostGroup(ctx, db, HostGroupInput{Name: "vms", Gateway: "proxy-3"}, now)
	require.NoError(t, err)
	_, err = CreateDevice(ctx, db, mesh.cn.ID, DeviceInput{Hostname: "laptop"}, now)
	require.NoError(t, err)

	via := func(ids ...string) []string { return ids }
	for name, input := range map[string]PathInput{
		"no waypoint":              {Source: "node:proxy-1", Target: "node:proxy-2", Via: nil},
		"blank waypoints":          {Source: "node:proxy-1", Target: "node:proxy-2", Via: via(" ", "")},
		"seven waypoints":          {Source: "node:proxy-1", Target: "node:proxy-2", Via: via("node:proxy-3", "node:proxy-5", "node:proxy-6", "node:proxy-7", "node:proxy-8", "node:proxy-9", deviceID(mesh.router))},
		"no source":                {Target: "node:proxy-2", Via: via("node:proxy-3")},
		"source without kind":      {Source: "proxy-1", Target: "node:proxy-2", Via: via("node:proxy-3")},
		"source not in the mesh":   {Source: "node:proxy-10", Target: "node:proxy-2", Via: via("node:proxy-3")},
		"access device as source":  {Source: deviceID(mesh.phone), Target: "node:proxy-2", Via: via("node:proxy-3")},
		"unknown device as source": {Source: "device:99", Target: "node:proxy-2", Via: via("node:proxy-3")},
		"target is the source":     {Source: "node:proxy-1", Target: "node:proxy-1", Via: via("node:proxy-3")},
		"target in another region": {Source: "node:proxy-1", Target: "node:proxy-4", Via: via("node:proxy-3")},
		"unknown target device":    {Source: "node:proxy-1", Target: "device:99", Via: via("node:proxy-3")},
		"target neither":           {Source: "node:proxy-1", Target: "somewhere", Via: via("node:proxy-3")},
		"public subnet":            {Source: "node:proxy-1", Target: "8.8.8.0/24", Via: via("node:proxy-3")},
		"subnet not masked":        {Source: "node:proxy-1", Target: "192.168.1.1/24", Via: via("node:proxy-3")},
		"IPv6 subnet":              {Source: "node:proxy-1", Target: "fd00::/64", Via: via("node:proxy-3")},
		"subnet in the region":     {Source: "node:proxy-1", Target: "10.150.0.0/25", Via: via("node:proxy-3")},
		"subnet over the region":   {Source: "node:proxy-1", Target: "10.0.0.0/8", Via: via("node:proxy-3")},
		"inside a host group":      {Source: "node:proxy-1", Target: "10.127.0.0/25", Via: via("node:proxy-2")},
		"access device waypoint":   {Source: "node:proxy-1", Target: "node:proxy-2", Via: via(deviceID(mesh.phone))},
		"waypoint in other region": {Source: "node:proxy-1", Target: "node:proxy-2", Via: via("node:proxy-4")},
		"waypoint is the source":   {Source: "node:proxy-1", Target: "node:proxy-2", Via: via("node:proxy-3", "node:proxy-1")},
		"waypoint is the target":   {Source: "node:proxy-1", Target: "node:proxy-2", Via: via("node:proxy-2")},
		"duplicate waypoints":      {Source: "node:proxy-1", Target: "node:proxy-2", Via: via("node:proxy-3", "node:proxy-3")},
		"unknown waypoint device":  {Source: "node:proxy-1", Target: "node:proxy-2", Via: via("device:99")},
		"malformed waypoint":       {Source: "node:proxy-1", Target: "node:proxy-2", Via: via("host_group:1")},
	} {
		_, err := CreatePath(ctx, db, input, now)
		require.ErrorIs(t, err, ErrInvalid, name)
	}
	var count int64
	require.NoError(t, db.Model(&model.MeshPath{}).Count(&count).Error)
	require.Zero(t, count)

	peer, err := CreatePath(ctx, db, PathInput{Source: " node:proxy-1 ", Target: "node:proxy-2", Via: via("node:proxy-3", deviceID(mesh.router))}, now)
	require.NoError(t, err)
	require.Equal(t, "node:proxy-1", peer.SourceRef)
	require.Equal(t, PathTargetPeer, peer.TargetKind)
	require.Equal(t, "node:proxy-3,"+deviceID(mesh.router), peer.Via)
	_, err = CreatePath(ctx, db, PathInput{Source: "node:proxy-1", Target: "node:proxy-2", Via: via("node:proxy-5")}, now)
	require.ErrorIs(t, err, ErrConflict, "one path per target")
	subnet, err := CreatePath(ctx, db, PathInput{Source: "node:proxy-1", Target: "192.168.5.0/24", Via: via("node:proxy-2", "node:proxy-3", "node:proxy-5", "node:proxy-6", "node:proxy-7", "node:proxy-8")}, now)
	require.NoError(t, err)
	require.Equal(t, PathTargetSubnet, subnet.TargetKind)
	// An access device may be a target; the whole host group segment too.
	_, err = CreatePath(ctx, db, PathInput{Source: "node:proxy-1", Target: deviceID(mesh.phone), Via: via("node:proxy-2")}, now)
	require.NoError(t, err)
	_, err = CreatePath(ctx, db, PathInput{Source: "node:proxy-1", Target: "10.127.0.0/24", Via: via("node:proxy-2")}, now)
	require.NoError(t, err)
	// A full node device is a source of its own.
	_, err = CreatePath(ctx, db, PathInput{Source: deviceID(mesh.router), Target: "node:proxy-1", Via: via("node:proxy-2")}, now)
	require.NoError(t, err)

	// An update changes the target and the waypoints, never the source,
	// and keeps the one path per target.
	_, err = UpdatePath(ctx, db, peer.ID, PathInput{Source: "node:proxy-2", Target: "node:proxy-2", Via: via("node:proxy-3")}, now)
	require.ErrorIs(t, err, ErrInvalid, "the source never changes")
	_, err = UpdatePath(ctx, db, peer.ID, PathInput{Target: "192.168.5.0/24", Via: via("node:proxy-3")}, now)
	require.ErrorIs(t, err, ErrConflict)
	_, err = UpdatePath(ctx, db, 999, PathInput{Target: "node:proxy-2", Via: via("node:proxy-3")}, now)
	require.ErrorIs(t, err, ErrNotFound)
	updated, err := UpdatePath(ctx, db, peer.ID, PathInput{Target: "node:proxy-2", Via: via("node:proxy-5")}, now.Add(time.Minute))
	require.NoError(t, err)
	require.Equal(t, "node:proxy-5", updated.Via)
	require.True(t, updated.CreatedAt.Equal(now))
	require.True(t, updated.UpdatedAt.Equal(now.Add(time.Minute)))

	require.NoError(t, DeletePath(ctx, db, subnet.ID))
	require.ErrorIs(t, DeletePath(ctx, db, subnet.ID), ErrNotFound)
}

func TestPathDelivery(t *testing.T) {
	mesh := newPathMesh(t)
	db, ctx := mesh.db, context.Background()
	_, err := CreatePath(ctx, db, PathInput{Source: "node:proxy-1", Target: "node:proxy-3", Via: []string{"node:proxy-2", deviceID(mesh.router)}}, now)
	require.NoError(t, err)
	_, err = CreatePath(ctx, db, PathInput{Source: "node:proxy-1", Target: "192.168.5.0/24", Via: []string{"node:proxy-5"}}, now)
	require.NoError(t, err)
	_, err = CreatePath(ctx, db, PathInput{Source: deviceID(mesh.router), Target: "node:proxy-2", Via: []string{"node:proxy-3"}}, now)
	require.NoError(t, err)

	// The source's network lists its paths with the members' addresses;
	// another member's lists none.
	network := nodeNetwork(t, db, proxy(1), "cn")
	require.Equal(t, []map[string]any{
		{"to_peer": mesh.address(3), "via": []string{mesh.address(2), *mesh.router.Address}},
		{"to_subnet": "192.168.5.0/24", "via": []string{mesh.address(5)}},
	}, routingPaths(t, network))
	require.Equal(t, "latency", network["routing"].(map[string]any)["policy"])
	require.Equal(t, []any{}, routingPaths(t, nodeNetwork(t, db, proxy(2), "cn")))

	// A device without an Agent is given paths when its kernel lists
	// mesh_paths.
	config, err := DeviceConfig(ctx, db, mesh.router, meshFeatures, now)
	require.NoError(t, err)
	require.Equal(t, []any{}, routingPaths(t, config["networks"].([]map[string]any)[0]))
	config, err = DeviceConfig(ctx, db, mesh.router, append(append([]string{}, meshFeatures...), FeaturePaths), now)
	require.NoError(t, err)
	require.Equal(t, []map[string]any{{"to_peer": mesh.address(2), "via": []string{mesh.address(3)}}}, routingPaths(t, config["networks"].([]map[string]any)[0]))

	// A waypoint that left the region: the path is not delivered, the
	// view warns.
	require.NoError(t, RemoveNode(ctx, db, mesh.members[5].ID))
	network = nodeNetwork(t, db, proxy(1), "cn")
	require.Equal(t, []map[string]any{{"to_peer": mesh.address(3), "via": []string{mesh.address(2), *mesh.router.Address}}}, routingPaths(t, network))
	warned := map[string][]string{}
	for _, view := range pathViews(t, db) {
		warned[view.Source+">"+view.Target] = view.Warnings
		if view.Target == "192.168.5.0/24" {
			require.Equal(t, "n5", view.Via[0].Name)
			require.Empty(t, view.Via[0].Address)
		}
	}
	require.Equal(t, []string{WarningPathMemberMissing}, warned["node:proxy-1>192.168.5.0/24"])
	require.Empty(t, warned["node:proxy-1>node:proxy-3"])

	// A device that lost its address does not resolve either.
	require.NoError(t, db.Model(&model.MeshDevice{}).Where("id = ?", mesh.router.ID).Update("address", gorm.Expr("NULL")).Error)
	require.Equal(t, []any{}, routingPaths(t, nodeNetwork(t, db, proxy(1), "cn")))
	require.NoError(t, db.Model(&model.MeshDevice{}).Where("id = ?", mesh.router.ID).Update("address", *mesh.router.Address).Error)

	// A removed node takes the paths it is the source of; paths that only
	// name it stay and warn.
	require.NoError(t, RemoveNode(ctx, db, mesh.members[3].ID))
	var left []model.MeshPath
	require.NoError(t, db.Order("id").Find(&left).Error)
	require.Len(t, left, 3)
	views := pathViews(t, db)
	for _, view := range views {
		if view.Source == deviceID(mesh.router) {
			require.Equal(t, []string{WarningPathMemberMissing, WarningPathUnsupported}, view.Warnings, "its waypoint left")
		}
	}
	require.NoError(t, RemoveNode(ctx, db, mesh.members[1].ID))
	require.NoError(t, db.Order("id").Find(&left).Error)
	require.Len(t, left, 1)
	require.Equal(t, deviceID(mesh.router), left[0].SourceRef)

	// A deleted device takes the paths it is the source of.
	require.NoError(t, DeleteDevice(ctx, db, mesh.router.ID, now))
	require.NoError(t, db.Find(&left).Error)
	require.Empty(t, left)
}

func TestPathsOfADeletedTargetDevice(t *testing.T) {
	mesh := newPathMesh(t)
	db, ctx := mesh.db, context.Background()
	_, err := CreatePath(ctx, db, PathInput{Source: "node:proxy-1", Target: deviceID(mesh.phone), Via: []string{"node:proxy-2"}}, now)
	require.NoError(t, err)
	require.Len(t, deliveredOf(t, db, "node:proxy-1", mesh.cn.ID), 1)
	require.NoError(t, DeleteDevice(ctx, db, mesh.phone.ID, now))
	require.Empty(t, deliveredOf(t, db, "node:proxy-1", mesh.cn.ID))
	views := pathViews(t, db)
	require.Len(t, views, 1)
	require.Equal(t, []string{WarningPathMemberMissing}, views[0].Warnings)
	require.Empty(t, views[0].TargetName)
}

func deliveredOf(t *testing.T, db *gorm.DB, source string, regionID uint) []map[string]any {
	t.Helper()
	entries, err := deliveredPaths(db, source, regionID)
	require.NoError(t, err)
	return entries
}

func TestPathStatusAndSupport(t *testing.T) {
	mesh := newPathMesh(t)
	db, ctx := mesh.db, context.Background()
	_, err := CreatePath(ctx, db, PathInput{Source: "node:proxy-1", Target: "node:proxy-3", Via: []string{"node:proxy-2"}}, now)
	require.NoError(t, err)
	_, err = CreatePath(ctx, db, PathInput{Source: "node:proxy-1", Target: "192.168.5.0/24", Via: []string{"node:proxy-2"}}, now)
	require.NoError(t, err)
	_, err = CreatePath(ctx, db, PathInput{Source: deviceID(mesh.router), Target: "node:proxy-2", Via: []string{"node:proxy-3"}}, now)
	require.NoError(t, err)
	for _, view := range pathViews(t, db) {
		require.Nil(t, view.Status)
	}

	report := `{"networks":[{"id":"eu","paths":[{"id":"peer:` + mesh.address(3) + `","state":"active"}]},` +
		`{"id":"cn","running":true,"paths":[` +
		`{"id":"peer:` + mesh.address(3) + `","state":"first_hop_only","reason":"unreachable","unreachable":"` + mesh.address(2) + `"},` +
		`{"id":"subnet:192.168.5.0/24","state":"active"}]}]}`
	require.NoError(t, RecordStatus(ctx, db, proxy(1), []byte(report), now, now))
	views := pathViews(t, db)
	require.Len(t, views, 3)
	require.NotNil(t, views[0].Status)
	require.Equal(t, "first_hop_only", views[0].Status.State)
	require.Equal(t, "unreachable", views[0].Status.Reason)
	require.Equal(t, mesh.address(2), views[0].Status.Unreachable)
	require.NotNil(t, views[1].Status)
	require.Equal(t, "active", views[1].Status.State)
	require.Equal(t, "cn", views[0].RegionCode)
	require.Equal(t, "a", views[0].SourceName)
	require.Equal(t, "c", views[0].TargetName)
	require.Equal(t, []PathMemberView{{Entry: "node:proxy-2", Name: "b", Address: mesh.address(2)}}, views[0].Via)
	require.Empty(t, views[0].Warnings)

	// A device without a report, or whose kernel does not list mesh_paths,
	// is not given its paths.
	require.Equal(t, []string{WarningPathUnsupported}, views[2].Warnings)
	save := func(features string) {
		require.NoError(t, db.Save(&model.MeshStatus{NodeRef: DeviceRef(mesh.router.ID), Payload: `{"features":` + features + `,"networks":[]}`, ObservedAt: now, ReceivedAt: now}).Error)
	}
	save(`["mesh","mesh_native_keys"]`)
	require.Equal(t, []string{WarningPathUnsupported}, pathViews(t, db)[2].Warnings)
	save(`["mesh","mesh_native_keys","mesh_paths"]`)
	require.Empty(t, pathViews(t, db)[2].Warnings)
}
