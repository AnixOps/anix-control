package meshregion

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Devices without an Agent (R31; agent-contract.md section 4): an
// administrator issues a one-time enrollment credential; the device (an
// install command, a personal device's client) exchanges it once for its
// own device token and its network, and from then on reads its network,
// sends its read-only status and asks to change region with that token.
// The device is an ordinary device of the pool (devices.go): its address
// stays while it is used, and every read or report uses it. A credential of
// the role full enrolls a full node instead (fullnode.go).

const (
	// DefaultEnrollmentTTL and MaxEnrollmentTTL bound a credential's life.
	DefaultEnrollmentTTL = 24 * time.Hour
	MaxEnrollmentTTL     = 7 * 24 * time.Hour
	enrollmentPrefix     = "mse_"
	deviceTokenPrefix    = "msd_"
	// EnrollRetryWindow: a device whose enrollment answer was lost
	// enrolls again with the same credential and device key for this long.
	EnrollRetryWindow = 10 * time.Minute
	// maxLatencies bounds the regions an enrollment reports.
	maxLatencies = 64
	// The use of a device token is written at most this often.
	seenEvery = time.Minute
	usedEvery = time.Hour
)

// ErrUnauthorized: the credential or device token is unknown, used,
// expired or revoked.
var ErrUnauthorized = errors.New("invalid mesh credential")

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// DeviceRef is a device's status key.
func DeviceRef(id uint) string { return "device-" + strconv.FormatUint(uint64(id), 10) }

// EnrollmentInput issues a credential: for a new device (Hostname, and
// RegionID or 0 to let the device's latencies choose), or for the device
// DeviceID to enroll again, which keeps its name, region, role and
// address.
type EnrollmentInput struct {
	RegionID uint   `json:"region_id"`
	Hostname string `json:"hostname"`
	DeviceID uint   `json:"device_id"`
	// Hours the credential stays valid: 1 to 168, 24 when 0.
	Hours int `json:"hours"`
	// Role is access (default) or full (a full node without an Agent,
	// which needs a region).
	Role string `json:"role" enums:"access,full"`
	// Address is a full node's fixed address of the region's server
	// segment; empty: the lowest free one when the device enrolls.
	Address string `json:"address"`
	// PreferredRegion is the code of the region whose latency counts 0.8
	// times when the device's latencies choose the region (D.5); empty:
	// the settings' default. Only for a credential without a region.
	PreferredRegion string `json:"preferred_region"`
}

// EnrollmentView is a credential with the role of the device it enrolls,
// a full node's fixed address and the code of its preferred region.
type EnrollmentView struct {
	model.MeshEnrollment
	Role            string `json:"role"`
	Address         string `json:"address"`
	PreferredRegion string `json:"preferred_region"`
}

// IssuedEnrollment is a credential just issued with its token, which no
// later answer carries.
type IssuedEnrollment struct {
	EnrollmentView
	Token string `json:"token"`
}

