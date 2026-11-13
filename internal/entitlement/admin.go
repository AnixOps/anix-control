package entitlement

import (
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// settingsGroup is the system configuration group of the settings keys.
const settingsGroup = "entitlement"

// ErrInvalidSettings means a settings change was out of range.
var ErrInvalidSettings = errors.New("entitlement: invalid settings")

// SettingsPatch changes some settings; a nil field keeps its value.
type SettingsPatch struct {
	Enabled   *bool
	Require   *bool
	TrialDays *int
	PlanIDs   *[]uint
}

// SaveSettings applies patch and answers the settings now in force. The
// ANX nodes get the change with their next configuration.
func SaveSettings(db *gorm.DB, patch SettingsPatch) (Settings, error) {
	if patch.TrialDays != nil && (*patch.TrialDays < 0 || *patch.TrialDays > MaxTrialDays) {
		return Settings{}, ErrInvalidSettings
	}
	if patch.PlanIDs != nil {
		for _, id := range *patch.PlanIDs {
			if id == 0 {
				return Settings{}, ErrInvalidSettings
			}
		}
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		put := func(key, kind, value, remark string) error {
			return tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "key"}},
				DoUpdates: clause.AssignmentColumns([]string{"value", "type", "group", "remark", "updated_at"}),
			}).Create(&model.SystemConfig{Key: key, Value: value, Type: kind, Group: settingsGroup, Remark: remark}).Error
		}
		if patch.Enabled != nil {
			if err := put(KeyANXEnabled, "boolean", strconv.FormatBool(*patch.Enabled), "ANX licensing on"); err != nil {
				return err
			}
		}
		if patch.Require != nil {
			if err := put(KeyANXRequire, "boolean", strconv.FormatBool(*patch.Require), "ANX nodes refuse unlicensed sessions"); err != nil {
				return err
			}
		}
		if patch.TrialDays != nil {
			if err := put(KeyANXTrialDays, "number", strconv.Itoa(*patch.TrialDays), "ANX trial days per account (0 = off)"); err != nil {
				return err
			}
		}
		if patch.PlanIDs != nil {
			ids := append([]uint(nil), *patch.PlanIDs...)
			sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
			parts := make([]string, 0, len(ids))
			for i, id := range ids {
				if i > 0 && ids[i-1] == id {
					continue
				}
				parts = append(parts, strconv.FormatUint(uint64(id), 10))
			}
			if err := put(KeyANXPlanIDs, "string", strings.Join(parts, ","), "plans that include ANX (empty = every plan)"); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return Settings{}, err
	}
	return LoadSettings(db)
}

// DeviceCounts sums up a feature's devices.
type DeviceCounts struct {
	// Total is every device ever registered.
	Total int64 `json:"total"`
	// Seated devices asked within SeatWindow and are not revoked.
	Seated int64 `json:"seated"`
	// Revoked devices get no license.
	Revoked int64 `json:"revoked"`
}

// Counts sums up feature's devices at now.
func Counts(db *gorm.DB, feature string, now time.Time) (DeviceCounts, error) {
	var counts DeviceCounts
	base := func() *gorm.DB { return db.Model(&model.EntitlementDevice{}).Where("feature = ?", feature) }
	if err := base().Count(&counts.Total).Error; err != nil {
		return counts, err
	}
	if err := base().Where("revoked_at IS NULL AND last_seen_at > ?", now.Add(-SeatWindow)).Count(&counts.Seated).Error; err != nil {
		return counts, err
	}
	err := base().Where("revoked_at IS NOT NULL").Count(&counts.Revoked).Error
	return counts, err
}

// DeviceQuery selects devices for the administrator's list.
type DeviceQuery struct {
	Feature string
	// UserID limits the list to one user; 0 lists everyone's.
	UserID uint
	// Limit and Offset page the list (newest activity first).
	Limit, Offset int
}

// ListDevices answers a page of devices and how many match.
func ListDevices(db *gorm.DB, q DeviceQuery) ([]model.EntitlementDevice, int64, error) {
	query := db.Model(&model.EntitlementDevice{}).Where("feature = ?", q.Feature)
	if q.UserID != 0 {
		query = query.Where("user_id = ?", q.UserID)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []model.EntitlementDevice
	err := query.Order("last_seen_at DESC").Order("id DESC").Limit(q.Limit).Offset(q.Offset).Find(&rows).Error
	return rows, total, err
}

// SetRevoked revokes (or restores) the device with row id id of feature.
// It answers the row afterwards and whether it changed;
// gorm.ErrRecordNotFound when there is no such device.
func SetRevoked(db *gorm.DB, feature string, id uint, revoked bool, now time.Time) (model.EntitlementDevice, bool, error) {
	var row model.EntitlementDevice
	if err := db.Where("id = ? AND feature = ?", id, feature).First(&row).Error; err != nil {
		return row, false, err
	}
	if (row.RevokedAt != nil) == revoked {
		return row, false, nil
	}
	var at *time.Time
	if revoked {
		t := now.UTC()
		at = &t
	}
	if err := db.Model(&row).Update("revoked_at", at).Error; err != nil {
		return row, false, err
	}
	row.RevokedAt = at
	return row, true, nil
}
