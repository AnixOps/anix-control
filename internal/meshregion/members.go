package meshregion

import (
	"errors"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

// MemberInput creates or changes a member.
type MemberInput struct {
	RegionID uint
	// NodeRef is the Agent node, proxy-<id> or forward-<id>; empty for none.
	NodeRef  string
	Hostname string
	Role     string
	// AddressMode is fixed or pool; empty picks the role's default (pool
	// for a device, fixed otherwise).
	AddressMode string
	// Address is a fixed address; empty takes the lowest free address of
	// the server range.
	Address      string
	ProbeAddress string
}

// Member warnings.
const (
	// WarningNoAddress: the member has no address yet.
	WarningNoAddress = "no_address"
	// WarningPoolAddressIdle: the pool address has gone unused for 30 days
	// and goes to the next device that needs one.
	WarningPoolAddressIdle = "pool_address_idle"
)

// MemberView is a member with its region code and intranet name.
type MemberView struct {
	model.MeshMember
	RegionCode string   `json:"region_code"`
	FQDN       string   `json:"fqdn"`
	Warnings   []string `json:"warnings"`
}

func validRole(role string) bool {
	switch role {
	case model.MeshRoleBorder, model.MeshRoleGateway, model.MeshRoleServer, model.MeshRoleDevice:
		return true
	}
	return false
}

// normalizeNodeRef reads proxy-<id> or forward-<id>.
func normalizeNodeRef(raw string) (*string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	kind, id, ok := strings.Cut(raw, "-")
	number, err := strconv.ParseUint(id, 10, 32)
	if !ok || (kind != "proxy" && kind != "forward") || err != nil || number == 0 {
		return nil, invalid("node_ref: proxy-<id> or forward-<id>")
	}
	ref := kind + "-" + strconv.FormatUint(number, 10)
	return &ref, nil
}

// normalizeProbe reads a border node's public host:port.
func normalizeProbe(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	host, port, err := net.SplitHostPort(raw)
	number, portErr := strconv.Atoi(port)
	if err != nil || host == "" || portErr != nil || number < 1 || number > 65535 || len(raw) > 255 {
		return "", invalid("probe_address: host:port, such as 203.0.113.10:11010 or [2001:db8::1]:11010")
	}
	return net.JoinHostPort(host, strconv.Itoa(number)), nil
}

// reclaimable tells whether member's pool address may go to another
// device at now.
func reclaimable(member model.MeshMember, now time.Time) bool {
	if member.AddressMode != model.MeshAddressPool || member.Address == nil {
		return false
	}
	used := member.CreatedAt
	if member.AddressUsedAt != nil {
		used = *member.AddressUsedAt
	}
	return now.Sub(used) >= PoolReclaimAfter
}

// allocation is a picked address and the member whose idle pool address it
// takes, if any.
type allocation struct {
	addr  netip.Addr
	taken *model.MeshMember
}

// allocate picks the lowest free address of within in region, for a member
// other than self. A pool address idle for 30 days counts as free.
func allocate(tx *gorm.DB, region model.MeshRegion, within netip.Prefix, self uint, now time.Time) (allocation, error) {
	var members []model.MeshMember
	if err := tx.Where("region_id = ? AND address IS NOT NULL AND id <> ?", region.ID, self).Find(&members).Error; err != nil {
		return allocation{}, err
	}
	used := make(map[netip.Addr]*model.MeshMember, len(members))
	for i := range members {
		if addr, err := netip.ParseAddr(*members[i].Address); err == nil {
			used[addr] = &members[i]
		}
	}
	subnet := mustPrefix(region.Subnet)
	var idle *allocation
	for addr := within.Masked().Addr(); within.Contains(addr); addr = addr.Next() {
		if !usableIn(subnet, addr) {
			continue
		}
		holder, ok := used[addr]
		if !ok {
			return allocation{addr: addr}, nil
		}
		if idle == nil && reclaimable(*holder, now) {
			idle = &allocation{addr: addr, taken: holder}
		}
	}
	if idle != nil {
		return *idle, nil
	}
	return allocation{}, ErrExhausted
}

// assign stores addr as member's address, first clearing the idle pool
// address it takes.
func assign(tx *gorm.DB, member *model.MeshMember, picked allocation, now time.Time) error {
	if picked.taken != nil {
		if err := tx.Model(&model.MeshMember{}).Where("id = ?", picked.taken.ID).
			Updates(map[string]any{"address": nil, "updated_at": now}).Error; err != nil {
			return err
		}
	}
	text := picked.addr.String()
	member.Address = &text
	member.AddressUsedAt = &now
	return nil
}

// checkFixed validates an administrator's fixed address in region.
func checkFixed(tx *gorm.DB, region model.MeshRegion, raw string, self uint) (netip.Addr, error) {
	addr, err := parseAddr("address", raw)
	if err != nil {
		return addr, err
	}
	subnet := mustPrefix(region.Subnet)
	if !usableIn(subnet, addr) {
		return addr, invalid("address: an address of the region's subnet %s, neither its first nor its last", subnet)
	}
	if mustPrefix(region.DevicePool).Contains(addr) {
		return addr, invalid("address: %s is in the device pool %s; fixed addresses come from the rest of the subnet", addr, region.DevicePool)
	}
	var holder model.MeshMember
	err = tx.Where("region_id = ? AND address = ? AND id <> ?", region.ID, addr.String(), self).First(&holder).Error
	if err == nil {
		return addr, conflict("address: %s belongs to %s", addr, holder.Hostname)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return addr, err
	}
	return addr, nil
}

// placeAddress gives member an address in region by its mode: the fixed
// address requested (or the lowest free one of the server range), or the
// device's pool address (kept when it still holds one in the pool).
func placeAddress(tx *gorm.DB, region model.MeshRegion, member *model.MeshMember, requested string, now time.Time) error {
	if member.AddressMode == model.MeshAddressFixed {
		if strings.TrimSpace(requested) != "" {
			addr, err := checkFixed(tx, region, requested, member.ID)
			if err != nil {
				return err
			}
			text := addr.String()
			member.Address = &text
			member.AddressUsedAt = &now
			return nil
		}
		if member.Address != nil && member.RegionID != nil && *member.RegionID == region.ID {
			return nil
		}
		picked, err := allocate(tx, region, mustPrefix(region.ServerRange), member.ID, now)
		if err != nil {
			return err
		}
		if picked.taken != nil {
			// The server range holds no pool addresses; never take one.
			return ErrExhausted
		}
		return assign(tx, member, picked, now)
	}
	if strings.TrimSpace(requested) != "" {
		return invalid("address: a pool member's address comes from the device pool")
	}
	pool := mustPrefix(region.DevicePool)
	if member.Address != nil && member.RegionID != nil && *member.RegionID == region.ID {
		if addr, err := netip.ParseAddr(*member.Address); err == nil && pool.Contains(addr) {
			return nil
		}
	}
	picked, err := allocate(tx, region, pool, member.ID, now)
	if err != nil {
		return err
	}
	return assign(tx, member, picked, now)
}

// checkHostname refuses a hostname another member of region uses.
func checkHostname(tx *gorm.DB, regionID uint, hostname string, self uint) error {
	var same int64
	if err := tx.Model(&model.MeshMember{}).Where("region_id = ? AND hostname = ? AND id <> ?", regionID, hostname, self).Count(&same).Error; err != nil {
		return err
	}
	if same > 0 {
		return conflict("hostname: %s is taken in the region", hostname)
	}
	return nil
}

func memberByID(tx *gorm.DB, id uint) (model.MeshMember, error) {
	var member model.MeshMember
	if err := tx.First(&member, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return member, notFound("member")
		}
		return member, err
	}
	return member, nil
}