// CreateEnrollment issues a one-time credential.
func CreateEnrollment(ctx context.Context, db *gorm.DB, input EnrollmentInput, createdBy *uint, now time.Time) (IssuedEnrollment, error) {
	db = db.WithContext(ctx)
	role := input.Role
	if role == "" {
		role = RoleAccess
	}
	if role != RoleAccess && role != RoleFull {
		return IssuedEnrollment{}, invalid("role: access or full")
	}
	var target *uint
	if input.DeviceID != 0 {
		var device model.MeshDevice
		if err := findDevice(db, input.DeviceID, &device); err != nil {
			return IssuedEnrollment{}, err
		}
		node, full, err := isFullNode(db, device.ID)
		if err != nil {
			return IssuedEnrollment{}, err
		}
		current := RoleAccess
		if full {
			current = RoleFull
		}
		if (input.Role != "" && input.Role != current) || input.Address != "" {
			return IssuedEnrollment{}, invalid("role, address: a device enrolling again keeps its role and address")
		}
		id := device.ID
		target, input.Hostname, input.RegionID, role = &id, device.Hostname, device.RegionID, current
		input.Address = node.Address
	}
	hostname, err := normalizeLabel("hostname", input.Hostname)
	if err != nil {
		return IssuedEnrollment{}, err
	}
	ttl := time.Duration(input.Hours) * time.Hour
	if input.Hours == 0 {
		ttl = DefaultEnrollmentTTL
	}
	if ttl < time.Hour || ttl > MaxEnrollmentTTL {
		return IssuedEnrollment{}, invalid("hours: 1 to 168")
	}
	if role == RoleFull && input.RegionID == 0 {
		return IssuedEnrollment{}, invalid("region_id: a full node needs a region")
	}
	if role == RoleAccess && input.Address != "" {
		return IssuedEnrollment{}, invalid("address: only a full node has a fixed address")
	}
	preferred := strings.TrimSpace(input.PreferredRegion)
	if preferred != "" && (input.RegionID != 0 || target != nil) {
		return IssuedEnrollment{}, invalid("preferred_region: only for a credential without a region")
	}
	token := enrollmentPrefix + rand.Text()
	row := model.MeshEnrollment{RegionID: input.RegionID, Hostname: hostname, TokenHash: hashToken(token), DeviceID: target, ExpiresAt: now.Add(ttl), CreatedBy: createdBy, CreatedAt: now}
	address := input.Address
	// No node's configuration changes: a plain transaction, no push.
	err = db.Transaction(func(tx *gorm.DB) error {
		if preferred != "" {
			id, err := regionIDOfCode(tx, preferred)
			if errors.Is(err, ErrNotFound) {
				return invalid("preferred_region: unknown region code %q", preferred)
			}
			if err != nil {
				return err
			}
			row.PreferredRegionID = &id
		}
		if input.RegionID != 0 {
			var region model.MeshRegion
			if err := findRegion(tx, input.RegionID, &region); err != nil {
				return err
			}
			if role == RoleFull && target == nil && address != "" {
				// The address is kept for this credential until it is used.
				checked, err := serverAddress(tx, region, address, 0, now)
				if err != nil {
					return err
				}
				address = checked
			}
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		if role != RoleFull || target != nil {
			return nil
		}
		return tx.Create(&model.MeshFullNode{EnrollmentID: row.ID, Address: address, Subnets: "", CreatedAt: now, UpdatedAt: now}).Error
	})
	if err != nil {
		return IssuedEnrollment{}, err
	}
	return IssuedEnrollment{EnrollmentView: EnrollmentView{MeshEnrollment: row, Role: role, Address: address, PreferredRegion: preferred}, Token: token}, nil
}

// Enrollments answers the newest 200 credentials.
func Enrollments(ctx context.Context, db *gorm.DB) ([]EnrollmentView, error) {
	db = db.WithContext(ctx)
	rows := []model.MeshEnrollment{}
	if err := db.Order("id DESC").Limit(200).Find(&rows).Error; err != nil {
		return nil, err
	}
	ids, devices := []uint{}, []uint{}
	for _, row := range rows {
		ids = append(ids, row.ID)
		if row.DeviceID != nil {
			devices = append(devices, *row.DeviceID)
		}
	}
	var issued []model.MeshFullNode
	if len(ids) > 0 {
		if err := db.Where("enrollment_id IN ?", ids).Find(&issued).Error; err != nil {
			return nil, err
		}
	}
	byEnrollment := map[uint]model.MeshFullNode{}
	for _, node := range issued {
		byEnrollment[node.EnrollmentID] = node
	}
	byDevice, err := fullNodes(db, devices)
	if err != nil {
		return nil, err
	}
	codes, err := regionCodes(db)
	if err != nil {
		return nil, err
	}
	views := make([]EnrollmentView, 0, len(rows))
	for _, row := range rows {
		view := EnrollmentView{MeshEnrollment: row, Role: RoleAccess}
		if row.PreferredRegionID != nil {
			view.PreferredRegion = codes[*row.PreferredRegionID]
		}
		node, full := byEnrollment[row.ID]
		if !full && row.DeviceID != nil {
			node, full = byDevice[*row.DeviceID]
		}
		if full {
			view.Role, view.Address = RoleFull, node.Address
		}
		views = append(views, view)
	}
	return views, nil
}

// DeleteEnrollment withdraws an unused credential; a used one stays as the
// record of the device it enrolled.
func DeleteEnrollment(ctx context.Context, db *gorm.DB, id uint) error {
	var rows []model.MeshEnrollment
	if err := db.WithContext(ctx).Where("id = ?", id).Limit(1).Find(&rows).Error; err != nil {
		return err
	}
	if len(rows) > 0 && rows[0].UsedAt != nil {
		return conflict("a used credential stays as the record of its device")
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Where("id = ? AND used_at IS NULL", id).Delete(&model.MeshEnrollment{})
		if result.Error == nil && result.RowsAffected == 0 {
			return fmt.Errorf("%w: enrollment %d", ErrNotFound, id)
		}
		if result.Error != nil {
			return result.Error
		}
		// The fixed address it kept is free again.
		return tx.Where("enrollment_id = ? AND device_id IS NULL", id).Delete(&model.MeshFullNode{}).Error
	})
}

