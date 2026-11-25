package meshregion

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

// Devices without an Agent (R29, R31; docs/design/mesh-regions): an
// administrator issues a one-time enrollment credential; the device (a
// node's install command, or a personal device's client) exchanges it once
// for its own device token and its network, and from then on reads its
// network and sends its read-only status with that token. A "node" device
// is a full node with a fixed server segment address; a "personal" device
// is an access device with an address from the device pool, kept by its
// device key and given up after 30 days unused. A device may ask to move
// to another region; an administrator approves or rejects the request.

// Device kinds.
const (
	KindNode     = "node"
	KindPersonal = "personal"
)

const (
	// ReclaimAfter is how long a personal device keeps its address unused.
	ReclaimAfter = 30 * 24 * time.Hour
	// DefaultEnrollmentTTL and MaxEnrollmentTTL bound a credential's life.
	DefaultEnrollmentTTL = 24 * time.Hour
	MaxEnrollmentTTL     = 7 * 24 * time.Hour
	enrollmentPrefix     = "mse_"
	deviceTokenPrefix    = "msd_"
)

// ErrUnauthorized: the credential or device token is unknown, used,
// expired or revoked.
var ErrUnauthorized = errors.New("invalid mesh credential")

var deviceKeyPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{8,128}$`)

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// DeviceRef is the status key of a device.
func DeviceRef(id uint) string { return "device-" + strconv.FormatUint(uint64(id), 10) }

func checkName(name string) error {
	if n := utf8.RuneCountInString(name); n == 0 || n > 64 {
		return invalid("name: 1 to 64 characters")
	}
	return nil
}

// EnrollmentInput issues a credential.
type EnrollmentInput struct {
	// RegionID 0 lets the device's latencies choose.
	RegionID uint   `json:"region_id"`
	Kind     string `json:"kind"`
	Name     string `json:"name"`
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
	if input.Kind != KindNode && input.Kind != KindPersonal {
		return IssuedEnrollment{}, invalid("kind: node or personal")
	}
	if err := checkName(input.Name); err != nil {
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
		var region model.MeshRegion
		if err := findRegion(db, input.RegionID, &region); err != nil {
			return IssuedEnrollment{}, err
		}
	}
	token := enrollmentPrefix + rand.Text()
	row := model.MeshEnrollment{RegionID: input.RegionID, Kind: input.Kind, Name: input.Name, TokenHash: hashToken(token),
		ExpiresAt: now.Add(ttl), CreatedBy: createdBy, CreatedAt: now}
	if err := db.Create(&row).Error; err != nil {
		return IssuedEnrollment{}, err
	}
	return IssuedEnrollment{MeshEnrollment: row, Token: token}, nil
}

// ListEnrollments answers the credentials, the newest first.
func ListEnrollments(ctx context.Context, db *gorm.DB) ([]model.MeshEnrollment, error) {
	var rows []model.MeshEnrollment
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
	// Name replaces the credential's name when set.
	Name string `json:"name"`
	// Latencies are the device's round trips in milliseconds by region
	// code, for a credential without a region.
	Latencies map[string]float64 `json:"latencies"`
}

// Enrolled answers an enrollment: the device's id and token (answered only
// here) and its networks.
type Enrolled struct {
	DeviceID    uint           `json:"device_id"`
	DeviceToken string         `json:"device_token"`
	Config      map[string]any `json:"config"`
}

// Enroll exchanges a credential for a device token. A device key enrolled
// before keeps its row and, in the same region, its address.
func Enroll(ctx context.Context, db *gorm.DB, request EnrollRequest, now time.Time) (Enrolled, error) {
	if !deviceKeyPattern.MatchString(request.DeviceKey) {
		return Enrolled{}, invalid("device_key: 8 to 128 letters, digits or ._:-")
	}
	token := deviceTokenPrefix + rand.Text()
	var device model.MeshDevice
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var enrollments []model.MeshEnrollment
		if err := tx.Where("token_hash = ? AND used_at IS NULL AND expires_at > ?", hashToken(request.Token), now).Limit(1).Find(&enrollments).Error; err != nil {
			return err
		}
		if len(enrollments) == 0 {
			return ErrUnauthorized
		}
		enrollment := enrollments[0]
		region, err := enrollmentRegion(tx, enrollment.RegionID, request.Latencies)
		if err != nil {
			return err
		}
		name := enrollment.Name
		if request.Name != "" {
			if err := checkName(request.Name); err != nil {
				return err
			}
			name = request.Name
		}
		var rows []model.MeshDevice
		if err := tx.Where("device_key = ?", request.DeviceKey).Limit(1).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) > 0 {
			device = rows[0]
			if device.RegionID != region.ID || device.Kind != enrollment.Kind {
				device.Address = ""
			}
		} else {
			device = model.MeshDevice{DeviceKey: request.DeviceKey, CreatedAt: now}
		}
		device.RegionID, device.Kind, device.Name, device.TokenHash = region.ID, enrollment.Kind, name, hashToken(token)
		device.RevokedAt, device.LastSeenAt = nil, &now
		if device.Address == "" {
			if device.Address, err = deviceAddress(tx, region, device.Kind, now); err != nil {
				return err
			}
		}
		if err := tx.Save(&device).Error; err != nil {
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

// deviceAddress gives a device of kind a free address in region: from the
// server segment for a node, from the device pool for a personal device,
// where a personal device unused for ReclaimAfter gives its address up.
func deviceAddress(tx *gorm.DB, region model.MeshRegion, kind string, now time.Time) (string, error) {
	prefix, err := netip.ParsePrefix(region.CIDR)
	if err != nil {
		return "", fmt.Errorf("region %q: %w", region.Code, err)
	}
	used, err := usedAddresses(tx, region.ID)
	if err != nil {
		return "", err
	}
	pool := kind == KindPersonal
	if pool {
		// Stale personal devices' addresses are free.
		var stale []string
		cutoff := now.Add(-ReclaimAfter)
		if err := tx.Model(&model.MeshDevice{}).Where("region_id = ? AND kind = ? AND address <> ? AND (last_seen_at IS NULL OR last_seen_at < ?) AND created_at < ?",
			region.ID, KindPersonal, "", cutoff, cutoff).Pluck("address", &stale).Error; err != nil {
			return "", err
		}
		free := make(map[string]bool, len(stale))
		for _, address := range stale {
			free[address] = true
		}
		kept := used[:0]
		for _, address := range used {
			if !free[address] {
				kept = append(kept, address)
			}
		}
		used = kept
	}
	address, ok := freeAddress(prefix, used, pool)
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrFull, region.Code)
	}
	// The stale holder, if any, gives the address up.
	if err := tx.Model(&model.MeshDevice{}).Where("region_id = ? AND address = ?", region.ID, address.String()).Update("address", "").Error; err != nil {
		return "", err
	}
	return address.String(), nil
}

// Authenticate answers the device of a device token and records that it
// was seen at now.
func Authenticate(ctx context.Context, db *gorm.DB, token string, now time.Time) (model.MeshDevice, error) {
	var rows []model.MeshDevice
	if err := db.WithContext(ctx).Where("token_hash = ? AND revoked_at IS NULL", hashToken(token)).Limit(1).Find(&rows).Error; err != nil {
		return model.MeshDevice{}, err
	}
	if len(rows) == 0 {
		return model.MeshDevice{}, ErrUnauthorized
	}
	device := rows[0]
	device.LastSeenAt = &now
	return device, db.WithContext(ctx).Model(&device).Update("last_seen_at", now).Error
}

// DeviceConfig answers the device's networks in the shape of the member
// "mesh" (agent-contract.md section 2): its region's network while the
// region is enabled, none otherwise. A device whose address was reclaimed
// gets a new one.
func DeviceConfig(ctx context.Context, db *gorm.DB, device model.MeshDevice, now time.Time) (map[string]any, error) {
	db = db.WithContext(ctx)
	var region model.MeshRegion
	if err := findRegion(db, device.RegionID, &region); err != nil {
		return nil, err
	}
	if !region.Enabled {
		return map[string]any{"networks": []any{}}, nil
	}
	if device.Address == "" {
		err := db.Transaction(func(tx *gorm.DB) error {
			address, err := deviceAddress(tx, region, device.Kind, now)
			if err != nil {
				return err
			}
			device.Address = address
			return tx.Model(&device).Update("address", address).Error
		})
		if err != nil {
			return nil, err
		}
	}
	prefix, err := netip.ParsePrefix(region.CIDR)
	if err != nil {
		return nil, fmt.Errorf("region %q: %w", region.Code, err)
	}
	peers, err := boundaryPeers(db, []uint{region.ID}, "")
	if err != nil {
		return nil, err
	}
	return map[string]any{"networks": []map[string]any{network(region, prefix, device.Address, device.Kind == KindNode, peers[region.ID], []string{})}}, nil
}

// MoveDevice moves a device to another region, where it gets a new address.
func MoveDevice(ctx context.Context, db *gorm.DB, id, regionID uint, now time.Time) (model.MeshDevice, error) {
	var device model.MeshDevice
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		device, err = moveDevice(tx, id, regionID, now)
		return err
	})
	return device, err
}

func moveDevice(tx *gorm.DB, id, regionID uint, now time.Time) (model.MeshDevice, error) {
	device, err := findDevice(tx, id)
	if err != nil || device.RegionID == regionID {
		return device, err
	}
	var region model.MeshRegion
	if err := findRegion(tx, regionID, &region); err != nil {
		return device, err
	}
	address, err := deviceAddress(tx, region, device.Kind, now)
	if err != nil {
		return device, err
	}
	device.RegionID, device.Address = regionID, address
	return device, tx.Model(&device).Updates(map[string]any{"region_id": regionID, "address": address}).Error
}

// RevokeDevice refuses the device from now on and releases its address.
func RevokeDevice(ctx context.Context, db *gorm.DB, id uint, now time.Time) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		device, err := findDevice(tx, id)
		if err != nil {
			return err
		}
		return tx.Model(&device).Updates(map[string]any{"revoked_at": now, "address": ""}).Error
	})
}

func findDevice(tx *gorm.DB, id uint) (model.MeshDevice, error) {
	var rows []model.MeshDevice
	if err := tx.Where("id = ?", id).Limit(1).Find(&rows).Error; err != nil {
		return model.MeshDevice{}, err
	}
	if len(rows) == 0 {
		return model.MeshDevice{}, fmt.Errorf("%w: device %d", ErrNotFound, id)
	}
	return rows[0], nil
}

// RequestRegionChange records the device's request to move to the region
// code; a device has at most one pending request.
func RequestRegionChange(ctx context.Context, db *gorm.DB, device model.MeshDevice, code, reason string, now time.Time) (model.MeshRegionChange, error) {
	if utf8.RuneCountInString(reason) > 200 {
		return model.MeshRegionChange{}, invalid("reason: at most 200 characters")
	}
	var row model.MeshRegionChange
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var regions []model.MeshRegion
		if err := tx.Where("code = ?", code).Limit(1).Find(&regions).Error; err != nil {
			return err
		}
		if len(regions) == 0 {
			return fmt.Errorf("%w: region %q", ErrNotFound, code)
		}
		if regions[0].ID == device.RegionID {
			return conflict("the device is already in region %q", code)
		}
		var count int64
		if err := tx.Model(&model.MeshRegionChange{}).Where("device_id = ? AND state = ?", device.ID, "pending").Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return conflict("the device has a pending request")
		}
		row = model.MeshRegionChange{DeviceID: device.ID, FromRegionID: device.RegionID, ToRegionID: regions[0].ID, Reason: reason, State: "pending", CreatedAt: now}
		return tx.Create(&row).Error
	})
	return row, err
}

// ListRegionChanges answers the requests, the newest first.
func ListRegionChanges(ctx context.Context, db *gorm.DB) ([]model.MeshRegionChange, error) {
	var rows []model.MeshRegionChange
	return rows, db.WithContext(ctx).Order("id DESC").Limit(200).Find(&rows).Error
}

// DecideRegionChange approves (the device moves) or rejects a pending
// request.
func DecideRegionChange(ctx context.Context, db *gorm.DB, id uint, approve bool, decidedBy *uint, now time.Time) (model.MeshRegionChange, error) {
	var row model.MeshRegionChange
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []model.MeshRegionChange
		if err := tx.Where("id = ? AND state = ?", id, "pending").Limit(1).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return fmt.Errorf("%w: pending request %d", ErrNotFound, id)
		}
		row = rows[0]
		row.State, row.DecidedBy, row.DecidedAt = "rejected", decidedBy, &now
		if approve {
			row.State = "approved"
			if _, err := moveDevice(tx, row.DeviceID, row.ToRegionID, now); err != nil {
				return err
			}
		}
		return tx.Save(&row).Error
	})
	return row, err
}
