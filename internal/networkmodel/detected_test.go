package networkmodel

import (
	"context"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	networkv1 "github.com/AnixOps/anix-control/sdk/api/network/v1"
	"github.com/stretchr/testify/require"
)

// The node page offers the interfaces the Agent last reported, addresses
// split by family without their prefix length.
func TestNodeNetworkListsDetectedInterfaces(t *testing.T) {
	s := newService(t)
	seenAgents(t, s, 1)
	ctx := context.Background()

	view, err := s.NodeNetwork(ctx, "proxy-1")
	require.NoError(t, err)
	require.Empty(t, view.Detected)
	require.Nil(t, view.DetectedAt)

	node := agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: 1}
	require.NoError(t, s.RecordHello(ctx, node, &networkv1.DeviceCapabilities{DeviceRef: "proxy-1"}))
	at := time.Date(2026, 10, 30, 18, 0, 0, 0, time.UTC)
	require.NoError(t, s.RecordStatus(ctx, node, &networkv1.NetworkStatus{DeviceRef: "proxy-1", Interfaces: []*networkv1.InterfaceFact{
		{Name: "eth1", Addresses: []string{"10.0.0.5/24"}},
		{Name: "eth0", Addresses: []string{"203.0.113.7/24", "2001:db8::7/64"}, GatewayV4: "203.0.113.1", GatewayV6: "fe80::1"},
	}}, at))
	_, err = s.SetNodeSettings(ctx, "proxy-1", NodeSettings{Interface: "eth1", SourceV4: "10.0.0.5"}, Actor{})
	require.NoError(t, err)

	view, err = s.NodeNetwork(ctx, "proxy-1")
	require.NoError(t, err)
	require.Equal(t, "eth1", view.Interface)
	require.Equal(t, []DetectedInterface{
		{Name: "eth0", AddressesV4: []string{"203.0.113.7"}, AddressesV6: []string{"2001:db8::7"}, GatewayV4: "203.0.113.1", GatewayV6: "fe80::1"},
		{Name: "eth1", AddressesV4: []string{"10.0.0.5"}, AddressesV6: []string{}},
	}, view.Detected)
	require.NotNil(t, view.DetectedAt)
	require.True(t, at.Equal(*view.DetectedAt))
}
