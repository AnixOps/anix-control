package networkmodel

import (
	"context"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// A device's network settings are stored, audited and shape its plan.
func TestSetNodeSettingsReplans(t *testing.T) {
	s := newService(t)
	seenAgents(t, s, 1, 2)
	ctx := context.Background()
	s.Forwards = func(*gorm.DB) ([]Forward, error) { return []Forward{autoForward()}, nil }
	_, err := s.Replace(ctx, &Document{}, nil, Actor{})
	require.NoError(t, err)

	empty, err := s.NodeSettings(ctx, "proxy-1")
	require.NoError(t, err)
	require.Zero(t, empty)

	saved, err := s.SetNodeSettings(ctx, "proxy-1", NodeSettings{Interface: " eth1 ", SourceV4: "10.0.0.7", GatewayV4: "10.0.0.1"}, Actor{Name: "admin"})
	require.NoError(t, err)
	require.Equal(t, "eth1", saved.Interface)
	got, err := s.NodeSettings(ctx, "proxy-1")
	require.NoError(t, err)
	require.Equal(t, saved, got)
	var audits int64
	require.NoError(t, s.DB.Model(&model.OperationLog{}).Where("action = ?", AuditActionNodeSettings).Count(&audits).Error)
	require.Equal(t, int64(1), audits)

	plan, found, err := s.DevicePlan(ctx, "proxy-1")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "eth1", plan.GetLines()[0].GetInterface())

	_, err = s.SetNodeSettings(ctx, "proxy-9", NodeSettings{}, Actor{})
	require.ErrorIs(t, err, ErrUnknownNode)
	_, err = s.NodeSettings(ctx, "forward-1")
	require.ErrorIs(t, err, ErrUnknownNode)
}

func TestCheckSettings(t *testing.T) {
	for name, settings := range map[string]NodeSettings{
		"bad interface":       {Interface: "eth 1"},
		"v6 as v4":            {Interface: "eth0", SourceV4: "2001:db8::1"},
		"source without nic":  {SourceV4: "10.0.0.7"},
		"gateway sans source": {Interface: "eth0", GatewayV6: "2001:db8::1"},
		"port":                {HopPort: 70000},
	} {
		_, err := checkSettings(settings)
		var invalid *Invalid
		require.ErrorAs(t, err, &invalid, name)
	}
	ok, err := checkSettings(NodeSettings{Interface: "ens18", SourceV6: "2001:db8::7", GatewayV6: "fe80::1", HopPort: 30001})
	require.NoError(t, err)
	require.Equal(t, "2001:db8::7", ok.SourceV6)
}
