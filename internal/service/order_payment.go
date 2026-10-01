package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/subscriber"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Outcomes of CompleteOrderPaymentTx: what a paid payment record did to the
// order it names (docs/architecture/order-service.md).
const (
	// OrderPaymentCompleted: the order was marked paid and completed.
	OrderPaymentCompleted = "completed"
	// OrderPaymentPaid: the order was marked paid, but granting its plan
	// failed; it stays paid for an administrator to complete.
	OrderPaymentPaid = "paid"
	// OrderPaymentRefused: the record does not pay its order, which is left
	// unchanged; the payment stays recorded.
	OrderPaymentRefused = "refused"
)

// OrderPaymentMethod is the request ledger method of a payment's order
// completion.
const OrderPaymentMethod = "complete_order_payment"

// MaxOrderPaymentTradeNo keeps OrderPaymentRequestID within the request
// ledger's 128 bytes. Generated trade numbers are 26 bytes and the column
// holds 64.
const MaxOrderPaymentTradeNo = 120

// Errors of CompleteOrderPaymentTx. They record nothing: a later call may
// apply.
var (
	ErrOrderPaymentRecordNotFound = errors.New("payment record not found")
	ErrOrderPaymentNotPaid        = errors.New("payment record is not paid")
	ErrOrderPaymentOtherOrder     = errors.New("payment record does not name the order")
	ErrOrderPaymentRequestTaken   = errors.New("the payment's request id is taken by another write")
)

// OrderPaymentResult is the outcome of CompleteOrderPaymentTx, as the
// request ledger records it.
type OrderPaymentResult struct {
	// Applied is false when the trade number was applied before; the other
	// fields then describe that first application.
	Applied bool   `json:"-"`
	OrderID uint64 `json:"order_id"`
	Outcome string `json:"outcome"`
	// Reason says why a refused record does not pay its order.
	Reason string `json:"reason,omitempty"`
}

// OrderPaymentRequestID names a payment's order completion in the request
// ledger (v4_kernel_subscriber_request).
func OrderPaymentRequestID(tradeNo string) string {
	return "payment:" + tradeNo
}

// recordPaysOrder says why a paid payment record does not pay its order, or
// "" when it does: the order belongs to the record's user, costs no more
// than the record's amount (#73) and is still pending. Records created
// before CheckOrderPayable could name any order with any amount, and an
// order may be cancelled, or paid by another record, after its payment was
// created; such a payment is still recorded, but the order is left
// unchanged.
func recordPaysOrder(record model.PaymentRecord, order model.Order) string {
	switch {
	case order.UserID != record.UserID:
		return "the order is another user's"
	case math.Round(record.Amount*100) < float64(order.TotalAmount):
		return "the payment is below the order's total"
	case order.Status != 0:
		return fmt.Sprintf("the order is not pending (status %d)", order.Status)
	}
	return ""
}

// CompleteOrderPaymentTx applies the paid payment record tradeNo to the
// order it names, orderID, in tx: it re-checks that the record pays the
// order, marks the order paid and completes it (grants its plan with
// request id "order:<id>", in a savepoint). The outcome is recorded in the
// request ledger under OrderPaymentRequestID, so a trade number is applied
// once and a repeat answers the first outcome.
//
// The kernel's payment callbacks call it in the transaction that marks the
// record paid; KernelOrder.CompleteOrderPayment calls it after the payment
// module committed the record. It locks the record, then the order.
func CompleteOrderPaymentTx(tx *gorm.DB, tradeNo string, orderID uint64, now time.Time) (OrderPaymentResult, error) {
	if tradeNo == "" || len(tradeNo) > MaxOrderPaymentTradeNo {
		return OrderPaymentResult{}, ErrOrderPaymentRecordNotFound
	}
	var record model.PaymentRecord
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("trade_no = ?", tradeNo).Take(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return OrderPaymentResult{}, ErrOrderPaymentRecordNotFound
		}
		return OrderPaymentResult{}, err
	}
	requestID := OrderPaymentRequestID(tradeNo)
	var ledger model.SubscriberRequest
	switch err := tx.Where("request_id = ?", requestID).Take(&ledger).Error; {
	case err == nil:
		if ledger.Method != OrderPaymentMethod {
			return OrderPaymentResult{}, ErrOrderPaymentRequestTaken
		}
		var previous OrderPaymentResult
		if err := json.Unmarshal([]byte(ledger.Result), &previous); err != nil {
			return OrderPaymentResult{}, fmt.Errorf("payment %s: stored result: %w", tradeNo, err)
		}
		if previous.OrderID != orderID {
			return OrderPaymentResult{}, ErrOrderPaymentOtherOrder
		}
		return previous, nil
	case !errors.Is(err, gorm.ErrRecordNotFound):
		return OrderPaymentResult{}, err
	}
	if record.Status != model.PaymentStatusPaid {
		return OrderPaymentResult{}, ErrOrderPaymentNotPaid
	}
	if record.OrderID == nil || uint64(*record.OrderID) != orderID {
		return OrderPaymentResult{}, ErrOrderPaymentOtherOrder
	}

	result := OrderPaymentResult{Applied: true, OrderID: orderID}
	var order model.Order
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id", "user_id", "status", "total_amount").
		Where("id = ?", orderID).Take(&order).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		result.Reason = "the order does not exist"
	case err != nil:
		return OrderPaymentResult{}, err
	default:
		result.Reason = recordPaysOrder(record, order)
	}
	if result.Reason != "" {
		result.Outcome = OrderPaymentRefused
		log.Printf("payment %s does not pay order %d: %s; the order is left unchanged", tradeNo, orderID, result.Reason)
	} else {
		if err := tx.Model(&model.Order{}).Where("id = ?", orderID).Updates(map[string]any{
			"status":  1, // paid
			"paid_at": now.Unix(),
		}).Error; err != nil {
			return OrderPaymentResult{}, err
		}
		// A paid order activates its plan at once. The activation runs in a
		// savepoint: if it fails, the payment stays recorded and the order
		// stays paid for an administrator to complete.
		result.Outcome = OrderPaymentCompleted
		if err := tx.Transaction(func(inner *gorm.DB) error {
			return completeOrderTx(inner, order.ID, now)
		}); err != nil {
			result.Outcome = OrderPaymentPaid
			log.Printf("payment %s: order %d is paid but was not activated: %v", tradeNo, orderID, err)
		}
	}
	if err := subscriber.Record(tx, requestID, OrderPaymentMethod, record.UserID, result, now); err != nil {
		return OrderPaymentResult{}, err
	}
	return result, nil
}

// FinishPaidOrder applies a payment record that is already paid to its
// order, once: a repeated callback calls it, so that a payment recorded
// without its order completion (the payment module records the payment,
// then calls KernelOrder; a failure in between leaves the order pending)
// converges whichever side serves the repeat. A record applied before, or
// not paid, or without an order, changes nothing.
func (s *PaymentGatewayService) FinishPaidOrder(record model.PaymentRecord) {
	if record.Status != model.PaymentStatusPaid || record.OrderID == nil {
		return
	}
	err := WithRetryableTransaction(s.db, func(tx *gorm.DB) error {
		_, err := CompleteOrderPaymentTx(tx, record.TradeNo, uint64(*record.OrderID), time.Now())
		return err
	})
	if err != nil {
		log.Printf("payment %s: completing order %d failed: %v", record.TradeNo, *record.OrderID, err)
	}
}
