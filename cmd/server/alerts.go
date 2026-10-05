package main

import (
	"context"

	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/handler"
	"github.com/AnixOps/anix-control/v4/internal/kernelalerts"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"gorm.io/gorm"
)

// adminAlertNotifier delivers the alert monitor's digests through the
// notification service: an in-app entry for every administrator, e-mail
// when the notification e-mail settings are configured, and Telegram for
// administrators who bound an account.
type adminAlertNotifier struct {
	db  *gorm.DB
	cfg *config.Config
}

func (n adminAlertNotifier) Notify(ctx context.Context, notification kernelalerts.Notification) error {
	notifications := service.NewNotificationService(n.db.WithContext(ctx), n.cfg)
	// A missing or unreadable e-mail configuration leaves e-mail off; the
	// other channels still deliver.
	if email, err := handler.LoadNotificationEmailConfig(n.db); err == nil && email != nil {
		notifications.SetEmailConfig(email)
	}
	err := notifications.NotifyAdministrators(model.EventSystemAlert, notification.Title, notification.Content,
		map[string]any{"alerts": len(notification.Alerts)})
	notifications.Drain()
	return err
}

// kernelAlertMonitor builds the alert monitor from the configuration
// (alerts.*).
func kernelAlertMonitor(db *gorm.DB, cfg *config.Config) *kernelalerts.Monitor {
	if cfg == nil {
		cfg = &config.Config{}
	}
	return &kernelalerts.Monitor{
		DB: db, Settings: cfg.Alerts.Settings(), Notifier: adminAlertNotifier{db: db, cfg: cfg},
	}
}
