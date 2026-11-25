// Package meshregion is the panel side of the intranet mesh
// (docs/design/mesh-regions): the regions an administrator defines and
// their border nodes, the members' virtual addresses, the host groups and
// the names their gateways publish, and the region changes members ask
// for.
//
// The mesh itself runs in the node kernels. A region is an independent
// mesh network with its own network name and key and a virtual IPv4 subnet
// no other region, host group or reserved range overlaps. The subnet is
// split into the server range, where infrastructure nodes (border nodes,
// gateways, servers) get fixed addresses, and the device pool, where a
// personal device gets an address it keeps: the address stays the device's
// until it has gone unused for 30 days, and only then goes to the next
// device that needs one. Host groups take their segments from a separate
// supernet. Every member's name is <hostname>.<region code>.<suffix>.
//
// A member enters its first region automatically; moving to another
// region changes its address, so a member only asks, and an administrator
// approves or rejects (or moves the member directly). Every write runs in
// one transaction, so the address checks and the unique index on (region,
// address) agree.
package meshregion

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

// Defaults of a new mesh.
const (
	DefaultNameSuffix        = "mesh.internal"
	DefaultHostGroupSupernet = "10.127.0.0/16"
	DefaultHostGroupPrefix   = 24
	// PoolReclaimAfter is how long a pool address stays its device's
	// without use (R29).
	PoolReclaimAfter = 30 * 24 * time.Hour
	// MinBorderNodes is how many border nodes a region needs for redundancy.
	MinBorderNodes = 2
	settingsID     = 1
)

// DefaultRegion is one of the regions a new mesh starts with.
type DefaultRegion struct {
	Code, Name, Subnet, ServerRange, DevicePool string
}

// DefaultRegions are the four regions of a new mesh (R27): mainland China,
// Asia-Pacific (Japan, Hong Kong, Taiwan and the rest, not split), Europe
// and North America. Administrators add and delete regions.
var DefaultRegions = []DefaultRegion{
	{Code: "cn", Name: "中国大陆", Subnet: "10.121.0.0/16", ServerRange: "10.121.0.0/20", DevicePool: "10.121.64.0/18"},
	{Code: "ap", Name: "亚太", Subnet: "10.122.0.0/16", ServerRange: "10.122.0.0/20", DevicePool: "10.122.64.0/18"},
	{Code: "eu", Name: "欧洲", Subnet: "10.123.0.0/16", ServerRange: "10.123.0.0/20", DevicePool: "10.123.64.0/18"},
	{Code: "na", Name: "北美", Subnet: "10.124.0.0/16", ServerRange: "10.124.0.0/20", DevicePool: "10.124.64.0/18"},
}

var (
	// ErrInvalid: a field is invalid; the error says which.
	ErrInvalid = errors.New("invalid mesh request")
	// ErrNotFound: no such region, member, host group, name or change.
	ErrNotFound = errors.New("not found")
	// ErrConflict: the change conflicts with the stored state; the error
	// says how.
	ErrConflict = errors.New("mesh conflict")
	// ErrExhausted: no free address is left in the range.
	ErrExhausted = errors.New("no free address left")
)

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

func conflict(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrConflict, fmt.Sprintf(format, args...))
}

func notFound(what string) error {
	return fmt.Errorf("%w: %s", ErrNotFound, what)
}

