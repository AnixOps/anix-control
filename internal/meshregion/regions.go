package meshregion

import (
	"errors"
	"net/netip"
	"strings"
	"unicode/utf8"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

// Region warnings.
const (
	// WarningFewBorders: fewer than MinBorderNodes border nodes.
	WarningFewBorders = "few_border_nodes"
	// WarningBorderWithoutProbe: a border node has no probe address.
	WarningBorderWithoutProbe = "border_without_probe_address"
	// WarningPoolLow: less than a tenth of the device pool is free.
	WarningPoolLow = "device_pool_low"
)

// RegionView is a region with its members counted and its warnings.
type RegionView struct {
	model.MeshRegion
	// Security is always "forward_secrecy" (R30).
	Security    string `json:"security"`
	BorderNodes int    `json:"border_nodes"`
	Members     int    `json:"members"`
	// PoolUsed and PoolSize count the device pool's addresses.
	PoolUsed int      `json:"pool_used"`
	PoolSize int      `json:"pool_size"`
	Warnings []string `json:"warnings"`
}

// SecurityForwardSecrecy is the only security mode of a region network.
const SecurityForwardSecrecy = "forward_secrecy"

// RegionInput creates or changes a region.
type RegionInput struct {
	Code        string
	Name        string
	SortOrder   int
	Subnet      string
	ServerRange string
	DevicePool  string
}

// usableCount answers how many member addresses a range inside subnet has.
func usableCount(subnet, rangePrefix netip.Prefix) int {
	size := 1 << (32 - rangePrefix.Bits())
	if rangePrefix.Contains(subnet.Masked().Addr()) {
		size--
	}
	if rangePrefix.Contains(lastAddr(subnet)) {
		size--
	}
	if size < 0 {
		return 0
	}
	return size
}

// Regions answers every region, by sort order and code.
func (s *Service) Regions() ([]RegionView, error) {
	var views []RegionView
	err := s.transaction(func(tx *gorm.DB) error {
		var err error
		views, err = regionViews(tx)
		return err
	})
	return views, err
}

func regionViews(tx *gorm.DB) ([]RegionView, error) {
	var regions []model.MeshRegion
	if err := tx.Order("sort_order, code").Find(&regions).Error; err != nil {
		return nil, err
	}
	var members []model.MeshMember
	if err := tx.Where("region_id IS NOT NULL").Find(&members).Error; err != nil {
		return nil, err
	}
	views := make([]RegionView, 0, len(regions))
	for _, region := range regions {
		view := RegionView{MeshRegion: region, Security: SecurityForwardSecrecy, Warnings: []string{}}
		subnet, pool := mustPrefix(region.Subnet), mustPrefix(region.DevicePool)
		view.PoolSize = usableCount(subnet, pool)
		withoutProbe := false
		for _, member := range members {
			if *member.RegionID != region.ID {
				continue
			}
			view.Members++
			if member.Role == model.MeshRoleBorder {
				view.BorderNodes++
				if strings.TrimSpace(member.ProbeAddress) == "" {
					withoutProbe = true
				}
			}
			if member.Address != nil {
				if addr, err := netip.ParseAddr(*member.Address); err == nil && pool.Contains(addr) {
					view.PoolUsed++
				}
			}
		}
		if view.BorderNodes < MinBorderNodes {
			view.Warnings = append(view.Warnings, WarningFewBorders)
		}
		if withoutProbe {
			view.Warnings = append(view.Warnings, WarningBorderWithoutProbe)
		}
		if view.PoolSize > 0 && (view.PoolSize-view.PoolUsed)*10 < view.PoolSize {
			view.Warnings = append(view.Warnings, WarningPoolLow)
		}
		views = append(views, view)
	}
	return views, nil
}

// checkRegionInput validates input against the settings and the other
// regions (all but the one with id skip).
func checkRegionInput(tx *gorm.DB, input RegionInput, skip uint) (subnet, server, pool netip.Prefix, err error) {
	if name := strings.TrimSpace(input.Name); name == "" || utf8.RuneCountInString(name) > 64 {
		return subnet, server, pool, invalid("name: 1 to 64 characters")
	}
	if input.SortOrder < 0 || input.SortOrder > 10000 {
		return subnet, server, pool, invalid("sort_order: 0 to 10000")
	}
	if subnet, err = parsePrivatePrefix("subnet", input.Subnet); err != nil {
		return
	}
	if subnet.Bits() > 24 {
		return subnet, server, pool, invalid("subnet: a prefix length of 8 to 24")
	}
	if server, err = parsePrefix("server_range", input.ServerRange); err != nil {
		return
	}
	if pool, err = parsePrefix("device_pool", input.DevicePool); err != nil {
		return
	}
	for field, prefix := range map[string]netip.Prefix{"server_range": server, "device_pool": pool} {
		if prefix.Bits() < subnet.Bits() || !subnet.Contains(prefix.Addr()) {
			return subnet, server, pool, invalid("%s: a range inside the subnet %s", field, subnet)
		}
	}
	if server.Overlaps(pool) {
		return subnet, server, pool, invalid("server_range and device_pool: the two ranges may not overlap")
	}
	settings, err := loadSettings(tx)
	if err != nil {
		return
	}
	if subnet.Overlaps(mustPrefix(settings.HostGroupSupernet)) {
		return subnet, server, pool, invalid("subnet: overlaps the host group supernet %s", settings.HostGroupSupernet)
	}
	for _, raw := range reservedOf(settings) {
		if subnet.Overlaps(mustPrefix(raw)) {
			return subnet, server, pool, invalid("subnet: overlaps the reserved range %s", raw)
		}
	}
	var others []model.MeshRegion
	if err = tx.Where("id <> ?", skip).Find(&others).Error; err != nil {
		return
	}
	for _, other := range others {
		if subnet.Overlaps(mustPrefix(other.Subnet)) {
			return subnet, server, pool, conflict("subnet: overlaps region %s (%s)", other.Code, other.Subnet)
		}
	}
	return subnet, server, pool, nil
}

// CreateRegion adds a region with a new network name.
func (s *Service) CreateRegion(input RegionInput) (model.MeshRegion, error) {
	code, err := normalizeCode(input.Code)
	if err != nil {
		return model.MeshRegion{}, err
	}
	var row model.MeshRegion
	err = s.transaction(func(tx *gorm.DB) error {
		subnet, server, pool, err := checkRegionInput(tx, input, 0)
		if err != nil {
			return err
		}
		var same int64
		if err := tx.Model(&model.MeshRegion{}).Where("code = ?", code).Count(&same).Error; err != nil {
			return err
		}
		if same > 0 {
			return conflict("code: region %s exists", code)
		}
		name, err := networkName(code)
		if err != nil {
			return err
		}
		now := s.now()
		row = model.MeshRegion{
			Code: code, Name: strings.TrimSpace(input.Name), SortOrder: input.SortOrder, Subnet: subnet.String(),
			ServerRange: server.String(), DevicePool: pool.String(), NetworkName: name, CreatedAt: now, UpdatedAt: now,
		}
		return tx.Create(&row).Error
	})
	return row, err
}

// UpdateRegion changes a region's name, order and ranges. The code never
// changes. The subnet changes only while the region has no member, and
// the ranges only while every member's address stays inside the range its
// mode uses.
func (s *Service) UpdateRegion(id uint, input RegionInput) (model.MeshRegion, error) {
	var row model.MeshRegion
	err := s.transaction(func(tx *gorm.DB) error {
		if err := tx.First(&row, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return notFound("region")
			}
			return err
		}
		subnet, server, pool, err := checkRegionInput(tx, input, id)
		if err != nil {
			return err
		}
		var members []model.MeshMember
		if err := tx.Where("region_id = ?", id).Find(&members).Error; err != nil {
			return err
		}
		if subnet.String() != row.Subnet && len(members) > 0 {
			return conflict("subnet: the region has members; move them first")
		}
		for _, member := range members {
			if member.Address == nil {
				continue
			}
			addr, err := netip.ParseAddr(*member.Address)
			if err != nil {
				continue
			}
			if member.AddressMode == model.MeshAddressPool && !pool.Contains(addr) {
				return conflict("device_pool: member %s keeps %s outside the new pool", member.Hostname, addr)
			}
			if member.AddressMode == model.MeshAddressFixed && pool.Contains(addr) {
				return conflict("device_pool: the fixed address %s of member %s would be inside the new pool", addr, member.Hostname)
			}
		}
		row.Name = strings.TrimSpace(input.Name)
		row.SortOrder = input.SortOrder
		row.Subnet, row.ServerRange, row.DevicePool = subnet.String(), server.String(), pool.String()
		row.UpdatedAt = s.now()
		return tx.Save(&row).Error
	})
	return row, err
}

// DeleteRegion removes a region without members or pending changes.
func (s *Service) DeleteRegion(id uint) error {
	return s.transaction(func(tx *gorm.DB) error {
		var row model.MeshRegion
		if err := tx.First(&row, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return notFound("region")
			}
			return err
		}
		var members int64
		if err := tx.Model(&model.MeshMember{}).Where("region_id = ?", id).Count(&members).Error; err != nil {
			return err
		}
		if members > 0 {
			return conflict("the region has %d members; move or remove them first", members)
		}
		var pending int64
		if err := tx.Model(&model.MeshRegionChange{}).Where("status = ? AND to_region_id = ?", model.MeshChangePending, id).Count(&pending).Error; err != nil {
			return err
		}
		if pending > 0 {
			return conflict("%d pending region changes name the region; decide them first", pending)
		}
		return tx.Delete(&row).Error
	})
}

// regionByID loads a region.
func regionByID(tx *gorm.DB, id uint) (model.MeshRegion, error) {
	var region model.MeshRegion
	if err := tx.First(&region, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return region, notFound("region")
		}
		return region, err
	}
	return region, nil
}
