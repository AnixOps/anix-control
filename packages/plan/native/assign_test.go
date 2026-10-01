package native

import (
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/stretchr/testify/require"
)

func TestAssignRequestIDNamesOneRequest(t *testing.T) {
	expires := int64(1893456000)
	id := AssignRequestID(7, 2, &expires, "key")
	require.Equal(t, id, AssignRequestID(7, 2, &expires, "key"), "a retry names the same grant")
	require.True(t, strings.HasPrefix(id, "plan.assign:7:2:"), id)
	later := expires + 1
	for _, other := range []string{
		AssignRequestID(7, 2, &expires, "other"),
		AssignRequestID(7, 2, nil, "key"),
		AssignRequestID(7, 2, &later, "key"),
		AssignRequestID(7, 3, &expires, "key"),
		AssignRequestID(8, 2, &expires, "key"),
	} {
		require.NotEqual(t, id, other)
	}
	huge := AssignRequestID(4294967295, 4294967295, &expires, strings.Repeat("k", 8192))
	require.LessOrEqual(t, len(huge), 128, "the kernel takes request ids up to 128 bytes")
}

func TestRequestTokenPrefersTheIdempotencyKey(t *testing.T) {
	service := &Service{NewToken: func() string { return "fresh" }}
	request := func(headers map[string][]string) pluginhostsdk.NativeRequest {
		return pluginhostsdk.NativeRequest{Metadata: pluginhostsdk.RequestMetadata{Headers: headers}}
	}
	require.Equal(t, "key", service.requestToken(request(map[string][]string{"Idempotency-Key": {" key "}, "X-Request-Id": {"req"}})))
	require.Equal(t, "req", service.requestToken(request(map[string][]string{"X-Request-Id": {"req"}})))
	require.Equal(t, "req", service.requestToken(request(map[string][]string{"x-request-id": {"req"}})))
	require.Equal(t, "fresh", service.requestToken(request(nil)))
}

func TestSnapshotOfAPlan(t *testing.T) {
	speed, devices := int64(10), 1<<40
	got := snapshot(Plan{ID: 3, GroupID: 2, TransferEnable: 5, SpeedLimit: &speed, DeviceLimit: &devices})
	require.Equal(t, uint64(5<<30), got.GetTransferBytes())
	require.Equal(t, int64(10), got.GetSpeedLimitMbps())
	require.Equal(t, int32(1<<31-1), got.GetDeviceLimit())
	require.Empty(t, got.GetSubscriptionGroupIds(), "an assignment keeps the subscriber's groups")

	unlimited := snapshot(Plan{ID: 3})
	require.Nil(t, unlimited.SpeedLimitMbps, "absent limits are left unchanged")
	require.Nil(t, unlimited.DeviceLimit)
	require.Zero(t, snapshot(Plan{TransferEnable: -1}).GetTransferBytes())
	require.Equal(t, uint64(1<<62), snapshot(Plan{TransferEnable: 1 << 40}).GetTransferBytes())
}
