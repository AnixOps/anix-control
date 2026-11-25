package meshregion

import (
	"context"
	"fmt"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

// DeviceNameInput creates or changes a name a gateway node publishes
// (R25, R26): a host of a host group (HostGroupID, an address in its
// segment; the group's gateway publishes it) or a device of the gateway's
// local network (Gateway, a private address outside the regions and the
// host group supernet).
type DeviceNameInput struct {
	Gateway     string `json:"gateway"`
	HostGroupID uint   `json:"host_group_id"`
	Name        string `json:"name"`
	Address     string `json:"address"`
}

// Name sources.
const (
	NameNode      = "node"
	NameDevice    = "device"
	NameHostGroup = "host_group"
	NameLocal     = "local"
)

// NameEntry is one intranet name.
type NameEntry struct {
	FQDN       string `json:"fqdn"`
	Name       string `json:"name"`
	RegionCode string `json:"region_code"`
	Address    string `json:"address"`
	// Source is node, device, host_group or local; Ref is the node, the
	// device id or the device name id.
	Source string `json:"source"`
	Ref    string `json:"ref"`
	// Duplicate: another entry has the same name; every one is answered
	// (R28).
	Duplicate bool `json:"duplicate"`
}

// UnnamedNode is a node whose name has no letter or digit, so it has no
// intranet name (normalizeName).
type UnnamedNode struct {
	NodeRef  string `json:"node_ref"`
	NodeName string `json:"node_name"`
}

// NameList is the intranet name list.
type NameList struct {
	Suffix  string        `json:"suffix"`
	Entries []NameEntry   `json:"entries"`
	Unnamed []UnnamedNode `json:"unnamed"`
}

func checkDeviceName(tx *gorm.DB, input DeviceNameInput) (model.MeshDeviceName, error) {
	name, err := normalizeLabel("name", input.Name)
	if err != nil {
		return model.MeshDeviceName{}, err
	}
	address, err := netip.ParseAddr(strings.TrimSpace(input.Address))
	if err != nil || !address.Is4() {
		return model.MeshDeviceName{}, invalid("address: an IPv4 address")
	}
	row := model.MeshDeviceName{Name: name, Address: address.String()}
	if input.HostGroupID != 0 {
		var group model.MeshHostGroup
		if err := findHostGroup(tx, input.HostGroupID, &group); err != nil {
			return row, err
		}
		segment, err := netip.ParsePrefix(group.Segment)
		if err != nil || !segment.Contains(address) {
			return row, invalid("address: inside the host group's segment %s", group.Segment)
		}
		id := group.ID
		row.HostGroupID, row.GatewayRef = &id, group.GatewayRef
		return row, nil
	}
	home, err := homeOf(forUpdate(tx), strings.TrimSpace(input.Gateway))
	if err != nil {
		return row, err
	}
	if !isPrivate(netip.PrefixFrom(address, 32)) {
		return row, invalid("address: a private address")
	}
	var regions []model.MeshRegion
	if err := tx.Find(&regions).Error; err != nil {
		return row, err
	}
	for _, region := range regions {
		if prefix, err := netip.ParsePrefix(region.CIDR); err == nil && prefix.Contains(address) {
			return row, invalid("address: inside region %q; a device of the gateway's local network has an address outside the regions", region.Code)
		}
	}
	settings, err := loadSettings(tx)
	if err != nil {
		return row, err
	}
	if supernet, err := netip.ParsePrefix(settings.HostGroupSupernet); err == nil && supernet.Contains(address) {
		return row, invalid("address: inside the host group supernet; name a host group instead")
	}
	row.GatewayRef = home.NodeRef
	return row, nil
}

// CreateDeviceName adds a published name.
func CreateDeviceName(ctx context.Context, db *gorm.DB, input DeviceNameInput, now time.Time) (model.MeshDeviceName, error) {
	var row model.MeshDeviceName
	err := change(ctx, db, func(tx *gorm.DB) error {
		var err error
		if row, err = checkDeviceName(tx, input); err != nil {
			return err
		}
		row.CreatedAt, row.UpdatedAt = now, now
		return tx.Create(&row).Error
	})
	return row, err
}

// UpdateDeviceName changes a published name.
func UpdateDeviceName(ctx context.Context, db *gorm.DB, id uint, input DeviceNameInput, now time.Time) (model.MeshDeviceName, error) {
	var row model.MeshDeviceName
	err := change(ctx, db, func(tx *gorm.DB) error {
		var current model.MeshDeviceName
		if err := findDeviceName(tx, id, &current); err != nil {
			return err
		}
		checked, err := checkDeviceName(tx, input)
		if err != nil {
			return err
		}
		row = current
		row.GatewayRef, row.HostGroupID, row.Name, row.Address, row.UpdatedAt = checked.GatewayRef, checked.HostGroupID, checked.Name, checked.Address, now
		return saveExisting(tx, &row, "name")
	})
	return row, err
}

// DeleteDeviceName removes a published name.
func DeleteDeviceName(ctx context.Context, db *gorm.DB, id uint) error {
	return change(ctx, db, func(tx *gorm.DB) error {
		var row model.MeshDeviceName
		if err := findDeviceName(tx, id, &row); err != nil {
			return err
		}
		return tx.Delete(&row).Error
	})
}

// DeviceNames answers the published names by gateway and name.
func DeviceNames(ctx context.Context, db *gorm.DB) ([]model.MeshDeviceName, error) {
	var rows []model.MeshDeviceName
	err := db.WithContext(ctx).Order("gateway_ref ASC, name ASC, id ASC").Find(&rows).Error
	return rows, err
}

func findDeviceName(tx *gorm.DB, id uint, row *model.MeshDeviceName) error {
	var rows []model.MeshDeviceName
	if err := tx.Where("id = ?", id).Limit(1).Find(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return fmt.Errorf("%w: device name %d", ErrNotFound, id)
	}
	*row = rows[0]
	return nil
}

// Names answers the intranet name list (R28): every node in the mesh, every
// device with an address and every published name, as
// <name>.<region code>.<suffix>, where the region is the node's or the
// gateway's home region. Names that occur more than once are all answered,
// marked duplicate.
func Names(ctx context.Context, db *gorm.DB) (NameList, error) {
	db = db.WithContext(ctx)
	settings, err := loadSettings(db)
	if err != nil {
		return NameList{}, err
	}
	codes, err := regionCodes(db)
	if err != nil {
		return NameList{}, err
	}
	list := NameList{Suffix: settings.NameSuffix, Entries: []NameEntry{}, Unnamed: []UnnamedNode{}}
	add := func(name string, region uint, address, source, ref string) {
		code := codes[region]
		list.Entries = append(list.Entries, NameEntry{FQDN: fqdn(name, code, settings.NameSuffix), Name: name, RegionCode: code, Address: address, Source: source, Ref: ref})
	}
	var homes []model.MeshMember
	if err := db.Where("home = ?", true).Order("id ASC").Find(&homes).Error; err != nil {
		return NameList{}, err
	}
	homeRegion := map[string]uint{}
	for _, home := range homes {
		homeRegion[home.NodeRef] = home.RegionID
		info, err := lookupNode(db, home.NodeRef)
		if err != nil {
			continue
		}
		label := normalizeName(info.name)
		if label == "" {
			list.Unnamed = append(list.Unnamed, UnnamedNode{NodeRef: home.NodeRef, NodeName: info.name})
			continue
		}
		add(label, home.RegionID, home.Address, NameNode, home.NodeRef)
	}
	var devices []model.MeshDevice
	if err := db.Where("address IS NOT NULL").Order("id ASC").Find(&devices).Error; err != nil {
		return NameList{}, err
	}
	for _, device := range devices {
		add(device.Hostname, device.RegionID, *device.Address, NameDevice, strconv.FormatUint(uint64(device.ID), 10))
	}
	var names []model.MeshDeviceName
	if err := db.Order("id ASC").Find(&names).Error; err != nil {
		return NameList{}, err
	}
	for _, name := range names {
		region, ok := homeRegion[name.GatewayRef]
		if !ok {
			continue
		}
		source := NameLocal
		if name.HostGroupID != nil {
			source = NameHostGroup
		}
		add(name.Name, region, name.Address, source, strconv.FormatUint(uint64(name.ID), 10))
	}
	count := map[string]int{}
	for _, entry := range list.Entries {
		count[entry.FQDN]++
	}
	for i := range list.Entries {
		list.Entries[i].Duplicate = count[list.Entries[i].FQDN] > 1
	}
	sort.SliceStable(list.Entries, func(i, j int) bool {
		if list.Entries[i].FQDN != list.Entries[j].FQDN {
			return list.Entries[i].FQDN < list.Entries[j].FQDN
		}
		return list.Entries[i].Address < list.Entries[j].Address
	})
	return list, nil
}
