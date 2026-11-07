package service

import (
	"crypto/rand"
	"encoding/base64"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/subscriber"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Shadowsocks 2022 keys. A user's legacy key is base64(uuid[:keyLen]),
// which anyone who knows the UUID can derive. A user now also gets a random
// key (model.UserSS2022Key) at the first subscription pull; users.v1 sends
// it to the nodes, whose Agents (agentcontrol.ProtocolRevisionUserKeys and
// later) serve it as a second user next to the legacy one, so devices
// configured with the legacy key keep working. Server keys stay derived
// from the node's creation time: a relay child node runs no Agent and
// derives its parent's key, so a random key per protocol of the parent
// would break it (docs/manual-intervention.md).

// EnsureSS2022UserKey answers the user's stored Shadowsocks 2022 key,
// creating it on the first call. A new key is recorded as a change of the
// user, so users.v1 sends it to the nodes at once.
func EnsureSS2022UserKey(db *gorm.DB, userID uint) (string, error) {
	var row model.UserSS2022Key
	if err := db.Where("user_id = ?", userID).Limit(1).Find(&row).Error; err != nil || row.UserKey != "" {
		return row.UserKey, err
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	key := base64.StdEncoding.EncodeToString(raw)
	err := db.Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		created := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.UserSS2022Key{UserID: userID, UserKey: key, CreatedAt: now})
		if created.Error != nil {
			return created.Error
		}
		if created.RowsAffected > 0 {
			if err := subscriber.RecordChangesTx(tx, []uint{userID}, false, now); err != nil {
				return err
			}
		}
		return tx.Where("user_id = ?", userID).Take(&row).Error
	})
	return row.UserKey, err
}

// SS2022UserKeys answers the stored keys of users, by user id.
func SS2022UserKeys(db *gorm.DB, userIDs []uint) (map[uint]string, error) {
	keys := make(map[uint]string, len(userIDs))
	if len(userIDs) == 0 {
		return keys, nil
	}
	var rows []model.UserSS2022Key
	if err := db.Where("user_id IN ?", userIDs).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		keys[row.UserID] = row.UserKey
	}
	return keys, nil
}

// agentUserKeyNodes answers which of the proxy nodes have an Agent that
// serves the stored Shadowsocks 2022 keys, as its last Hello said.
func agentUserKeyNodes(db *gorm.DB, nodeIDs []uint) (map[uint]bool, error) {
	nodes := map[uint]bool{}
	if len(nodeIDs) == 0 {
		return nodes, nil
	}
	var ids []uint
	if err := db.Model(&model.AgentTransport{}).
		Where("node_kind = ? AND transport = ? AND protocol_revision >= ? AND node_id IN ?",
			agentcontrol.NodeKindProxy, model.AgentTransportMTLSStream, agentcontrol.ProtocolRevisionUserKeys, nodeIDs).
		Pluck("node_id", &ids).Error; err != nil {
		return nil, err
	}
	for _, id := range ids {
		nodes[id] = true
	}
	return nodes, nil
}
