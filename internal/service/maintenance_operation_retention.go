package service

import (
	"encoding/json"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
)

// CleanupMaintenanceChangesTx expires completed change summaries and their
// audit together after a full year from the last terminal operation. Queued,
// malformed, missing, unresolved, and currently pinned evidence is retained.
func CleanupMaintenanceChangesTx(tx *gorm.DB, now time.Time) error {
	cutoff := now.AddDate(-1, 0, 0)
	var afterID uint
	for {
		var candidates []model.MaintenanceChange
		if err := tx.Where("status = ? AND queued_at < ? AND id > ?", "queued", cutoff, afterID).Order("id").Limit(200).Find(&candidates).Error; err != nil {
			return err
		}
		if len(candidates) == 0 {
			return nil
		}
		for _, change := range candidates {
			afterID = change.ID
			var pinned int64
			if err := tx.Model(&model.MaintenancePluginOverride{}).Joins("JOIN v3_kernel_node_plugin_lifecycle AS current_lifecycle ON current_lifecycle.node_id = v3_maintenance_plugin_override.node_id AND current_lifecycle.plugin_id = v3_maintenance_plugin_override.plugin_id AND current_lifecycle.desired_generation = v3_maintenance_plugin_override.generation AND current_lifecycle.desired_version = v3_maintenance_plugin_override.version").Where("v3_maintenance_plugin_override.change_id = ?", change.ID).Count(&pinned).Error; err != nil {
				return err
			}
			if pinned > 0 {
				continue
			}
			var ids []string
			if json.Unmarshal([]byte(change.OperationIDsJSON), &ids) != nil || len(ids) == 0 {
				continue
			}
			var operations []model.KernelOperation
			if err := tx.Where("id IN ?", ids).Find(&operations).Error; err != nil {
				return err
			}
			if len(operations) != len(ids) {
				continue
			}
			expired := true
			for _, operation := range operations {
				switch operation.State {
				case "succeeded", "completed", "failed", "cancelled", "timed_out":
				default:
					expired = false
				}
				if !operation.UpdatedAt.Before(cutoff) {
					expired = false
				}
			}
			if !expired {
				continue
			}
			var recentAudit int64
			if err := tx.Model(&model.MaintenanceChangeAudit{}).Where("change_id = ? AND created_at >= ?", change.ID, cutoff).Count(&recentAudit).Error; err != nil {
				return err
			}
			if recentAudit > 0 {
				continue
			}
			if err := tx.Where("change_id = ?", change.ID).Delete(&model.MaintenancePluginOverride{}).Error; err != nil {
				return err
			}
			if err := tx.Where("change_id = ?", change.ID).Delete(&model.MaintenanceChangeAudit{}).Error; err != nil {
				return err
			}
			if err := tx.Delete(&model.MaintenanceChange{}, change.ID).Error; err != nil {
				return err
			}
		}
		if len(candidates) < 200 {
			return nil
		}
	}
}
