package subscriber

import (
	"errors"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrSubscriptionGroupNotFound means no v2_subscription_group row has the id.
var ErrSubscriptionGroupNotFound = errors.New("subscription group not found")

// ErrMembershipNotFound means the subscriber does not hold the group.
var ErrMembershipNotFound = errors.New("subscription group membership not found")

// Request ledger methods of the membership writes.
const (
	methodGrantGroup   = "grant_subscription_group"
	methodRevokeGroup  = "revoke_subscription_group"
	methodRemoveGroups = "remove_group_members"
)

// GroupGrant gives a subscriber one subscription group.
type GroupGrant struct {
	// RequestID is unique per grant; empty means the write is not
	// retryable and is not recorded.
	RequestID string
	UserID    uint
	GroupID   uint
	// ExpiresAt, TransferBytes and NextRenewPrice are set when non-nil and
	// left unchanged otherwise, as given.
	ExpiresAt      *int64
	TransferBytes  *int64
	NextRenewPrice *int64
}

// GrantResult is the outcome of GrantSubscriptionGroupTx.
type GrantResult struct {
	// Applied is false when the request id was applied before; Created
	// then describes that first application.
	Applied bool `json:"-"`
	Created bool `json:"created"`
}

// MembershipResult is the outcome of RevokeSubscriptionGroupTx and
// RemoveSubscriptionGroupMembersTx.
type MembershipResult struct {
	Applied bool `json:"-"`
	// Removed counts the memberships deleted.
	Removed int64 `json:"removed"`
}

// lockSubscriber takes the subscriber's row lock.
func lockSubscriber(tx *gorm.DB, userID uint) error {
	if userID == 0 {
		return ErrSubscriberNotFound
	}
	var user model.User
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").Where("id = ?", userID).Take(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrSubscriberNotFound
		}
		return err
	}
	return nil
}

// subscriptionGroupExists checks the group as the v2 handlers do.
func subscriptionGroupExists(tx *gorm.DB, groupID uint) error {
	var group model.SubscriptionGroup
	if err := tx.Select("id").First(&group, groupID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrSubscriptionGroupNotFound
		}
		return err
	}
	return nil
}

// recordMembershipChangesTx appends a change for each subscriber the scope
// selects that is active. A membership change does not alter whether a
// node serves a subscriber (Active ignores subscription groups), but every
// UPSERT carries the subscriber's subscription groups, so a watcher's copy
// of an active subscriber must be refreshed. An inactive subscriber is not
// on a watcher's list, and whatever makes them active again records its own
// change.
func recordMembershipChangesTx(tx *gorm.DB, now time.Time, scope func(*gorm.DB) *gorm.DB) error {
	var active []uint
	if err := Active(tx.Model(&model.User{}), now).Scopes(scope).Order("id").Pluck("id", &active).Error; err != nil {
		return err
	}
	return RecordChangesTx(tx, active, false, now)
}

// subscriberScope selects one subscriber.
func subscriberScope(userID uint) func(*gorm.DB) *gorm.DB {
	return func(query *gorm.DB) *gorm.DB { return query.Where("id = ?", userID) }
}

