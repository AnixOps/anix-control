package meshregion

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/require"
)

var probeFeatures = append([]string{FeatureRegionProbe, FeatureRegionRequestState}, meshFeatures...)

func regionOf(t *testing.T, config map[string]any) map[string]any {
	t.Helper()
	// What a device or an Agent reads is the JSON of the answer.
	raw, err := json.Marshal(config["region"])
	require.NoError(t, err)
	var object map[string]any
	require.NoError(t, json.Unmarshal(raw, &object))
	return object
}

func TestDeviceConfigRegionObjectFollowsTheKernelsFeatures(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	cn := region(t, db, "cn", "10.150.0.0/16")
	eu := region(t, db, "eu", "10.152.0.0/16")
	_, err := AddNode(ctx, db, eu.ID, proxy(1), true, now)
	require.NoError(t, err)
	setAddress(t, db, 1, "203.0.113.7")
	reportProbe(t, db, "proxy-1", probeKeyOf(1))

	enroll := func(host, key string, input EnrollmentInput, features []string, latencies map[string]float64) (Enrolled, map[string]any) {
		input.Hostname = host
		issued, err := CreateEnrollment(ctx, db, input, nil, now)
		require.NoError(t, err)
		enrolled, err := Enroll(ctx, db, EnrollRequest{Features: features, Token: issued.Token, DeviceKey: key + "-device-key-padding", Latencies: latencies}, now)
		require.NoError(t, err)
		return enrolled, enrolled.Config
	}
	// Chosen by latency; the enrollment's features decide the fields.
	enrolled, config := enroll("one", "one", EnrollmentInput{}, probeFeatures, map[string]float64{"cn": 50, "eu": 10})
	object := regionOf(t, config)
	require.Equal(t, "eu", object["current"])
	require.Equal(t, "latency", object["basis"])
	require.Nil(t, object["responder"])
	directory := object["directory"].(map[string]any)
	require.EqualValues(t, 1, directory["version"])
	require.Len(t, directory["regions"], 1)
	network := config["networks"].([]map[string]any)[0]
	require.Contains(t, network, "region_request")
	require.Nil(t, network["region_request"])

	// An old kernel gets neither the region object nor the request state.
	_, config = enroll("two", "two", EnrollmentInput{}, meshFeatures, map[string]float64{"cn": 50, "eu": 10})
	require.NotContains(t, config, "region")
	require.NotContains(t, config["networks"].([]map[string]any)[0], "region_request")
	_, config = enroll("three", "three", EnrollmentInput{}, append([]string{FeatureRegionProbe}, meshFeatures...), map[string]float64{"cn": 50, "eu": 10})
	require.Contains(t, config, "region")
	require.NotContains(t, config["networks"].([]map[string]any)[0], "region_request")
	_, config = enroll("four", "four", EnrollmentInput{}, append([]string{FeatureRegionRequestState}, meshFeatures...), map[string]float64{"cn": 50, "eu": 10})
	require.NotContains(t, config, "region")
	require.Contains(t, config["networks"].([]map[string]any)[0], "region_request")

	// A credential that fixes the region.
	_, config = enroll("five", "five", EnrollmentInput{RegionID: cn.ID}, probeFeatures, nil)
	object = regionOf(t, config)
	require.Equal(t, "cn", object["current"])
	require.Equal(t, "credential", object["basis"])

	// A request: pending, then rejected until it is replaced; approved:
	// null and the basis is migration.
	device, err := Authenticate(ctx, db, enrolled.DeviceToken, now)
	require.NoError(t, err)
	asked, err := RequestDeviceChange(ctx, db, device, "cn", "closer", now)
	require.NoError(t, err)
	config, err = DeviceConfig(ctx, db, device, probeFeatures, now)
	require.NoError(t, err)
	request := config["networks"].([]map[string]any)[0]["region_request"].(map[string]any)
	require.Equal(t, map[string]any{"id": strconv.Itoa(int(asked.ID)), "region": "cn", "state": "pending", "reason": "closer", "reject_reason": ""}, request)
	_, err = RejectChange(ctx, db, asked.ID, "not yet", nil, now)
	require.NoError(t, err)
	config, err = DeviceConfig(ctx, db, device, probeFeatures, now)
	require.NoError(t, err)
	request = config["networks"].([]map[string]any)[0]["region_request"].(map[string]any)
	require.Equal(t, "rejected", request["state"])
	require.Equal(t, "not yet", request["reject_reason"])
	asked, err = RequestDeviceChange(ctx, db, device, "cn", "again", now)
	require.NoError(t, err)
	_, err = ApproveChange(ctx, db, asked.ID, "", nil, now)
	require.NoError(t, err)
	device, err = Authenticate(ctx, db, enrolled.DeviceToken, now)
	require.NoError(t, err)
	config, err = DeviceConfig(ctx, db, device, probeFeatures, now)
	require.NoError(t, err)
	require.Nil(t, config["networks"].([]map[string]any)[0]["region_request"])
	require.Equal(t, "migration", regionOf(t, config)["basis"])
	require.Equal(t, "cn", regionOf(t, config)["current"])

	// An administrator's move.
	_, err = MoveDevice(ctx, db, device.ID, eu.ID, nil, now)
	require.NoError(t, err)
	device, err = Authenticate(ctx, db, enrolled.DeviceToken, now)
	require.NoError(t, err)
	config, err = DeviceConfig(ctx, db, device, probeFeatures, now)
	require.NoError(t, err)
	require.Equal(t, "admin", regionOf(t, config)["basis"])

	// A disabled region delivers no network, so no current region.
	_, err = UpdateRegion(ctx, db, eu.ID, RegionInput{Code: "eu", Name: "eu", CIDR: eu.CIDR}, now)
	require.NoError(t, err)
	config, err = DeviceConfig(ctx, db, device, probeFeatures, now)
	require.NoError(t, err)
	object = regionOf(t, config)
	require.Nil(t, object["current"])
	require.Nil(t, object["basis"])
}

