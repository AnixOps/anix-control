package native

import (
	"context"
	"log"
	"net/http"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
)

// topInviter is a row of the statistics' top inviters, with the kernel's
// field order and tags.
type topInviter struct {
	UserID      uint    `json:"user_id"`
	InviteCount int64   `json:"invite_count"`
	Commission  float64 `json:"commission"`
}

// AdminStats is GET /api/v2/admin/invite/stats. Users and who invited them
// come from kapi_user_referral_v1. As in the kernel, a query that fails is
// logged and its figure stays zero.
func (s *Service) AdminStats(ctx context.Context, _ pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	db, err := s.Open(ctx)
	if err != nil {
		return errorAnswer(http.StatusInternalServerError, err.Error())
	}
	var totalUsers, invitedUsers int64
	var totalCommission, pendingCommission, withdrawnCommission, pendingWithdraw float64

	if err := db.Model(&ReferralUser{}).Count(&totalUsers).Error; err != nil {
		log.Printf("failed to count total users: %v", err)
	}
	if err := db.Model(&ReferralUser{}).Where("invite_user_id IS NOT NULL").Count(&invitedUsers).Error; err != nil {
		log.Printf("failed to count invited users: %v", err)
	}
	if err := db.Model(&Commission{}).Where("status IN ?", []int{1, 2}).
		Select("COALESCE(SUM(amount), 0)").Scan(&totalCommission).Error; err != nil {
		log.Printf("failed to sum total commission: %v", err)
	}
	if err := db.Model(&Commission{}).Where("status = 0").
		Select("COALESCE(SUM(amount), 0)").Scan(&pendingCommission).Error; err != nil {
		log.Printf("failed to sum pending commission: %v", err)
	}
	if err := db.Model(&Withdrawal{}).Where("status = 1").
		Select("COALESCE(SUM(amount), 0)").Scan(&withdrawnCommission).Error; err != nil {
		log.Printf("failed to sum withdrawn commission: %v", err)
	}
	if err := db.Model(&Withdrawal{}).Where("status = 0").
		Select("COALESCE(SUM(amount), 0)").Scan(&pendingWithdraw).Error; err != nil {
		log.Printf("failed to sum pending withdraw: %v", err)
	}

	var topInviters []topInviter
	if err := db.Table(ReferralUser{}.TableName()).
		Select("invite_user_id AS user_id, COUNT(*) AS invite_count").
		Where("invite_user_id IS NOT NULL").
		Group("invite_user_id").
		Order("invite_count DESC").
		Limit(10).
		Scan(&topInviters).Error; err != nil {
		log.Printf("failed to scan top inviters: %v", err)
	}

	var commissions []struct {
		UserID     uint
		Commission float64
	}
	if err := db.Model(&Commission{}).
		Select("user_id, COALESCE(SUM(amount), 0) AS commission").
		Where("status IN ?", []int{1, 2}).
		Group("user_id").
		Scan(&commissions).Error; err != nil {
		log.Printf("failed to scan commissions: %v", err)
	}
	byUser := make(map[uint]float64, len(commissions))
	for _, row := range commissions {
		byUser[row.UserID] = row.Commission
	}
	for i := range topInviters {
		topInviters[i].Commission = byUser[topInviters[i].UserID]
	}

	return s.panel(map[string]any{
		"total_invites":        invitedUsers,
		"total_commission":     totalCommission,
		"pending_commission":   pendingCommission,
		"withdrawn_commission": withdrawnCommission,
		"top_inviters":         topInviters,
		"total_users":          totalUsers,
		"invited_users":        invitedUsers,
		"pending_withdraw":     pendingWithdraw,
	})
}
