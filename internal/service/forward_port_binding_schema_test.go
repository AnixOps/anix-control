package service

import (
	"path/filepath"
	"testing"

	"github.com/anixops/v2board/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

func TestEnsureForwardPortBindingSchema(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "binding.db")), &gorm.Config{})
	assert.NoError(t, err)

	assert.NoError(t, EnsureForwardPortBindingSchema(db))
	assert.True(t, db.Migrator().HasTable(&model.ForwardPortBinding{}))
	assert.True(t, db.Migrator().HasIndex(&model.ForwardPortBinding{}, "idx_forward_port_binding_socket"))
}

func TestEnsureForwardPortBindingSchema_BackfillsExistingForwards(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "binding.db")), &gorm.Config{})
	assert.NoError(t, err)
	assert.NoError(t, db.AutoMigrate(&model.ForwardTunnel{}, &model.Forward{}))

	tunnel := &model.ForwardTunnel{
		Name:          "Existing Tunnel",
		InNodeID:      10,
		Type:          1,
		Protocol:      "both",
		TCPListenAddr: "0.0.0.0",
		UDPListenAddr: "127.0.0.1",
		Status:        model.ForwardTunnelStatusActive,
	}
	assert.NoError(t, db.Create(tunnel).Error)
	forward := &model.Forward{
		UserID:     1,
		Name:       "Existing Forward",
		TunnelID:   tunnel.ID,
		InPort:     12000,
		RemoteAddr: "example.com:443",
		Status:     model.ForwardStatusPaused,
	}
	assert.NoError(t, db.Create(forward).Error)

	assert.NoError(t, EnsureForwardPortBindingSchema(db))
	assert.NoError(t, db.Create(&model.ForwardPortBinding{
		ForwardID:  9999,
		NodeID:     99,
		Transport:  "tcp",
		ListenAddr: "127.0.0.1",
		InPort:     12000,
	}).Error)
	assert.NoError(t, EnsureForwardPortBindingSchema(db))

	var bindings []model.ForwardPortBinding
	assert.NoError(t, db.Order("transport ASC").Find(&bindings).Error)
	if assert.Len(t, bindings, 2) {
		assert.Equal(t, forward.ID, bindings[0].ForwardID)
		assert.Equal(t, uint(10), bindings[0].NodeID)
		assert.Equal(t, "tcp", bindings[0].Transport)
		assert.Equal(t, "0.0.0.0", bindings[0].ListenAddr)
		assert.Equal(t, "udp", bindings[1].Transport)
		assert.Equal(t, "127.0.0.1", bindings[1].ListenAddr)
	}
}

func TestEnsureForwardPortBindingSchema_BackfillsWithStoredRuntimeBackend(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "binding.db")), &gorm.Config{})
	assert.NoError(t, err)
	assert.NoError(t, db.AutoMigrate(&model.ForwardTunnel{}, &model.Forward{}))

	executionNodeID := uint(23)
	tunnel := &model.ForwardTunnel{
		Name:          "Clean Agent Tunnel",
		OutNodeID:     &executionNodeID,
		Type:          1,
		Protocol:      "tcp",
		TCPListenAddr: "127.0.0.1",
		UDPListenAddr: "127.0.0.1",
		Status:        model.ForwardTunnelStatusActive,
	}
	assert.NoError(t, db.Create(tunnel).Error)
	forward := &model.Forward{
		UserID:         1,
		Name:           "Clean Agent Forward",
		TunnelID:       tunnel.ID,
		InPort:         12001,
		RemoteAddr:     "example.com:443",
		Status:         model.ForwardStatusPaused,
		RuntimeBackend: model.ForwardRuntimeBackendCleanAgent,
	}
	assert.NoError(t, db.Create(forward).Error)

	assert.NoError(t, EnsureForwardPortBindingSchema(db))

	var binding model.ForwardPortBinding
	assert.NoError(t, db.Where("forward_id = ? AND transport = ?", forward.ID, "tcp").First(&binding).Error)
	assert.Equal(t, executionNodeID, binding.NodeID)
	assert.Equal(t, "127.0.0.1", binding.ListenAddr)
}

func TestEnsureForwardPortBindingSchema_RejectsExistingConflicts(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "binding.db")), &gorm.Config{})
	assert.NoError(t, err)
	assert.NoError(t, db.AutoMigrate(&model.ForwardTunnel{}, &model.Forward{}))

	tunnelA := &model.ForwardTunnel{
		Name:          "Conflict Tunnel A",
		InNodeID:      10,
		Type:          1,
		Protocol:      "tcp",
		TCPListenAddr: "127.0.0.1",
		UDPListenAddr: "127.0.0.1",
		Status:        model.ForwardTunnelStatusActive,
	}
	tunnelB := &model.ForwardTunnel{
		Name:          "Conflict Tunnel B",
		InNodeID:      10,
		Type:          1,
		Protocol:      "tcp",
		TCPListenAddr: "0.0.0.0",
		UDPListenAddr: "0.0.0.0",
		Status:        model.ForwardTunnelStatusActive,
	}
	assert.NoError(t, db.Create(tunnelA).Error)
	assert.NoError(t, db.Create(tunnelB).Error)
	assert.NoError(t, db.Create(&model.Forward{
		UserID:     1,
		Name:       "Conflict Forward A",
		TunnelID:   tunnelA.ID,
		InPort:     12002,
		RemoteAddr: "a.example.com:443",
		Status:     model.ForwardStatusPaused,
	}).Error)
	assert.NoError(t, db.Create(&model.Forward{
		UserID:     2,
		Name:       "Conflict Forward B",
		TunnelID:   tunnelB.ID,
		InPort:     12002,
		RemoteAddr: "b.example.com:443",
		Status:     model.ForwardStatusPaused,
	}).Error)

	err = EnsureForwardPortBindingSchema(db)
	assert.ErrorContains(t, err, "port binding conflict")

	var count int64
	assert.NoError(t, db.Model(&model.ForwardPortBinding{}).Count(&count).Error)
	assert.Zero(t, count)
}
