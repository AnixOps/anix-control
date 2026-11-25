package meshregion

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// testStaticKey is a static public key made of 32 bytes of b.
func testStaticKey(b byte) string {
	return base64.StdEncoding.EncodeToString([]byte(strings.Repeat(string(rune(b)), 32)))
}

func statusWithKey(code, key string) []byte {
	return fmt.Appendf(nil, `{"features":["mesh","mesh_native_keys"],"networks":[{"id":%q,"running":true,"static_key":%q,"peers":[]}]}`, code, key)
}

func nodeNetwork(t *testing.T, db *gorm.DB, node agentcontrol.AgentNode, code string) map[string]any {
	t.Helper()
	doc, err := NodeMember(db, node)
	require.NoError(t, err)
	for _, network := range doc["networks"].([]map[string]any) {
		if network["id"] == code {
			return network
		}
	}
	t.Fatalf("no network %s", code)
	return nil
}

func TestStaticKeyFormat(t *testing.T) {
	_, ok := staticKey(testStaticKey(1))
	require.True(t, ok)
	for _, bad := range []string{
		"",
		base64.StdEncoding.EncodeToString(make([]byte, 31)),
		base64.StdEncoding.EncodeToString(make([]byte, 33)),
		base64.RawStdEncoding.EncodeToString(make([]byte, 32)),
		base64.URLEncoding.EncodeToString([]byte(strings.Repeat("\xff", 32))),
		// The last character carries bits past the 32 bytes.
		strings.TrimSuffix(base64.StdEncoding.EncodeToString(make([]byte, 32)), "A=") + "B=",
	} {
		_, ok := staticKey(bad)
		require.False(t, ok, bad)
	}
}

func TestNativeKeysOfNodes(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	cn := region(t, db, "cn", "10.150.0.0/16")
	eu := region(t, db, "eu", "10.152.0.0/16")
	_, err := AddNode(ctx, db, cn.ID, proxy(1), true, now)
	require.NoError(t, err)
	_, err = AddNode(ctx, db, eu.ID, proxy(2), false, now)
	require.NoError(t, err)

	// Before any report a region network is managed and lists no keys.
	network := nodeNetwork(t, db, proxy(1), "cn")
	require.Equal(t, KindManaged, network["kind"])
	require.Equal(t, []string{}, network["native_keys"])

	var changed [][]agentcontrol.AgentNode
	cancel := OnChange(func(nodes []agentcontrol.AgentNode) { changed = append(changed, nodes) })
	defer cancel()

	// The boundary node is in both regions, with a key in each network.
	report := fmt.Appendf(nil, `{"networks":[{"id":"cn","static_key":%q},{"id":"eu","static_key":%q},{"id":"xx","static_key":%q}]}`,
		testStaticKey(1), testStaticKey(2), testStaticKey(3))
	require.NoError(t, RecordStatus(ctx, db, proxy(1), report, now, now))
	require.Equal(t, [][]agentcontrol.AgentNode{{proxy(2)}, {proxy(1)}}, changed,
		"new keys change the regions' configurations, the other members' first")
	require.Equal(t, []string{testStaticKey(1)}, nodeNetwork(t, db, proxy(1), "cn")["native_keys"])
	require.Equal(t, []string{testStaticKey(2)}, nodeNetwork(t, db, proxy(2), "eu")["native_keys"])

	// The same keys again change nothing; an invalid key is left out.
	require.NoError(t, RecordStatus(ctx, db, proxy(1), report, now, now))
	require.NoError(t, RecordStatus(ctx, db, proxy(2), statusWithKey("eu", "not-a-key"), now, now))
	require.Len(t, changed, 2)

	// The other node's key joins its region's list, sorted.
	require.NoError(t, RecordStatus(ctx, db, proxy(2), statusWithKey("eu", testStaticKey(0)), now, now))
	require.Equal(t, [][]agentcontrol.AgentNode{{proxy(1)}, {proxy(2)}}, changed[2:])
	require.Equal(t, []string{testStaticKey(0), testStaticKey(2)}, nodeNetwork(t, db, proxy(1), "eu")["native_keys"])

	// The Agent channel is trusted: a node's new key replaces its old one.
	require.NoError(t, RecordStatus(ctx, db, proxy(2), statusWithKey("eu", testStaticKey(4)), now, now))
	require.Len(t, changed, 6)
	require.Equal(t, []string{testStaticKey(2), testStaticKey(4)}, nodeNetwork(t, db, proxy(1), "eu")["native_keys"])

	// A node that leaves takes its keys off the lists.
	members, err := List(ctx, db)
	require.NoError(t, err)
	for _, r := range members {
		for _, member := range r.Members {
			if member.NodeRef == proxy(2).String() && member.Home {
				require.NoError(t, RemoveNode(ctx, db, member.ID))
			}
		}
	}
	require.Equal(t, []string{testStaticKey(2)}, nodeNetwork(t, db, proxy(1), "eu")["native_keys"])
}

