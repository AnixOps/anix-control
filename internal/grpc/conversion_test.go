package grpc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntToInt32RejectsOverflow(t *testing.T) {
	got, err := intToInt32("node port", int(maxInt32Value))
	require.NoError(t, err)
	assert.Equal(t, int32(maxInt32Value), got)

	_, err = int64ToInt32("node port", maxInt32Value+1)
	assert.Error(t, err)

	_, err = int64ToInt32("node port", minInt32Value-1)
	assert.Error(t, err)
}
