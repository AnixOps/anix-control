package main

import (
	"context"

	"github.com/AnixOps/anix-control/v5/internal/builtinpkg"
	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/handler"
	"github.com/AnixOps/anix-control/v5/internal/kernelalerts"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/service"
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
		BuiltinPackages: builtinPackageProblems(db, cfg),
	}
}

// builtinPackageProblems is the alert scan's check of the official packages
// built into this binary (builtin_package_invalid, builtin_package_failed).
func builtinPackageProblems(db *gorm.DB, cfg *config.Config) func(context.Context) ([]kernelalerts.BuiltinPackageProblem, error) {
	return func(ctx context.Context) ([]kernelalerts.BuiltinPackageProblem, error) {
		set, setErr := builtinpkg.Default()
		statuses, err := service.BuiltinPackageProblems(db.WithContext(ctx), cfg.Plugins.OfficialPublicKey, set, setErr)
		if err != nil {
			return nil, err
		}
		problems := make([]kernelalerts.BuiltinPackageProblem, 0, len(statuses))
		for _, status := range statuses {
			problems = append(problems, kernelalerts.BuiltinPackageProblem{
				PackageID: status.ID, Version: status.Version, Invalid: status.Invalid,
				State: status.State, LastError: status.LastError, Reason: status.Reason,
			})
		}
		return problems, nil
	}
}
