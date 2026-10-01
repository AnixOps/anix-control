package service

import (
	"context"
	"log"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/subscriber"
	"gorm.io/gorm"
)

const (
	// OrderPaymentReconcileGrace is how long a paid record is left to the
	// callback that paid it before the reconciler completes its order. The
	// payment module commits the record, then asks KernelOrder; the grace
	// keeps the reconciler out of that live second step.
	OrderPaymentReconcileGrace = 2 * time.Minute
	// OrderPaymentReconcileBatch is how many records one query reads.
	OrderPaymentReconcileBatch = 100
	// OrderPaymentReconcileMaxBatches bounds the batches of one run; a
	// larger backlog continues on the next run.
	OrderPaymentReconcileMaxBatches = 10
)

// orderPaymentReconcileCandidates selects the paid records past the grace,
// paid within the request ledger's retention, whose order exists and is
// pending, and which have no outcome under their request id yet (a record
// refused before is left alone). It is ordered by record id after a cursor.
const orderPaymentReconcileCandidates = `SELECT r.id, r.trade_no, r.order_id
FROM v2_payment_record r
JOIN v2_order o ON o.id = r.order_id
WHERE r.status = ? AND o.status = 0
  AND r.paid_at < ? AND r.paid_at >= ?
  AND r.id > ?
  AND NOT EXISTS (
    SELECT 1 FROM v4_kernel_subscriber_request q
    WHERE q.request_id = '` + orderPaymentRequestPrefix + `' || r.trade_no
  )
ORDER BY r.id
LIMIT ?`

// OrderPaymentReconciler completes the orders of paid payment records that
// are still pending (docs/architecture/order-service.md): the payment
// module records a paid callback, then asks KernelOrder to complete the
// order; if that second step fails and the provider never delivers again,
// the record stays paid and the order pending. The reconciler runs the same
// second step, CompleteOrderPaymentTx with request id payment:<trade_no>,
// so it re-checks the record against the order and applies it once; a
// record that fails the checks is refused and logged, and the order is left
// unchanged.
type OrderPaymentReconciler struct {
	db *gorm.DB
	// Grace, Batch and MaxBatches default to the package constants.
	Grace      time.Duration
	Batch      int
	MaxBatches int
}

// NewOrderPaymentReconciler returns a reconciler on db, or on the kernel's
// database when db is nil.
func NewOrderPaymentReconciler(db *gorm.DB) *OrderPaymentReconciler {
	if db == nil {
		db = database.Get()
	}
	return &OrderPaymentReconciler{
		db: db, Grace: OrderPaymentReconcileGrace, Batch: OrderPaymentReconcileBatch, MaxBatches: OrderPaymentReconcileMaxBatches,
	}
}

// OrderPaymentReconciliation counts what one run did.
type OrderPaymentReconciliation struct {
	// Examined is the number of candidate records the run read.
	Examined int
	// Completed, Paid and Refused count the outcomes the run recorded.
	Completed int
	Paid      int
	Refused   int
	// Unchanged counts records another caller applied first (a callback's
	// repeat, or a reconciler in another process).
	Unchanged int
	// Failed counts records whose completion failed; nothing was recorded,
	// so a later run tries again.
	Failed int
}

type orderPaymentCandidate struct {
	ID      uint
	TradeNo string
	OrderID uint
}

// RunOnce applies the candidate records as of now, in batches, each record
// in its own transaction. It stops early when ctx is cancelled. Running it
// again, or in two processes at once, changes nothing more: the record's
// row lock and its request id apply it once.
func (r *OrderPaymentReconciler) RunOnce(ctx context.Context, now time.Time) (OrderPaymentReconciliation, error) {
	var run OrderPaymentReconciliation
	db := r.db.WithContext(ctx)
	batch := r.Batch
	if batch <= 0 {
		batch = OrderPaymentReconcileBatch
	}
	maxBatches := r.MaxBatches
	if maxBatches <= 0 {
		maxBatches = OrderPaymentReconcileMaxBatches
	}
	grace := r.Grace
	if grace < 0 {
		grace = 0
	}
	var after uint
	for range maxBatches {
		var candidates []orderPaymentCandidate
		if err := db.Raw(orderPaymentReconcileCandidates, model.PaymentStatusPaid,
			now.Add(-grace), now.Add(-subscriber.RequestRetention), after, batch).Scan(&candidates).Error; err != nil {
			return run, err
		}
		for _, candidate := range candidates {
			if err := ctx.Err(); err != nil {
				return run, err
			}
			after = candidate.ID
			run.Examined++
			r.apply(db, candidate, now, &run)
		}
		if len(candidates) < batch {
			break
		}
	}
	if run.Examined > 0 {
		log.Printf("Order payment reconciler: %d paid payments with a pending order: %d completed, %d paid, %d refused, %d unchanged, %d failed",
			run.Examined, run.Completed, run.Paid, run.Refused, run.Unchanged, run.Failed)
	}
	return run, nil
}

// apply runs a callback's second step for one record.
func (r *OrderPaymentReconciler) apply(db *gorm.DB, candidate orderPaymentCandidate, now time.Time, run *OrderPaymentReconciliation) {
	var result OrderPaymentResult
	err := WithRetryableTransaction(db, func(tx *gorm.DB) error {
		var err error
		result, err = CompleteOrderPaymentTx(tx, candidate.TradeNo, uint64(candidate.OrderID), now)
		return err
	})
	switch {
	case err != nil:
		run.Failed++
		log.Printf("Order payment reconciler: payment %s: completing order %d failed: %v", candidate.TradeNo, candidate.OrderID, err)
	case !result.Applied:
		run.Unchanged++
	case result.Outcome == OrderPaymentCompleted:
		run.Completed++
		log.Printf("Order payment reconciler: payment %s completed order %d, which its callback left pending", candidate.TradeNo, candidate.OrderID)
	case result.Outcome == OrderPaymentPaid:
		// CompleteOrderPaymentTx logged why the order was not activated.
		run.Paid++
	default:
		// CompleteOrderPaymentTx logged the reason; the order is unchanged.
		run.Refused++
	}
}
