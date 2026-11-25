package meshregion

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

// PoolReclaimAfter is how long a device's pool address stays its own
// without use (R29): only an address unused that long goes to another
// device, and only when the pool has no free address.
const PoolReclaimAfter = 30 * 24 * time.Hour

// ErrPoolFull: the region's device pool has neither a free address nor one
// unused for PoolReclaimAfter.
var ErrPoolFull = errors.New("the region's device pool is full")

// Device warnings.
const (
	// WarningNoAddress: the device's address went to another device; it
	// gets a new one when it next joins.
	WarningNoAddress = "no_address"
	// WarningAddressIdle: unused for PoolReclaimAfter; the address may go
	// to another device.
	WarningAddressIdle = "address_idle"
)

var deviceKeyPattern = regexp.MustCompile(`^[A-Za-z0-9+/=_-]{16,128}$`)

// DeviceInput creates or changes a device.
type DeviceInput struct {
	Hostname string `json:"hostname"`
	// DeviceKey is the device's identity key, empty for none.
	DeviceKey string `json:"device_key"`
}

// DeviceView is a device with its region and name.
type DeviceView struct {
	model.MeshDevice
	RegionCode string   `json:"region_code"`
	FQDN       string   `json:"fqdn"`
	Warnings   []string `json:"warnings"`
	// Enrolled: the device holds a device token (enroll.go); LastSeenAt is
	// its last read or report, Status its last report.
	Enrolled         bool            `json:"enrolled"`
	LastSeenAt       *time.Time      `json:"last_seen_at"`
	Status           json.RawMessage `json:"status,omitempty" swaggertype:"object"`
	StatusReceivedAt *time.Time      `json:"status_received_at,omitempty"`
	// Role is access (a pool address) or full (a fixed address of the
	// server segment, R32); Subnets are a full node's confirmed subnets.
	Role    string   `json:"role"`
	Subnets []string `json:"subnets"`
}

// pool answers the first and last address of the device pool of prefix:
// its upper half without the broadcast address.
func pool(prefix netip.Prefix) (first, last uint32) {
	half := uint32(1) << (31 - prefix.Bits())
	base := prefix.Masked().Addr().As4()
	start := binary.BigEndian.Uint32(base[:])
	return start + half, start + 2*half - 2
}

// PoolSize is how many addresses the device pool of prefix holds.
func PoolSize(prefix netip.Prefix) int {
	return 1<<(31-prefix.Bits()) - 1
}

func addrOf(value uint32) netip.Addr {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], value)
	return netip.AddrFrom4(b)
}

func normalizeDevice(input DeviceInput) (string, *string, error) {
	hostname, err := normalizeLabel("hostname", input.Hostname)
	if err != nil {
		return "", nil, err
	}
	key := strings.TrimSpace(input.DeviceKey)
	if key == "" {
		return hostname, nil, nil
	}
	if !deviceKeyPattern.MatchString(key) {
		return "", nil, invalid("device_key: 16 to 128 base64 characters")
	}
	return hostname, &key, nil
}

// checkDeviceKey refuses a key another device (not id) has.
func checkDeviceKey(tx *gorm.DB, id uint, key *string) error {
	if key == nil {
		return nil
	}
	var count int64
	if err := tx.Model(&model.MeshDevice{}).Where("device_key = ? AND id <> ?", *key, id).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return conflict("another device has this device_key")
	}
	return nil
}

// allocate answers a pool address of region for a device: the lowest free
// one, or else the one unused longest if unused for PoolReclaimAfter, which
// its device loses.
func allocate(tx *gorm.DB, region model.MeshRegion, now time.Time) (string, error) {
	prefix, err := netip.ParsePrefix(region.CIDR)
	if err != nil {
		return "", fmt.Errorf("region %q: %w", region.Code, err)
	}
	if err := lockRegion(tx, region.ID); err != nil {
		return "", err
	}
	var holders []model.MeshDevice
	if err := tx.Where("region_id = ? AND address IS NOT NULL", region.ID).Find(&holders).Error; err != nil {
		return "", err
	}
	taken := make(map[string]bool, len(holders))
	for _, holder := range holders {
		taken[*holder.Address] = true
	}
	first, last := pool(prefix)
	for value := first; value <= last; value++ {
		if address := addrOf(value).String(); !taken[address] {
			return address, nil
		}
	}
	var idlest *model.MeshDevice
	for i := range holders {
		holder := &holders[i]
		if holder.AddressUsedAt != nil && now.Sub(*holder.AddressUsedAt) < PoolReclaimAfter {
			continue
		}
		// A full node's fixed address is never reclaimed.
		if address, err := netip.ParseAddr(*holder.Address); err != nil || address.Less(addrOf(first)) {
			continue
		}
		if idlest == nil || used(holder).Before(used(idlest)) {
			idlest = holder
		}
	}
	if idlest == nil {
		return "", fmt.Errorf("%w: %s", ErrPoolFull, region.Code)
	}
	address := *idlest.Address
	if err := tx.Model(&model.MeshDevice{}).Where("id = ?", idlest.ID).
		Updates(map[string]any{"address": nil, "address_used_at": nil, "updated_at": now}).Error; err != nil {
		return "", err
	}
	return address, nil
}

