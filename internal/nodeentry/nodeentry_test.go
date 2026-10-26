package nodeentry

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newService(t *testing.T) *Service {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "entries.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&model.Node{}, &model.NodeEntry{}, &model.OperationLog{}))
	require.NoError(t, db.Create(&model.Node{ID: 60, Name: "sh relay", Host: "relay.example.com", APIKey: "k60"}).Error)
	return New(db)
}

func shanghai() []Input {
	off := false
	return []Input{
		{Label: "CN2 v6", Kind: model.NodeEntryDirect, Host: "[2001:DB8::10]"},
		{Label: "9929 v6", Kind: model.NodeEntryDirect, Host: "2001:db8:1::20"},
		{Label: "CN2 NAT", Kind: model.NodeEntryNAT, Host: "198.51.100.7", PortMap: []PortRange{{From: 30000, To: 30099, External: 21000}, {From: 443, To: 443, External: 20443}}},
		{Label: "9929 NAT", Kind: model.NodeEntryNAT, Host: "NAT.Example.com", PortMap: []PortRange{{From: 30000, To: 30009, External: 41000}}, Enabled: &off},
	}
}

func TestReplaceNormalizesAndAudits(t *testing.T) {
	s := newService(t)
	views, err := s.Replace(context.Background(), 60, shanghai(), Actor{UserID: 1, Name: "admin"})
	require.NoError(t, err)
	require.Len(t, views, 4)
	require.Equal(t, "2001:db8::10", views[0].Host)
	require.Equal(t, "ipv6", views[0].Family)
	require.Empty(t, views[0].PortMap)
	require.Equal(t, "ipv4", views[2].Family)
	require.Equal(t, []PortRange{{From: 30000, To: 30099, External: 21000}, {From: 443, To: 443, External: 20443}}, views[2].PortMap)
	require.Equal(t, "nat.example.com", views[3].Host)
	require.Equal(t, "domain", views[3].Family)
	require.False(t, views[3].Enabled)

	var logs []model.OperationLog
	require.NoError(t, s.DB.Find(&logs).Error)
	require.Len(t, logs, 1)
	require.Equal(t, AuditActionWrite, logs[0].Action)
	require.NotContains(t, logs[0].Content, "198.51.100.7", "the audit entry names no address")

	// Replacing keeps only the new list; an empty one removes them all.
	views, err = s.Replace(context.Background(), 60, shanghai()[:1], Actor{})
	require.NoError(t, err)
	require.Len(t, views, 1)
	views, err = s.Replace(context.Background(), 60, nil, Actor{})
	require.NoError(t, err)
	require.Empty(t, views)
}

func TestReplaceRefusesInvalidEntries(t *testing.T) {
	s := newService(t)
	cases := map[string]Input{
		"no label":            {Kind: model.NodeEntryDirect, Host: "2001:db8::1"},
		"pipe in label":       {Label: "a|b", Kind: model.NodeEntryDirect, Host: "2001:db8::1"},
		"bad kind":            {Label: "x", Kind: "tunnel", Host: "2001:db8::1"},
		"bad host":            {Label: "x", Kind: model.NodeEntryDirect, Host: "not a host"},
		"direct with map":     {Label: "x", Kind: model.NodeEntryDirect, Host: "2001:db8::1", PortMap: []PortRange{{From: 1, To: 1, External: 1}}},
		"nat without map":     {Label: "x", Kind: model.NodeEntryNAT, Host: "198.51.100.7"},
		"range reversed":      {Label: "x", Kind: model.NodeEntryNAT, Host: "198.51.100.7", PortMap: []PortRange{{From: 20, To: 10, External: 1000}}},
		"external past 65535": {Label: "x", Kind: model.NodeEntryNAT, Host: "198.51.100.7", PortMap: []PortRange{{From: 10, To: 20, External: 65530}}},
		"ranges overlap":      {Label: "x", Kind: model.NodeEntryNAT, Host: "198.51.100.7", PortMap: []PortRange{{From: 10, To: 20, External: 1000}, {From: 20, To: 30, External: 2000}}},
	}
	for name, input := range cases {
		_, err := s.Replace(context.Background(), 60, []Input{input}, Actor{})
		require.True(t, errors.Is(err, ErrInvalid), "%s: %v", name, err)
	}
	_, err := s.Replace(context.Background(), 60, []Input{shanghai()[0], shanghai()[0]}, Actor{})
	require.True(t, errors.Is(err, ErrInvalid), "a label used twice")
	_, err = s.Replace(context.Background(), 61, shanghai(), Actor{})
	require.True(t, errors.Is(err, ErrNotFound))
}

func TestPortMapsNATEntriesOnly(t *testing.T) {
	s := newService(t)
	_, err := s.Replace(context.Background(), 60, shanghai(), Actor{})
	require.NoError(t, err)
	byNode, err := EnabledByNode(s.DB, []uint{60, 61})
	require.NoError(t, err)
	entries := byNode[60]
	require.Len(t, entries, 3, "the disabled entry is left out")

	port, ok := Port(entries[0], 30005)
	require.True(t, ok)
	require.Equal(t, 30005, port, "a direct entry keeps the protocol's port")
	port, ok = Port(entries[2], 30005)
	require.True(t, ok)
	require.Equal(t, 21005, port)
	port, ok = Port(entries[2], 443)
	require.True(t, ok)
	require.Equal(t, 20443, port)
	_, ok = Port(entries[2], 8443)
	require.False(t, ok, "a port the NAT entry does not map")
}
