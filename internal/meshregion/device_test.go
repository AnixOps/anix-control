package meshregion

import (
	"context"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/require"
)

func TestEnrollmentIsUsedOnce(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	cn := region(t, db, "cn", "10.150.0.0/16")
	_, err := AddNode(ctx, db, cn.ID, proxy(1), true, now)
	require.NoError(t, err)

	for _, input := range []EnrollmentInput{
		{Kind: "phone", Name: "x"},
		{Kind: KindNode, Name: ""},
		{Kind: KindNode, Name: "x", Hours: 169},
		{Kind: KindNode, Name: "x", Hours: -1},
	} {
		_, err := CreateEnrollment(ctx, db, input, nil, now)
		require.ErrorIs(t, err, ErrInvalid, "%+v", input)
	}
	_, err = CreateEnrollment(ctx, db, EnrollmentInput{RegionID: 99, Kind: KindNode, Name: "x"}, nil, now)
	require.ErrorIs(t, err, ErrNotFound)

	issued, err := CreateEnrollment(ctx, db, EnrollmentInput{RegionID: cn.ID, Kind: KindNode, Name: "office"}, nil, now)
	require.NoError(t, err)
	require.Contains(t, issued.Token, enrollmentPrefix)
	require.True(t, issued.ExpiresAt.Equal(now.Add(DefaultEnrollmentTTL)))

	_, err = Enroll(ctx, db, EnrollRequest{Token: "mse_unknown", DeviceKey: "office-key-1"}, now)
	require.ErrorIs(t, err, ErrUnauthorized)
	_, err = Enroll(ctx, db, EnrollRequest{Token: issued.Token, DeviceKey: "short"}, now)
	require.ErrorIs(t, err, ErrInvalid)
	_, err = Enroll(ctx, db, EnrollRequest{Token: issued.Token, DeviceKey: "office-key-1"}, now.Add(25*time.Hour))
	require.ErrorIs(t, err, ErrUnauthorized, "expired")

	enrolled, err := Enroll(ctx, db, EnrollRequest{Token: issued.Token, DeviceKey: "office-key-1"}, now)
	require.NoError(t, err)
	require.Contains(t, enrolled.DeviceToken, deviceTokenPrefix)
	networks := enrolled.Config["networks"].([]map[string]any)
	require.Len(t, networks, 1)
	require.Equal(t, "full", networks[0]["role"])
	require.Equal(t, "10.150.0.2/16", networks[0]["address"], "a node device takes the server segment")
	require.Len(t, networks[0]["peers"], 1)

	_, err = Enroll(ctx, db, EnrollRequest{Token: issued.Token, DeviceKey: "office-key-1"}, now)
	require.ErrorIs(t, err, ErrUnauthorized, "used")
	rows, err := ListEnrollments(ctx, db)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, enrolled.DeviceID, *rows[0].DeviceID)
	require.NoError(t, DeleteEnrollment(ctx, db, rows[0].ID))
	require.ErrorIs(t, DeleteEnrollment(ctx, db, rows[0].ID), ErrNotFound)

	// The same device key enrolled again keeps its row and address.
	again, err := CreateEnrollment(ctx, db, EnrollmentInput{RegionID: cn.ID, Kind: KindNode, Name: "office"}, nil, now)
	require.NoError(t, err)
	second, err := Enroll(ctx, db, EnrollRequest{Token: again.Token, DeviceKey: "office-key-1", Name: "office 2"}, now)
	require.NoError(t, err)
	require.Equal(t, enrolled.DeviceID, second.DeviceID)
	require.Equal(t, "10.150.0.2/16", second.Config["networks"].([]map[string]any)[0]["address"])
	_, err = Authenticate(ctx, db, enrolled.DeviceToken, now)
	require.ErrorIs(t, err, ErrUnauthorized, "the earlier token is replaced")
	device, err := Authenticate(ctx, db, second.DeviceToken, now)
	require.NoError(t, err)
	require.Equal(t, "office 2", device.Name)
}

