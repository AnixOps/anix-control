package service

import (
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRotateWireGuardPeerKeysPreservesAssignments(t *testing.T) {
	db := setupWireGuardTestDB(t)
	peers := []model.WireGuardPeer{
		{NodeProtocolID: 10, UserID: 100, PeerIP: "10.77.0.2", PrivateKey: "old-private-1", PublicKey: "old-public-1", PresharedKey: "old-psk-1"},
		{NodeProtocolID: 10, UserID: 101, PeerIP: "10.77.0.3", PrivateKey: "old-private-2", PublicKey: "old-public-2", PresharedKey: "old-psk-2"},
		{NodeProtocolID: 20, UserID: 100, PeerIP: "10.88.0.2", PrivateKey: "old-private-3", PublicKey: "old-public-3", PresharedKey: "old-psk-3"},
	}
	require.NoError(t, db.Create(&peers).Error)

	result, err := RotateWireGuardPeerKeys(db, 10)
	require.NoError(t, err)
	require.Equal(t, 2, result.RotatedPeerCount)
	require.Equal(t, []uint{10}, result.ProtocolIDs)

	var rotated []model.WireGuardPeer
	require.NoError(t, db.Order("id ASC").Find(&rotated).Error)
	require.Len(t, rotated, 3)
	for i := 0; i < 2; i++ {
		require.Equal(t, peers[i].ID, rotated[i].ID)
		require.Equal(t, peers[i].UserID, rotated[i].UserID)
		require.Equal(t, peers[i].PeerIP, rotated[i].PeerIP)
		require.NotEqual(t, peers[i].PrivateKey, rotated[i].PrivateKey)
		require.NotEqual(t, peers[i].PublicKey, rotated[i].PublicKey)
		require.NotEqual(t, peers[i].PresharedKey, rotated[i].PresharedKey)
		require.Len(t, rotated[i].PrivateKey, 44)
		require.Len(t, rotated[i].PublicKey, 44)
		require.Len(t, rotated[i].PresharedKey, 44)
	}
	require.Equal(t, peers[2].PrivateKey, rotated[2].PrivateKey)
	require.Equal(t, peers[2].PublicKey, rotated[2].PublicKey)
	require.Equal(t, peers[2].PresharedKey, rotated[2].PresharedKey)
}

func TestRotateWireGuardPeerKeysAllProtocols(t *testing.T) {
	db := setupWireGuardTestDB(t)
	require.NoError(t, db.Create(&model.WireGuardPeer{NodeProtocolID: 20, UserID: 200, PeerIP: "10.88.0.2"}).Error)
	require.NoError(t, db.Create(&model.WireGuardPeer{NodeProtocolID: 10, UserID: 100, PeerIP: "10.77.0.2"}).Error)

	result, err := RotateWireGuardPeerKeys(db, 0)
	require.NoError(t, err)
	require.Equal(t, 2, result.RotatedPeerCount)
	require.Equal(t, []uint{10, 20}, result.ProtocolIDs)
}

func TestRotateWireGuardPeerKeysRejectsNilDatabase(t *testing.T) {
	result, err := RotateWireGuardPeerKeys(nil, 0)
	require.Error(t, err)
	require.Nil(t, result)
}

func setupWireGuardTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.WireGuardPeer{}))
	return db
}
