package service

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"strings"
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
// configured with the legacy key keep working. A new root node's
// Shadowsocks 2022 protocol gets a random server key instead of the one
// derived from the node's creation time; existing nodes keep theirs, since
// changing it breaks every client.

// isSS2022 says whether cipher is a Shadowsocks 2022 cipher.
func isSS2022(cipher string) bool { return strings.HasPrefix(cipher, "2022-blake3-") }

// randomSS2022Key is n random bytes in std base64.
func randomSS2022Key(n int) (string, error) {
	raw := make([]byte, n)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(raw), nil
}

// EnsureSS2022UserKey answers the user's stored Shadowsocks 2022 key,
// creating it on the first call. A new key is recorded as a change of the
// user, so users.v1 sends it to the nodes at once.
func EnsureSS2022UserKey(db *gorm.DB, userID uint) (string, error) {
	var row model.UserSS2022Key
	if err := db.Where("user_id = ?", userID).Limit(1).Find(&row).Error; err != nil || row.UserKey != "" {
		return row.UserKey, err
	}
	key, err := randomSS2022Key(32)
	if err != nil {
		return "", err
	}
	err = db.Transaction(func(tx *gorm.DB) error {
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

// ensureSS2022ServerKey gives a new Shadowsocks 2022 protocol of a root
// node a random server key when its settings name none, instead of the key
// derived from the node's creation time.
func ensureSS2022ServerKey(protocol *model.NodeProtocol, node *model.Node) error {
	if protocol.Type != model.ProtocolShadowsocks || node.ParentID != nil {
		return nil
	}
	settings := map[string]any{}
	if protocol.Settings != nil && *protocol.Settings != "" {
		if err := json.Unmarshal([]byte(*protocol.Settings), &settings); err != nil {
			return nil // validation reports it
		}
	}
	cipher, _ := settings["cipher"].(string)
	if cipher == "" {
		cipher, _ = settings["method"].(string)
	}
	if key, _ := settings["server_key"].(string); !isSS2022(cipher) || key != "" {
		return nil
	}
	key, err := randomSS2022Key(ss2022KeyLen(cipher))
	if err != nil {
		return err
	}
	settings["server_key"] = key
	encoded, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	text := string(encoded)
	protocol.Settings = &text
	return nil
}
