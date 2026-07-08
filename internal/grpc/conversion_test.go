package grpc

import (
	"testing"

	"github.com/anixops/v2board/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUintToUint32RejectsOverflow(t *testing.T) {
	got, err := uintToUint32("node id", uint(maxUint32Value))
	require.NoError(t, err)
	assert.Equal(t, uint32(maxUint32Value), got)

	_, err = uintToUint32("node id", uint(maxUint32Value+1))
	assert.Error(t, err)
}

func TestIntToInt32RejectsOverflow(t *testing.T) {
	got, err := intToInt32("node port", int(maxInt32Value))
	require.NoError(t, err)
	assert.Equal(t, int32(maxInt32Value), got)

	_, err = int64ToInt32("node port", maxInt32Value+1)
	assert.Error(t, err)

	_, err = int64ToInt32("node port", minInt32Value-1)
	assert.Error(t, err)
}

func TestAnyToInt32RejectsInvalidNumbers(t *testing.T) {
	got, ok, err := anyToInt32("server_port", 8443)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, int32(8443), got)

	_, ok, err = anyToInt32("server_port", float64(maxInt32Value)+1)
	assert.Error(t, err)
	assert.False(t, ok)

	_, ok, err = anyToInt32("server_port", "8443")
	assert.NoError(t, err)
	assert.False(t, ok)
}

func TestUserInfoFromModelRejectsOutOfRangeFields(t *testing.T) {
	deviceLimit := int(maxInt32Value) + 1
	user := &model.User{
		ID:          1,
		UUID:        "user-uuid",
		DeviceLimit: &deviceLimit,
	}

	_, err := userInfoFromModel(user)
	assert.Error(t, err)
}
