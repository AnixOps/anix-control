package meshregion

import (
	"context"
	"encoding/binary"
	"fmt"
	"net/netip"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

// HostGroupInput creates or changes a host group (R21). Segment is empty
// to take the next free segment of the supernet; it never changes.
type HostGroupInput struct {
	Name    string `json:"name"`
	Segment string `json:"segment"`
	// Gateway and Standby are nodes (proxy-<id>, forward-<id>) in the
	// mesh; Standby is empty for none and has the gateway's home region.
	Gateway string `json:"gateway"`
	Standby string `json:"standby"`
}

// HostGroupView is a host group with its gateways' region.
type HostGroupView struct {
	model.MeshHostGroup
	RegionID   uint   `json:"region_id"`
	RegionCode string `json:"region_code"`
}

// homeOf answers the home membership of nodeRef.
func homeOf(tx *gorm.DB, nodeRef string) (model.MeshMember, error) {
	var rows []model.MeshMember
	if err := tx.Where("node_ref = ? AND home = ?", nodeRef, true).Limit(1).Find(&rows).Error; err != nil {
		return model.MeshMember{}, err
	}
	if len(rows) == 0 {
		return model.MeshMember{}, invalid("%s is not in the mesh", nodeRef)
	}
	return rows[0], nil
}

// checkGateways answers the gateway's home membership and the normalized
// standby; the standby is another node of the same home region.
func checkGateways(tx *gorm.DB, gateway, standby string) (model.MeshMember, *string, error) {
	home, err := homeOf(tx, strings.TrimSpace(gateway))
	if err != nil {
		return home, nil, err
	}
	standby = strings.TrimSpace(standby)
	if standby == "" {
		return home, nil, nil
	}
	if standby == home.NodeRef {
		return home, nil, invalid("standby: another node than the gateway")
	}
	other, err := homeOf(tx, standby)
	if err != nil {
		return home, nil, err
	}
	if other.RegionID != home.RegionID {
		return home, nil, invalid("standby: a node of the gateway's region")
	}
	return home, &other.NodeRef, nil
}

// pickSegment answers the segment of a new host group: raw, checked, or
// the first free segment of the supernet.
func pickSegment(tx *gorm.DB, settings Settings, raw string) (netip.Prefix, error) {
	supernet, err := netip.ParsePrefix(settings.HostGroupSupernet)
	if err != nil {
		return netip.Prefix{}, err
	}
	var segments []string
	if err := tx.Model(&model.MeshHostGroup{}).Pluck("segment", &segments).Error; err != nil {
		return netip.Prefix{}, err
	}
	free := func(candidate netip.Prefix) bool {
		for _, raw := range segments {
			if other, err := netip.ParsePrefix(raw); err == nil && other.Overlaps(candidate) {
				return false
			}
		}
		return true
	}
	if strings.TrimSpace(raw) != "" {
		segment, err := parseRange("segment", raw)
		if err != nil {
			return segment, err
		}
		if segment.Bits() != settings.HostGroupPrefix || !supernet.Contains(segment.Addr()) {
			return segment, invalid("segment: a /%d inside the host group supernet %s", settings.HostGroupPrefix, supernet)
		}
		if !free(segment) {
			return segment, conflict("segment: %s overlaps another host group", segment)
		}
		return segment, nil
	}
	step := uint32(1) << (32 - settings.HostGroupPrefix)
	base := supernet.Masked().Addr().As4()
	start := binary.BigEndian.Uint32(base[:])
	count := uint32(1) << (settings.HostGroupPrefix - supernet.Bits())
	for i := uint32(0); i < count; i++ {
		segment := netip.PrefixFrom(addrOf(start+i*step), settings.HostGroupPrefix)
		if free(segment) {
			return segment, nil
		}
	}
	return netip.Prefix{}, conflict("the host group supernet %s has no free segment", supernet)
}

func checkGroupName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if n := utf8.RuneCountInString(name); n == 0 || n > 63 {
		return "", invalid("name: 1 to 63 characters")
	}
	return name, nil
}

// CreateHostGroup adds a host group behind a gateway node.
func CreateHostGroup(ctx context.Context, db *gorm.DB, input HostGroupInput, now time.Time) (model.MeshHostGroup, error) {
	name, err := checkGroupName(input.Name)
	if err != nil {
		return model.MeshHostGroup{}, err
	}
	var row model.MeshHostGroup
	err = change(ctx, db, func(tx *gorm.DB) error {
		home, standby, err := checkGateways(tx, input.Gateway, input.Standby)
		if err != nil {
			return err
		}
		settings, err := loadSettings(tx)
		if err != nil {
			return err
		}
		segment, err := pickSegment(tx, settings, input.Segment)
		if err != nil {
			return err
		}
		row = model.MeshHostGroup{Name: name, Segment: segment.String(), GatewayRef: home.NodeRef, StandbyRef: standby, CreatedAt: now, UpdatedAt: now}
		return tx.Create(&row).Error
	})
	return row, err
}

