package meshregion

import (
	"errors"
	"net/netip"
	"strings"
	"unicode/utf8"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

// HostGroupInput creates or changes a host group.
type HostGroupInput struct {
	Name            string
	GatewayMemberID uint
	// StandbyMemberID is the optional second gateway; 0 for none.
	StandbyMemberID uint
	// Segment is the group's segment; empty takes the first free one of
	// the supernet. It is set only when the group is created.
	Segment string
}

// HostGroupView is a host group with its region and gateways.
type HostGroupView struct {
	model.MeshHostGroup
	RegionID        *uint  `json:"region_id"`
	RegionCode      string `json:"region_code"`
	GatewayHostname string `json:"gateway_hostname"`
	StandbyHostname string `json:"standby_hostname"`
	Names           int    `json:"names"`
}

// gatewayMember loads a member that may be a host group's gateway: a
// gateway in a region.
func gatewayMember(tx *gorm.DB, id uint, field string) (model.MeshMember, error) {
	member, err := memberByID(tx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return member, invalid("%s: no such member", field)
		}
		return member, err
	}
	if member.Role != model.MeshRoleGateway {
		return member, invalid("%s: %s is not a gateway", field, member.Hostname)
	}
	if member.RegionID == nil {
		return member, invalid("%s: %s has not entered a region", field, member.Hostname)
	}
	return member, nil
}

// checkGateways validates a group's gateways; both are in one region.
func checkGateways(tx *gorm.DB, input HostGroupInput) (*uint, error) {
	gateway, err := gatewayMember(tx, input.GatewayMemberID, "gateway_member_id")
	if err != nil {
		return nil, err
	}
	if input.StandbyMemberID == 0 {
		return nil, nil
	}
	if input.StandbyMemberID == input.GatewayMemberID {
		return nil, invalid("standby_member_id: a different gateway than gateway_member_id")
	}
	standby, err := gatewayMember(tx, input.StandbyMemberID, "standby_member_id")
	if err != nil {
		return nil, err
	}
	if *standby.RegionID != *gateway.RegionID {
		return nil, invalid("standby_member_id: %s is in another region than the gateway", standby.Hostname)
	}
	id := standby.ID
	return &id, nil
}

func checkGroupName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" || utf8.RuneCountInString(name) > 64 {
		return "", invalid("name: 1 to 64 characters")
	}
	return name, nil
}

// pickSegment answers the requested segment, or the first free one of the
// supernet.
func pickSegment(tx *gorm.DB, settings model.MeshSettings, requested string) (netip.Prefix, error) {
	supernet := mustPrefix(settings.HostGroupSupernet)
	var groups []model.MeshHostGroup
	if err := tx.Find(&groups).Error; err != nil {
		return netip.Prefix{}, err
	}
	taken := make([]netip.Prefix, 0, len(groups))
	for _, group := range groups {
		taken = append(taken, mustPrefix(group.Segment))
	}
	free := func(segment netip.Prefix) bool {
		for _, other := range taken {
			if other.Overlaps(segment) {
				return false
			}
		}
		return true
	}
	if strings.TrimSpace(requested) != "" {
		segment, err := parsePrefix("segment", requested)
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
	base := addrToUint(supernet.Masked().Addr())
	count := uint32(1) << (settings.HostGroupPrefix - supernet.Bits())
	for i := uint32(0); i < count; i++ {
		value := base + i*step
		segment := netip.PrefixFrom(addrFromUint(value), settings.HostGroupPrefix)
		if free(segment) {
			return segment, nil
		}
	}
	return netip.Prefix{}, ErrExhausted
}

// CreateHostGroup adds a host group with its segment.
func (s *Service) CreateHostGroup(input HostGroupInput) (model.MeshHostGroup, error) {
	name, err := checkGroupName(input.Name)
	if err != nil {
		return model.MeshHostGroup{}, err
	}
	var group model.MeshHostGroup
	err = s.transaction(func(tx *gorm.DB) error {
		standby, err := checkGateways(tx, input)
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
		now := s.now()
		group = model.MeshHostGroup{
			Name: name, Segment: segment.String(), GatewayMemberID: input.GatewayMemberID, StandbyMemberID: standby,
			CreatedAt: now, UpdatedAt: now,
		}
		return tx.Create(&group).Error
	})
	return group, err
}

func hostGroupByID(tx *gorm.DB, id uint) (model.MeshHostGroup, error) {
	var group model.MeshHostGroup
	if err := tx.First(&group, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return group, notFound("host group")
		}
		return group, err
	}
	return group, nil
}

