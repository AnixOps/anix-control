package meshregion

import (
	"context"
	"net/netip"
	"regexp"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Defaults of the global settings, until an administrator saves them.
const (
	DefaultNameSuffix        = "mesh.internal"
	DefaultHostGroupSupernet = "10.127.0.0/16"
	DefaultHostGroupPrefix   = 24
	settingsID               = 1
)

// Settings are the mesh's global settings.
type Settings struct {
	// NameSuffix: a name is <name>.<region code>.<suffix> (R28).
	NameSuffix string `json:"name_suffix"`
	// HostGroupSupernet and HostGroupPrefix: the host groups' segments
	// are /HostGroupPrefix ranges of the supernet (R21).
	HostGroupSupernet string `json:"host_group_supernet"`
	HostGroupPrefix   int    `json:"host_group_prefix"`
	// ReservedRanges: ranges no region or host group may overlap, such as
	// the range of a network the mesh replaces. They have the regions'
	// prefix length: during a migration the old network and the regions
	// carry traffic for each other (spec section 15.7, item 2).
	ReservedRanges []string   `json:"reserved_ranges"`
	UpdatedAt      *time.Time `json:"updated_at"`
	// Warnings: WarningSharedAddressSpace when the host group supernet
	// lies in 100.64.0.0/10.
	Warnings []string `json:"warnings"`
}

// withWarnings answers settings with their warnings.
func withWarnings(settings Settings) Settings {
	settings.Warnings = []string{}
	if inSharedSpace(settings.HostGroupSupernet) {
		settings.Warnings = append(settings.Warnings, WarningSharedAddressSpace)
	}
	return settings
}

// maxSuffix keeps a full name <63-byte name>.<16-byte code>.<suffix>
// within the 253 bytes of a DNS name.
const maxSuffix = 253 - 63 - 1 - 16 - 1

// privateSuffix tells whether suffix lies under a domain reserved for
// private networks, so the names never shadow public ones.
func privateSuffix(suffix string) bool {
	for _, private := range []string{"internal", "home.arpa"} {
		if suffix == private || strings.HasSuffix(suffix, "."+private) {
			return true
		}
	}
	return false
}

var (
	labelPattern  = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
	suffixPattern = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)*[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
)

// normalizeLabel reads a name as one DNS label: lowercase letters, digits
// and inner hyphens, 1 to 63 characters.
func normalizeLabel(field, raw string) (string, error) {
	label := strings.ToLower(strings.TrimSpace(raw))
	if !labelPattern.MatchString(label) {
		return "", invalid("%s: letters, digits and inner hyphens, 1 to 63 characters (one DNS label)", field)
	}
	return label, nil
}

// parseRange reads an IPv4 network in its canonical form.
func parseRange(field, raw string) (netip.Prefix, error) {
	prefix, err := netip.ParsePrefix(strings.TrimSpace(raw))
	if err != nil || !prefix.Addr().Is4() || prefix.Masked() != prefix {
		return netip.Prefix{}, invalid("%s: an IPv4 network such as 10.127.0.0/16", field)
	}
	return prefix, nil
}

// isPrivate tells whether prefix lies inside one of the private ranges.
func isPrivate(prefix netip.Prefix) bool {
	for _, private := range privateRanges {
		if private.Bits() <= prefix.Bits() && private.Contains(prefix.Addr()) {
			return true
		}
	}
	return false
}

func defaultSettings() Settings {
	return Settings{NameSuffix: DefaultNameSuffix, HostGroupSupernet: DefaultHostGroupSupernet, HostGroupPrefix: DefaultHostGroupPrefix, ReservedRanges: []string{}}
}

// loadSettings answers the saved settings, or the defaults.
func loadSettings(tx *gorm.DB) (Settings, error) {
	var rows []model.MeshSettings
	if err := tx.Where("id = ?", settingsID).Limit(1).Find(&rows).Error; err != nil {
		return Settings{}, err
	}
	if len(rows) == 0 {
		return withWarnings(defaultSettings()), nil
	}
	row := rows[0]
	settings := Settings{NameSuffix: row.NameSuffix, HostGroupSupernet: row.HostGroupSupernet, HostGroupPrefix: row.HostGroupPrefix, ReservedRanges: []string{}, UpdatedAt: &row.UpdatedAt}
	for _, raw := range strings.Split(row.ReservedRanges, ",") {
		if raw = strings.TrimSpace(raw); raw != "" {
			settings.ReservedRanges = append(settings.ReservedRanges, raw)
		}
	}
	return withWarnings(settings), nil
}

// LoadSettings answers the global settings.
func LoadSettings(ctx context.Context, db *gorm.DB) (Settings, error) {
	return loadSettings(db.WithContext(ctx))
}

// SettingsInput changes the global settings.
type SettingsInput struct {
	NameSuffix        string   `json:"name_suffix"`
	HostGroupSupernet string   `json:"host_group_supernet"`
	HostGroupPrefix   int      `json:"host_group_prefix"`
	ReservedRanges    []string `json:"reserved_ranges"`
}

// SaveSettings changes the global settings. The supernet and the segment
// length change only while no host group exists; neither the supernet nor
// a reserved range may overlap a region.
func SaveSettings(ctx context.Context, db *gorm.DB, input SettingsInput, now time.Time) (Settings, error) {
	suffix := strings.Trim(strings.ToLower(strings.TrimSpace(input.NameSuffix)), ".")
	if len(suffix) > maxSuffix || !suffixPattern.MatchString(suffix) {
		return Settings{}, invalid("name_suffix: DNS labels separated by dots, such as mesh.internal, at most %d characters", maxSuffix)
	}
	if !privateSuffix(suffix) {
		return Settings{}, invalid("name_suffix: under internal or home.arpa, which no public name uses")
	}
	supernet, err := parseRange("host_group_supernet", input.HostGroupSupernet)
	if err != nil {
		return Settings{}, err
	}
	if supernet.Bits() < 8 || supernet.Bits() > 24 || !isPrivate(supernet) {
		return Settings{}, invalid("host_group_supernet: a private network of /8 to /24")
	}
	if input.HostGroupPrefix < supernet.Bits() || input.HostGroupPrefix > 30 || input.HostGroupPrefix-supernet.Bits() > 16 {
		return Settings{}, invalid("host_group_prefix: %d to %d", supernet.Bits(), min(30, supernet.Bits()+16))
	}
	reserved := []string{}
	for _, raw := range input.ReservedRanges {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		prefix, err := parseRange("reserved_ranges", raw)
		if err != nil {
			return Settings{}, err
		}
		if prefix.Overlaps(supernet) {
			return Settings{}, invalid("reserved_ranges: %s overlaps the host group supernet", prefix)
		}
		reserved = append(reserved, prefix.String())
	}
	if len(reserved) > 32 {
		return Settings{}, invalid("reserved_ranges: at most 32")
	}
	var answer Settings
	err = change(ctx, db, func(tx *gorm.DB) error {
		if err := lockAddressPlan(tx); err != nil {
			return err
		}
		current, err := loadSettings(tx)
		if err != nil {
			return err
		}
		if supernet.String() != current.HostGroupSupernet || input.HostGroupPrefix != current.HostGroupPrefix {
			var groups int64
			if err := tx.Model(&model.MeshHostGroup{}).Count(&groups).Error; err != nil {
				return err
			}
			if groups > 0 {
				return conflict("the host group supernet and segment length change only while no host group exists")
			}
		}
		var regions []model.MeshRegion
		if err := tx.Find(&regions).Error; err != nil {
			return err
		}
		for _, region := range regions {
			prefix, err := netip.ParsePrefix(region.CIDR)
			if err != nil {
				continue
			}
			if prefix.Overlaps(supernet) {
				return conflict("host_group_supernet: overlaps region %q (%s)", region.Code, region.CIDR)
			}
			for _, raw := range reserved {
				if prefix.Overlaps(netip.MustParsePrefix(raw)) {
					return conflict("reserved_ranges: %s overlaps region %q (%s)", raw, region.Code, region.CIDR)
				}
				if bits := netip.MustParsePrefix(raw).Bits(); bits != prefix.Bits() {
					return conflict("reserved_ranges: %s has /%d; the regions have /%d", raw, bits, prefix.Bits())
				}
			}
		}
		if err := checkFullNodeSubnets(tx, "host_group_supernet", supernet); err != nil {
			return err
		}
		for _, raw := range reserved {
			if err := checkFullNodeSubnets(tx, "reserved_ranges", netip.MustParsePrefix(raw)); err != nil {
				return err
			}
		}
		row := model.MeshSettings{ID: settingsID, NameSuffix: suffix, HostGroupSupernet: supernet.String(), HostGroupPrefix: input.HostGroupPrefix, ReservedRanges: strings.Join(reserved, ","), UpdatedAt: now}
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{"name_suffix", "host_group_supernet", "host_group_prefix", "reserved_ranges", "updated_at"}),
		}).Create(&row).Error; err != nil {
			return err
		}
		answer, err = loadSettings(tx)
		return err
	})
	return answer, err
}

// checkOutsideSettings refuses a region range that overlaps the host group
// supernet or a reserved range, or whose prefix length is not the reserved
// ranges'.
func checkOutsideSettings(tx *gorm.DB, prefix netip.Prefix) error {
	settings, err := loadSettings(tx)
	if err != nil {
		return err
	}
	if supernet, err := netip.ParsePrefix(settings.HostGroupSupernet); err == nil && supernet.Overlaps(prefix) {
		return conflict("the range overlaps the host group supernet %s", settings.HostGroupSupernet)
	}
	for _, raw := range settings.ReservedRanges {
		reserved, err := netip.ParsePrefix(raw)
		if err != nil {
			continue
		}
		if reserved.Overlaps(prefix) {
			return conflict("the range overlaps the reserved range %s", raw)
		}
		if reserved.Bits() != prefix.Bits() {
			return conflict("every region has the prefix length of the reserved ranges: /%d like %s", reserved.Bits(), raw)
		}
	}
	return nil
}