// EnrollRequest is what a device sends with its credential.
type EnrollRequest struct {
	Token     string `json:"token"`
	DeviceKey string `json:"device_key"`
	// Latencies are the device's round trips in milliseconds by region
	// code, for a credential without a region.
	Latencies map[string]float64 `json:"latencies"`
	// StaticKey is the device's static public key in its region's
	// network, when it has one already (nativekeys.go); without it the
	// device's first report names it.
	StaticKey string `json:"static_key"`
	// Features are the capabilities of the device's kernel (its
	// GET /v1/status features); the answered configuration follows them
	// (DeviceConfig).
	Features []string `json:"features"`
}

// checkFeatures refuses more capability names than MaxFeatures.
func checkFeatures(features []string) error {
	if len(features) > MaxFeatures {
		return invalid("features: at most %d names", MaxFeatures)
	}
	return nil
}

// Enrolled answers an enrollment: the device's id, its token (answered
// only here) and its networks.
type Enrolled struct {
	DeviceID    uint           `json:"device_id"`
	DeviceToken string         `json:"device_token"`
	Config      map[string]any `json:"config"`
}

// Enroll exchanges a credential for a device token. A credential for a
// new device creates one; a device key some device already has is refused
// (it is no proof of holding it). A credential issued for a device enrolls
// that device again: its key, if it has one, must match, and its earlier
// token stops working. A credential is used once; within
// EnrollRetryWindow the same device key may use it again, for an answer
// that was lost, and gets a new token.
func Enroll(ctx context.Context, db *gorm.DB, request EnrollRequest, now time.Time) (Enrolled, error) {
	_, key, err := normalizeDevice(DeviceInput{Hostname: "x", DeviceKey: request.DeviceKey})
	if err != nil {
		return Enrolled{}, err
	}
	if key == nil {
		return Enrolled{}, invalid("device_key: required")
	}
	if err := checkFeatures(request.Features); err != nil {
		return Enrolled{}, err
	}
	if len(request.Latencies) > maxLatencies {
		return Enrolled{}, invalid("latencies: at most %d regions", maxLatencies)
	}
	var static *string
	if request.StaticKey != "" {
		value, ok := staticKey(request.StaticKey)
		if !ok {
			return Enrolled{}, invalid("static_key: 32 bytes in standard base64 with padding")
		}
		static = &value
	}
	token := deviceTokenPrefix + rand.Text()
	var device model.MeshDevice
	err = change(ctx, db, func(tx *gorm.DB) error {
		var enrollments []model.MeshEnrollment
		if err := tx.Where("token_hash = ? AND expires_at > ?", hashToken(request.Token), now).Limit(1).Find(&enrollments).Error; err != nil {
			return err
		}
		if len(enrollments) == 0 {
			return ErrUnauthorized
		}
		enrollment := enrollments[0]
		var known []model.MeshDevice
		if err := tx.Where("device_key = ?", *key).Limit(1).Find(&known).Error; err != nil {
			return err
		}
		switch {
		case enrollment.UsedAt != nil:
			// A retry: the device this credential enrolled, with its key.
			if now.Sub(*enrollment.UsedAt) > EnrollRetryWindow || enrollment.DeviceID == nil || len(known) == 0 || known[0].ID != *enrollment.DeviceID {
				return ErrUnauthorized
			}
			device = known[0]
			// The same enrollment again: a key the device named
			// meanwhile stays; changing it takes a new credential.
			if static != nil && device.StaticKey == nil {
				device.StaticKey = static
				if err := updateExisting(tx, &model.MeshDevice{}, device.ID, map[string]any{"static_key": *static, "updated_at": now}, "device"); err != nil {
					return err
				}
			} else if static != nil && *device.StaticKey != *static {
				return conflict("the device has another static key; enroll again with a new credential to change it")
			}
		case enrollment.DeviceID != nil:
			if err := findDevice(tx, *enrollment.DeviceID, &device); err != nil {
				return ErrUnauthorized
			}
			if device.DeviceKey != nil && *device.DeviceKey != *key {
				return ErrUnauthorized
			}
			if len(known) > 0 && known[0].ID != device.ID {
				return conflict("another device has this device_key")
			}
			// Enrolling again is how a device changes its static key: it
			// holds the one it gives now, or none until its first report.
			device.DeviceKey, device.StaticKey = key, static
			var staticColumn any = gorm.Expr("NULL")
			if static != nil {
				staticColumn = *static
			}
			if err := updateExisting(tx, &model.MeshDevice{}, device.ID, map[string]any{"device_key": *key, "static_key": staticColumn, "updated_at": now}, "device"); err != nil {
				return err
			}
		case len(known) > 0:
			return conflict("another device has this device_key; enroll it again with a credential issued for it")
		default:
			region, err := enrollmentRegion(tx, enrollment, request.Latencies)
			if err != nil {
				return err
			}
			var nodes []model.MeshFullNode
			if err := tx.Where("enrollment_id = ?", enrollment.ID).Limit(1).Find(&nodes).Error; err != nil {
				return err
			}
			var address string
			if len(nodes) > 0 {
				address, err = serverAddress(tx, region, nodes[0].Address, enrollment.ID, now)
			} else {
				address, err = allocate(tx, region, now)
			}
			if err != nil {
				return err
			}
			device = model.MeshDevice{RegionID: region.ID, DeviceKey: key, StaticKey: static, Hostname: enrollment.Hostname, Address: &address, AddressUsedAt: &now, CreatedAt: now, UpdatedAt: now}
			if err := tx.Create(&device).Error; err != nil {
				return err
			}
			if len(nodes) > 0 {
				if err := tx.Model(&nodes[0]).Updates(map[string]any{"device_id": device.ID, "address": address, "updated_at": now}).Error; err != nil {
					return err
				}
			}
		}
		if static != nil {
			taken, err := keyTaken(tx, *static, device.ID)
			if err != nil {
				return err
			}
			if taken {
				return conflict("static_key: a member or another device has this key")
			}
		}
		if enrollment.UsedAt == nil {
			// One use: of two requests at once, only the first claims it.
			claimed := tx.Model(&model.MeshEnrollment{}).Where("id = ? AND used_at IS NULL", enrollment.ID).
				Updates(map[string]any{"used_at": now, "device_id": device.ID})
			if claimed.Error != nil {
				return claimed.Error
			}
			if claimed.RowsAffected != 1 {
				return ErrUnauthorized
			}
		}
		row := model.MeshDeviceToken{DeviceID: device.ID, TokenHash: hashToken(token), LastSeenAt: &now, CreatedAt: now}
		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "device_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"token_hash", "last_seen_at", "created_at"}),
		}).Create(&row).Error
	})
	if err != nil {
		return Enrolled{}, err
	}
	config, err := DeviceConfig(ctx, db, device, request.Features, now)
	return Enrolled{DeviceID: device.ID, DeviceToken: token, Config: config}, err
}

