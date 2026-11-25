package meshregion

import (
	"context"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/require"
)

func TestSubnetRequests(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	cn := region(t, db, "cn", "10.150.0.0/16")
	access, err := CreateDevice(ctx, db, cn.ID, DeviceInput{Hostname: "phone"}, now)
	require.NoError(t, err)
	require.ErrorIs(t, RequestDeviceSubnets(ctx, db, access, []string{"192.168.1.0/24"}, now), ErrConflict, "an access device advertises nothing")

	routers := []model.MeshDevice{}
	for i, name := range []string{"router", "office"} {
		issued, err := CreateEnrollment(ctx, db, EnrollmentInput{Hostname: name, RegionID: cn.ID, Role: RoleFull}, nil, now)
		require.NoError(t, err)
		enrolled, err := Enroll(ctx, db, EnrollRequest{Features: meshFeatures, Token: issued.Token, DeviceKey: strings.Repeat(string(rune('a'+i)), 32)}, now)
		require.NoError(t, err)
		var row model.MeshDevice
		require.NoError(t, db.First(&row, enrolled.DeviceID).Error)
		routers = append(routers, row)
	}
	router, office := routers[0], routers[1]

	for _, bad := range [][]string{{"8.8.8.0/24"}, {"192.168.1.1/24"}, {"fd00::/64"}, {"x"}, distinctHosts(maxRequestedSubnets + 1)} {
		require.ErrorIs(t, RequestDeviceSubnets(ctx, db, router, bad, now), ErrInvalid, "%v", bad)
	}
	pending := func(device model.MeshDevice) *SubnetRequest {
		t.Helper()
		views, err := Devices(ctx, db, cn.ID, now)
		require.NoError(t, err)
		for _, view := range views {
			if view.ID == device.ID {
				return view.SubnetRequest
			}
		}
		t.Fatalf("no device %d", device.ID)
		return nil
	}
	_, err = RejectDeviceSubnets(ctx, db, router.ID, now)
	require.ErrorIs(t, err, ErrNotFound, "nothing pending")
	_, err = RejectDeviceSubnets(ctx, db, access.ID, now)
	require.ErrorIs(t, err, ErrNotFound)
	_, err = ApproveDeviceSubnets(ctx, db, 99, nil, now)
	require.ErrorIs(t, err, ErrNotFound)

	// A new request replaces the pending one; an empty one withdraws it.
	require.NoError(t, RequestDeviceSubnets(ctx, db, router, []string{"192.168.1.0/24"}, now))
	require.NoError(t, RequestDeviceSubnets(ctx, db, router, []string{"192.168.2.0/24", " 192.168.2.0/24", "172.16.0.0/16"}, now))
	request := pending(router)
	require.NotNil(t, request)
	require.Equal(t, []string{"192.168.2.0/24", "172.16.0.0/16"}, request.Subnets)
	require.True(t, request.RequestedAt.Equal(now))
	require.NoError(t, RequestDeviceSubnets(ctx, db, router, []string{}, now))
	require.Nil(t, pending(router))

	// Approval confirms what the administrator saw, with the usual checks;
	// rejection keeps the confirmed subnets.
	_, err = SetDeviceSubnets(ctx, db, router.ID, []string{"192.168.9.0/24"}, now)
	require.NoError(t, err)
	require.NoError(t, RequestDeviceSubnets(ctx, db, router, []string{"192.168.2.0/24"}, now))
	_, err = ApproveDeviceSubnets(ctx, db, router.ID, []string{"192.168.3.0/24"}, now)
	require.ErrorIs(t, err, ErrConflict, "the device changed its request")
	_, err = RejectDeviceSubnets(ctx, db, router.ID, now)
	require.NoError(t, err)
	require.Nil(t, pending(router))
	views, err := Devices(ctx, db, cn.ID, now)
	require.NoError(t, err)
	for _, view := range views {
		if view.ID == router.ID {
			require.Equal(t, []string{"192.168.9.0/24"}, view.Subnets)
		}
	}

	require.NoError(t, RequestDeviceSubnets(ctx, db, office, []string{"192.168.9.128/25"}, now))
	_, err = ApproveDeviceSubnets(ctx, db, office.ID, []string{"192.168.9.128/25"}, now)
	require.ErrorIs(t, err, ErrConflict, "the other full node advertises it")
	require.NotNil(t, pending(office), "a refused approval keeps the request")
	require.NoError(t, RequestDeviceSubnets(ctx, db, office, []string{"10.150.4.0/24"}, now))
	_, err = ApproveDeviceSubnets(ctx, db, office.ID, []string{"10.150.4.0/24"}, now)
	require.ErrorIs(t, err, ErrConflict, "inside a region")

	require.NoError(t, RequestDeviceSubnets(ctx, db, office, []string{"192.168.4.0/24", "172.17.0.0/16"}, now))
	subnets, err := ApproveDeviceSubnets(ctx, db, office.ID, []string{"192.168.4.0/24", "172.17.0.0/16"}, now)
	require.NoError(t, err)
	require.Equal(t, []string{"192.168.4.0/24", "172.17.0.0/16"}, subnets)
	require.Nil(t, pending(office))
	config, err := DeviceConfig(ctx, db, office, meshFeatures, now)
	require.NoError(t, err)
	require.Equal(t, []string{"192.168.4.0/24", "172.17.0.0/16"}, config["networks"].([]map[string]any)[0]["subnets"].(map[string]any)["advertise"])
}
