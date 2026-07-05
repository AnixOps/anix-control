package main

import (
	"fmt"

	"github.com/anixops/v2board/internal/model"
)

// buildUsers converts old v2_user rows into model.User. password/token/uuid
// are copied verbatim: bcrypt hashes are cross-compatible between PHP and Go,
// and keeping token/uuid unchanged is what keeps /s/:token subscription
// links working after migration.
func buildUsers(t *DumpTable) ([]*model.User, error) {
	users := make([]*model.User, 0, len(t.Rows))
	for _, r := range t.Rows {
		id, err := mustUint(r, "id")
		if err != nil {
			return nil, fmt.Errorf("v2_user: %w", err)
		}
		balance, err := mustInt64(r, "balance")
		if err != nil {
			return nil, fmt.Errorf("v2_user id=%d: %w", id, err)
		}
		commissionType, err := mustInt(r, "commission_type")
		if err != nil {
			return nil, fmt.Errorf("v2_user id=%d: %w", id, err)
		}
		commissionBalance, err := mustInt64(r, "commission_balance")
		if err != nil {
			return nil, fmt.Errorf("v2_user id=%d: %w", id, err)
		}
		u, err := mustInt64(r, "u")
		if err != nil {
			return nil, fmt.Errorf("v2_user id=%d: %w", id, err)
		}
		d, err := mustInt64(r, "d")
		if err != nil {
			return nil, fmt.Errorf("v2_user id=%d: %w", id, err)
		}
		transferEnable, err := mustInt64(r, "transfer_enable")
		if err != nil {
			return nil, fmt.Errorf("v2_user id=%d: %w", id, err)
		}
		banned, err := mustInt(r, "banned")
		if err != nil {
			return nil, fmt.Errorf("v2_user id=%d: %w", id, err)
		}
		isAdmin, err := mustInt(r, "is_admin")
		if err != nil {
			return nil, fmt.Errorf("v2_user id=%d: %w", id, err)
		}
		isStaff, err := mustInt(r, "is_staff")
		if err != nil {
			return nil, fmt.Errorf("v2_user id=%d: %w", id, err)
		}
		createdAt, err := unixTime(r, "created_at")
		if err != nil {
			return nil, fmt.Errorf("v2_user id=%d: %w", id, err)
		}
		updatedAt, err := unixTime(r, "updated_at")
		if err != nil {
			return nil, fmt.Errorf("v2_user id=%d: %w", id, err)
		}

		users = append(users, &model.User{
			ID:                id,
			InviteUserID:      uintPtr(r, "invite_user_id"),
			TelegramID:        int64Ptr(r, "telegram_id"),
			Email:             str(r, "email"),
			Password:          str(r, "password"),
			Balance:           balance,
			Discount:          intPtr(r, "discount"),
			CommissionType:    commissionType,
			CommissionRate:    intPtr(r, "commission_rate"),
			CommissionBalance: commissionBalance,
			Token:             str(r, "token"),
			UUID:              str(r, "uuid"),
			SpeedLimit:        int64Ptr(r, "speed_limit"),
			TransferEnable:    transferEnable,
			U:                 u,
			D:                 d,
			PlanID:            uintPtr(r, "plan_id"),
			GroupID:           uintPtr(r, "group_id"),
			ExpiredAt:         int64Ptr(r, "expired_at"),
			Banned:            banned,
			RemarkContent:     strPtr(r, "remarks"),
			IsAdmin:           isAdmin,
			IsStaff:           isStaff,
			LastLoginAt:       int64Ptr(r, "last_login_at"),
			CreatedAt:         createdAt,
			UpdatedAt:         updatedAt,
		})
	}
	return users, nil
}

// buildUserSubscriptionGroups converts each migrated user's old single
// group_id column into a UserSubscriptionGroup row. SubscriptionGroup IDs
// were preserved verbatim from old v2_server_group IDs, so the old group_id
// value can be used directly as the new GroupID.
func buildUserSubscriptionGroups(users []*model.User) []*model.UserSubscriptionGroup {
	var rows []*model.UserSubscriptionGroup
	for _, u := range users {
		if u.GroupID == nil {
			continue
		}
		rows = append(rows, &model.UserSubscriptionGroup{
			UserID:  u.ID,
			GroupID: *u.GroupID,
		})
	}
	return rows
}

// buildInviteCodes converts old v2_invite_code rows into model.InviteCode.
func buildInviteCodes(t *DumpTable) ([]*model.InviteCode, error) {
	codes := make([]*model.InviteCode, 0, len(t.Rows))
	for _, r := range t.Rows {
		id, err := mustUint(r, "id")
		if err != nil {
			return nil, fmt.Errorf("v2_invite_code: %w", err)
		}
		status, err := mustInt(r, "status")
		if err != nil {
			return nil, fmt.Errorf("v2_invite_code id=%d: %w", id, err)
		}
		createdAt, err := unixTime(r, "created_at")
		if err != nil {
			return nil, fmt.Errorf("v2_invite_code id=%d: %w", id, err)
		}
		updatedAt, err := unixTime(r, "updated_at")
		if err != nil {
			return nil, fmt.Errorf("v2_invite_code id=%d: %w", id, err)
		}

		codes = append(codes, &model.InviteCode{
			ID:        id,
			Code:      str(r, "code"),
			UserID:    uintPtr(r, "user_id"),
			Status:    status,
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
		})
	}
	return codes, nil
}