// UpdateHostGroup changes a host group's name and gateways; its segment
// stays. The names published for its hosts follow the new gateway.
func UpdateHostGroup(ctx context.Context, db *gorm.DB, id uint, input HostGroupInput, now time.Time) (model.MeshHostGroup, error) {
	name, err := checkGroupName(input.Name)
	if err != nil {
		return model.MeshHostGroup{}, err
	}
	var row model.MeshHostGroup
	err = change(ctx, db, func(tx *gorm.DB) error {
		if err := findHostGroup(tx, id, &row); err != nil {
			return err
		}
		if strings.TrimSpace(input.Segment) != "" && strings.TrimSpace(input.Segment) != row.Segment {
			return invalid("segment: a host group's segment does not change")
		}
		home, standby, err := checkGateways(tx, input.Gateway, input.Standby)
		if err != nil {
			return err
		}
		if home.NodeRef != row.GatewayRef {
			if err := tx.Model(&model.MeshDeviceName{}).Where("host_group_id = ?", id).
				Updates(map[string]any{"gateway_ref": home.NodeRef, "updated_at": now}).Error; err != nil {
				return err
			}
		}
		row.Name, row.GatewayRef, row.StandbyRef, row.UpdatedAt = name, home.NodeRef, standby, now
		return saveExisting(tx, &row, "host group")
	})
	return row, err
}

// DeleteHostGroup removes a host group and the names of its hosts.
func DeleteHostGroup(ctx context.Context, db *gorm.DB, id uint) error {
	return change(ctx, db, func(tx *gorm.DB) error {
		var row model.MeshHostGroup
		if err := findHostGroup(tx, id, &row); err != nil {
			return err
		}
		if err := tx.Where("host_group_id = ?", id).Delete(&model.MeshDeviceName{}).Error; err != nil {
			return err
		}
		return tx.Delete(&row).Error
	})
}

// HostGroups answers every host group by segment.
func HostGroups(ctx context.Context, db *gorm.DB) ([]HostGroupView, error) {
	db = db.WithContext(ctx)
	var rows []model.MeshHostGroup
	if err := db.Order("segment ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	homes, err := homeRegions(db)
	if err != nil {
		return nil, err
	}
	codes, err := regionCodes(db)
	if err != nil {
		return nil, err
	}
	views := make([]HostGroupView, 0, len(rows))
	for _, row := range rows {
		region := homes[row.GatewayRef]
		views = append(views, HostGroupView{MeshHostGroup: row, RegionID: region, RegionCode: codes[region]})
	}
	return views, nil
}

// homeRegions maps every node in the mesh to its home region.
func homeRegions(tx *gorm.DB) (map[string]uint, error) {
	var homes []model.MeshMember
	if err := tx.Where("home = ?", true).Find(&homes).Error; err != nil {
		return nil, err
	}
	answer := make(map[string]uint, len(homes))
	for _, home := range homes {
		answer[home.NodeRef] = home.RegionID
	}
	return answer, nil
}

func findHostGroup(tx *gorm.DB, id uint, row *model.MeshHostGroup) error {
	var rows []model.MeshHostGroup
	if err := tx.Where("id = ?", id).Limit(1).Find(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return fmt.Errorf("%w: host group %d", ErrNotFound, id)
	}
	*row = rows[0]
	return nil
}

// checkGatewayStays refuses to move or remove a node that is a host
// group's gateway or standby or publishes names: change those first.
func checkGatewayStays(tx *gorm.DB, nodeRef string) error {
	var groups int64
	if err := tx.Model(&model.MeshHostGroup{}).Where("gateway_ref = ? OR standby_ref = ?", nodeRef, nodeRef).Count(&groups).Error; err != nil {
		return err
	}
	if groups > 0 {
		return conflict("the node is the gateway or standby of %d host groups; change them first", groups)
	}
	var names int64
	if err := tx.Model(&model.MeshDeviceName{}).Where("gateway_ref = ?", nodeRef).Count(&names).Error; err != nil {
		return err
	}
	if names > 0 {
		return conflict("the node publishes %d device names; remove them first", names)
	}
	return nil
}