func TestDeviceKeyBindingGuards(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	cn := region(t, db, "cn", "10.150.0.0/16")
	_, err := AddNode(ctx, db, cn.ID, proxy(1), true, now)
	require.NoError(t, err)
	require.NoError(t, RecordStatus(ctx, db, proxy(1), statusWithKey("cn", testStaticKey(1)), now, now))
	issued, err := CreateEnrollment(ctx, db, EnrollmentInput{RegionID: cn.ID, Hostname: "phone"}, nil, now)
	require.NoError(t, err)
	first, err := Enroll(ctx, db, EnrollRequest{Features: meshFeatures, Token: issued.Token, DeviceKey: laptopKey}, now)
	require.NoError(t, err)
	device, err := Authenticate(ctx, db, first.DeviceToken, now)
	require.NoError(t, err)

	// A member's key is not bound, and the list says why.
	require.NoError(t, RecordDeviceStatus(ctx, db, device, first.DeviceToken, statusWithKey("cn", testStaticKey(1)), now))
	views, err := Devices(ctx, db, cn.ID, now)
	require.NoError(t, err)
	require.Nil(t, views[0].StaticKey)
	require.Equal(t, []string{WarningStaticKeyTaken}, views[0].Warnings)

	// A retry of the enrollment binds a key while there is none, and
	// cannot change it afterwards.
	_, err = Enroll(ctx, db, EnrollRequest{Features: meshFeatures, Token: issued.Token, DeviceKey: laptopKey, StaticKey: testStaticKey(2)}, now.Add(time.Minute))
	require.NoError(t, err)
	_, err = Enroll(ctx, db, EnrollRequest{Features: meshFeatures, Token: issued.Token, DeviceKey: laptopKey, StaticKey: testStaticKey(2)}, now.Add(time.Minute))
	require.NoError(t, err, "the same key again")
	_, err = Enroll(ctx, db, EnrollRequest{Features: meshFeatures, Token: issued.Token, DeviceKey: laptopKey, StaticKey: testStaticKey(3)}, now.Add(time.Minute))
	require.ErrorIs(t, err, ErrConflict)
	require.Equal(t, []string{testStaticKey(1), testStaticKey(2)}, nodeNetwork(t, db, proxy(1), "cn")["native_keys"])

	// Enrolled again without a key: a report still on its way with the
	// earlier token binds nothing; one with the new token does.
	again, err := CreateEnrollment(ctx, db, EnrollmentInput{DeviceID: device.ID}, nil, now)
	require.NoError(t, err)
	second, err := Enroll(ctx, db, EnrollRequest{Features: meshFeatures, Token: again.Token, DeviceKey: laptopKey}, now)
	require.NoError(t, err)
	var row model.MeshDevice
	require.NoError(t, db.First(&row, device.ID).Error)
	require.Nil(t, row.StaticKey)
	require.NoError(t, RecordDeviceStatus(ctx, db, row, first.DeviceToken, statusWithKey("cn", testStaticKey(8)), now))
	require.NoError(t, db.First(&row, device.ID).Error)
	require.Nil(t, row.StaticKey, "the earlier token no longer binds")
	require.NoError(t, RecordDeviceStatus(ctx, db, row, second.DeviceToken, statusWithKey("cn", testStaticKey(9)), now))
	require.NoError(t, db.First(&row, device.ID).Error)
	require.Equal(t, testStaticKey(9), *row.StaticKey)

	// The Agent channel takes no key a device has either.
	require.NoError(t, RecordStatus(ctx, db, proxy(1), statusWithKey("cn", testStaticKey(9)), now, now))
	require.Equal(t, []string{testStaticKey(1), testStaticKey(9)}, nodeNetwork(t, db, proxy(1), "cn")["native_keys"])
	var member model.MeshMember
	require.NoError(t, db.Where("node_ref = ?", proxy(1).String()).First(&member).Error)
	require.Equal(t, testStaticKey(1), *member.StaticKey)
	regions, err := List(ctx, db)
	require.NoError(t, err)
	require.Equal(t, []string{WarningStaticKeyTaken}, regions[0].Members[0].Warnings, "the region list says why")
	// A repeat that predates the check shows too.
	require.NoError(t, db.Model(&model.MeshMember{}).Where("id = ?", member.ID).Update("static_key", testStaticKey(9)).Error)
	require.NoError(t, RecordStatus(ctx, db, proxy(1), statusWithKey("cn", testStaticKey(9)), now, now))
	regions, err = List(ctx, db)
	require.NoError(t, err)
	require.Equal(t, []string{WarningStaticKeyTaken}, regions[0].Members[0].Warnings)
	require.NoError(t, db.Model(&model.MeshMember{}).Where("id = ?", member.ID).Update("static_key", testStaticKey(1)).Error)

	// A revoked device releases its key for another device.
	require.NoError(t, RevokeDevice(ctx, db, device.ID))
	require.NoError(t, db.First(&row, device.ID).Error)
	require.Nil(t, row.StaticKey)
	taken, err := keyTaken(db, testStaticKey(9), 0)
	require.NoError(t, err)
	require.False(t, taken)

	// A deleted device leaves the list.
	require.NoError(t, DeleteDevice(ctx, db, device.ID, now))
	require.Equal(t, []string{testStaticKey(1)}, nodeNetwork(t, db, proxy(1), "cn")["native_keys"])
}