func TestAgentConfigCarriesTheRegionObject(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	cn := region(t, db, "cn", "10.150.0.0/16")
	eu := region(t, db, "eu", "10.152.0.0/16")
	_, err := AddNode(ctx, db, cn.ID, proxy(1), true, now)
	require.NoError(t, err)
	_, err = AddNode(ctx, db, cn.ID, proxy(2), false, now)
	require.NoError(t, err)
	setAddress(t, db, 1, "203.0.113.7")
	reportProbe(t, db, "proxy-1", probeKeyOf(1))

	border, err := NodeMember(db, proxy(1))
	require.NoError(t, err)
	object := regionOf(t, border)
	require.Equal(t, "cn", object["current"])
	require.Equal(t, "admin", object["basis"])
	require.Equal(t, map[string]any{"listen": []any{"0.0.0.0:11013", "[::]:11013"}}, object["responder"])
	require.Len(t, border["networks"], 2, "a border node has a network in every region; the region is its home")
	require.Len(t, object["directory"].(map[string]any)["regions"], 1)

	plain, err := NodeMember(db, proxy(2))
	require.NoError(t, err)
	object = regionOf(t, plain)
	require.Equal(t, "cn", object["current"])
	require.Nil(t, object["responder"], "only a border node probes")
	require.Nil(t, plain["networks"].([]map[string]any)[0]["region_request"])

	// A request of the node shows on its home network; its approval is the
	// basis migration.
	asked, err := RequestChange(ctx, db, ChangeInput{Node: "proxy-2", ToRegionID: eu.ID, Reason: "closer"}, model.MeshChangeMember, nil, now)
	require.NoError(t, err)
	plain, err = NodeMember(db, proxy(2))
	require.NoError(t, err)
	require.Equal(t, "pending", plain["networks"].([]map[string]any)[0]["region_request"].(map[string]any)["state"])
	_, err = ApproveChange(ctx, db, asked.ID, "", nil, now)
	require.NoError(t, err)
	plain, err = NodeMember(db, proxy(2))
	require.NoError(t, err)
	object = regionOf(t, plain)
	require.Equal(t, "eu", object["current"])
	require.Equal(t, "migration", object["basis"])
	require.Nil(t, plain["networks"].([]map[string]any)[0]["region_request"])

	// The administrator's move from the member list is admin.
	var home model.MeshMember
	require.NoError(t, db.Where("node_ref = ? AND home = ?", "proxy-2", true).First(&home).Error)
	_, err = UpdateMember(ctx, db, home.ID, cn.ID, false, nil, now)
	require.NoError(t, err)
	plain, err = NodeMember(db, proxy(2))
	require.NoError(t, err)
	require.Equal(t, "admin", regionOf(t, plain)["basis"])
}