// readMemberInput checks the fields shared by create and update.
func readMemberInput(input MemberInput) (hostname, role, mode, probe string, nodeRef *string, err error) {
	if hostname, err = normalizeLabel("hostname", input.Hostname); err != nil {
		return
	}
	role = strings.TrimSpace(input.Role)
	if !validRole(role) {
		err = invalid("role: border, gateway, server or device")
		return
	}
	mode = strings.TrimSpace(input.AddressMode)
	if mode == "" {
		mode = model.MeshAddressFixed
		if role == model.MeshRoleDevice {
			mode = model.MeshAddressPool
		}
	}
	if mode != model.MeshAddressFixed && mode != model.MeshAddressPool {
		err = invalid("address_mode: fixed or pool")
		return
	}
	if mode == model.MeshAddressPool && role != model.MeshRoleDevice {
		err = invalid("address_mode: border nodes, gateways and servers use fixed addresses")
		return
	}
	if probe, err = normalizeProbe(input.ProbeAddress); err != nil {
		return
	}
	if probe != "" && role != model.MeshRoleBorder {
		err = invalid("probe_address: only a border node has one")
		return
	}
	nodeRef, err = normalizeNodeRef(input.NodeRef)
	return
}

// checkNodeRef refuses an Agent node another member names.
func checkNodeRef(tx *gorm.DB, nodeRef *string, self uint) error {
	if nodeRef == nil {
		return nil
	}
	var same int64
	if err := tx.Model(&model.MeshMember{}).Where("node_ref = ? AND id <> ?", *nodeRef, self).Count(&same).Error; err != nil {
		return err
	}
	if same > 0 {
		return conflict("node_ref: %s is already a member", *nodeRef)
	}
	return nil
}

// CreateMember adds a member to a region with its address.
func (s *Service) CreateMember(input MemberInput) (model.MeshMember, error) {
	hostname, role, mode, probe, nodeRef, err := readMemberInput(input)
	if err != nil {
		return model.MeshMember{}, err
	}
	var member model.MeshMember
	err = s.transaction(func(tx *gorm.DB) error {
		region, err := regionByID(tx, input.RegionID)
		if err != nil {
			return err
		}
		if err := checkNameFree(tx, region.ID, hostname, 0, 0); err != nil {
			return err
		}
		if err := checkNodeRef(tx, nodeRef, 0); err != nil {
			return err
		}
		now := s.now()
		member = model.MeshMember{
			NodeRef: nodeRef, Hostname: hostname, Role: role, AddressMode: mode, ProbeAddress: probe,
			CreatedAt: now, UpdatedAt: now,
		}
		if err := placeAddress(tx, region, &member, input.Address, now); err != nil {
			return err
		}
		member.RegionID = &region.ID
		return tx.Create(&member).Error
	})
	return member, err
}

