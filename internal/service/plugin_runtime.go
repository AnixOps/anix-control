package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrRemoteRuntimeDisabled means the remote runtime is not enabled in the
// kernel configuration.
var ErrRemoteRuntimeDisabled = errors.New("the remote module runtime is not enabled (module_runtime.enabled)")

// PluginRuntimeOf returns the runtime of a Control package: remote only when
// a row selects it.
func PluginRuntimeOf(ctx context.Context, db *gorm.DB, pluginID string) (string, error) {
	if db == nil {
		return "", errors.New("database is not initialized")
	}
	var row model.PluginRuntime
	err := db.WithContext(ctx).First(&row, "plugin_id = ?", pluginID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.PluginRuntimeLocal, nil
	}
	if err != nil {
		return "", err
	}
	return row.Runtime, nil
}

// PluginRuntimeIsRemote adapts PluginRuntimeOf for the host supervisor.
func PluginRuntimeIsRemote(db *gorm.DB) func(context.Context, string) (bool, error) {
	return func(ctx context.Context, pluginID string) (bool, error) {
		runtime, err := PluginRuntimeOf(ctx, db, pluginID)
		return runtime == model.PluginRuntimeRemote, err
	}
}

// SetPluginRuntime selects a package's runtime. It takes effect at the
// package's next lifecycle operation. Remote requires the module runtime and
// PostgreSQL, because module instances cannot share a SQLite file.
func SetPluginRuntime(ctx context.Context, db *gorm.DB, pluginID, runtime string, remoteEnabled bool, actorID uint) (*model.PluginRuntime, error) {
	if db == nil {
		return nil, errors.New("database is not initialized")
	}
	if !safePluginSegment(pluginID) {
		return nil, fmt.Errorf("invalid plugin id %q", pluginID)
	}
	switch runtime {
	case model.PluginRuntimeLocal:
	case model.PluginRuntimeRemote:
		if !remoteEnabled {
			return nil, ErrRemoteRuntimeDisabled
		}
		if db.Name() != "postgres" {
			return nil, errors.New("the remote runtime requires PostgreSQL")
		}
	default:
		return nil, fmt.Errorf("runtime must be %q or %q", model.PluginRuntimeLocal, model.PluginRuntimeRemote)
	}
	var plugin model.Plugin
	if err := db.WithContext(ctx).First(&plugin, "id = ?", pluginID).Error; err != nil {
		return nil, err
	}
	row := model.PluginRuntime{PluginID: pluginID, Runtime: runtime, UpdatedBy: actorID, UpdatedAt: time.Now().UTC()}
	if err := db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "plugin_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"runtime", "updated_by", "updated_at"}),
	}).Create(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}
