package subscriber

import (
	"errors"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrInsufficientBalance means a debit would take a balance below zero.
var ErrInsufficientBalance = errors.New("insufficient balance")

// Balance kinds.
const (
	BalanceAccount    = "balance"
	BalanceCommission = "commission_balance"
)

// BalanceResult is the outcome of AdjustBalanceTx.
type BalanceResult struct {
	Applied bool  `json:"-"`
	Balance int64 `json:"balance"`
}

// AdjustBalanceTx adds amount (cents, possibly negative) to a subscriber's
// account or commission balance in tx, once per request id.
func AdjustBalanceTx(tx *gorm.DB, requestID string, userID uint, kind string, amount int64, now time.Time) (BalanceResult, error) {
	if kind != BalanceAccount && kind != BalanceCommission {
		return BalanceResult{}, errors.New("unknown balance kind")
	}
	var previous BalanceResult
	if seen, err := Replay(tx, requestID, &previous); err != nil || seen {
		return previous, err
	}
	var user model.User
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id", "balance", "commission_balance").
		Where("id = ?", userID).Take(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return BalanceResult{}, ErrSubscriberNotFound
		}
		return BalanceResult{}, err
	}
	current := user.Balance
	if kind == BalanceCommission {
		current = user.CommissionBalance
	}
	next := current + amount
	if next < 0 {
		return BalanceResult{}, ErrInsufficientBalance
	}
	if err := tx.Model(&model.User{}).Where("id = ?", userID).Update(kind, next).Error; err != nil {
		return BalanceResult{}, err
	}
	result := BalanceResult{Applied: true, Balance: next}
	return result, Record(tx, requestID, "adjust_balance", userID, result, now)
}