func used(device *model.MeshDevice) time.Time {
	if device.AddressUsedAt == nil {
		return time.Time{}
	}
	return *device.AddressUsedAt
}

// CreateDevice adds a device to region regionID with a pool address.
func CreateDevice(ctx context.Context, db *gorm.DB, regionID uint, input DeviceInput, now time.Time) (model.MeshDevice, error) {
	hostname, key, err := normalizeDevice(input)
	if err != nil {
		return model.MeshDevice{}, err
	}
	var row model.MeshDevice
	err = change(ctx, db, func(tx *gorm.DB) error {
		var region model.MeshRegion
		if err := findRegion(tx, regionID, &region); err != nil {
			return err
		}
		if err := checkDeviceKey(tx, 0, key); err != nil {
			return err
		}
		address, err := allocate(tx, region, now)
		if err != nil {
			return err
		}
		row = model.MeshDevice{RegionID: region.ID, DeviceKey: key, Hostname: hostname, Address: &address, AddressUsedAt: &now, CreatedAt: now, UpdatedAt: now}
		return tx.Create(&row).Error
	})
	return row, err
}

// UpdateDevice changes a device's hostname and key; its region changes
// only through a region change.
func UpdateDevice(ctx context.Context, db *gorm.DB, id uint, input DeviceInput, now time.Time) (model.MeshDevice, error) {
	hostname, key, err := normalizeDevice(input)
	if err != nil {
		return model.MeshDevice{}, err
	}
	var row model.MeshDevice
	err = change(ctx, db, func(tx *gorm.DB) error {
		if err := findDevice(tx, id, &row); err != nil {
			return err
		}
		if err := checkDeviceKey(tx, id, key); err != nil {
			return err
		}
		row.Hostname, row.DeviceKey, row.UpdatedAt = hostname, key, now
		return tx.Save(&row).Error
	})
	return row, err
}

