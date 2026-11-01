package egressroute

import (
	"context"
	"encoding/base64"
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
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "egress.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&model.Node{}, &model.NodeProtocol{}, &model.EgressRoute{}, &model.OperationLog{}))
	require.NoError(t, db.Create(&[]model.Node{
		{ID: 29, Name: "hk", Host: "hk.example.com"},
		{ID: 50, Name: "jp", Host: "jp.example.com"},
		{ID: 57, Name: "de"},
	}).Error)
	require.NoError(t, db.Create(&model.NodeProtocol{ID: 1, NodeID: 50, Name: "vless", Type: model.ProtocolVLESS, Port: 443, Enable: 1}).Error)
	return New(db)
}

func hkToJP() Input {
	return Input{Name: "AI via JP", SourceNodeID: 29, ExitNodeID: 50, RelayPort: 24680,
		Domains: []string{" OpenAI.com", ".claude.ai", "", "openai.com", "full:api.x.ai", "keyword:gemini"}}
}

func TestCreateNormalizesAndHidesTheKey(t *testing.T) {
	s := newService(t)
	view, err := s.Create(context.Background(), hkToJP(), Actor{UserID: 1, Name: "admin"})
	require.NoError(t, err)
	require.Equal(t, []string{"openai.com", "claude.ai", "full:api.x.ai", "keyword:gemini"}, view.Domains)
	require.True(t, view.Enabled)
	require.Equal(t, "hk", view.SourceNode.Name)
	require.Equal(t, model.EgressRouteMethod, view.Method)

	var row model.EgressRoute
	require.NoError(t, s.DB.First(&row, view.ID).Error)
	key, err := base64.StdEncoding.DecodeString(row.RelayKey)
	require.NoError(t, err)
	require.Len(t, key, 16)

	var log model.OperationLog
	require.NoError(t, s.DB.Where("action = ?", AuditActionCreate).First(&log).Error)
	require.NotContains(t, log.Content, row.RelayKey)
}

func TestCreateRefusesInvalidRoutes(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	cases := map[string]func(*Input){
		"same node":      func(in *Input) { in.ExitNodeID = 29 },
		"missing node":   func(in *Input) { in.ExitNodeID = 99 },
		"no domains":     func(in *Input) { in.Domains = []string{" "} },
		"bad domain":     func(in *Input) { in.Domains = []string{"not a domain"} },
		"bad matcher":    func(in *Input) { in.Domains = []string{"geoip:cn"} },
		"bad regexp":     func(in *Input) { in.Domains = []string{"regexp:("} },
		"bad port":       func(in *Input) { in.RelayPort = 70000 },
		"exit has no ip": func(in *Input) { in.ExitNodeID = 57 },
		"no name":        func(in *Input) { in.Name = " " },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			input := hkToJP()
			mutate(&input)
			_, err := s.Create(ctx, input, Actor{})
			require.ErrorIs(t, err, ErrInvalid)
		})
	}

	// The exit node's protocol port and another relay's port are taken.
	input := hkToJP()
	input.RelayPort = 443
	_, err := s.Create(ctx, input, Actor{})
	require.ErrorIs(t, err, ErrPortTaken)
	_, err = s.Create(ctx, hkToJP(), Actor{})
	require.NoError(t, err)
	_, err = s.Create(ctx, hkToJP(), Actor{})
	require.ErrorIs(t, err, ErrPortTaken)

	// An exit node without a host is fine with exit_host.
	input = hkToJP()
	input.ExitNodeID, input.ExitHost = 57, "203.0.113.57"
	_, err = s.Create(ctx, input, Actor{})
	require.NoError(t, err)
}

func TestNodeMember(t *testing.T) {
	s := newService(t)
	ctx := context.Background()
	member, err := NodeMember(s.DB, 29)
	require.NoError(t, err)
	require.Nil(t, member, "a node without routes has no member")

	view, err := s.Create(ctx, hkToJP(), Actor{})
	require.NoError(t, err)
	var row model.EgressRoute
	require.NoError(t, s.DB.First(&row, view.ID).Error)

	source, err := NodeMember(s.DB, 29)
	require.NoError(t, err)
	require.Equal(t, map[string]any{
		"routes": []map[string]any{{
			"id": view.ID, "domains": view.Domains,
			"server": map[string]any{"protocol": "shadowsocks", "host": "jp.example.com", "port": 24680, "method": model.EgressRouteMethod, "password": row.RelayKey},
		}},
		"relays": []map[string]any{},
	}, source)

	exit, err := NodeMember(s.DB, 50)
	require.NoError(t, err)
	require.Equal(t, map[string]any{
		"routes": []map[string]any{},
		"relays": []map[string]any{{"id": view.ID, "protocol": "shadowsocks", "port": 24680, "method": model.EgressRouteMethod, "password": row.RelayKey}},
	}, exit)

	other, err := NodeMember(s.DB, 57)
	require.NoError(t, err)
	require.Nil(t, other)

	// exit_host overrides the exit node's host; a rotated key reaches both.
	input := hkToJP()
	input.ExitHost, input.RotateKey = "203.0.113.50", true
	_, err = s.Update(ctx, view.ID, input, Actor{})
	require.NoError(t, err)
	source, err = NodeMember(s.DB, 29)
	require.NoError(t, err)
	server := source["routes"].([]map[string]any)[0]["server"].(map[string]any)
	require.Equal(t, "203.0.113.50", server["host"])
	require.NotEqual(t, row.RelayKey, server["password"])

	// A disabled route is in neither node's member.
	disabled := false
	input.Enabled, input.RotateKey = &disabled, false
	_, err = s.Update(ctx, view.ID, input, Actor{})
	require.NoError(t, err)
	for _, node := range []uint{29, 50} {
		member, err := NodeMember(s.DB, node)
		require.NoError(t, err)
		require.Nil(t, member)
	}

	require.NoError(t, s.Delete(ctx, view.ID, Actor{}))
	require.ErrorIs(t, s.Delete(ctx, view.ID, Actor{}), ErrNotFound)
	views, err := s.List(ctx)
	require.NoError(t, err)
	require.Empty(t, views)
}
