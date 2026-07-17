package service

import (
	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
)

func deletePanelForwardRecordTx(tx *gorm.DB, forwardID uint) error {
	if tx == nil || forwardID == 0 {
		return nil
	}
	if err := tx.Where("forward_id = ?", forwardID).Delete(&model.ForwardPortBinding{}).Error; err != nil {
		return err
	}
	if err := tx.Where("forward_id = ?", forwardID).Delete(&model.ForwardTrafficCursor{}).Error; err != nil {
		return err
	}
	return tx.Delete(&model.Forward{}, forwardID).Error
}

func cleanupPanelForwardAfterRuntimeDeleteTx(tx *gorm.DB, job *model.ForwardRuntimeJob) error {
	if job == nil || job.Action != model.ForwardRuntimeJobActionDelete || job.ForwardID == nil {
		return nil
	}
	return deletePanelForwardRecordTx(tx, *job.ForwardID)
}
