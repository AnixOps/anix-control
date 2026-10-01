// Package subscriber is the kernel's single writer of shared subscriber
// state: entitlements, traffic counters, subscription credentials and
// balances in v2_user and v2_user_subscription_group
// (docs/architecture/subscriber-service.md). The kernel's own callers and
// the KernelSubscriber contract use the same functions, so both behave
// alike. Every retryable write carries a request id, applied once.
package subscriber

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrSubscriberNotFound means no v2_user row has the id.
var ErrSubscriberNotFound = errors.New("subscriber not found")

// bytesPerGiB converts a plan's transfer (GiB) to bytes.
const bytesPerGiB = 1073741824

// PlanSnapshot is what an entitlement grants.
type PlanSnapshot struct {
	PlanID  uint
	GroupID uint
	// SubscriptionGroupIDs replace the subscriber's subscription groups,
	// unless the entitlement keeps them.
	SubscriptionGroupIDs []uint
	TransferBytes        int64
	// SpeedLimit and DeviceLimit are left unchanged when nil.
	SpeedLimit  *int64
	DeviceLimit *int
}

// PlanSnapshotFromPlan builds a snapshot of a v2 plan and its subscription
// groups.
func PlanSnapshotFromPlan(plan model.Plan, groups []model.PlanSubscriptionGroup) PlanSnapshot {
	snapshot := PlanSnapshot{
		PlanID: plan.ID, GroupID: plan.GroupID, TransferBytes: plan.TransferEnable * bytesPerGiB,
		SpeedLimit: plan.SpeedLimit, DeviceLimit: plan.DeviceLimit,
	}
	for _, group := range groups {
		snapshot.SubscriptionGroupIDs = append(snapshot.SubscriptionGroupIDs, group.GroupID)
	}
	return snapshot
}

// Period is how long an entitlement lasts.
type Period struct {
	Months int
	Days   int
}

// PeriodMonths maps a v2 order period to months.
var PeriodMonths = map[string]int{
	"month": 1, "quarter": 3, "half_year": 6, "year": 12, "two_year": 24, "three_year": 36, "onetime": 1200,
}

// Entitlement activates or renews a plan.
type Entitlement struct {
	// RequestID is unique per grant, e.g. "order:42"; empty means the
	// write is not retryable and is not recorded.
	RequestID string
	UserID    uint
	Plan      PlanSnapshot
	// Period extends the expiry from now, or with RenewSamePlan from the
	// current expiry of the same, unexpired plan. ExpiresAt sets it
	// exactly. With neither, the expiry is unchanged.
	Period                 *Period
	ExpiresAt              *int64
	RenewSamePlan          bool
	ResetTraffic           bool
	KeepSubscriptionGroups bool
}

// EntitlementResult is the outcome of ApplyEntitlementTx.
type EntitlementResult struct {
	// Applied is false when the request id was applied before; ExpiresAt
	// then describes that first application.
	Applied   bool   `json:"-"`
	ExpiresAt *int64 `json:"expires_at,omitempty"`
}

// ApplyEntitlementTx applies an entitlement in tx, under the subscriber's
// row lock.
func ApplyEntitlementTx(tx *gorm.DB, e Entitlement, now time.Time) (EntitlementResult, error) {
	if e.UserID == 0 {
		return EntitlementResult{}, ErrSubscriberNotFound
	}
	if e.Period != nil && e.ExpiresAt != nil {
		return EntitlementResult{}, errors.New("an entitlement takes a period or an expiry, not both")
	}
	var previous EntitlementResult
	if seen, err := Replay(tx, e.RequestID, &previous); err != nil || seen {
		return previous, err
	}
	var user model.User
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id", "plan_id", "expired_at").
		Where("id = ?", e.UserID).Take(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return EntitlementResult{}, ErrSubscriberNotFound
		}
		return EntitlementResult{}, err
	}
	updates := map[string]any{
		"plan_id":         e.Plan.PlanID,
		"group_id":        e.Plan.GroupID,
		"transfer_enable": e.Plan.TransferBytes,
	}
	result := EntitlementResult{Applied: true, ExpiresAt: user.ExpiredAt}
	switch {
	case e.ExpiresAt != nil:
		expires := *e.ExpiresAt
		result.ExpiresAt = &expires
	case e.Period != nil:
		base := now
		if e.RenewSamePlan && user.PlanID != nil && *user.PlanID == e.Plan.PlanID && user.ExpiredAt != nil && *user.ExpiredAt > now.Unix() {
			base = time.Unix(*user.ExpiredAt, 0).In(now.Location())
		}
		expires := base.AddDate(0, e.Period.Months, e.Period.Days).Unix()
		result.ExpiresAt = &expires
	}
	if e.ExpiresAt != nil || e.Period != nil {
		updates["expired_at"] = *result.ExpiresAt
	}
	if e.ResetTraffic {
		updates["u"], updates["d"] = 0, 0
	}
	if e.Plan.SpeedLimit != nil {
		updates["speed_limit"] = *e.Plan.SpeedLimit
	}
	if e.Plan.DeviceLimit != nil {
		updates["device_limit"] = *e.Plan.DeviceLimit
	}
	if err := tx.Model(&model.User{}).Where("id = ?", e.UserID).Updates(updates).Error; err != nil {
		return EntitlementResult{}, err
	}
	if err := RecordChangesTx(tx, []uint{e.UserID}, false, now); err != nil {
		return EntitlementResult{}, err
	}
	if !e.KeepSubscriptionGroups {
		if err := tx.Where("user_id = ?", e.UserID).Delete(&model.UserSubscriptionGroup{}).Error; err != nil {
			return EntitlementResult{}, err
		}
		for _, groupID := range e.Plan.SubscriptionGroupIDs {
			if err := tx.Create(&model.UserSubscriptionGroup{UserID: e.UserID, GroupID: groupID}).Error; err != nil {
				return EntitlementResult{}, err
			}
		}
	}
	return result, Record(tx, e.RequestID, "apply_entitlement", e.UserID, result, now)
}

// Replay loads the recorded result of requestID into result and reports
// whether it was applied before.
func Replay(tx *gorm.DB, requestID string, result any) (bool, error) {
	if requestID == "" {
		return false, nil
	}
	var row model.SubscriberRequest
	err := tx.Where("request_id = ?", requestID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if row.Result != "" {
		if err := json.Unmarshal([]byte(row.Result), result); err != nil {
			return true, fmt.Errorf("subscriber request %s: stored result: %w", requestID, err)
		}
	}
	return true, nil
}

// Record stores a request id with its result in tx.
func Record(tx *gorm.DB, requestID, method string, userID uint, result any, now time.Time) error {
	if requestID == "" {
		return nil
	}
	if len(requestID) > 128 {
		return fmt.Errorf("subscriber request id is longer than 128 bytes")
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return tx.Create(&model.SubscriberRequest{
		RequestID: requestID, Method: method, UserID: userID, Result: string(encoded), CreatedAt: now,
	}).Error
}
