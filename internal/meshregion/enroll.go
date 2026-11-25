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
	"strconv"
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
}

// EnrollmentView is a credential with the role of the device it enrolls
// and a full node's fixed address.
type EnrollmentView struct {
	model.MeshEnrollment
	Role    string `json:"role"`
	Address string `json:"address"`
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
	token := enrollmentPrefix + rand.Text()
	row := model.MeshEnrollment{RegionID: input.RegionID, Hostname: hostname, TokenHash: hashToken(token), DeviceID: target, ExpiresAt: now.Add(ttl), CreatedBy: createdBy, CreatedAt: now}
	address := input.Address
	// No node's configuration changes: a plain transaction, no push.
	err = db.Transaction(func(tx *gorm.DB) error {
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
	return IssuedEnrollment{EnrollmentView: EnrollmentView{MeshEnrollment: row, Role: role, Address: address}, Token: token}, nil
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
	views := make([]EnrollmentView, 0, len(rows))
	for _, row := range rows {
		view := EnrollmentView{MeshEnrollment: row, Role: RoleAccess}
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
	if len(request.Latencies) > maxLatencies {
		return Enrolled{}, invalid("latencies: at most %d regions", maxLatencies)
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
			if device.DeviceKey == nil {
				device.DeviceKey = key
				if err := tx.Model(&device).Updates(map[string]any{"device_key": *key, "updated_at": now}).Error; err != nil {
					return err
				}
			}
		case len(known) > 0:
			return conflict("another device has this device_key; enroll it again with a credential issued for it")
		default:
			region, err := enrollmentRegion(tx, enrollment.RegionID, request.Latencies)
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
			device = model.MeshDevice{RegionID: region.ID, DeviceKey: key, Hostname: enrollment.Hostname, Address: &address, AddressUsedAt: &now, CreatedAt: now, UpdatedAt: now}
			if err := tx.Create(&device).Error; err != nil {
				return err
			}
			if len(nodes) > 0 {
				if err := tx.Model(&nodes[0]).Updates(map[string]any{"device_id": device.ID, "address": address, "updated_at": now}).Error; err != nil {
					return err
				}
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
	config, err := DeviceConfig(ctx, db, device, now)
	return Enrolled{DeviceID: device.ID, DeviceToken: token, Config: config}, err
}

// enrollmentRegion is the credential's region, or the enabled region of
// lowest latency.
func enrollmentRegion(tx *gorm.DB, id uint, latencies map[string]float64) (model.MeshRegion, error) {
	var region model.MeshRegion
	if id != 0 {
		return region, findRegion(tx, id, &region)
	}
	var regions []model.MeshRegion
	if err := tx.Where("enabled = ?", true).Order("sort ASC, id ASC").Find(&regions).Error; err != nil {
		return region, err
	}
	best := -1.0
	for _, candidate := range regions {
		if latency, ok := latencies[candidate.Code]; ok && latency >= 0 && (best < 0 || latency < best) {
			region, best = candidate, latency
		}
	}
	if best < 0 {
		return region, invalid("latencies: the round trip to at least one enabled region, by code")
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
// enrolls again only with a new credential.
func RevokeDevice(ctx context.Context, db *gorm.DB, id uint) error {
	result := db.WithContext(ctx).Where("device_id = ?", id).Delete(&model.MeshDeviceToken{})
	if result.Error == nil && result.RowsAffected == 0 {
		return fmt.Errorf("%w: enrolled device %d", ErrNotFound, id)
	}
	return result.Error
}

// DeviceConfig answers the device's networks in the shape of the member
// "mesh" (agent-contract.md section 2): its region's network while the
// region is enabled, none otherwise; an access member, or for a full node
// a full member that listens and advertises its confirmed subnets. A
// device whose address went to another device gets a new one.
func DeviceConfig(ctx context.Context, db *gorm.DB, device model.MeshDevice, now time.Time) (map[string]any, error) {
	db = db.WithContext(ctx)
	var region model.MeshRegion
	if err := findRegion(db, device.RegionID, &region); err != nil {
		return nil, err
	}
	if !region.Enabled {
		return map[string]any{"networks": []any{}}, nil
	}
	if device.Address == nil {
		err := change(ctx, db, func(tx *gorm.DB) error {
			address, err := deviceAddress(tx, region, device.ID, now)
			if err != nil {
				return err
			}
			device.Address = &address
			return tx.Model(&device).Updates(map[string]any{"address": address, "address_used_at": now, "updated_at": now}).Error
		})
		if err != nil {
			return nil, err
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
	return map[string]any{"networks": []map[string]any{{
		"id": region.Code, "name": "mesh-" + region.Code, "secret": region.Secret, "secure_mode": true, "cipher": cipher,
		"address": fmt.Sprintf("%s/%d", *device.Address, prefix.Bits()), "role": role, "listen": listen,
		"peers":   peers,
		"subnets": map[string]any{"accept": []string{}, "advertise": advertise},
		"routing": map[string]any{"policy": policy, "paths": []any{}},
		"dns":     dns,
	}}}, nil
}

// RecordDeviceStatus keeps payload, the device's "mesh.status" report, as
// its latest status.
func RecordDeviceStatus(ctx context.Context, db *gorm.DB, device model.MeshDevice, payload []byte, now time.Time) error {
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
	return db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "node_ref"}},
		DoUpdates: clause.AssignmentColumns([]string{"payload", "observed_at", "received_at"}),
	}).Create(&row).Error
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