// Service is the mesh on one database.
type Service struct {
	DB *gorm.DB
	// Now defaults to time.Now.
	Now func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

// transaction runs fn in a transaction after the defaults are in place.
func (s *Service) transaction(fn func(tx *gorm.DB) error) error {
	return s.DB.Transaction(func(tx *gorm.DB) error {
		if err := ensureDefaults(tx, s.now()); err != nil {
			return err
		}
		return fn(tx)
	})
}

// ensureDefaults creates the settings row and the default regions the
// first time the mesh is used.
func ensureDefaults(tx *gorm.DB, now time.Time) error {
	var count int64
	if err := tx.Model(&model.MeshSettings{}).Where("id = ?", settingsID).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	settings := model.MeshSettings{
		ID: settingsID, NameSuffix: DefaultNameSuffix, HostGroupSupernet: DefaultHostGroupSupernet,
		HostGroupPrefix: DefaultHostGroupPrefix, UpdatedAt: now,
	}
	if err := tx.Create(&settings).Error; err != nil {
		return err
	}
	for i, region := range DefaultRegions {
		name, err := networkName(region.Code)
		if err != nil {
			return err
		}
		row := model.MeshRegion{
			Code: region.Code, Name: region.Name, SortOrder: i + 1, Subnet: region.Subnet,
			ServerRange: region.ServerRange, DevicePool: region.DevicePool, NetworkName: name,
			CreatedAt: now, UpdatedAt: now,
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

// networkName generates a region's network name: the code and 12 random
// hexadecimal digits, so that two panels never share a name.
func networkName(code string) (string, error) {
	random := make([]byte, 6)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	return "mesh-" + code + "-" + hex.EncodeToString(random), nil
}

func loadSettings(tx *gorm.DB) (model.MeshSettings, error) {
	var settings model.MeshSettings
	err := tx.First(&settings, settingsID).Error
	return settings, err
}

// Settings answers the global settings.
func (s *Service) Settings() (model.MeshSettings, error) {
	var settings model.MeshSettings
	err := s.transaction(func(tx *gorm.DB) error {
		var err error
		settings, err = loadSettings(tx)
		return err
	})
	return settings, err
}

// SettingsUpdate changes the global settings.
type SettingsUpdate struct {
	NameSuffix        string
	HostGroupSupernet string
	HostGroupPrefix   int
	ReservedSubnets   []string
}

// UpdateSettings changes the global settings. The host group supernet and
// prefix change only while no host group exists; the reserved ranges may
// not overlap a region.
func (s *Service) UpdateSettings(update SettingsUpdate) (model.MeshSettings, error) {
	suffix, err := normalizeSuffix(update.NameSuffix)
	if err != nil {
		return model.MeshSettings{}, err
	}
	supernet, err := parsePrivatePrefix("host_group_supernet", update.HostGroupSupernet)
	if err != nil {
		return model.MeshSettings{}, err
	}
	if update.HostGroupPrefix < supernet.Bits() || update.HostGroupPrefix > 30 {
		return model.MeshSettings{}, invalid("host_group_prefix: %d to 30", supernet.Bits())
	}
	if update.HostGroupPrefix-supernet.Bits() > 16 {
		return model.MeshSettings{}, invalid("host_group_prefix: at most 16 bits longer than the supernet's")
	}
	reserved := make([]string, 0, len(update.ReservedSubnets))
	for _, raw := range update.ReservedSubnets {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		prefix, err := parsePrefix("reserved_subnets", raw)
		if err != nil {
			return model.MeshSettings{}, err
		}
		if prefix.Overlaps(supernet) {
			return model.MeshSettings{}, invalid("reserved_subnets: %s overlaps the host group supernet", prefix)
		}
		reserved = append(reserved, prefix.String())
	}
	var settings model.MeshSettings
	err = s.transaction(func(tx *gorm.DB) error {
		var err error
		settings, err = loadSettings(tx)
		if err != nil {
			return err
		}
		if supernet.String() != settings.HostGroupSupernet || update.HostGroupPrefix != settings.HostGroupPrefix {
			var groups int64
			if err := tx.Model(&model.MeshHostGroup{}).Count(&groups).Error; err != nil {
				return err
			}
			if groups > 0 {
				return conflict("the host group supernet and prefix change only while no host group exists")
			}
		}
		var regions []model.MeshRegion
		if err := tx.Find(&regions).Error; err != nil {
			return err
		}
		for _, region := range regions {
			subnet := mustPrefix(region.Subnet)
			if subnet.Overlaps(supernet) {
				return invalid("host_group_supernet: overlaps region %s (%s)", region.Code, region.Subnet)
			}
			for _, raw := range reserved {
				if subnet.Overlaps(mustPrefix(raw)) {
					return invalid("reserved_subnets: %s overlaps region %s (%s)", raw, region.Code, region.Subnet)
				}
			}
		}
		settings.NameSuffix = suffix
		settings.HostGroupSupernet = supernet.String()
		settings.HostGroupPrefix = update.HostGroupPrefix
		settings.ReservedSubnets = strings.Join(reserved, ",")
		settings.UpdatedAt = s.now()
		return tx.Save(&settings).Error
	})
	return settings, err
}

// reservedOf answers the settings' reserved ranges.
func reservedOf(settings model.MeshSettings) []string {
	var out []string
	for _, raw := range strings.Split(settings.ReservedSubnets, ",") {
		if raw = strings.TrimSpace(raw); raw != "" {
			out = append(out, raw)
		}
	}
	return out
}
