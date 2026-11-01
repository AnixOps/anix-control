package grpc

import (
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/require"
)

func TestFirstEnabledProtocolSkipsDisabledOnes(t *testing.T) {
	protocols := []model.NodeProtocol{
		{Type: model.ProtocolVLESS, Enable: 0},
		{Type: model.ProtocolWireGuard, Enable: 1},
	}
	require.Equal(t, model.ProtocolWireGuard, firstEnabledProtocol(protocols).Type)
	require.Nil(t, firstEnabledProtocol(protocols[:1]))
}
