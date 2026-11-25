package meshregion

import (
	"context"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Full nodes without an Agent (R32, R29): an OpenWrt router or a server
// enrolls with a credential of the role full. It gets a fixed address of
// its region's server segment, like the Agent nodes, listens, relays, and
// advertises only the subnets an administrator confirmed (R17). Its
// address never goes back to the device pool.

// Device roles.
const (
	RoleAccess = "access"
	RoleFull   = "full"
)

// maxFullNodeSubnets bounds the subnets a full node advertises.
const maxFullNodeSubnets = 32

// subnetsLock is the PostgreSQL advisory lock key of the address plan
// (lockAddressPlan; "mesh_sn" in ASCII).
const subnetsLock int64 = 0x6d6573685f736e

// lockRegion locks the region's row until the transaction ends, so that
// two transactions choosing an address of the region (a node's, a full
// node's or a pool address) take turns (PostgreSQL; SQLite writes one
// transaction at a time). The members, the devices and the credentials
// keep their addresses in separate tables, which no one unique index
// covers. It answers ErrNotFound for a region that does not exist.
func lockRegion(tx *gorm.DB, regionID uint) error {
	var row model.MeshRegion
	return findLockedRegion(tx, regionID, &row)
}

// findLockedRegion reads the region after locking its row (lockRegion), so
// that what it answers stays true until the transaction ends.
func findLockedRegion(tx *gorm.DB, id uint, row *model.MeshRegion) error {
	query := tx
	if tx.Name() == "postgres" {
		query = tx.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var rows []model.MeshRegion
	if err := query.Where("id = ?", id).Limit(1).Find(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return fmt.Errorf("%w: region %d", ErrNotFound, id)
	}
	*row = rows[0]
	return nil
}

// lockAllRegions locks every region's row in ascending id order. A change
// that may choose addresses in more than one region (a boundary node, a
// move between regions) takes it first, so that two such changes, or one
// and a change of a single region, always lock regions in the same order
// and never wait on each other in a cycle. It takes the address plan lock
// before (lockAddressPlan), which a new region takes too: a boundary node
// that joins every region and a region created at the same time take
// turns, so the node joins the new region.
func lockAllRegions(tx *gorm.DB) error {
	if tx.Name() != "postgres" {
		return nil
	}
	if err := lockAddressPlan(tx); err != nil {
		return err
	}
	return tx.Clauses(clause.Locking{Strength: "UPDATE"}).Order("id ASC").Find(&[]model.MeshRegion{}).Error
}

// lockAddressPlan serializes the changes of the address plan: the regions'
// ranges, the host group supernet, the reserved ranges and the full nodes'
// subnets (PostgreSQL advisory lock), so that the overlap checks of two
// changes at once each see the other. A transaction takes it before any
// region lock.
func lockAddressPlan(tx *gorm.DB) error {
	if tx.Name() != "postgres" {
		return nil
	}
	return tx.Exec("SELECT pg_advisory_xact_lock(?)", subnetsLock).Error
}

// checkFullNodeSubnets refuses ranges that overlap a full node's confirmed
// subnets.
func checkFullNodeSubnets(tx *gorm.DB, field string, ranges ...netip.Prefix) error {
	var rows []model.MeshFullNode
	if err := tx.Where("subnets <> ''").Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		for _, raw := range splitSubnets(row.Subnets) {
			subnet, err := netip.ParsePrefix(raw)
			if err != nil {
				continue
			}
			for _, prefix := range ranges {
				if prefix.Overlaps(subnet) {
					return conflict("%s: %s overlaps the confirmed subnet %s of a full node", field, prefix, raw)
				}
			}
		}
	}
	return nil
}

// usedServerAddresses answers the addresses of region the members and the
// devices hold, and those that unused, unexpired full node credentials
// other than except keep, which a new fixed address must avoid. It locks
// the region first.
func usedServerAddresses(tx *gorm.DB, regionID, except uint, now time.Time) ([]string, error) {
	if err := lockRegion(tx, regionID); err != nil {
		return nil, err
	}
	var used []string
	if err := tx.Model(&model.MeshMember{}).Where("region_id = ?", regionID).Pluck("address", &used).Error; err != nil {
		return nil, err
	}
	var devices []string
	if err := tx.Model(&model.MeshDevice{}).Where("region_id = ? AND address IS NOT NULL", regionID).Pluck("address", &devices).Error; err != nil {
		return nil, err
	}
	var kept []string
	if err := tx.Model(&model.MeshFullNode{}).
		Joins("JOIN v5_mesh_enrollment ON v5_mesh_enrollment.id = v5_mesh_full_node.enrollment_id").
		Where("v5_mesh_enrollment.region_id = ? AND v5_mesh_enrollment.used_at IS NULL AND v5_mesh_enrollment.expires_at > ? AND v5_mesh_full_node.device_id IS NULL AND v5_mesh_full_node.address <> '' AND v5_mesh_full_node.enrollment_id <> ?", regionID, now, except).
		Pluck("v5_mesh_full_node.address", &kept).Error; err != nil {
		return nil, err
	}
	return append(append(used, devices...), kept...), nil
}

// serverAddress answers a fixed address of region's server segment:
// requested, which must be in the segment and free, or the lowest free one.
// The address the credential except keeps counts as free.
func serverAddress(tx *gorm.DB, region model.MeshRegion, requested string, except uint, now time.Time) (string, error) {
	prefix, err := netip.ParsePrefix(region.CIDR)
	if err != nil {
		return "", err
	}
	used, err := usedServerAddresses(tx, region.ID, except, now)
	if err != nil {
		return "", err
	}
	if requested == "" {
		address, ok := freeServerAddress(prefix, used)
		if !ok {
			return "", conflict("the server segment of region %q is full", region.Code)
		}
		return address.String(), nil
	}
	address, err := checkServerAddress(prefix, requested)
	if err != nil {
		return "", err
	}
	for _, taken := range used {
		if taken == address {
			return "", conflict("the address %s is taken", address)
		}
	}
	return address, nil
}

// checkServerAddress checks that raw is a host address of prefix's server
// segment (its lower half, without the network address).
func checkServerAddress(prefix netip.Prefix, raw string) (string, error) {
	address, err := netip.ParseAddr(strings.TrimSpace(raw))
	first, _ := freeServerAddress(prefix, nil)
	if err != nil || !address.Is4() || !prefix.Contains(address) || address.Less(first) || !address.Less(poolStart(prefix)) {
		return "", invalid("address: a host address of the region's server segment (the lower half of %s)", prefix)
	}
	return address.String(), nil
}

// poolStart is the first address of prefix's device pool.
func poolStart(prefix netip.Prefix) netip.Addr {
	first, _ := pool(prefix)
	return addrOf(first)
}

// fullNodes answers the full node rows of devices, by device id.
func fullNodes(tx *gorm.DB, deviceIDs []uint) (map[uint]model.MeshFullNode, error) {
	rows := map[uint]model.MeshFullNode{}
	if len(deviceIDs) == 0 {
		return rows, nil
	}
	var found []model.MeshFullNode
	if err := tx.Where("device_id IN ?", deviceIDs).Find(&found).Error; err != nil {
		return nil, err
	}
	for _, row := range found {
		rows[*row.DeviceID] = row
	}
	return rows, nil
}

// isFullNode tells whether the device is a full node.
func isFullNode(tx *gorm.DB, deviceID uint) (model.MeshFullNode, bool, error) {
	rows, err := fullNodes(tx, []uint{deviceID})
	if err != nil {
		return model.MeshFullNode{}, false, err
	}
	row, ok := rows[deviceID]
	return row, ok, nil
}

func splitSubnets(raw string) []string {
	subnets := []string{}
	for _, subnet := range strings.Split(raw, ",") {
		if subnet = strings.TrimSpace(subnet); subnet != "" {
			subnets = append(subnets, subnet)
		}
	}
	return subnets
}

// SetDeviceSubnets confirms the subnets a full node without an Agent
// advertises (R17): private IPv4 networks outside every region, the host
// group supernet, the reserved ranges and the other full nodes' subnets,
// at most 32. It answers them.
func SetDeviceSubnets(ctx context.Context, db *gorm.DB, deviceID uint, raw []string, now time.Time) ([]string, error) {
	subnets := []string{}
	seen := map[string]bool{}
	for _, entry := range raw {
		if strings.TrimSpace(entry) == "" {
			continue
		}
		prefix, err := parseRange("subnets", entry)
		if err != nil {
			return nil, err
		}
		if !isPrivate(prefix) {
			return nil, invalid("subnets: %s is not a private network", prefix)
		}
		if !seen[prefix.String()] {
			seen[prefix.String()] = true
			subnets = append(subnets, prefix.String())
		}
	}
	if len(subnets) > maxFullNodeSubnets {
		return nil, invalid("subnets: at most %d", maxFullNodeSubnets)
	}
	// Only the full node's own configuration changes, which it reads.
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Two full nodes saving at once take turns, so that the overlap
		// check sees the other's subnets (PostgreSQL).
		if err := lockAddressPlan(tx); err != nil {
			return err
		}
		row, full, err := isFullNode(tx, deviceID)
		if err != nil {
			return err
		}
		if !full {
			var device model.MeshDevice
			if err := findDevice(tx, deviceID, &device); err != nil {
				return err
			}
			return invalid("only a full node advertises subnets")
		}
		var regions []model.MeshRegion
		if err := tx.Find(&regions).Error; err != nil {
			return err
		}
		settings, err := loadSettings(tx)
		if err != nil {
			return err
		}
		blocked := []string{settings.HostGroupSupernet}
		blocked = append(blocked, settings.ReservedRanges...)
		for _, region := range regions {
			blocked = append(blocked, region.CIDR)
		}
		// Two full nodes do not advertise overlapping subnets.
		var others []model.MeshFullNode
		if err := tx.Where("id <> ? AND device_id IS NOT NULL", row.ID).Find(&others).Error; err != nil {
			return err
		}
		for _, other := range others {
			blocked = append(blocked, splitSubnets(other.Subnets)...)
		}
		for _, subnet := range subnets {
			prefix := netip.MustParsePrefix(subnet)
			for _, raw := range blocked {
				if other, err := netip.ParsePrefix(raw); err == nil && other.Overlaps(prefix) {
					return conflict("subnets: %s overlaps %s", subnet, raw)
				}
			}
		}
		return tx.Model(&row).Updates(map[string]any{"subnets": strings.Join(subnets, ","), "updated_at": now}).Error
	})
	return subnets, err
}

// unusedEnrollments is a subquery of the unused credentials for region id.
func unusedEnrollments(tx *gorm.DB, id uint) *gorm.DB {
	return tx.Session(&gorm.Session{NewDB: true}).Model(&model.MeshEnrollment{}).Select("id").Where("region_id = ? AND used_at IS NULL", id)
}
