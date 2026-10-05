package main

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/kernelalerts"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// The monitor's digest goes to every unbanned administrator through the
// notification service; e-mail is added when the notification e-mail
// settings are saved and complete.
func TestAlertNotifierDeliversDigestsToAdministrators(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "alerts.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.TelegramUser{}, &model.NotificationLog{}, &model.SystemConfig{}))
	for _, user := range []model.User{
		{Email: "admin@example.com", Password: "h", Token: "t1", UUID: "u1", IsAdmin: 1},
		{Email: "banned@example.com", Password: "h", Token: "t2", UUID: "u2", IsAdmin: 1, Banned: 1},
		{Email: "member@example.com", Password: "h", Token: "t3", UUID: "u3"},
	} {
		require.NoError(t, db.Create(&user).Error)
	}
	notifier := adminAlertNotifier{db: db, cfg: &config.Config{}}
	notification := kernelalerts.Notification{Title: "AnixOps alert: 1 need attention", Content: "warning: something", Alerts: make([]model.KernelAlert, 1)}

	require.NoError(t, notifier.Notify(t.Context(), notification))
	var logs []model.NotificationLog
	require.NoError(t, db.Order("id").Find(&logs).Error)
	require.Len(t, logs, 1, "one in-app entry for the one unbanned administrator, no e-mail without settings")
	assert.Equal(t, "inapp", logs[0].Type)
	assert.Equal(t, model.EventSystemAlert, logs[0].Event)
	assert.Equal(t, "warning: something", logs[0].Content)

	// Incomplete e-mail settings stay off; complete ones add the e-mail
	// channel (the send itself fails without an SMTP server).
	emailSettings, err := json.Marshal(map[string]any{"host": "127.0.0.1", "port": 1, "from_address": "alerts@example.com", "encryption_type": "none"})
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.SystemConfig{Key: "notification.email.config", Value: string(emailSettings), Type: "json", Group: "notification"}).Error)
	require.NoError(t, db.Where("1 = 1").Delete(&model.NotificationLog{}).Error)
	require.NoError(t, notifier.Notify(t.Context(), notification))
	require.NoError(t, db.Order("type").Find(&logs).Error)
	types := []string{}
	for _, log := range logs {
		types = append(types, log.Type)
	}
	assert.ElementsMatch(t, []string{"inapp", "email"}, types)
	assert.Less(t, time.Since(logs[0].CreatedAt), time.Minute)
}

func TestKernelAlertMonitorFollowsTheConfiguration(t *testing.T) {
	off := false
	monitor := kernelAlertMonitor(nil, &config.Config{Alerts: config.AlertsConfig{Enabled: &off, LeafExpiryDays: 3}})
	assert.False(t, monitor.Settings.Enabled)
	assert.Equal(t, 3, monitor.Settings.LeafExpiryDays)
	assert.Equal(t, 60, monitor.Settings.CAExpiryDays)
	assert.NotNil(t, monitor.Notifier)
	monitor.Run(t.Context()) // disabled, and without a database: returns at once

	defaults := kernelAlertMonitor(nil, nil)
	assert.True(t, defaults.Settings.Enabled)
	assert.Equal(t, 14, defaults.Settings.LeafExpiryDays)
}
