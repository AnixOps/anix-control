package meshregion

import (
	"errors"
	"net/netip"
	"sort"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

// DeviceNameInput creates or changes a device name.
type DeviceNameInput struct {
	// MemberID is the gateway that publishes the name; for a host group's
	// virtual machine it is the group's gateway.
	MemberID uint
	// HostGroupID is the host group of a virtual machine; 0 for a device
	// in the gateway's local network.
	HostGroupID uint
	Name        string
	Address     string
}

// checkDeviceName validates input and answers the publishing gateway, the
// host group and the normalized name and address.
func checkDeviceName(tx *gorm.DB, input DeviceNameInput, self uint) (model.MeshMember, *uint, string, string, error) {
	name, err := normalizeLabel("name", input.Name)
	if err != nil {
		return model.MeshMember{}, nil, "", "", err
	}
	addr, err := parseAddr("address", input.Address)
	if err != nil {
		return model.MeshMember{}, nil, "", "", err
	}
	var hostGroup *uint
	memberID := input.MemberID
	settings, err := loadSettings(tx)
	if err != nil {
		return model.MeshMember{}, nil, "", "", err
	}
	if input.HostGroupID != 0 {
		group, err := hostGroupByID(tx, input.HostGroupID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return model.MeshMember{}, nil, "", "", invalid("host_group_id: no such host group")
			}
			return model.MeshMember{}, nil, "", "", err
		}
		segment := mustPrefix(group.Segment)
		if !segment.Contains(addr) || addr == segment.Masked().Addr() || addr == lastAddr(segment) {
			return model.MeshMember{}, nil, "", "", invalid("address: an address of the host group's segment %s", segment)
		}
		memberID = group.GatewayMemberID
		id := group.ID
		hostGroup = &id
	} else {
		if !isPrivateAddr(addr) {
			return model.MeshMember{}, nil, "", "", invalid("address: a private address (10/8, 172.16/12, 192.168/16 or 100.64/10)")
		}
		if mustPrefix(settings.HostGroupSupernet).Contains(addr) {
			return model.MeshMember{}, nil, "", "", invalid("address: inside the host group supernet; name it in its host group")
		}
		var regions []model.MeshRegion
		if err := tx.Find(&regions).Error; err != nil {
			return model.MeshMember{}, nil, "", "", err
		}
		for _, region := range regions {
			if mustPrefix(region.Subnet).Contains(addr) {
				return model.MeshMember{}, nil, "", "", invalid("address: inside region %s's subnet; members name themselves", region.Code)
			}
		}
	}
	gateway, err := gatewayMember(tx, memberID, "member_id")
	if err != nil {
		return model.MeshMember{}, nil, "", "", err
	}
	if err := checkNameFree(tx, *gateway.RegionID, name, 0, self); err != nil {
		return model.MeshMember{}, nil, "", "", err
	}
	return gateway, hostGroup, name, addr.String(), nil
}

// checkNameFree refuses a name that a member (other than selfMember) or a
// device name (other than selfDevice) of the region uses: every intranet
// name of a region is unique.
func checkNameFree(tx *gorm.DB, regionID uint, name string, selfMember, selfDevice uint) error {
	if err := checkHostname(tx, regionID, name, selfMember); err != nil {
		return err
	}
	var gateways []uint
	if err := tx.Model(&model.MeshMember{}).Where("region_id = ?", regionID).Pluck("id", &gateways).Error; err != nil {
		return err
	}
	if len(gateways) == 0 {
		return nil
	}
	var same int64
	if err := tx.Model(&model.MeshDeviceName{}).Where("member_id IN ? AND name = ? AND id <> ?", gateways, name, selfDevice).Count(&same).Error; err != nil {
		return err
	}
	if same > 0 {
		return conflict("name: %s is taken in the region", name)
	}
	return nil
}

// CreateDeviceName adds a name a gateway publishes.
func (s *Service) CreateDeviceName(input DeviceNameInput) (model.MeshDeviceName, error) {
	var row model.MeshDeviceName
	err := s.transaction(func(tx *gorm.DB) error {
		gateway, group, name, addr, err := checkDeviceName(tx, input, 0)
		if err != nil {
			return err
		}
		now := s.now()
		row = model.MeshDeviceName{MemberID: gateway.ID, HostGroupID: group, Name: name, Address: addr, CreatedAt: now, UpdatedAt: now}
		return tx.Create(&row).Error
	})
	return row, err
}

