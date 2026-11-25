package meshregion

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"sort"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

// A region network admits only our own nodes: its configuration carries
// native_keys, the static public keys of every member and device in it
// (agent-contract.md section 2). A node's key comes from its Agent's
// mesh.status; a device without an Agent gives its key at enrollment or,
// when it had none to give, in its first report after it, and cannot
// change it afterwards.

// KindManaged is the kind of every region network.
const KindManaged = "managed"

// staticKeyLength is the length of a 32-byte key in padded standard base64.
const staticKeyLength = 44

// staticKey answers s when it is a static public key: 32 bytes in
// standard base64 with padding, in its one canonical spelling.
func staticKey(s string) (string, bool) {
	if len(s) != staticKeyLength {
		return "", false
	}
	raw, err := base64.StdEncoding.Strict().DecodeString(s)
	if err != nil || len(raw) != 32 || base64.StdEncoding.EncodeToString(raw) != s {
		return "", false
	}
	return s, true
}

// reportedKeys answers the static keys a mesh.status payload reports, by
// network id (the region code); invalid keys are left out.
func reportedKeys(payload []byte) map[string]string {
	var shape struct {
		Networks []struct {
			ID        string `json:"id"`
			StaticKey string `json:"static_key"`
		} `json:"networks"`
	}
	keys := map[string]string{}
	if json.Unmarshal(payload, &shape) != nil {
		return keys
	}
	for _, network := range shape.Networks {
		if key, ok := staticKey(network.StaticKey); ok && network.ID != "" {
			keys[network.ID] = key
		}
	}
	return keys
}

// nativeKeys answers the native_keys of every region network, by region
// id: the keys of its members and of its devices that hold a device token,
// sorted and without repeats.
func nativeKeys(db *gorm.DB) (map[uint][]string, error) {
	var rows []struct {
		RegionID  uint
		StaticKey string
	}
	if err := db.Model(&model.MeshMember{}).Select("region_id", "static_key").
		Where("static_key IS NOT NULL AND static_key <> ''").Scan(&rows).Error; err != nil {
		return nil, err
	}
	var devices []struct {
		RegionID  uint
		StaticKey string
	}
	if err := db.Model(&model.MeshDevice{}).Select("region_id", "static_key").
		Where("static_key IS NOT NULL AND static_key <> '' AND id IN (?)", db.Model(&model.MeshDeviceToken{}).Select("device_id")).
		Scan(&devices).Error; err != nil {
		return nil, err
	}
	rows = append(rows, devices...)
	seen := map[uint]map[string]bool{}
	keys := map[uint][]string{}
	for _, row := range rows {
		if seen[row.RegionID] == nil {
			seen[row.RegionID] = map[string]bool{}
		}
		if !seen[row.RegionID][row.StaticKey] {
			seen[row.RegionID][row.StaticKey] = true
			keys[row.RegionID] = append(keys[row.RegionID], row.StaticKey)
		}
	}
	for _, list := range keys {
		sort.Strings(list)
	}
	return keys, nil
}

// regionKeys answers the native_keys of one region, never nil.
func regionKeys(keys map[uint][]string, regionID uint) []string {
	if list := keys[regionID]; list != nil {
		return list
	}
	return []string{}
}

// recordMemberKeys keeps the static keys a node's Agent reported for its
// member rows, the regions' networks; a new or changed key changes the
// configuration of every member of the region at once.
func recordMemberKeys(ctx context.Context, db *gorm.DB, nodeRef string, payload []byte) error {
	reported := reportedKeys(payload)
	if len(reported) == 0 {
		return nil
	}
	var rows []struct {
		ID        uint
		Code      string
		StaticKey *string
	}
	if err := db.Table(model.MeshMember{}.TableName()+" AS m").
		Select("m.id AS id", "r.code AS code", "m.static_key AS static_key").
		Joins("JOIN "+model.MeshRegion{}.TableName()+" AS r ON r.id = m.region_id").
		Where("m.node_ref = ?", nodeRef).Scan(&rows).Error; err != nil {
		return err
	}
	updates := map[uint]string{}
	for _, row := range rows {
		if key, ok := reported[row.Code]; ok && (row.StaticKey == nil || *row.StaticKey != key) {
			updates[row.ID] = key
		}
	}
	if len(updates) == 0 {
		return nil
	}
	return change(ctx, db, func(tx *gorm.DB) error {
		for id, key := range updates {
			// A row that left meanwhile stays gone.
			if err := tx.Model(&model.MeshMember{}).Where("id = ? AND node_ref = ?", id, nodeRef).
				Update("static_key", key).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// bindDeviceKey keeps the static key a device without one reported for
// its region's network: the first report after its enrollment, or after a
// move to another region, names it; later reports cannot change it.
func bindDeviceKey(ctx context.Context, db *gorm.DB, device model.MeshDevice, payload []byte) error {
	if device.StaticKey != nil {
		return nil
	}
	var region model.MeshRegion
	if err := db.Select("id", "code").Where("id = ?", device.RegionID).Limit(1).Find(&region).Error; err != nil || region.ID == 0 {
		return err
	}
	key, ok := reportedKeys(payload)[region.Code]
	if !ok {
		return nil
	}
	return change(ctx, db, func(tx *gorm.DB) error {
		// Only while it still has none, in the region the key is of.
		return tx.Model(&model.MeshDevice{}).Where("id = ? AND region_id = ? AND static_key IS NULL", device.ID, region.ID).
			Update("static_key", key).Error
	})
}

// keyMismatch answers whether the device's latest report names another
// static key for its region's network than the one it is held to.
func keyMismatch(device model.MeshDevice, code, payload string) bool {
	if device.StaticKey == nil || payload == "" {
		return false
	}
	key, ok := reportedKeys([]byte(payload))[code]
	return ok && key != *device.StaticKey
}
