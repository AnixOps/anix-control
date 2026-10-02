package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
)

// Administrator's invite codes: registration control, served in every
// edition by identity-platform (GET, POST /api/v2/admin/invite/codes and
// DELETE /api/v2/admin/invite/codes/:id). Commissions, withdrawals and the
// invite statistics stay with the affiliate package.
//
// An administrator's codes belong to no user (user_id is null): they admit a
// registration and attribute no referral, and the per-user limit of unused
// codes (code_count, LockUserInviteCodes) does not apply to them, as it does
// not to any code made through GenerateInviteCode.

// MaxAdminInviteCodeBatch is how many codes one administrator request may
// generate.
const MaxAdminInviteCodeBatch = 50

// MaxAdminInviteCodeExpireDays bounds an administrator's expiry override.
const MaxAdminInviteCodeExpireDays = 3650

// Invite code list filters (the status query parameter).
const (
	InviteCodeFilterAll     = ""
	InviteCodeFilterUnused  = "unused"
	InviteCodeFilterUsed    = "used"
	InviteCodeFilterExpired = "expired"
)

var (
	// ErrInviteCodeNotFound is a code id that does not exist.
	ErrInviteCodeNotFound = errors.New("invite code not found")
	// ErrInviteCodeUsed is a code that registration already consumed; it
	// stays as the record of who used it.
	ErrInviteCodeUsed = errors.New("invite code already used")
	// ErrInviteCodeFilter is a status filter the list does not know.
	ErrInviteCodeFilter = errors.New("invalid invite code status filter")
)

// InviteCodeListQuery selects a page of invite codes.
type InviteCodeListQuery struct {
	Status   string
	Page     int
	PageSize int
}

// ListInviteCodes returns a page of every invite code, newest first, and the
// number of codes the filter matches. An unused code past its expiry counts
// as expired, not unused.
func (s *InviteService) ListInviteCodes(query InviteCodeListQuery, now time.Time) ([]model.InviteCode, int64, error) {
	db := s.db.Model(&model.InviteCode{})
	switch strings.ToLower(strings.TrimSpace(query.Status)) {
	case InviteCodeFilterAll:
	case InviteCodeFilterUnused:
		db = db.Where("status = ? AND (expired_at IS NULL OR expired_at > ?)", 0, now)
	case InviteCodeFilterUsed:
		db = db.Where("status = ?", 1)
	case InviteCodeFilterExpired:
		db = db.Where("status = ? AND expired_at IS NOT NULL AND expired_at <= ?", 0, now)
	default:
		return nil, 0, ErrInviteCodeFilter
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	codes := []model.InviteCode{}
	offset := (query.Page - 1) * query.PageSize
	if err := db.Order("created_at DESC").Order("id DESC").Limit(query.PageSize).Offset(offset).Find(&codes).Error; err != nil {
		return nil, 0, err
	}
	return codes, total, nil
}

// GenerateAdminInviteCodes creates count codes that belong to no user, in
// one transaction. expireDays nil uses the configured code_expire_days, 0
// makes codes that never expire, and a positive value expires them after so
// many days.
func (s *InviteService) GenerateAdminInviteCodes(count int, expireDays *int, now time.Time) ([]model.InviteCode, error) {
	if count < 1 || count > MaxAdminInviteCodeBatch {
		return nil, fmt.Errorf("count must be between 1 and %d", MaxAdminInviteCodeBatch)
	}
	days := 0
	if expireDays != nil {
		if *expireDays < 0 || *expireDays > MaxAdminInviteCodeExpireDays {
			return nil, fmt.Errorf("expire_days must be between 0 and %d", MaxAdminInviteCodeExpireDays)
		}
		days = *expireDays
	} else if cfg, _ := s.GetConfig(); cfg != nil && cfg.CodeExpireDays > 0 {
		days = cfg.CodeExpireDays
	}
	codes := make([]model.InviteCode, 0, count)
	for range count {
		code, err := generateInviteCodeStr(8)
		if err != nil {
			return nil, err
		}
		record := model.InviteCode{Code: code, Status: 0}
		if days > 0 {
			expiredAt := now.AddDate(0, 0, days)
			record.ExpiredAt = &expiredAt
		}
		codes = append(codes, record)
	}
	err := WithRetryableTransaction(s.db, func(tx *gorm.DB) error {
		for i := range codes {
			codes[i].ID = 0
			if err := tx.Create(&codes[i]).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return codes, nil
}

// RevokeInviteCode deletes an unused code, whoever it belongs to, so no
// registration can consume it. A used code is refused: it records who
// registered with it. The delete names the unused state, so a registration
// that consumes the code first wins.
func (s *InviteService) RevokeInviteCode(id uint) error {
	result := s.db.Where("id = ? AND status = ?", id, 0).Delete(&model.InviteCode{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		return nil
	}
	var existing int64
	if err := s.db.Model(&model.InviteCode{}).Where("id = ?", id).Count(&existing).Error; err != nil {
		return err
	}
	if existing == 0 {
		return ErrInviteCodeNotFound
	}
	return ErrInviteCodeUsed
}
