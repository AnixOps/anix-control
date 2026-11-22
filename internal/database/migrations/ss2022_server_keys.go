package migrations

import (
	"context"
	"crypto/md5" // #nosec G501 -- reproduces the key existing clients already hold; nothing new is derived this way.
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/sdk/v2compat"
	"gorm.io/gorm"
)

// ss2022ServerKeysVersion is the number of 0041_ss2022_server_keys.
const ss2022ServerKeysVersion = 41

// storeSS2022ServerKeys is 0041_ss2022_server_keys. A Shadowsocks 2022
// protocol (a 2022-blake3-* cipher) without a server_key used to get one
// derived at every read from its node's created_at (the parent node's for
// a relay), by the formula of XBoard, the panel Control's first databases
// were imported from: base64 of the first key-length hex characters of
// md5(created_at as Unix seconds). This step stores that same key as the
// protocol's secret, so nodes and subscriptions keep the key their clients
// hold, and Control no longer derives anything; a new protocol brings its
// own random key. It works on the tables as this step's schema has them.
func storeSS2022ServerKeys(ctx context.Context, tx *gorm.DB, _ string) error {
	tx = tx.WithContext(ctx)
	var protocols []struct {
		ID       uint64
		NodeID   uint64
		Settings *string
	}
	if err := tx.Raw(`SELECT id, node_id, settings FROM v5_node_protocol WHERE type = 'shadowsocks' ORDER BY id`).Scan(&protocols).Error; err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, protocol := range protocols {
		settings := map[string]any{}
		if protocol.Settings != nil && strings.TrimSpace(*protocol.Settings) != "" {
			if err := json.Unmarshal([]byte(*protocol.Settings), &settings); err != nil || settings == nil {
				continue // not an object: nothing ever derived a key for it
			}
		}
		cipher, _ := settings["cipher"].(string)
		if cipher == "" {
			cipher, _ = settings["method"].(string)
		}
		if !strings.HasPrefix(cipher, "2022-blake3-") {
			continue
		}
		if key, _ := settings["server_key"].(string); key != "" {
			continue
		}
		var stored int64
		if err := tx.Raw(`SELECT count(*) FROM v4_kernel_protocol_secret WHERE scope = 'node_protocol' AND owner_id = ? AND column_name = 'settings' AND json_pointer = '/server_key'`,
			protocol.ID).Scan(&stored).Error; err != nil {
			return err
		}
		if stored > 0 {
			continue
		}
		createdAt, err := keyCreatedAt(tx, protocol.NodeID)
		if err != nil {
			return err
		}
		value, err := json.Marshal(formerSS2022ServerKey(createdAt, cipher))
		if err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO v4_kernel_protocol_secret (scope, owner_id, column_name, json_pointer, value, sealed, kek_id, version, updated_at) VALUES ('node_protocol', ?, 'settings', '/server_key', ?, ?, '', 1, ?)`,
			protocol.ID, string(value), false, now).Error; err != nil {
			return err
		}
		settings["server_key"] = v2compat.NodeSecretPlaceholder
		document, err := json.Marshal(settings)
		if err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE v5_node_protocol SET settings = ? WHERE id = ?`, string(document), protocol.ID).Error; err != nil {
			return err
		}
	}
	return nil
}

// keyCreatedAt answers the created_at the key was derived from: the node's,
// or its parent's when it has one with a created_at.
func keyCreatedAt(tx *gorm.DB, nodeID uint64) (time.Time, error) {
	var node struct {
		CreatedAt time.Time
		ParentID  *uint64
	}
	if err := tx.Raw(`SELECT created_at, parent_id FROM v5_node WHERE id = ?`, nodeID).Scan(&node).Error; err != nil {
		return time.Time{}, err
	}
	if node.ParentID != nil {
		var parent struct{ CreatedAt time.Time }
		if err := tx.Raw(`SELECT created_at FROM v5_node WHERE id = ?`, *node.ParentID).Scan(&parent).Error; err != nil {
			return time.Time{}, err
		}
		if !parent.CreatedAt.IsZero() {
			return parent.CreatedAt, nil
		}
	}
	return node.CreatedAt, nil
}

// formerSS2022ServerKey is the key a protocol without a stored one was
// given (storeSS2022ServerKeys): 16 characters for an aes-128 cipher, 32
// otherwise.
func formerSS2022ServerKey(createdAt time.Time, cipher string) string {
	length := 32
	if strings.Contains(cipher, "128") {
		length = 16
	}
	sum := md5.Sum([]byte(strconv.FormatInt(createdAt.Unix(), 10))) // #nosec G401 -- see the import
	return base64.StdEncoding.EncodeToString([]byte(hex.EncodeToString(sum[:])[:length]))
}
