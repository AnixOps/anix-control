package meshregion

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/require"
)

var laptopKey = strings.Repeat("k", 32)

func TestEnrollmentIsUsedOnce(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	cn := region(t, db, "cn", "10.150.0.0/16")
	_, err := AddNode(ctx, db, cn.ID, proxy(1), true, now)
	require.NoError(t, err)

	for _, input := range []EnrollmentInput{{Hostname: ""}, {Hostname: "x", Hours: 169}, {Hostname: "x", Hours: -1}} {
		_, err := CreateEnrollment(ctx, db, input, nil, now)
		require.ErrorIs(t, err, ErrInvalid, "%+v", input)
	}
	_, err = CreateEnrollment(ctx, db, EnrollmentInput{RegionID: 99, Hostname: "x"}, nil, now)
	require.ErrorIs(t, err, ErrNotFound)

	issued, err := CreateEnrollment(ctx, db, EnrollmentInput{RegionID: cn.ID, Hostname: "Laptop"}, nil, now)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(issued.Token, enrollmentPrefix))
	require.Equal(t, "laptop", issued.Hostname)
	require.True(t, issued.ExpiresAt.Equal(now.Add(DefaultEnrollmentTTL)))

	_, err = Enroll(ctx, db, EnrollRequest{Token: "mse_unknown", DeviceKey: laptopKey}, now)
	require.ErrorIs(t, err, ErrUnauthorized)
	_, err = Enroll(ctx, db, EnrollRequest{Token: issued.Token, DeviceKey: "short"}, now)
	require.ErrorIs(t, err, ErrInvalid)
	_, err = Enroll(ctx, db, EnrollRequest{Token: issued.Token, DeviceKey: laptopKey}, now.Add(25*time.Hour))
	require.ErrorIs(t, err, ErrUnauthorized, "expired")

	enrolled, err := Enroll(ctx, db, EnrollRequest{Token: issued.Token, DeviceKey: laptopKey}, now)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(enrolled.DeviceToken, deviceTokenPrefix))
	network := enrolled.Config["networks"].([]map[string]any)[0]
	require.Equal(t, "cn", network["id"])
	require.Equal(t, "access", network["role"])
	require.Equal(t, "10.150.128.0/16", network["address"], "the first pool address")
	require.Empty(t, network["listen"])
	require.Equal(t, []map[string]any{{"server": "a.example.com", "port": Port, "transport": "tcp"}}, network["peers"])
	_, err = Enroll(ctx, db, EnrollRequest{Token: issued.Token, DeviceKey: laptopKey}, now)
	require.ErrorIs(t, err, ErrUnauthorized, "used")

	rows, err := Enrollments(ctx, db)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, enrolled.DeviceID, *rows[0].DeviceID)
	require.NoError(t, DeleteEnrollment(ctx, db, rows[0].ID))
	require.ErrorIs(t, DeleteEnrollment(ctx, db, rows[0].ID), ErrNotFound)

	// The same key enrolled again keeps its device; the earlier token stops.
	again, err := CreateEnrollment(ctx, db, EnrollmentInput{Hostname: "laptop"}, nil, now)
	require.NoError(t, err)
	second, err := Enroll(ctx, db, EnrollRequest{Token: again.Token, DeviceKey: laptopKey}, now)
	require.NoError(t, err)
	require.Equal(t, enrolled.DeviceID, second.DeviceID)
	_, err = Authenticate(ctx, db, enrolled.DeviceToken, now)
	require.ErrorIs(t, err, ErrUnauthorized)
	device, err := Authenticate(ctx, db, second.DeviceToken, now.Add(time.Hour))
	require.NoError(t, err)
	require.True(t, device.AddressUsedAt.Equal(now.Add(time.Hour)), "a read uses the address")

	// The report and the token show in the device list; revoking refuses
	// the token.
	require.ErrorIs(t, RecordDeviceStatus(ctx, db, device, []byte(`[]`), now), ErrInvalid)
	require.NoError(t, RecordDeviceStatus(ctx, db, device, []byte(`{"networks":[]}`), now))
	views, err := Devices(ctx, db, 0, now)
	require.NoError(t, err)
	require.Len(t, views, 1)
	require.True(t, views[0].Enrolled)
	require.JSONEq(t, `{"networks":[]}`, string(views[0].Status))
	require.NoError(t, RevokeDevice(ctx, db, device.ID))
	require.ErrorIs(t, RevokeDevice(ctx, db, device.ID), ErrNotFound)
	_, err = Authenticate(ctx, db, second.DeviceToken, now)
	require.ErrorIs(t, err, ErrUnauthorized)
	views, err = Devices(ctx, db, 0, now)
	require.NoError(t, err)
	require.False(t, views[0].Enrolled)
}

func TestEnrolledDeviceChoosesByLatencyAndAsksToMove(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	cn := region(t, db, "cn", "10.150.0.0/16")
	region(t, db, "eu", "10.152.0.0/16")
	issued, err := CreateEnrollment(ctx, db, EnrollmentInput{Hostname: "laptop"}, nil, now)
	require.NoError(t, err)
	_, err = Enroll(ctx, db, EnrollRequest{Token: issued.Token, DeviceKey: laptopKey, Latencies: map[string]float64{"xx": 1}}, now)
	require.ErrorIs(t, err, ErrInvalid)
	enrolled, err := Enroll(ctx, db, EnrollRequest{Token: issued.Token, DeviceKey: laptopKey, Latencies: map[string]float64{"cn": 80, "eu": 30}}, now)
	require.NoError(t, err)
	require.Equal(t, "eu", enrolled.Config["networks"].([]map[string]any)[0]["id"])

	device, err := Authenticate(ctx, db, enrolled.DeviceToken, now)
	require.NoError(t, err)
	_, err = RequestDeviceChange(ctx, db, device, "na", "", now)
	require.ErrorIs(t, err, ErrNotFound)
	change, err := RequestDeviceChange(ctx, db, device, "cn", "moved office", now)
	require.NoError(t, err)
	require.Equal(t, model.MeshChangeMember, change.Source)
	_, err = ApproveChange(ctx, db, change.ID, "", nil, now)
	require.NoError(t, err)
	device, err = Authenticate(ctx, db, enrolled.DeviceToken, now)
	require.NoError(t, err)
	require.Equal(t, cn.ID, device.RegionID)
	config, err := DeviceConfig(ctx, db, device, now)
	require.NoError(t, err)
	require.Equal(t, "10.150.128.0/16", config["networks"].([]map[string]any)[0]["address"])

	// A disabled region answers no networks; deleting the device drops its
	// token.
	_, err = UpdateRegion(ctx, db, cn.ID, RegionInput{Code: "cn", Name: "cn", CIDR: cn.CIDR}, now)
	require.NoError(t, err)
	config, err = DeviceConfig(ctx, db, device, now)
	require.NoError(t, err)
	require.Empty(t, config["networks"])
	require.NoError(t, DeleteDevice(ctx, db, device.ID, now))
	_, err = Authenticate(ctx, db, enrolled.DeviceToken, now)
	require.ErrorIs(t, err, ErrUnauthorized)
}