// PreferredFactor is the factor of the preferred region's latency in the
// automatic region choice (appendix D.5).
const PreferredFactor = 0.8

// chooseRegion answers the region of the lowest score among regions,
// false when none has a latency. The score is the latency, times
// PreferredFactor for the region preferredID (0: none). Ties go to the
// region of the lower sort, then the lower id: regions are ordered so
// here, whatever the order they come in, and the first of equal scores
// stays. Only the regions given count: the caller passes the enabled ones.
func chooseRegion(regions []model.MeshRegion, latencies map[string]float64, preferredID uint) (model.MeshRegion, bool) {
	ordered := append([]model.MeshRegion(nil), regions...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Sort != ordered[j].Sort {
			return ordered[i].Sort < ordered[j].Sort
		}
		return ordered[i].ID < ordered[j].ID
	})
	var best model.MeshRegion
	score, found := 0.0, false
	for _, candidate := range ordered {
		latency, ok := latencies[candidate.Code]
		if !ok || latency < 0 {
			continue
		}
		if candidate.ID == preferredID && preferredID != 0 {
			latency *= PreferredFactor
		}
		if !found || latency < score {
			best, score, found = candidate, latency, true
		}
	}
	return best, found
}

// preferredRegionID is the credential's preferred region, else the
// settings' default; 0 for none.
func preferredRegionID(tx *gorm.DB, credential *uint) (uint, error) {
	if credential != nil {
		return *credential, nil
	}
	settings, err := loadSettings(tx)
	return settings.PreferredRegionID, err
}

