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

// nativeKeys answers the native_keys of the region networks regionIDs, by
// region id: the keys of its members and of its devices that hold a device
// token, sorted and without repeats.
func nativeKeys(db *gorm.DB, regionIDs []uint) (map[uint][]string, error) {
	if len(regionIDs) == 0 {
		return map[uint][]string{}, nil
	}
	var rows []struct {
		RegionID  uint
		StaticKey string
	}
	if err := db.Model(&model.MeshMember{}).Select("region_id", "static_key").
		Where("region_id IN ? AND static_key IS NOT NULL AND static_key <> ''", regionIDs).Scan(&rows).Error; err != nil {
		return nil, err
	}
	var devices []struct {
		RegionID  uint
		StaticKey string
	}
	if err := db.Model(&model.MeshDevice{}).Select("region_id", "static_key").
		Where("region_id IN ? AND static_key IS NOT NULL AND static_key <> '' AND id IN (?)", regionIDs, db.Model(&model.MeshDeviceToken{}).Select("device_id")).
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
// configuration of every member of the region at once, the node's own
// last (§8.3.1: the others admit its key before it uses it).
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
		key, ok := reported[row.Code]
		if !ok || row.StaticKey != nil && *row.StaticKey == key {
			continue
		}
		// A key a device or another node has is not taken: it would
		// keep that device admitted through this node's entry. The
		// region list marks the member (memberKeyWarnings).
		owned, err := keyOwnedElsewhere(db, key, nodeRef)
		if err != nil {
			return err
		}
		if !owned {
			updates[row.ID] = key
		}
	}
	if len(updates) == 0 {
		return nil
	}
	return changeLast(ctx, db, nodeRef, func(tx *gorm.DB) error {
		for id, key := range updates {
			// A row that left meanwhile stays gone; a key another
			// owner took meanwhile is not taken, in the same statement.
			if err := tx.Model(&model.MeshMember{}).Where("id = ? AND node_ref = ?", id, nodeRef).
				Where("NOT EXISTS (?)", tx.Table(model.MeshMember{}.TableName()+" AS o").Select("1").Where("o.static_key = ? AND o.node_ref <> ?", key, nodeRef)).
				Where("NOT EXISTS (?)", tx.Table(model.MeshDevice{}.TableName()+" AS d").Select("1").Where("d.static_key = ?", key)).
				Update("static_key", key).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// bindDeviceKey keeps the static key a device without one reported for
// its region's network, sent with token: the first report after its
// enrollment, or after a move to another region, names it; later reports
// cannot change it. A report sent with a token the device no longer holds
// (it enrolled again or was revoked meanwhile) binds nothing, nor does a
// key a member or another device has already.
func bindDeviceKey(ctx context.Context, db *gorm.DB, device model.MeshDevice, token string, payload []byte) error {
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
		taken, err := keyTaken(tx, key, device.ID)
		if err != nil || taken {
			return err
		}
		// Only while it still has none, in the region the key is of, and
		// while the token is the device's.
		return tx.Model(&model.MeshDevice{}).
			Where("id = ? AND region_id = ? AND static_key IS NULL", device.ID, region.ID).
			Where("EXISTS (?)", tx.Model(&model.MeshDeviceToken{}).Select("1").Where("device_id = ? AND token_hash = ?", device.ID, hashToken(token))).
			Update("static_key", key).Error
	})
}

// keyOwnedElsewhere answers whether key is the static key of a device or
// of a member of another node than nodeRef.
func keyOwnedElsewhere(db *gorm.DB, key, nodeRef string) (bool, error) {
	var members, devices int64
	if err := db.Model(&model.MeshMember{}).Where("static_key = ? AND node_ref <> ?", key, nodeRef).Count(&members).Error; err != nil {
		return false, err
	}
	if err := db.Model(&model.MeshDevice{}).Where("static_key = ?", key).Count(&devices).Error; err != nil {
		return false, err
	}
	return members+devices > 0, nil
}

// keyTaken answers whether key is the static key of a member or of a
// device other than deviceID.
func keyTaken(db *gorm.DB, key string, deviceID uint) (bool, error) {
	var members, devices int64
	if err := db.Model(&model.MeshMember{}).Where("static_key = ?", key).Count(&members).Error; err != nil {
		return false, err
	}
	if err := db.Model(&model.MeshDevice{}).Where("static_key = ? AND id <> ?", key, deviceID).Count(&devices).Error; err != nil {
		return false, err
	}
	return members+devices > 0, nil
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

// keyOwners answers the device id of every bound static key, 0 for a
// member's key.
func keyOwners(db *gorm.DB) (map[string]uint, error) {
	owners := map[string]uint{}
	var members []string
	if err := db.Model(&model.MeshMember{}).Where("static_key IS NOT NULL").Pluck("static_key", &members).Error; err != nil {
		return nil, err
	}
	for _, key := range members {
		owners[key] = 0
	}
	var devices []struct {
		ID        uint
		StaticKey string
	}
	if err := db.Model(&model.MeshDevice{}).Select("id", "static_key").Where("static_key IS NOT NULL").Scan(&devices).Error; err != nil {
		return nil, err
	}
	for _, device := range devices {
		if _, member := owners[device.StaticKey]; !member {
			owners[device.StaticKey] = device.ID
		}
	}
	return owners, nil
}

// reportsTakenKey answers whether the device's latest report names, for
// its region's network, a static key that a member or another device has.
func reportsTakenKey(owners map[string]uint, device model.MeshDevice, code, payload string) bool {
	key, ok := reportedKeys([]byte(payload))[code]
	if !ok {
		return false
	}
	owner, taken := owners[key]
	return taken && owner != device.ID
}

// memberKeyWarnings answers, by member id, WarningStaticKeyTaken for the
// members whose bound key, or whose node's latest reported key for the
// member's region, a device or another node has (also a repeat that
// predates the check). reports are the nodes' latest payloads; codes the
// region codes by id.
func memberKeyWarnings(db *gorm.DB, members []model.MeshMember, reports map[string]string, codes map[uint]string) (map[uint][]string, error) {
	owners := map[string]map[string]bool{}
	own := func(key, owner string) {
		if owners[key] == nil {
			owners[key] = map[string]bool{}
		}
		owners[key][owner] = true
	}
	for _, member := range members {
		if member.StaticKey != nil {
			own(*member.StaticKey, member.NodeRef)
		}
	}
	var devices []struct {
		ID        uint
		StaticKey string
	}
	if err := db.Model(&model.MeshDevice{}).Select("id", "static_key").Where("static_key IS NOT NULL").Scan(&devices).Error; err != nil {
		return nil, err
	}
	for _, device := range devices {
		own(device.StaticKey, DeviceRef(device.ID))
	}
	elsewhere := func(key, nodeRef string) bool {
		for owner := range owners[key] {
			if owner != nodeRef {
				return true
			}
		}
		return false
	}
	reported := map[string]map[string]string{}
	warnings := map[uint][]string{}
	for _, member := range members {
		keys, done := reported[member.NodeRef]
		if !done {
			keys = reportedKeys([]byte(reports[member.NodeRef]))
			reported[member.NodeRef] = keys
		}
		taken := member.StaticKey != nil && elsewhere(*member.StaticKey, member.NodeRef)
		if key, ok := keys[codes[member.RegionID]]; ok && elsewhere(key, member.NodeRef) {
			taken = true
		}
		if taken {
			warnings[member.ID] = []string{WarningStaticKeyTaken}
		}
	}
	return warnings, nil
}
