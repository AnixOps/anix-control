package packagebridgesdk

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestBridgeErrorKeepsResourceExhaustedForOversizedResponses(t *testing.T) {
	err := bridgeError(status.Error(codes.ResourceExhausted, "too large"))
	require.ErrorIs(t, err, ErrResponseTooLarge)
	require.Equal(t, codes.ResourceExhausted, status.Code(err))

	err = bridgeError(status.Error(codes.Internal, "failed"))
	require.ErrorIs(t, err, ErrBridgeUnavailable)
	require.NotErrorIs(t, err, ErrResponseTooLarge)
}