func TestPersonalDevicesChooseByLatencyAndMove(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	cn := region(t, db, "cn", "10.150.0.0/16")
	eu := region(t, db, "eu", "10.152.0.0/16")
	issued, err := CreateEnrollment(ctx, db, EnrollmentInput{Kind: KindPersonal, Name: "laptop"}, nil, now)
	require.NoError(t, err)
	_, err = Enroll(ctx, db, EnrollRequest{Token: issued.Token, DeviceKey: "laptop-key-1", Latencies: map[string]float64{"xx": 1}}, now)
	require.ErrorIs(t, err, ErrInvalid)
	enrolled, err := Enroll(ctx, db, EnrollRequest{Token: issued.Token, DeviceKey: "laptop-key-1", Latencies: map[string]float64{"cn": 80, "eu": 30}}, now)
	require.NoError(t, err)
	network := enrolled.Config["networks"].([]map[string]any)[0]
	require.Equal(t, "eu", network["id"])
	require.Equal(t, "access", network["role"])
	require.Equal(t, "10.152.128.0/16", network["address"], "a personal device takes the device pool")
	require.Empty(t, network["listen"])

	device, err := Authenticate(ctx, db, enrolled.DeviceToken, now)
	require.NoError(t, err)
	_, err = RequestRegionChange(ctx, db, device, "eu", "", now)
	require.ErrorIs(t, err, ErrConflict)
	_, err = RequestRegionChange(ctx, db, device, "na", "", now)
	require.ErrorIs(t, err, ErrNotFound)
	change, err := RequestRegionChange(ctx, db, device, "cn", "moved office", now)
	require.NoError(t, err)
	_, err = RequestRegionChange(ctx, db, device, "cn", "", now)
	require.ErrorIs(t, err, ErrConflict, "one pending request")

	// A deleted region with a device is refused; approving moves the device.
	require.ErrorIs(t, DeleteRegion(ctx, db, eu.ID), ErrConflict)
	decided, err := DecideRegionChange(ctx, db, change.ID, true, nil, now)
	require.NoError(t, err)
	require.Equal(t, "approved", decided.State)
	_, err = DecideRegionChange(ctx, db, change.ID, false, nil, now)
	require.ErrorIs(t, err, ErrNotFound)
	device, err = findDevice(db, device.ID)
	require.NoError(t, err)
	require.Equal(t, cn.ID, device.RegionID)
	require.Equal(t, "10.150.128.0", device.Address)

	regions, err := List(ctx, db)
	require.NoError(t, err)
	require.Len(t, regions[0].Devices, 1)
	require.NoError(t, RecordStatus(ctx, db, DeviceRef(device.ID), []byte(`{"networks":[]}`), now, now))
	regions, err = List(ctx, db)
	require.NoError(t, err)
	require.JSONEq(t, `{"networks":[]}`, string(regions[0].Devices[0].Status))

	require.NoError(t, RevokeDevice(ctx, db, device.ID, now))
	_, err = Authenticate(ctx, db, enrolled.DeviceToken, now)
	require.ErrorIs(t, err, ErrUnauthorized)
	regions, err = List(ctx, db)
	require.NoError(t, err)
	require.Empty(t, regions[0].Devices)
	require.NoError(t, DeleteRegion(ctx, db, eu.ID))
}

func TestStalePersonalDevicesGiveUpTheirAddress(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	office := region(t, db, "office", "192.168.9.0/24")
	old := now.Add(-ReclaimAfter - time.Hour)
	stale := model.MeshDevice{RegionID: office.ID, Kind: KindPersonal, Name: "old", DeviceKey: "old-device-key", TokenHash: "h1", Address: "192.168.9.128", LastSeenAt: &old, CreatedAt: old}
	require.NoError(t, db.Create(&stale).Error)
	issued, err := CreateEnrollment(ctx, db, EnrollmentInput{RegionID: office.ID, Kind: KindPersonal, Name: "new"}, nil, now)
	require.NoError(t, err)
	enrolled, err := Enroll(ctx, db, EnrollRequest{Token: issued.Token, DeviceKey: "new-device-key"}, now)
	require.NoError(t, err)
	require.Equal(t, "192.168.9.128/24", enrolled.Config["networks"].([]map[string]any)[0]["address"])

	// The stale device, back, gets the next free address.
	stale, err = findDevice(db, stale.ID)
	require.NoError(t, err)
	require.Empty(t, stale.Address)
	config, err := DeviceConfig(ctx, db, stale, now)
	require.NoError(t, err)
	require.Equal(t, "192.168.9.129/24", config["networks"].([]map[string]any)[0]["address"])

	// A disabled region answers no networks.
	_, err = UpdateRegion(ctx, db, office.ID, RegionInput{Code: "office", Name: "office", CIDR: office.CIDR}, now)
	require.NoError(t, err)
	config, err = DeviceConfig(ctx, db, stale, now)
	require.NoError(t, err)
	require.Empty(t, config["networks"])
}