// enrollmentRegion is the credential's region, or the enabled region of
// the lowest score (chooseRegion) of the latencies, by code.
func enrollmentRegion(tx *gorm.DB, enrollment model.MeshEnrollment, latencies map[string]float64) (model.MeshRegion, error) {
	var region model.MeshRegion
	if enrollment.RegionID != 0 {
		return region, findRegion(tx, enrollment.RegionID, &region)
	}
	var regions []model.MeshRegion
	if err := tx.Where("enabled = ?", true).Order("sort ASC, id ASC").Find(&regions).Error; err != nil {
		return region, err
	}
	preferred, err := preferredRegionID(tx, enrollment.PreferredRegionID)
	if err != nil {
		return region, err
	}
	chosen, ok := chooseRegion(regions, latencies, preferred)
	if !ok {
		return region, invalid("latencies: the round trip to at least one enabled region, by code")
	}
	// Read again under the region's lock, which a change of its range
	// takes too.
	if err := findRegion(tx, chosen.ID, &region); err != nil {
		return region, err
	}
	if !region.Enabled {
		return region, conflict("the region %q was disabled meanwhile; enroll again", region.Code)
	}
	return region, nil
}

// Authenticate answers the device of a device token and records the use.
func Authenticate(ctx context.Context, db *gorm.DB, token string, now time.Time) (model.MeshDevice, error) {
	db = db.WithContext(ctx)
	var tokens []model.MeshDeviceToken
	if err := db.Where("token_hash = ?", hashToken(token)).Limit(1).Find(&tokens).Error; err != nil {
		return model.MeshDevice{}, err
	}
	if len(tokens) == 0 {
		return model.MeshDevice{}, ErrUnauthorized
	}
	var device model.MeshDevice
	if err := findDevice(db, tokens[0].DeviceID, &device); err != nil {
		return device, err
	}
	// The use is written at most once a minute (last seen) and once an
	// hour (the address, whose reclaim counts in days).
	if seen := tokens[0].LastSeenAt; seen == nil || now.Sub(*seen) >= seenEvery {
		if err := db.Model(&tokens[0]).Update("last_seen_at", now).Error; err != nil {
			return device, err
		}
	}
	if device.Address != nil && (device.AddressUsedAt == nil || now.Sub(*device.AddressUsedAt) >= usedEvery) {
		device.AddressUsedAt = &now
		return device, db.Model(&device).Update("address_used_at", now).Error
	}
	return device, nil
}

// RevokeDevice deletes the device's token: it is refused from now on and
// enrolls again only with a new credential. Its static key is released
// and leaves its region's native_keys, so our nodes refuse it at once.
func RevokeDevice(ctx context.Context, db *gorm.DB, id uint) error {
	return change(ctx, db, func(tx *gorm.DB) error {
		result := tx.Where("device_id = ?", id).Delete(&model.MeshDeviceToken{})
		if result.Error == nil && result.RowsAffected == 0 {
			return fmt.Errorf("%w: enrolled device %d", ErrNotFound, id)
		}
		if result.Error != nil {
			return result.Error
		}
		return tx.Model(&model.MeshDevice{}).Where("id = ?", id).Update("static_key", gorm.Expr("NULL")).Error
	})
}