// UpdateMember changes a member in its region: hostname, role, mode,
// probe address, Agent node and, for a fixed address, the address.
func (s *Service) UpdateMember(id uint, input MemberInput) (model.MeshMember, error) {
	hostname, role, mode, probe, nodeRef, err := readMemberInput(input)
	if err != nil {
		return model.MeshMember{}, err
	}
	var member model.MeshMember
	err = s.transaction(func(tx *gorm.DB) error {
		if member, err = memberByID(tx, id); err != nil {
			return err
		}
		if member.RegionID == nil {
			return conflict("the member has not entered a region yet")
		}
		region, err := regionByID(tx, *member.RegionID)
		if err != nil {
			return err
		}
		if err := checkNameFree(tx, region.ID, hostname, id, 0); err != nil {
			return err
		}
		if err := checkNodeRef(tx, nodeRef, id); err != nil {
			return err
		}
		if member.Role == model.MeshRoleGateway && role != model.MeshRoleGateway {
			if err := refuseIfGateway(tx, id); err != nil {
				return err
			}
			var published int64
			if err := tx.Model(&model.MeshDeviceName{}).Where("member_id = ?", id).Count(&published).Error; err != nil {
				return err
			}
			if published > 0 {
				return conflict("the gateway publishes %d device names; delete them first", published)
			}
		}
		now := s.now()
		modeChanged := mode != member.AddressMode
		member.Hostname, member.Role, member.ProbeAddress, member.NodeRef = hostname, role, probe, nodeRef
		member.AddressMode = mode
		member.UpdatedAt = now
		requested := strings.TrimSpace(input.Address)
		if mode == model.MeshAddressFixed && requested != "" && (member.Address == nil || requested != *member.Address) {
			if err := placeAddress(tx, region, &member, requested, now); err != nil {
				return err
			}
		} else if modeChanged {
			member.Address = nil
			if err := placeAddress(tx, region, &member, requested, now); err != nil {
				return err
			}
		}
		return tx.Save(&member).Error
	})
	return member, err
}

// refuseIfGateway refuses a change while the member is a host group's
// gateway.
func refuseIfGateway(tx *gorm.DB, id uint) error {
	var groups int64
	if err := tx.Model(&model.MeshHostGroup{}).Where("gateway_member_id = ? OR standby_member_id = ?", id, id).Count(&groups).Error; err != nil {
		return err
	}
	if groups > 0 {
		return conflict("the member is the gateway of %d host groups; change their gateways first", groups)
	}
	return nil
}

// DeleteMember removes a member, its names and its region changes. A
// host group's gateway is refused.
func (s *Service) DeleteMember(id uint) error {
	return s.transaction(func(tx *gorm.DB) error {
		member, err := memberByID(tx, id)
		if err != nil {
			return err
		}
		if err := refuseIfGateway(tx, id); err != nil {
			return err
		}
		if err := tx.Where("member_id = ?", id).Delete(&model.MeshDeviceName{}).Error; err != nil {
			return err
		}
		if err := tx.Where("member_id = ?", id).Delete(&model.MeshRegionChange{}).Error; err != nil {
			return err
		}
		return tx.Delete(&member).Error
	})
}

// Members answers the members of a region (0 for all), by region and
// hostname.
func (s *Service) Members(regionID uint) ([]MemberView, error) {
	var views []MemberView
	err := s.transaction(func(tx *gorm.DB) error {
		settings, err := loadSettings(tx)
		if err != nil {
			return err
		}
		var regions []model.MeshRegion
		if err := tx.Find(&regions).Error; err != nil {
			return err
		}
		codes := make(map[uint]string, len(regions))
		for _, region := range regions {
			codes[region.ID] = region.Code
		}
		query := tx.Order("region_id, hostname, id")
		if regionID != 0 {
			query = query.Where("region_id = ?", regionID)
		}
		var members []model.MeshMember
		if err := query.Find(&members).Error; err != nil {
			return err
		}
		now := s.now()
		views = make([]MemberView, 0, len(members))
		for _, member := range members {
			view := MemberView{MeshMember: member, Warnings: []string{}}
			if member.RegionID != nil {
				view.RegionCode = codes[*member.RegionID]
				view.FQDN = member.Hostname + "." + view.RegionCode + "." + settings.NameSuffix
			}
			if member.Address == nil {
				view.Warnings = append(view.Warnings, WarningNoAddress)
			} else if reclaimable(member, now) {
				view.Warnings = append(view.Warnings, WarningPoolAddressIdle)
			}
			views = append(views, view)
		}
		return nil
	})
	return views, err
}
