package service

// Registration with an invite code. It belongs to the commercial set
// (config/commercial-boundary.json) and reaches registration through
// RegisterInviteRedeemer.

import (
	"errors"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

func init() {
	RegisterInviteRedeemer(redeemInviteCode)
}

func redeemInviteCode(tx *gorm.DB, code string) (*InviteRedemption, error) {
	var record model.InviteCode
	if err := tx.Where("code = ? AND status = 0", code).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, InviteRefusal{"invalid or used invite code"}
		}
		return nil, err
	}
	if record.ExpiredAt != nil && record.ExpiredAt.Before(time.Now()) {
		return nil, InviteRefusal{"invite code expired"}
	}
	return &InviteRedemption{
		InviterID: record.UserID,
		Consume: func(tx *gorm.DB, userID uint) error {
			now := time.Now()
			record.Status = 1
			record.UsedBy = &userID
			record.UsedAt = &now
			return tx.Save(&record).Error
		},
	}, nil
}