func TestNativeKeysOfDevices(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	cn := region(t, db, "cn", "10.150.0.0/16")
	eu := region(t, db, "eu", "10.152.0.0/16")
	_, err := AddNode(ctx, db, cn.ID, proxy(1), true, now)
	require.NoError(t, err)
	require.NoError(t, RecordStatus(ctx, db, proxy(1), statusWithKey("cn", testStaticKey(1)), now, now))

	issued, err := CreateEnrollment(ctx, db, EnrollmentInput{RegionID: cn.ID, Hostname: "phone"}, nil, now)
	require.NoError(t, err)
	_, err = Enroll(ctx, db, EnrollRequest{Features: meshFeatures, Token: issued.Token, DeviceKey: laptopKey, StaticKey: "short"}, now)
	require.ErrorIs(t, err, ErrInvalid)

	// Enrolled without a key: the device gets the node's key and is
	// marked until its first report names its own.
	enrolled, err := Enroll(ctx, db, EnrollRequest{Features: meshFeatures, Token: issued.Token, DeviceKey: laptopKey}, now)
	require.NoError(t, err)
	network := enrolled.Config["networks"].([]map[string]any)[0]
	require.Equal(t, KindManaged, network["kind"])
	require.Equal(t, []string{testStaticKey(1)}, network["native_keys"])
	views, err := Devices(ctx, db, cn.ID, now)
	require.NoError(t, err)
	require.Equal(t, []string{WarningNativeKeysUnsupported, WarningNoStaticKey}, views[0].Warnings, "until its first report names its capabilities")

	var changed int
	cancel := OnChange(func([]agentcontrol.AgentNode) { changed++ })
	defer cancel()
	device, err := Authenticate(ctx, db, enrolled.DeviceToken, now)
	require.NoError(t, err)
	// A key for another region's network does not bind.
	require.NoError(t, RecordDeviceStatus(ctx, db, device, enrolled.DeviceToken, statusWithKey("eu", testStaticKey(5)), now))
	require.Zero(t, changed)
	require.NoError(t, RecordDeviceStatus(ctx, db, device, enrolled.DeviceToken, statusWithKey("cn", testStaticKey(5)), now))
	require.Equal(t, 1, changed, "the region's nodes get the device's key")
	require.Equal(t, []string{testStaticKey(1), testStaticKey(5)}, nodeNetwork(t, db, proxy(1), "cn")["native_keys"])

	// A later report cannot change it; the device list shows the mismatch.
	device, err = Authenticate(ctx, db, enrolled.DeviceToken, now)
	require.NoError(t, err)
	require.NoError(t, RecordDeviceStatus(ctx, db, device, enrolled.DeviceToken, statusWithKey("cn", testStaticKey(6)), now))
	require.Equal(t, 1, changed)
	require.Equal(t, []string{testStaticKey(1), testStaticKey(5)}, nodeNetwork(t, db, proxy(1), "cn")["native_keys"])
	views, err = Devices(ctx, db, cn.ID, now)
	require.NoError(t, err)
	require.Equal(t, []string{WarningStaticKeyMismatch}, views[0].Warnings)

	// Enrolling again with a credential for it is how the key changes.
	again, err := CreateEnrollment(ctx, db, EnrollmentInput{DeviceID: device.ID}, nil, now)
	require.NoError(t, err)
	second, err := Enroll(ctx, db, EnrollRequest{Features: meshFeatures, Token: again.Token, DeviceKey: laptopKey, StaticKey: testStaticKey(6)}, now)
	require.NoError(t, err)
	require.Equal(t, []string{testStaticKey(1), testStaticKey(6)}, nodeNetwork(t, db, proxy(1), "cn")["native_keys"])

	// A move to another region drops the key of the old network; the
	// first report from the new one names the new key.
	device, err = Authenticate(ctx, db, second.DeviceToken, now)
	require.NoError(t, err)
	_, err = MoveDevice(ctx, db, device.ID, eu.ID, nil, now)
	require.NoError(t, err)
	require.Equal(t, []string{testStaticKey(1)}, nodeNetwork(t, db, proxy(1), "cn")["native_keys"])
	var row model.MeshDevice
	require.NoError(t, db.First(&row, device.ID).Error)
	require.Nil(t, row.StaticKey)
	require.NoError(t, RecordDeviceStatus(ctx, db, row, second.DeviceToken, statusWithKey("eu", testStaticKey(7)), now))
	config, err := DeviceConfig(ctx, db, row, nil, now)
	require.NoError(t, err)
	require.Equal(t, []string{testStaticKey(7)}, config["networks"].([]map[string]any)[0]["native_keys"])

	// A revoked device leaves the list: our nodes refuse it at once.
	require.NoError(t, RevokeDevice(ctx, db, device.ID))
	config, err = DeviceConfig(ctx, db, row, nil, now)
	require.NoError(t, err)
	require.Equal(t, []string{}, config["networks"].([]map[string]any)[0]["native_keys"])
}

