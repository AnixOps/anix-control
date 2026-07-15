package service

import (
	"errors"
	"sort"
	"time"

	"github.com/AnixOps/anix-control/v3/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// WireGuardPeerRotationResult reports only non-secret rotation metadata.
type WireGuardPeerRotationResult struct {
	RotatedPeerCount int
	ProtocolIDs      []uint
}

// RotateWireGuardPeerKeys replaces every selected peer's client private key,
// public key, and preshared key in one database transaction. Peer IDs, user
// assignments, and tunnel IPs remain stable so the node can apply the update
// without reallocating addresses.
func RotateWireGuardPeerKeys(db *gorm.DB, protocolID uint) (*WireGuardPeerRotationResult, error) {
	if db == nil {
		return nil, errors.New("wireguard peer rotation requires a database")
	}

	wireGuardPeerMu.Lock()
	defer wireGuardPeerMu.Unlock()

	result := &WireGuardPeerRotationResult{}
	err := db.Transaction(func(tx *gorm.DB) error {
		query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Order("id ASC")
		if protocolID > 0 {
			query = query.Where("node_protocol_id = ?", protocolID)
		}

		var peers []model.WireGuardPeer
		if err := query.Find(&peers).Error; err != nil {
			return err
		}

		protocols := make(map[uint]struct{})
		rotatedAt := time.Now()
		for i := range peers {
			privateKey, publicKey, err := generateWireGuardKeypair()
			if err != nil {
				return err
			}
			presharedKey, err := generateWireGuardPresharedKey()
			if err != nil {
				return err
			}
			if err := tx.Model(&model.WireGuardPeer{}).
				Where("id = ?", peers[i].ID).
				Updates(map[string]any{
					"private_key":   privateKey,
					"public_key":    publicKey,
					"preshared_key": presharedKey,
					"updated_at":    rotatedAt,
				}).Error; err != nil {
				return err
			}
			protocols[peers[i].NodeProtocolID] = struct{}{}
		}

		result.RotatedPeerCount = len(peers)
		result.ProtocolIDs = make([]uint, 0, len(protocols))
		for id := range protocols {
			result.ProtocolIDs = append(result.ProtocolIDs, id)
		}
		sort.Slice(result.ProtocolIDs, func(i, j int) bool {
			return result.ProtocolIDs[i] < result.ProtocolIDs[j]
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