// UpdateHostGroup changes a group's name and gateways; the segment stays.
// The new gateways stay in the group's region, so the names stay valid.
func (s *Service) UpdateHostGroup(id uint, input HostGroupInput) (model.MeshHostGroup, error) {
	name, err := checkGroupName(input.Name)
	if err != nil {
		return model.MeshHostGroup{}, err
	}
	var group model.MeshHostGroup
	err = s.transaction(func(tx *gorm.DB) error {
		if group, err = hostGroupByID(tx, id); err != nil {
			return err
		}
		if requested := strings.TrimSpace(input.Segment); requested != "" && requested != group.Segment {
			return invalid("segment: a host group keeps its segment; create a new group for another one")
		}
		standby, err := checkGateways(tx, input)
		if err != nil {
			return err
		}
		if input.GatewayMemberID != group.GatewayMemberID {
			current, err := memberByID(tx, group.GatewayMemberID)
			if err != nil {
				return err
			}
			next, err := memberByID(tx, input.GatewayMemberID)
			if err != nil {
				return err
			}
			if current.RegionID != nil && next.RegionID != nil && *current.RegionID != *next.RegionID {
				return invalid("gateway_member_id: a gateway in the group's region; move the gateway to change the region")
			}
		}
		group.Name, group.GatewayMemberID, group.StandbyMemberID = name, input.GatewayMemberID, standby
		group.UpdatedAt = s.now()
		if err := tx.Save(&group).Error; err != nil {
			return err
		}
		// The names of the group are published by its gateway.
		return tx.Model(&model.MeshDeviceName{}).Where("host_group_id = ?", id).
			Updates(map[string]any{"member_id": group.GatewayMemberID, "updated_at": group.UpdatedAt}).Error
	})
	return group, err
}

// DeleteHostGroup removes a group and its names.
func (s *Service) DeleteHostGroup(id uint) error {
	return s.transaction(func(tx *gorm.DB) error {
		group, err := hostGroupByID(tx, id)
		if err != nil {
			return err
		}
		if err := tx.Where("host_group_id = ?", id).Delete(&model.MeshDeviceName{}).Error; err != nil {
			return err
		}
		return tx.Delete(&group).Error
	})
}

// HostGroups answers every host group, by name.
func (s *Service) HostGroups() ([]HostGroupView, error) {
	var views []HostGroupView
	err := s.transaction(func(tx *gorm.DB) error {
		var groups []model.MeshHostGroup
		if err := tx.Order("name, id").Find(&groups).Error; err != nil {
			return err
		}
		members, err := membersByID(tx)
		if err != nil {
			return err
		}
		codes, err := regionCodes(tx)
		if err != nil {
			return err
		}
		var names []model.MeshDeviceName
		if err := tx.Where("host_group_id IS NOT NULL").Find(&names).Error; err != nil {
			return err
		}
		counts := map[uint]int{}
		for _, name := range names {
			counts[*name.HostGroupID]++
		}
		views = make([]HostGroupView, 0, len(groups))
		for _, group := range groups {
			view := HostGroupView{MeshHostGroup: group, Names: counts[group.ID]}
			if gateway, ok := members[group.GatewayMemberID]; ok {
				view.GatewayHostname = gateway.Hostname
				view.RegionID = gateway.RegionID
				if gateway.RegionID != nil {
					view.RegionCode = codes[*gateway.RegionID]
				}
			}
			if group.StandbyMemberID != nil {
				view.StandbyHostname = members[*group.StandbyMemberID].Hostname
			}
			views = append(views, view)
		}
		return nil
	})
	return views, err
}

func membersByID(tx *gorm.DB) (map[uint]model.MeshMember, error) {
	var members []model.MeshMember
	if err := tx.Find(&members).Error; err != nil {
		return nil, err
	}
	out := make(map[uint]model.MeshMember, len(members))
	for _, member := range members {
		out[member.ID] = member
	}
	return out, nil
}

func regionCodes(tx *gorm.DB) (map[uint]string, error) {
	var regions []model.MeshRegion
	if err := tx.Find(&regions).Error; err != nil {
		return nil, err
	}
	out := make(map[uint]string, len(regions))
	for _, region := range regions {
		out[region.ID] = region.Code
	}
	return out, nil
}