func TestDeviceConfigFollowsTheDeviceFeatures(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	cn := region(t, db, "cn", "10.150.0.0/24")
	issued, err := CreateEnrollment(ctx, db, EnrollmentInput{Hostname: "laptop", RegionID: cn.ID}, nil, now)
	require.NoError(t, err)
	enrolled, err := Enroll(ctx, db, EnrollRequest{Token: issued.Token, DeviceKey: laptopKey, StaticKey: testStaticKey(3)}, now)
	require.NoError(t, err)
	device, err := Authenticate(ctx, db, enrolled.DeviceToken, now)
	require.NoError(t, err)
	networks := func(features []string) []map[string]any {
		config, err := DeviceConfig(ctx, db, device, features, now)
		require.NoError(t, err)
		list, _ := config["networks"].([]map[string]any)
		return list
	}
	warnings := func() []string {
		views, err := Devices(ctx, db, cn.ID, now)
		require.NoError(t, err)
		require.Len(t, views, 1)
		return views[0].Warnings
	}

	// A region network admits only the keys it lists: a kernel that has
	// not named mesh_native_keys gets none, before its first report too.
	require.Empty(t, enrolled.Config["networks"])
	require.Empty(t, networks(nil))
	require.Empty(t, networks([]string{"mesh"}))
	require.Contains(t, warnings(), WarningNativeKeysUnsupported)

	// Named in the request, or else in the latest report.
	current := networks(meshFeatures)[0]
	require.Equal(t, KindManaged, current["kind"])
	require.Equal(t, []string{testStaticKey(3)}, current["native_keys"])
	require.NotContains(t, current["dns"], "resolver_address")
	require.Equal(t, true, current["dns"].(map[string]any)["enabled"])

	report := func(features string) {
		payload := fmt.Appendf(nil, `{"features":%s,"networks":[{"id":"cn","running":true,"static_key":%q,"peers":[]}]}`, features, testStaticKey(3))
		require.NoError(t, RecordDeviceStatus(ctx, db, device, enrolled.DeviceToken, payload, now))
	}
	report(`["mesh","mesh_native_keys"]`)
	require.Equal(t, KindManaged, networks(nil)[0]["kind"])
	require.NotContains(t, networks(nil)[0], "dns")
	require.NotContains(t, warnings(), WarningNativeKeysUnsupported)
	require.Empty(t, networks([]string{}), "the request's features come first")

	report(`["mesh","mesh_native_keys","mesh_resolver_address"]`)
	require.NotContains(t, networks(nil)[0], "dns", "no dns section without mesh_dns")
	report(`["mesh","mesh_native_keys","mesh_dns","mesh_resolver_address"]`)
	require.Equal(t, "10.150.0.254", networks(nil)[0]["dns"].(map[string]any)["resolver_address"])

	report(`"not a list"`)
	require.Empty(t, networks(nil), "a malformed list names nothing")
	require.Contains(t, warnings(), WarningNativeKeysUnsupported)
}

