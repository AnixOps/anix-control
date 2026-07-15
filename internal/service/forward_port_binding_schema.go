package service

import (
	"fmt"

	"github.com/AnixOps/anix-control/v3/internal/model"
	"gorm.io/gorm"
)

func EnsureForwardPortBindingSchema(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database is required")
	}
	if err := db.AutoMigrate(&model.ForwardPortBinding{}); err != nil {
		return err
	}
	return BackfillForwardPortBindings(db)
}

func BackfillForwardPortBindings(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database is required")
	}
	if !db.Migrator().HasTable(&model.Forward{}) || !db.Migrator().HasTable(&model.ForwardTunnel{}) {
		return nil
	}

	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("1 = 1").Delete(&model.ForwardPortBinding{}).Error; err != nil {
			return err
		}

		var forwards []model.Forward
		if err := tx.Preload("Tunnel").Order("id ASC").Find(&forwards).Error; err != nil {
			return err
		}

		svc := NewPanelForwardService(tx)
		for i := range forwards {
			if forwards[i].Tunnel == nil {
				continue
			}
			backend, err := resolveForwardPortBindingBackfillBackend(tx, svc, &forwards[i])
			if err != nil {
				return fmt.Errorf("forward %d port binding backend: %w", forwards[i].ID, err)
			}
			bindings := buildForwardPortBindings(forwards[i].ID, forwards[i].Tunnel, backend, forwards[i].InPort)
			if err := svc.ensureForwardPortBindingsAvailableTx(tx, bindings, forwards[i].ID); err != nil {
				return fmt.Errorf("forward %d port binding conflict: %w", forwards[i].ID, err)
			}
			if len(bindings) == 0 {
				continue
			}
			if err := tx.Create(&bindings).Error; err != nil {
				return fmt.Errorf("forward %d port binding create: %w", forwards[i].ID, err)
			}
		}
		return nil
	})
}

func resolveForwardPortBindingBackfillBackend(db *gorm.DB, svc *PanelForwardService, forward *model.Forward) (string, error) {
	if backend, ok := normalizeForwardRuntimeBackend(forward.RuntimeBackend); ok {
		return backend, nil
	}
	if db != nil && !db.Migrator().HasTable(&model.SystemConfig{}) {
		return model.ForwardRuntimeBackendGost, nil
	}
	return svc.resolveRuntimeBackend()
}