func deviceNameByID(tx *gorm.DB, id uint) (model.MeshDeviceName, error) {
	var row model.MeshDeviceName
	if err := tx.First(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return row, notFound("device name")
		}
		return row, err
	}
	return row, nil
}

// UpdateDeviceName changes a name, its address and its gateway or group.
func (s *Service) UpdateDeviceName(id uint, input DeviceNameInput) (model.MeshDeviceName, error) {
	var row model.MeshDeviceName
	err := s.transaction(func(tx *gorm.DB) error {
		var err error
		if row, err = deviceNameByID(tx, id); err != nil {
			return err
		}
		gateway, group, name, addr, err := checkDeviceName(tx, input, id)
		if err != nil {
			return err
		}
		row.MemberID, row.HostGroupID, row.Name, row.Address, row.UpdatedAt = gateway.ID, group, name, addr, s.now()
		return tx.Save(&row).Error
	})
	return row, err
}

// DeleteDeviceName removes a name.
func (s *Service) DeleteDeviceName(id uint) error {
	return s.transaction(func(tx *gorm.DB) error {
		row, err := deviceNameByID(tx, id)
		if err != nil {
			return err
		}
		return tx.Delete(&row).Error
	})
}

// DeviceNames answers every device name, by name.
func (s *Service) DeviceNames() ([]model.MeshDeviceName, error) {
	var rows []model.MeshDeviceName
	err := s.transaction(func(tx *gorm.DB) error {
		return tx.Order("name, id").Find(&rows).Error
	})
	return rows, err
}

// Name sources.
const (
	NameSourceMember    = "member"
	NameSourceHostGroup = "host_group"
	NameSourceDevice    = "device"
)

// NameEntry is one name of the intranet names (R28).
type NameEntry struct {
	// FQDN is <name>.<region code>.<suffix>.
	FQDN       string `json:"fqdn"`
	Name       string `json:"name"`
	RegionCode string `json:"region_code"`
	Address    string `json:"address"`
	Source     string `json:"source"`
	// MemberID is the member itself, or the gateway that publishes the
	// name.
	MemberID uint `json:"member_id"`
	// Duplicate marks a name used twice in its region; the panel refuses
	// duplicates, so it shows data written outside it.
	Duplicate bool `json:"duplicate"`
}

// NameList is the intranet names the panel publishes.
type NameList struct {
	Suffix  string      `json:"suffix"`
	Entries []NameEntry `json:"entries"`
}

// Names answers the intranet names: every member with an address and every
// device name, under its region, by full name.
func (s *Service) Names() (NameList, error) {
	var list NameList
	err := s.transaction(func(tx *gorm.DB) error {
		settings, err := loadSettings(tx)
		if err != nil {
			return err
		}
		list.Suffix = settings.NameSuffix
		codes, err := regionCodes(tx)
		if err != nil {
			return err
		}
		members, err := membersByID(tx)
		if err != nil {
			return err
		}
		var devices []model.MeshDeviceName
		if err := tx.Find(&devices).Error; err != nil {
			return err
		}
		entries := []NameEntry{}
		add := func(name string, regionID *uint, address, source string, memberID uint) {
			if regionID == nil || address == "" {
				return
			}
			code, ok := codes[*regionID]
			if !ok {
				return
			}
			entries = append(entries, NameEntry{
				FQDN: name + "." + code + "." + settings.NameSuffix, Name: name, RegionCode: code,
				Address: address, Source: source, MemberID: memberID,
			})
		}
		for _, member := range members {
			if member.Address != nil {
				add(member.Hostname, member.RegionID, *member.Address, NameSourceMember, member.ID)
			}
		}
		for _, device := range devices {
			gateway, ok := members[device.MemberID]
			if !ok {
				continue
			}
			source := NameSourceDevice
			if device.HostGroupID != nil {
				source = NameSourceHostGroup
			}
			add(device.Name, gateway.RegionID, device.Address, source, gateway.ID)
		}
		seen := map[string]int{}
		for _, entry := range entries {
			seen[entry.FQDN]++
		}
		for i := range entries {
			entries[i].Duplicate = seen[entries[i].FQDN] > 1
		}
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].FQDN != entries[j].FQDN {
				return entries[i].FQDN < entries[j].FQDN
			}
			return addrLess(entries[i].Address, entries[j].Address)
		})
		list.Entries = entries
		return nil
	})
	return list, err
}

func addrLess(a, b string) bool {
	left, errLeft := netip.ParseAddr(a)
	right, errRight := netip.ParseAddr(b)
	if errLeft != nil || errRight != nil {
		return a < b
	}
	return left.Less(right)
}