func TestDeviceFeaturesFollowTheLatestReport(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	cn := region(t, db, "cn", "10.150.0.0/24")
	issued, err := CreateEnrollment(ctx, db, EnrollmentInput{Hostname: "laptop", RegionID: cn.ID}, nil, now)
	require.NoError(t, err)
	tooMany := make([]string, MaxFeatures+1)
	_, err = Enroll(ctx, db, EnrollRequest{Features: tooMany, Token: issued.Token, DeviceKey: laptopKey}, now)
	require.ErrorIs(t, err, ErrInvalid)
	// The enroll request's features answer only that request.
	enrolled, err := Enroll(ctx, db, EnrollRequest{Features: []string{"mesh", FeatureNativeKeys, FeatureResolverAddress}, Token: issued.Token, DeviceKey: laptopKey, StaticKey: testStaticKey(3)}, now)
	require.NoError(t, err)
	require.Len(t, enrolled.Config["networks"], 1)
	device, err := Authenticate(ctx, db, enrolled.DeviceToken, now)
	require.NoError(t, err)

	// The kernel went back to a release without the admission list: its
	// report, the latest, rules over the enrollment's list.
	require.NoError(t, RecordDeviceStatus(ctx, db, device, enrolled.DeviceToken, []byte(`{"features":["mesh"],"networks":[]}`), now))
	config, err := DeviceConfig(ctx, db, device, nil, now)
	require.NoError(t, err)
	require.Empty(t, config["networks"])
	// A read naming its features rules over the report.
	config, err = DeviceConfig(ctx, db, device, meshFeatures, now)
	require.NoError(t, err)
	require.Len(t, config["networks"], 1)
}
