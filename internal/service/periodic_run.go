package service

import (
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	periodicRunConfigGroup  = "scheduler"
	periodicRunConfigRemark = "last completed period of a scheduled job; written by the job itself"

	forwardFlowResetRunKey = "scheduler.forward_flow_reset.last_day"
	nodeMonthlyResetRunKey = "scheduler.node_monthly_reset.last_day"
)

// claimDailyRun records in tx that the daily job identified by key ran for
// the calendar day of now. It returns false when that day was already
// claimed, so running the job again after a restart, a reschedule, or on a
// second instance does not repeat it. The marker row is locked for the rest
// of tx, and the claim commits or rolls back together with the job's writes.
func claimDailyRun(tx *gorm.DB, key string, now time.Time) (bool, error) {
	day := now.Format("2006-01-02")
	marker := model.SystemConfig{Key: key, Type: "string", Group: periodicRunConfigGroup, Remark: periodicRunConfigRemark}
	if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "key"}}, DoNothing: true}).Create(&marker).Error; err != nil {
		return false, err
	}
	var current model.SystemConfig
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("key = ?", key).First(&current).Error; err != nil {
		return false, err
	}
	if current.Value == day {
		return false, nil
	}
	if err := tx.Model(&model.SystemConfig{}).Where("id = ?", current.ID).Update("value", day).Error; err != nil {
		return false, err
	}
	return true, nil
}
