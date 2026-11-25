package meshregion

import (
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
// stays while it is used, and every read or report uses it.

const (
	// DefaultEnrollmentTTL and MaxEnrollmentTTL bound a credential's life.
	DefaultEnrollmentTTL = 24 * time.Hour
	MaxEnrollmentTTL     = 7 * 24 * time.Hour
	enrollmentPrefix     = "mse_"
	deviceTokenPrefix    = "msd_"
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

// EnrollmentInput issues a credential.
type EnrollmentInput struct {
	// RegionID 0 lets the device's latencies choose.
	RegionID uint   `json:"region_id"`
	Hostname string `json:"hostname"`
	// Hours the credential stays valid: 1 to 168, 24 when 0.
	Hours int `json:"hours"`
}

// IssuedEnrollment is a credential just issued with its token, which no
// later answer carries.
type IssuedEnrollment struct {
	model.MeshEnrollment
	Token string `json:"token"`
}

// CreateEnrollment issues a one-time credential.
func CreateEnrollment(ctx context.Context, db *gorm.DB, input EnrollmentInput, createdBy *uint, now time.Time) (IssuedEnrollment, error) {
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
	db = db.WithContext(ctx)
	if input.RegionID != 0 {
		if err := findRegion(db, input.RegionID, &model.MeshRegion{}); err != nil {
			return IssuedEnrollment{}, err
		}
	}
	token := enrollmentPrefix + rand.Text()
	row := model.MeshEnrollment{RegionID: input.RegionID, Hostname: hostname, TokenHash: hashToken(token), ExpiresAt: now.Add(ttl), CreatedBy: createdBy, CreatedAt: now}
	if err := db.Create(&row).Error; err != nil {
		return IssuedEnrollment{}, err
	}
	return IssuedEnrollment{MeshEnrollment: row, Token: token}, nil
}

// Enrollments answers the newest 200 credentials.
func Enrollments(ctx context.Context, db *gorm.DB) ([]model.MeshEnrollment, error) {
	rows := []model.MeshEnrollment{}
	return rows, db.WithContext(ctx).Order("id DESC").Limit(200).Find(&rows).Error
}

// DeleteEnrollment withdraws a credential; a device that used it stays.
func DeleteEnrollment(ctx context.Context, db *gorm.DB, id uint) error {
	result := db.WithContext(ctx).Where("id = ?", id).Delete(&model.MeshEnrollment{})
	if result.Error == nil && result.RowsAffected == 0 {
		return fmt.Errorf("%w: enrollment %d", ErrNotFound, id)
	}
	return result.Error
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

// Enroll exchanges a credential for a device token. A device key enrolled
// before keeps its device, region and address; its earlier token stops
// working.
func Enroll(ctx context.Context, db *gorm.DB, request EnrollRequest, now time.Time) (Enrolled, error) {
	_, key, err := normalizeDevice(DeviceInput{Hostname: "x", DeviceKey: request.DeviceKey})
	if err != nil {
		return Enrolled{}, err
	}
	if key == nil {
		return Enrolled{}, invalid("device_key: required")
	}
	token := deviceTokenPrefix + rand.Text()
	var device model.MeshDevice
	err = change(ctx, db, func(tx *gorm.DB) error {
		var enrollments []model.MeshEnrollment
		if err := tx.Where("token_hash = ? AND used_at IS NULL AND expires_at > ?", hashToken(request.Token), now).Limit(1).Find(&enrollments).Error; err != nil {
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
		if len(known) > 0 {
			device = known[0]
		} else {
			region, err := enrollmentRegion(tx, enrollment.RegionID, request.Latencies)
			if err != nil {
				return err
			}
			address, err := allocate(tx, region, now)
			if err != nil {
				return err
			}
			device = model.MeshDevice{RegionID: region.ID, DeviceKey: key, Hostname: enrollment.Hostname, Address: &address, AddressUsedAt: &now, CreatedAt: now, UpdatedAt: now}
			if err := tx.Create(&device).Error; err != nil {
				return err
			}
		}
		row := model.MeshDeviceToken{DeviceID: device.ID, TokenHash: hashToken(token), LastSeenAt: &now, CreatedAt: now}
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "device_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"token_hash", "last_seen_at", "created_at"}),
		}).Create(&row).Error; err != nil {
			return err
		}
		return tx.Model(&enrollment).Updates(map[string]any{"used_at": now, "device_id": device.ID}).Error
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
	if err := db.Model(&tokens[0]).Update("last_seen_at", now).Error; err != nil {
		return device, err
	}
	if device.Address != nil {
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
// "mesh" (agent-contract.md section 2): its region's network as an access
// member while the region is enabled, none otherwise. A device whose
// address went to another device gets a new one.
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
			address, err := allocate(tx, region, now)
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
	return map[string]any{"networks": []map[string]any{{
		"id": region.Code, "name": "mesh-" + region.Code, "secret": region.Secret, "secure_mode": true, "cipher": cipher,
		"address": fmt.Sprintf("%s/%d", *device.Address, prefix.Bits()), "role": "access", "listen": []map[string]any{},
		"peers":   peers,
		"subnets": map[string]any{"accept": []string{}, "advertise": []string{}},
		"routing": map[string]any{"policy": policy, "paths": []any{}},
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
