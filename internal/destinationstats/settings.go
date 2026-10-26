// Package destinationstats records and answers each user's traffic by
// destination: the DestinationReport batches Agents send on the Agent
// Control stream (destinations.v1), summed per UTC hour, node, user, domain
// and sniffed protocol in v5_destination_stat, and the categories (video,
// messaging, download, ...) the domains fall in. The design and its limits
// are in docs/reference/destination-stats.md.
//
// The statistics are off until an administrator turns them on: Agents
// collect whenever they negotiate destinations.v1, and Control drops their
// batches while the setting is off.
package destinationstats

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

// The v2_system_config keys of the settings.
const (
	EnabledKey       = "traffic.destinations.enabled"
	RetentionDaysKey = "traffic.destinations.retention_days"
	settingsGroup    = "traffic"
)

// Retention bounds, in days.
const (
	DefaultRetentionDays = 30
	MinRetentionDays     = 7
	MaxRetentionDays     = 180
)

// ErrRetentionDays refuses a retention outside MinRetentionDays and
// MaxRetentionDays.
var ErrRetentionDays = errors.New("retention_days must be between 7 and 180")

// Settings are the statistics' settings.
type Settings struct {
	// Enabled records the Agents' batches; off, they are dropped.
	Enabled bool `json:"enabled"`
	// RetentionDays is how long a row is kept.
	RetentionDays int `json:"retention_days"`
}

// Retention is RetentionDays as a duration.
func (s Settings) Retention() time.Duration {
	return time.Duration(s.RetentionDays) * 24 * time.Hour
}

// LoadSettings reads the settings, with the defaults for missing or invalid
// values: off, DefaultRetentionDays.
func LoadSettings(db *gorm.DB) (Settings, error) {
	settings := Settings{RetentionDays: DefaultRetentionDays}
	var rows []model.SystemConfig
	if err := db.Where("key IN ?", []string{EnabledKey, RetentionDaysKey}).Find(&rows).Error; err != nil {
		return settings, err
	}
	for _, row := range rows {
		value := strings.TrimSpace(row.Value)
		switch row.Key {
		case EnabledKey:
			settings.Enabled, _ = strconv.ParseBool(value)
		case RetentionDaysKey:
			if days, err := strconv.Atoi(value); err == nil && days >= MinRetentionDays && days <= MaxRetentionDays {
				settings.RetentionDays = days
			}
		}
	}
	return settings, nil
}

// SaveSettings stores the settings, refusing an out-of-range retention.
func SaveSettings(db *gorm.DB, settings Settings) error {
	if settings.RetentionDays < MinRetentionDays || settings.RetentionDays > MaxRetentionDays {
		return ErrRetentionDays
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := saveSetting(tx, EnabledKey, strconv.FormatBool(settings.Enabled), "boolean", "按目标域名的流量统计"); err != nil {
			return err
		}
		return saveSetting(tx, RetentionDaysKey, strconv.Itoa(settings.RetentionDays), "number", "按目标域名的流量统计保留天数")
	})
}

func saveSetting(tx *gorm.DB, key, value, kind, remark string) error {
	var row model.SystemConfig
	err := tx.Where("key = ?", key).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return tx.Create(&model.SystemConfig{Key: key, Value: value, Type: kind, Group: settingsGroup, Remark: remark}).Error
	}
	if err != nil {
		return err
	}
	return tx.Model(&row).Update("value", value).Error
}