// GrantSubscriptionGroupTx gives a subscriber a subscription group in tx,
// under the subscriber's row lock, once per request id: it creates the
// membership, or sets the given fields of an existing one. A subscriber or
// group that does not exist is an error, and nothing is recorded. A new
// membership, or a new expiry, records a change for an active subscriber.
func GrantSubscriptionGroupTx(tx *gorm.DB, g GroupGrant, now time.Time) (GrantResult, error) {
	var previous GrantResult
	if seen, err := Replay(tx, g.RequestID, &previous); err != nil || seen {
		return previous, err
	}
	if err := lockSubscriber(tx, g.UserID); err != nil {
		return GrantResult{}, err
	}
	if err := subscriptionGroupExists(tx, g.GroupID); err != nil {
		return GrantResult{}, err
	}
	var rows []model.UserSubscriptionGroup
	if err := tx.Where("user_id = ? AND group_id = ?", g.UserID, g.GroupID).Order("id").Limit(1).Find(&rows).Error; err != nil {
		return GrantResult{}, err
	}
	result := GrantResult{Applied: true}
	changed := false
	if len(rows) == 0 {
		membership := model.UserSubscriptionGroup{
			UserID: g.UserID, GroupID: g.GroupID, ExpireAt: g.ExpiresAt, TransferEnable: g.TransferBytes,
			NextRenewPrice: g.NextRenewPrice, CreatedAt: now,
		}
		if err := tx.Create(&membership).Error; err != nil {
			return GrantResult{}, err
		}
		result.Created, changed = true, true
	} else {
		updates := map[string]any{}
		if g.ExpiresAt != nil {
			updates["expire_at"] = *g.ExpiresAt
			changed = rows[0].ExpireAt == nil || *rows[0].ExpireAt != *g.ExpiresAt
		}
		if g.TransferBytes != nil {
			updates["transfer_enable"] = *g.TransferBytes
		}
		if g.NextRenewPrice != nil {
			updates["next_renew_price"] = *g.NextRenewPrice
		}
		if len(updates) > 0 {
			if err := tx.Model(&model.UserSubscriptionGroup{}).Where("id = ?", rows[0].ID).Updates(updates).Error; err != nil {
				return GrantResult{}, err
			}
		}
	}
	if changed {
		if err := recordMembershipChangesTx(tx, now, subscriberScope(g.UserID)); err != nil {
			return GrantResult{}, err
		}
	}
	return result, Record(tx, g.RequestID, methodGrantGroup, g.UserID, result, now)
}

// RevokeSubscriptionGroupTx takes a subscription group from a subscriber
// in tx, under the subscriber's row lock, once per request id. A
// subscriber, group or membership that does not exist is an error, and
// nothing is recorded. It records a change for an active subscriber.
func RevokeSubscriptionGroupTx(tx *gorm.DB, requestID string, userID, groupID uint, now time.Time) (MembershipResult, error) {
	var previous MembershipResult
	if seen, err := Replay(tx, requestID, &previous); err != nil || seen {
		return previous, err
	}
	if err := lockSubscriber(tx, userID); err != nil {
		return MembershipResult{}, err
	}
	if err := subscriptionGroupExists(tx, groupID); err != nil {
		return MembershipResult{}, err
	}
	deleted := tx.Where("user_id = ? AND group_id = ?", userID, groupID).Delete(&model.UserSubscriptionGroup{})
	if deleted.Error != nil {
		return MembershipResult{}, deleted.Error
	}
	if deleted.RowsAffected == 0 {
		return MembershipResult{}, ErrMembershipNotFound
	}
	if err := recordMembershipChangesTx(tx, now, subscriberScope(userID)); err != nil {
		return MembershipResult{}, err
	}
	result := MembershipResult{Applied: true, Removed: deleted.RowsAffected}
	return result, Record(tx, requestID, methodRevokeGroup, userID, result, now)
}

// RemoveSubscriptionGroupMembersTx takes a subscription group from every
// subscriber who holds it in tx, under their row locks (taken in id order),
// once per request id, whether or not the group still exists. It records a
// change for each active member.
func RemoveSubscriptionGroupMembersTx(tx *gorm.DB, requestID string, groupID uint, now time.Time) (MembershipResult, error) {
	var previous MembershipResult
	if seen, err := Replay(tx, requestID, &previous); err != nil || seen {
		return previous, err
	}
	// The members are selected by a subquery: a group can have more members
	// than a statement takes parameters.
	members := func(query *gorm.DB) *gorm.DB {
		return query.Where("id IN (?)", tx.Session(&gorm.Session{NewDB: true}).Model(&model.UserSubscriptionGroup{}).
			Select("user_id").Where("group_id = ?", groupID))
	}
	var locked []model.User
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Model(&model.User{}).Scopes(members).Select("id").Order("id").Find(&locked).Error; err != nil {
		return MembershipResult{}, err
	}
	if err := recordMembershipChangesTx(tx, now, members); err != nil {
		return MembershipResult{}, err
	}
	deleted := tx.Where("group_id = ?", groupID).Delete(&model.UserSubscriptionGroup{})
	if deleted.Error != nil {
		return MembershipResult{}, deleted.Error
	}
	result := MembershipResult{Applied: true, Removed: deleted.RowsAffected}
	return result, Record(tx, requestID, methodRemoveGroups, 0, result, now)
}