// DeviceConfig answers the device's networks in the shape of the member
// "mesh" (agent-contract.md section 2): its region's network while the
// region is enabled, none otherwise; an access member, or for a full node
// a full member that listens and advertises its confirmed subnets. A
// device whose address went to another device gets a new one.
//
// The configuration follows the capabilities of the device's kernel:
// features, when the request names them (non-nil), or else those of its
// latest report.
func DeviceConfig(ctx context.Context, db *gorm.DB, device model.MeshDevice, features []string, now time.Time) (map[string]any, error) {
	db = db.WithContext(ctx)
	var region model.MeshRegion
	if err := readRegion(db, device.RegionID, &region); err != nil {
		return nil, err
	}
	if !region.Enabled {
		return map[string]any{"networks": []any{}}, nil
	}
	if device.Address == nil {
		err := change(ctx, db, func(tx *gorm.DB) error {
			// Read again under the region's lock: a request at the same
			// time may have given the device its address already.
			if err := findRegion(tx, region.ID, &region); err != nil {
				return err
			}
			if !region.Enabled {
				return nil
			}
			var current model.MeshDevice
			if err := findDevice(tx, device.ID, &current); err != nil {
				return err
			}
			if current.RegionID != region.ID {
				return conflict("the device moved to another region; read the configuration again")
			}
			if current.Address != nil {
				device.Address = current.Address
				return nil
			}
			address, err := deviceAddress(tx, region, device.ID, now)
			if err != nil {
				return err
			}
			device.Address = &address
			result := tx.Model(&device).Updates(map[string]any{"address": address, "address_used_at": now, "updated_at": now})
			if result.Error == nil && result.RowsAffected != 1 {
				return conflict("the device was removed meanwhile")
			}
			return result.Error
		})
		if err != nil {
			return nil, err
		}
		if !region.Enabled {
			return map[string]any{"networks": []any{}}, nil
		}
	}
	prefix, err := netip.ParsePrefix(region.CIDR)
	if err != nil {
		return nil, fmt.Errorf("region %q: %w", region.Code, err)
	}
	var boundaries []model.MeshMember
	if err := db.Where("region_id = ? AND (home = ? OR boundary = ?)", region.ID, false, true).Order("id ASC").Find(&boundaries).Error; err != nil {
		return nil, err
	}
	peers := []map[string]any{}
	for _, peer := range boundaries {
		info, err := lookupNode(db, peer.NodeRef)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return nil, err
		}
		if info.host != "" {
			peers = append(peers, map[string]any{"server": info.host, "port": Port, "transport": "tcp"})
		}
	}
	dns, err := dnsSection(db)
	if err != nil {
		return nil, err
	}
	node, full, err := isFullNode(db, device.ID)
	if err != nil {
		return nil, err
	}
	role, listen, advertise := RoleAccess, []map[string]any{}, []string{}
	if full {
		role, advertise = RoleFull, splitSubnets(node.Subnets)
		listen = []map[string]any{{"transport": "tcp", "port": Port}, {"transport": "udp", "port": Port}}
	}
	keys, err := nativeKeys(db, []uint{region.ID})
	if err != nil {
		return nil, err
	}
	resolvers, err := resolverAddresses(db, []model.MeshRegion{region})
	if err != nil {
		return nil, err
	}
	capabilities, err := deviceFeatures(db, device.ID)
	if err != nil {
		return nil, err
	}
	if features != nil {
		capabilities = featureSet(features)
	}
	// A region network admits only the keys it lists, which a kernel
	// without mesh_native_keys ignores: such a device, or one that has
	// not named its kernel's capabilities yet, gets no network until its
	// report does (spec section 8.3.1; the Agent does the same for nodes).
	if !capabilities[FeatureNativeKeys] {
		return map[string]any{"networks": []any{}}, nil
	}
	network := map[string]any{
		"id": region.Code, "name": "mesh-" + region.Code, "kind": KindManaged, "native_keys": regionKeys(keys, region.ID),
		"secret": region.Secret, "secure_mode": true, "cipher": cipher,
		"address": fmt.Sprintf("%s/%d", *device.Address, prefix.Bits()), "role": role, "listen": listen,
		"peers":   peers,
		"subnets": map[string]any{"accept": []string{}, "advertise": advertise},
		"routing": map[string]any{"policy": policy, "paths": []any{}},
		"dns":     dns,
	}
	// Custom paths go to a full node whose kernel lists mesh_paths only.
	if full && capabilities[FeaturePaths] {
		paths, err := deliveredPaths(db, deviceEntry(device.ID), region.ID)
		if err != nil {
			return nil, err
		}
		network["routing"] = map[string]any{"policy": policy, "paths": pathsValue(paths)}
	}
	// A field the device's kernel does not take is left out: it refuses
	// a configuration with unknown fields (agent-contract.md section 4).
	switch {
	case !capabilities[FeatureDNS]:
		delete(network, "dns")
	case capabilities[FeatureResolverAddress]:
		network["dns"] = regionDNS(dns, resolvers[region.ID])
	}
	return map[string]any{"networks": []map[string]any{network}}, nil
}