// DeleteDevice removes a device; its pending region changes are rejected.
func DeleteDevice(ctx context.Context, db *gorm.DB, id uint, now time.Time) error {
	return change(ctx, db, func(tx *gorm.DB) error {
		var row model.MeshDevice
		if err := findDevice(tx, id, &row); err != nil {
			return err
		}
		// The device's row before its changes, in the order an approval
		// of a change takes them.
		if err := lockRows(tx, &model.MeshDevice{}, "id = ?", id); err != nil {
			return err
		}
		if err := tx.Model(&model.MeshRegionChange{}).Where("device_id = ? AND status = ?", id, model.MeshChangePending).
			Updates(map[string]any{"status": model.MeshChangeRejected, "note": "the device was removed", "decided_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Where("device_id = ?", id).Delete(&model.MeshDeviceToken{}).Error; err != nil {
			return err
		}
		if err := tx.Where("node_ref = ?", DeviceRef(id)).Delete(&model.MeshStatus{}).Error; err != nil {
			return err
		}
		// A full node's credential stays as its record, without the device.
		if err := tx.Model(&model.MeshFullNode{}).Where("device_id = ?", id).Updates(map[string]any{"device_id": nil, "updated_at": now}).Error; err != nil {
			return err
		}
		return tx.Delete(&row).Error
	})
}

// Devices answers the devices of region regionID, or of every region for
// 0, by region and address.
func Devices(ctx context.Context, db *gorm.DB, regionID uint, now time.Time) ([]DeviceView, error) {
	db = db.WithContext(ctx)
	settings, err := loadSettings(db)
	if err != nil {
		return nil, err
	}
	codes, err := regionCodes(db)
	if err != nil {
		return nil, err
	}
	query := db.Order("region_id ASC, address ASC, id ASC")
	if regionID != 0 {
		query = query.Where("region_id = ?", regionID)
	}
	var rows []model.MeshDevice
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	var tokens []model.MeshDeviceToken
	if err := db.Find(&tokens).Error; err != nil {
		return nil, err
	}
	seen := make(map[uint]*time.Time, len(tokens))
	refs := make([]string, 0, len(tokens))
	for _, token := range tokens {
		seen[token.DeviceID] = token.LastSeenAt
		refs = append(refs, DeviceRef(token.DeviceID))
	}
	var statuses []model.MeshStatus
	if err := db.Where("node_ref IN ?", refs).Find(&statuses).Error; err != nil {
		return nil, err
	}
	reports := make(map[string]model.MeshStatus, len(statuses))
	for _, status := range statuses {
		reports[status.NodeRef] = status
	}
	ids := make([]uint, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	full, err := fullNodes(db, ids)
	if err != nil {
		return nil, err
	}
	views := make([]DeviceView, 0, len(rows))
	for _, row := range rows {
		view := DeviceView{MeshDevice: row, RegionCode: codes[row.RegionID], Warnings: []string{}, Role: RoleAccess, Subnets: []string{}}
		node, isFull := full[row.ID]
		if isFull {
			view.Role, view.Subnets = RoleFull, splitSubnets(node.Subnets)
		}
		view.LastSeenAt, view.Enrolled = seen[row.ID]
		if status, ok := reports[DeviceRef(row.ID)]; ok {
			view.Status, view.StatusReceivedAt = json.RawMessage(status.Payload), &status.ReceivedAt
		}
		view.FQDN = fqdn(row.Hostname, view.RegionCode, settings.NameSuffix)
		switch {
		case row.Address == nil:
			view.Warnings = append(view.Warnings, WarningNoAddress)
		case !isFull && (row.AddressUsedAt == nil || now.Sub(*row.AddressUsedAt) >= PoolReclaimAfter):
			view.Warnings = append(view.Warnings, WarningAddressIdle)
		}
		views = append(views, view)
	}
	return views, nil
}

func findDevice(tx *gorm.DB, id uint, row *model.MeshDevice) error {
	var rows []model.MeshDevice
	if err := tx.Where("id = ?", id).Limit(1).Find(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return fmt.Errorf("%w: device %d", ErrNotFound, id)
	}
	*row = rows[0]
	return nil
}

func regionCodes(tx *gorm.DB) (map[uint]string, error) {
	var regions []model.MeshRegion
	if err := tx.Select("id", "code").Find(&regions).Error; err != nil {
		return nil, err
	}
	codes := make(map[uint]string, len(regions))
	for _, region := range regions {
		codes[region.ID] = region.Code
	}
	return codes, nil
}

func fqdn(name, code, suffix string) string {
	if name == "" || code == "" {
		return ""
	}
	return name + "." + code + "." + suffix
}

func regionHasDevices(tx *gorm.DB, id uint) error {
	var count int64
	if err := tx.Model(&model.MeshDevice{}).Where("region_id = ?", id).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return conflict("the region has %d devices; move or remove them first", count)
	}
	return nil
}

// RegionPool is a region's device pool usage.
type RegionPool struct {
	RegionID uint `json:"region_id"`
	Size     int  `json:"size"`
	Used     int  `json:"used"`
	// Low: less than a tenth of the pool is free.
	Low bool `json:"low"`
}

// Summary is the mesh at a glance: the settings, each region's device
// pool and how many region changes wait for a decision.
type Summary struct {
	Settings       Settings     `json:"settings"`
	Pools          []RegionPool `json:"pools"`
	PendingChanges int64        `json:"pending_changes"`
}

// LoadSummary answers the summary.
func LoadSummary(ctx context.Context, db *gorm.DB) (Summary, error) {
	db = db.WithContext(ctx)
	settings, err := loadSettings(db)
	if err != nil {
		return Summary{}, err
	}
	var regions []model.MeshRegion
	if err := db.Order("sort ASC, id ASC").Find(&regions).Error; err != nil {
		return Summary{}, err
	}
	var counts []struct {
		RegionID uint
		Count    int
	}
	if err := db.Model(&model.MeshDevice{}).Select("region_id, COUNT(*) AS count").Where("address IS NOT NULL").Group("region_id").Scan(&counts).Error; err != nil {
		return Summary{}, err
	}
	usedBy := map[uint]int{}
	for _, count := range counts {
		usedBy[count.RegionID] = count.Count
	}
	summary := Summary{Settings: settings, Pools: make([]RegionPool, 0, len(regions))}
	for _, region := range regions {
		prefix, err := netip.ParsePrefix(region.CIDR)
		if err != nil {
			continue
		}
		size := PoolSize(prefix)
		summary.Pools = append(summary.Pools, RegionPool{RegionID: region.ID, Size: size, Used: usedBy[region.ID], Low: (size-usedBy[region.ID])*10 < size})
	}
	if err := db.Model(&model.MeshRegionChange{}).Where("status = ?", model.MeshChangePending).Count(&summary.PendingChanges).Error; err != nil {
		return Summary{}, err
	}
	return summary, nil
}

// maxTouched bounds the peer addresses one status refreshes per network.
const maxTouched = 4096

// touchDevices marks the pool addresses a node's status lists among its
// peers as used at observedAt (R29: a pool address goes to another device
// only after PoolReclaimAfter unused).
func touchDevices(db *gorm.DB, payload []byte, observedAt time.Time) error {
	var shape struct {
		Networks []struct {
			ID    string `json:"id"`
			Peers []struct {
				Address string `json:"address"`
			} `json:"peers"`
		} `json:"networks"`
	}
	if json.Unmarshal(payload, &shape) != nil {
		return nil
	}
	var regions []model.MeshRegion
	if err := db.Select("id", "code").Find(&regions).Error; err != nil {
		return err
	}
	ids := make(map[string]uint, len(regions))
	for _, region := range regions {
		ids[region.Code] = region.ID
	}
	for _, network := range shape.Networks {
		regionID, ok := ids[network.ID]
		if !ok {
			continue
		}
		addresses := []string{}
		for _, peer := range network.Peers {
			raw, _, _ := strings.Cut(peer.Address, "/")
			if address, err := netip.ParseAddr(raw); err == nil && address.Is4() && len(addresses) < maxTouched {
				addresses = append(addresses, address.String())
			}
		}
		if len(addresses) == 0 {
			continue
		}
		if err := db.Model(&model.MeshDevice{}).
			Where("region_id = ? AND address IN ? AND (address_used_at IS NULL OR address_used_at < ?)", regionID, addresses, observedAt).
			Update("address_used_at", observedAt).Error; err != nil {
			return err
		}
	}
	return nil
}

// AssignDeviceAddress gives a device that lost its pool address a new one
// of its region.
func AssignDeviceAddress(ctx context.Context, db *gorm.DB, id uint, now time.Time) (model.MeshDevice, error) {
	var row model.MeshDevice
	err := change(ctx, db, func(tx *gorm.DB) error {
		if err := findDevice(tx, id, &row); err != nil {
			return err
		}
		// Read again under the region's lock, which a request choosing
		// the device an address at the same time also takes.
		var region model.MeshRegion
		if err := findLockedRegion(tx, row.RegionID, &region); err != nil {
			return err
		}
		if err := findDevice(tx, id, &row); err != nil {
			return err
		}
		if row.RegionID != region.ID {
			return conflict("the device moved to another region; try again")
		}
		if row.Address != nil {
			return conflict("the device has the address %s", *row.Address)
		}
		address, err := deviceAddress(tx, region, row.ID, now)
		if err != nil {
			return err
		}
		row.Address, row.AddressUsedAt, row.UpdatedAt = &address, &now, now
		return tx.Save(&row).Error
	})
	return row, err
}

// deviceAddress answers a new address of region for the device: a fixed
// address of the server segment for a full node, a pool address otherwise.
func deviceAddress(tx *gorm.DB, region model.MeshRegion, deviceID uint, now time.Time) (string, error) {
	node, full, err := isFullNode(tx, deviceID)
	if err != nil {
		return "", err
	}
	if !full {
		return allocate(tx, region, now)
	}
	address, err := serverAddress(tx, region, "", 0, now)
	if err != nil {
		return "", err
	}
	return address, tx.Model(&node).Updates(map[string]any{"address": address, "updated_at": now}).Error
}
