package service

import (
	"errors"
	"strings"
	"time"
)

// ErrInvalidMemberStatus is a members status filter that is not "active" or
// "expired".
var ErrInvalidMemberStatus = errors.New("status must be active or expired")

// GroupMembersParams filters and pages a subscription group's members.
type GroupMembersParams struct {
	Page     int
	PageSize int
	// Email, when set, keeps the members whose e-mail contains it; % and _
	// match themselves.
	Email string
	// Status, when set, keeps the members whose membership is "active" (no
	// expiry, or one in the future: the stats' enabled users) or "expired".
	Status string
}

// GroupMember is a user the administrator granted the subscription group
// directly (a v2_user_subscription_group row, what POST
// /admin/subscription/users/:user_id/groups creates), with the grant's own
// fields and the user's e-mail, ban flag and plan. It carries no credential:
// never the subscription token or proxy uuid. The users the group reaches
// through their plan or their primary group (v2_user.group_id) are not
// members here, as the stats' user_count does not count them.
type GroupMember struct {
	UserID uint   `json:"user_id"`
	Email  string `json:"email"`
	Banned int    `json:"banned"`
	PlanID *uint  `json:"plan_id"`
	// ExpireAt is when the membership ends (Unix seconds), null for never.
	ExpireAt *int64 `json:"expire_at"`
	// TransferEnable overrides the user's traffic quota (bytes) while the
	// membership lasts, null for no override.
	TransferEnable *int64 `json:"transfer_enable"`
	// NextRenewPrice is the renewal price in cents, null for none.
	NextRenewPrice *int64 `json:"next_renew_price"`
	// CreatedAt is when the membership was granted.
	CreatedAt time.Time `json:"created_at"`
	// Active is whether the membership has not expired.
	Active bool `json:"active"`
}

// GroupMembersResult is a page of members and how many match in all.
type GroupMembersResult struct {
	Total    int64         `json:"total"`
	Page     int           `json:"page"`
	PageSize int           `json:"page_size"`
	Members  []GroupMember `json:"members"`
}

// ListGroupMembers lists a subscription group's direct members, newest grant
// first (members granted in the same instant by id, newest first), so pages
// neither repeat nor skip one. An unknown group is ErrSubscriptionGroupNotFound.
func (s *SubscriptionService) ListGroupMembers(groupID uint, params GroupMembersParams) (*GroupMembersResult, error) {
	if params.Status != "" && params.Status != "active" && params.Status != "expired" {
		return nil, ErrInvalidMemberStatus
	}
	if err := subscriptionGroupExists(s.db, groupID); err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	query := s.db.Table("v2_user_subscription_group AS m").
		Joins("JOIN v2_user AS u ON u.id = m.user_id").
		Where("m.group_id = ?", groupID)
	if params.Email != "" {
		escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(params.Email)
		query = query.Where(`u.email LIKE ? ESCAPE '\'`, "%"+escaped+"%")
	}
	switch params.Status {
	case "active":
		query = query.Where("(m.expire_at IS NULL OR m.expire_at > ?)", now)
	case "expired":
		query = query.Where("m.expire_at IS NOT NULL AND m.expire_at <= ?", now)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	members := []GroupMember{}
	if err := query.
		Select("m.user_id AS user_id, u.email AS email, u.banned AS banned, u.plan_id AS plan_id, " +
			"m.expire_at AS expire_at, m.transfer_enable AS transfer_enable, m.next_renew_price AS next_renew_price, m.created_at AS created_at").
		Order("m.created_at DESC, m.id DESC").
		Offset((params.Page - 1) * params.PageSize).Limit(params.PageSize).
		Scan(&members).Error; err != nil {
		return nil, err
	}
	for i := range members {
		members[i].Active = members[i].ExpireAt == nil || *members[i].ExpireAt > now
	}
	return &GroupMembersResult{Total: total, Page: params.Page, PageSize: params.PageSize, Members: members}, nil
}