// The kernel capabilities a device names in its report's features that
// add fields to its configuration (spec mesh-status.md).
const (
	FeatureNativeKeys      = "mesh_native_keys"
	FeatureDNS             = "mesh_dns"
	FeatureResolverAddress = "mesh_resolver_address"
)

// deviceFeatures answers the capabilities the device's latest report
// names in "features": none before its first report, or from a report
// without them.
func deviceFeatures(db *gorm.DB, deviceID uint) (map[string]bool, error) {
	var rows []model.MeshStatus
	if err := db.Where("node_ref = ?", DeviceRef(deviceID)).Limit(1).Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return map[string]bool{}, nil
	}
	return reportedFeatures(rows[0].Payload), nil
}

// reportedFeatures answers the capabilities a report names in "features".
func reportedFeatures(payload string) map[string]bool {
	var shape struct {
		Features []string `json:"features"`
	}
	if json.Unmarshal([]byte(payload), &shape) != nil {
		return map[string]bool{}
	}
	return featureSet(shape.Features)
}

// MaxFeatures bounds the capability names taken from one device.
const MaxFeatures = 64

// featureSet answers names as a set, at most MaxFeatures of them.
func featureSet(names []string) map[string]bool {
	features := map[string]bool{}
	for i, name := range names {
		if i == MaxFeatures {
			break
		}
		features[name] = true
	}
	return features
}

// RecordDeviceStatus keeps payload, the device's "mesh.status" report sent
// with its device token, as its latest status; a device without a static
// key gets the one it reports for its region's network while token is
// still its token (nativekeys.go).
func RecordDeviceStatus(ctx context.Context, db *gorm.DB, device model.MeshDevice, token string, payload []byte, now time.Time) error {
	var shape struct {
		Networks []json.RawMessage `json:"networks"`
	}
	if err := json.Unmarshal(payload, &shape); err != nil || shape.Networks == nil {
		return invalid("payload: a JSON object with a networks array")
	}
	if err := checkLatencies(shape.Networks); err != nil {
		return err
	}
	row := model.MeshStatus{NodeRef: DeviceRef(device.ID), Payload: string(payload), ObservedAt: now, ReceivedAt: now}
	if err := db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "node_ref"}},
		DoUpdates: clause.AssignmentColumns([]string{"payload", "observed_at", "received_at"}),
	}).Create(&row).Error; err != nil {
		return err
	}
	return bindDeviceKey(ctx, db.WithContext(ctx), device, token, payload)
}

// RequestDeviceChange records the device's request to move to the region
// of code (changes.go).
func RequestDeviceChange(ctx context.Context, db *gorm.DB, device model.MeshDevice, code, reason string, now time.Time) (model.MeshRegionChange, error) {
	var regions []model.MeshRegion
	if err := db.WithContext(ctx).Where("code = ?", code).Limit(1).Find(&regions).Error; err != nil {
		return model.MeshRegionChange{}, err
	}
	if len(regions) == 0 {
		return model.MeshRegionChange{}, fmt.Errorf("%w: region %q", ErrNotFound, code)
	}
	return RequestChange(ctx, db, ChangeInput{DeviceID: device.ID, ToRegionID: regions[0].ID, Reason: reason}, model.MeshChangeMember, nil, now)
}

// maxReportPeers bounds the peers of one network in a device's report.
const maxReportPeers = 1024

// checkLatencies refuses a report whose peers' latency_ms is not a
// non-negative number or null, or that lists too many peers.
func checkLatencies(networks []json.RawMessage) error {
	for _, raw := range networks {
		var network struct {
			Peers []struct {
				LatencyMS *json.Number `json:"latency_ms"`
			} `json:"peers"`
		}
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if err := decoder.Decode(&network); err != nil {
			return invalid("payload: networks[].peers[].latency_ms is a number of milliseconds")
		}
		if len(network.Peers) > maxReportPeers {
			return invalid("payload: at most %d peers in a network", maxReportPeers)
		}
		for _, peer := range network.Peers {
			if peer.LatencyMS == nil {
				continue
			}
			if value, err := peer.LatencyMS.Float64(); err != nil || value < 0 || value > 600000 {
				return invalid("payload: latency_ms from 0 to 600000")
			}
		}
	}
	return nil
}
