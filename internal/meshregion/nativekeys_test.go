package meshregion

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"

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
	return fmt.Appendf(nil, `{"networks":[{"id":%q,"running":true,"static_key":%q,"peers":[]}]}`, code, key)
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
	require.Len(t, changed, 1, "new keys change the regions' configurations")
	require.Equal(t, []string{testStaticKey(1)}, nodeNetwork(t, db, proxy(1), "cn")["native_keys"])
	require.Equal(t, []string{testStaticKey(2)}, nodeNetwork(t, db, proxy(2), "eu")["native_keys"])

	// The same keys again change nothing; an invalid key is left out.
	require.NoError(t, RecordStatus(ctx, db, proxy(1), report, now, now))
	require.NoError(t, RecordStatus(ctx, db, proxy(2), statusWithKey("eu", "not-a-key"), now, now))
	require.Len(t, changed, 1)

	// The other node's key joins its region's list, sorted.
	require.NoError(t, RecordStatus(ctx, db, proxy(2), statusWithKey("eu", testStaticKey(0)), now, now))
	require.Len(t, changed, 2)
	require.Equal(t, []string{testStaticKey(0), testStaticKey(2)}, nodeNetwork(t, db, proxy(1), "eu")["native_keys"])

	// The Agent channel is trusted: a node's new key replaces its old one.
	require.NoError(t, RecordStatus(ctx, db, proxy(2), statusWithKey("eu", testStaticKey(4)), now, now))
	require.Len(t, changed, 3)
	require.Equal(t, []string{testStaticKey(2), testStaticKey(4)}, nodeNetwork(t, db, proxy(1), "eu")["native_keys"])
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
	_, err = Enroll(ctx, db, EnrollRequest{Token: issued.Token, DeviceKey: laptopKey, StaticKey: "short"}, now)
	require.ErrorIs(t, err, ErrInvalid)

	// Enrolled without a key: the device gets the node's key and is
	// marked until its first report names its own.
	enrolled, err := Enroll(ctx, db, EnrollRequest{Token: issued.Token, DeviceKey: laptopKey}, now)
	require.NoError(t, err)
	network := enrolled.Config["networks"].([]map[string]any)[0]
	require.Equal(t, KindManaged, network["kind"])
	require.Equal(t, []string{testStaticKey(1)}, network["native_keys"])
	views, err := Devices(ctx, db, cn.ID, now)
	require.NoError(t, err)
	require.Equal(t, []string{WarningNoStaticKey}, views[0].Warnings)

	var changed int
	cancel := OnChange(func([]agentcontrol.AgentNode) { changed++ })
	defer cancel()
	device, err := Authenticate(ctx, db, enrolled.DeviceToken, now)
	require.NoError(t, err)
	// A key for another region's network does not bind.
	require.NoError(t, RecordDeviceStatus(ctx, db, device, statusWithKey("eu", testStaticKey(5)), now))
	require.Zero(t, changed)
	require.NoError(t, RecordDeviceStatus(ctx, db, device, statusWithKey("cn", testStaticKey(5)), now))
	require.Equal(t, 1, changed, "the region's nodes get the device's key")
	require.Equal(t, []string{testStaticKey(1), testStaticKey(5)}, nodeNetwork(t, db, proxy(1), "cn")["native_keys"])

	// A later report cannot change it; the device list shows the mismatch.
	device, err = Authenticate(ctx, db, enrolled.DeviceToken, now)
	require.NoError(t, err)
	require.NoError(t, RecordDeviceStatus(ctx, db, device, statusWithKey("cn", testStaticKey(6)), now))
	require.Equal(t, 1, changed)
	require.Equal(t, []string{testStaticKey(1), testStaticKey(5)}, nodeNetwork(t, db, proxy(1), "cn")["native_keys"])
	views, err = Devices(ctx, db, cn.ID, now)
	require.NoError(t, err)
	require.Equal(t, []string{WarningStaticKeyMismatch}, views[0].Warnings)

	// Enrolling again with a credential for it is how the key changes.
	again, err := CreateEnrollment(ctx, db, EnrollmentInput{DeviceID: device.ID}, nil, now)
	require.NoError(t, err)
	second, err := Enroll(ctx, db, EnrollRequest{Token: again.Token, DeviceKey: laptopKey, StaticKey: testStaticKey(6)}, now)
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
	require.NoError(t, RecordDeviceStatus(ctx, db, row, statusWithKey("eu", testStaticKey(7)), now))
	config, err := DeviceConfig(ctx, db, row, now)
	require.NoError(t, err)
	require.Equal(t, []string{testStaticKey(7)}, config["networks"].([]map[string]any)[0]["native_keys"])

	// A revoked device leaves the list: our nodes refuse it at once.
	require.NoError(t, RevokeDevice(ctx, db, device.ID))
	config, err = DeviceConfig(ctx, db, row, now)
	require.NoError(t, err)
	require.Equal(t, []string{}, config["networks"].([]map[string]any)[0]["native_keys"])
}
