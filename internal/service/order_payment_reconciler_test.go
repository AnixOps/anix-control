package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// reconcilerPostgresDSN enables the PostgreSQL runs; each run gets a
// throwaway schema.
const reconcilerPostgresDSN = "ANIX_TEST_POSTGRES_DSN"

// reconcilerPaidAt is when the fixture's records were paid.
var reconcilerPaidAt = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

var reconcilerModels = []any{
	&model.SubscriptionGroup{}, &model.Plan{}, &model.PlanSubscriptionGroup{}, &model.User{}, &model.UserSubscriptionGroup{},
	&model.Order{}, &model.PaymentRecord{}, &model.SubscriberRequest{}, &model.SubscriberChange{},
}

// forEachReconcilerDatabase runs body on SQLite and, when
// ANIX_TEST_POSTGRES_DSN is set, on PostgreSQL, each seeded with a plan and
// two buyers.
func forEachReconcilerDatabase(t *testing.T, body func(t *testing.T, db *gorm.DB)) {
	t.Run("sqlite", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		require.NoError(t, err)
		sqlDB, err := db.DB()
		require.NoError(t, err)
		sqlDB.SetMaxOpenConns(1)
		t.Cleanup(func() { _ = sqlDB.Close() })
		seedReconciler(t, db)
		body(t, db)
	})
	t.Run("postgres", func(t *testing.T) {
		db := openReconcilerPostgres(t)
		seedReconciler(t, db)
		body(t, db)
	})
}

// openReconcilerPostgres creates a throwaway schema in the test database
// and returns a connection whose search_path is that schema.
func openReconcilerPostgres(t *testing.T) *gorm.DB {
	t.Helper()
	base := strings.TrimSpace(os.Getenv(reconcilerPostgresDSN))
	if base == "" {
		t.Skip(reconcilerPostgresDSN + " is not set")
	}
	admin, err := gorm.Open(postgres.Open(base), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	adminDB, err := admin.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = adminDB.Close() })
	var databaseName string
	require.NoError(t, admin.Raw("SELECT current_database()").Scan(&databaseName).Error)
	if !strings.Contains(strings.ToLower(databaseName), "test") && os.Getenv("ANIX_TEST_POSTGRES_ALLOW_UNSAFE") != "1" {
		t.Skipf("refusing to run destructive postgres test against database %q", databaseName)
	}
	suffix := make([]byte, 4)
	_, err = rand.Read(suffix)
	require.NoError(t, err)
	schema := "order_reconcile_" + hex.EncodeToString(suffix)
	require.NoError(t, admin.Exec(`CREATE SCHEMA "`+schema+`"`).Error)
	t.Cleanup(func() { _ = admin.Exec(`DROP SCHEMA IF EXISTS "` + schema + `" CASCADE`).Error })
	db, err := gorm.Open(postgres.Open(base+" search_path="+schema), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

// seedReconciler creates plan 1 (group 8), buyer 2 and buyer 3.
func seedReconciler(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(reconcilerModels...))
	require.NoError(t, db.Create(&model.SubscriptionGroup{ID: 8, Name: "group 8"}).Error)
	require.NoError(t, db.Create(&model.Plan{ID: 1, Name: "Pro", GroupID: 4, TransferEnable: 10}).Error)
	require.NoError(t, db.Create(&model.PlanSubscriptionGroup{PlanID: 1, GroupID: 8}).Error)
	require.NoError(t, db.Create(&[]model.User{
		{ID: 2, Email: "buyer@x", Token: "t2", UUID: "u2"},
		{ID: 3, Email: "other@x", Token: "t3", UUID: "u3"},
	}).Error)
}

// stuckPayment is a paid record and the order it names, as a callback whose
// second step failed leaves them.
type stuckPayment struct {
	tradeNo     string
	recordUser  uint
	amount      float64
	recordState int
	paidAt      time.Time
	orderID     uint
	orderUser   uint
	orderState  int
	withOrder   bool
}

func paidRecord(tradeNo string, orderID uint) stuckPayment {
	return stuckPayment{
		tradeNo: tradeNo, recordUser: 2, amount: 100, recordState: model.PaymentStatusPaid, paidAt: reconcilerPaidAt,
		orderID: orderID, orderUser: 2, orderState: 0, withOrder: true,
	}
}

func createStuckPayments(t *testing.T, db *gorm.DB, payments ...stuckPayment) {
	t.Helper()
	for _, p := range payments {
		if p.withOrder && p.orderID != 0 {
			require.NoError(t, db.Create(&model.Order{
				ID: p.orderID, UserID: p.orderUser, PlanID: 1, Period: "month", TradeNo: fmt.Sprintf("T%d", p.orderID),
				TotalAmount: 10000, Status: p.orderState,
			}).Error)
		}
		record := model.PaymentRecord{
			TradeNo: p.tradeNo, UserID: p.recordUser, Amount: p.amount, ActualAmount: p.amount, Status: p.recordState,
		}
		if p.orderID != 0 {
			orderID := p.orderID
			record.OrderID = &orderID
		}
		if p.recordState == model.PaymentStatusPaid {
			paidAt := p.paidAt
			record.PaidAt = &paidAt
		}
		require.NoError(t, db.Create(&record).Error)
	}
}

func reconcilerOrderStatus(t *testing.T, db *gorm.DB, id uint) int {
	t.Helper()
	var order model.Order
	require.NoError(t, db.Take(&order, id).Error)
	return order.Status
}

func reconcilerLedger(t *testing.T, db *gorm.DB, requestID string) *model.SubscriberRequest {
	t.Helper()
	var row model.SubscriberRequest
	err := db.Take(&row, "request_id = ?", requestID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	require.NoError(t, err)
	return &row
}

func reconcilerLedgerSize(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var rows int64
	require.NoError(t, db.Model(&model.SubscriberRequest{}).Count(&rows).Error)
	return rows
}

// A paid record whose order its callback left pending is completed once:
// the order is paid and completed, the plan granted with order:<id>, and
// neither the next run nor the callback's repeat changes anything.
func TestOrderPaymentReconcilerCompletesAStuckOrderOnce(t *testing.T) {
	forEachReconcilerDatabase(t, func(t *testing.T, db *gorm.DB) {
		createStuckPayments(t, db, paidRecord("PAY1", 1))
		reconciler := NewOrderPaymentReconciler(db)
		ctx := context.Background()

		run, err := reconciler.RunOnce(ctx, reconcilerPaidAt.Add(3*time.Minute))
		require.NoError(t, err)
		require.Equal(t, OrderPaymentReconciliation{Examined: 1, Completed: 1}, run)
		var order model.Order
		require.NoError(t, db.Take(&order, 1).Error)
		require.Equal(t, 3, order.Status)
		require.NotNil(t, order.PaidAt)
		var buyer model.User
		require.NoError(t, db.Take(&buyer, 2).Error)
		require.NotNil(t, buyer.PlanID)
		require.Equal(t, uint(1), *buyer.PlanID)
		require.Equal(t, int64(10<<30), buyer.TransferEnable)
		payment := reconcilerLedger(t, db, "payment:PAY1")
		require.NotNil(t, payment)
		require.Equal(t, OrderPaymentMethod, payment.Method)
		require.JSONEq(t, `{"order_id":1,"outcome":"completed"}`, payment.Result)
		require.NotNil(t, reconcilerLedger(t, db, "order:1"), "the plan is granted with the order's request id")

		// The buyer's plan changes after the grant; nothing grants it again.
		require.NoError(t, db.Model(&model.User{}).Where("id = ?", 2).Update("transfer_enable", 7).Error)
		again, err := reconciler.RunOnce(ctx, reconcilerPaidAt.Add(10*time.Minute))
		require.NoError(t, err)
		require.Equal(t, OrderPaymentReconciliation{}, again, "a completed order is not a candidate")

		// The provider delivers the callback after all: its repeat is a no-op.
		var record model.PaymentRecord
		require.NoError(t, db.Take(&record, "trade_no = ?", "PAY1").Error)
		NewPaymentGatewayService(db).FinishPaidOrder(record)
		var replay OrderPaymentResult
		require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
			var err error
			replay, err = CompleteOrderPaymentTx(tx, "PAY1", 1, reconcilerPaidAt.Add(time.Hour))
			return err
		}))
		require.False(t, replay.Applied)
		require.Equal(t, OrderPaymentCompleted, replay.Outcome)
		require.NoError(t, db.Take(&buyer, 2).Error)
		require.Equal(t, int64(7), buyer.TransferEnable)
		require.Equal(t, 3, reconcilerOrderStatus(t, db, 1))
		require.EqualValues(t, 2, reconcilerLedgerSize(t, db), "payment:PAY1 and order:1, once")
	})
}

// A record paid less than the grace period ago belongs to its live
// callback; the reconciler applies it only after the grace period.
func TestOrderPaymentReconcilerWaitsForTheGracePeriod(t *testing.T) {
	forEachReconcilerDatabase(t, func(t *testing.T, db *gorm.DB) {
		createStuckPayments(t, db, paidRecord("PAY1", 1))
		reconciler := NewOrderPaymentReconciler(db)
		ctx := context.Background()

		for _, elapsed := range []time.Duration{0, time.Minute, OrderPaymentReconcileGrace - time.Second, OrderPaymentReconcileGrace} {
			run, err := reconciler.RunOnce(ctx, reconcilerPaidAt.Add(elapsed))
			require.NoError(t, err)
			require.Zero(t, run.Examined, "%s after the payment", elapsed)
		}
		require.Equal(t, 0, reconcilerOrderStatus(t, db, 1))
		require.Zero(t, reconcilerLedgerSize(t, db))

		run, err := reconciler.RunOnce(ctx, reconcilerPaidAt.Add(OrderPaymentReconcileGrace+time.Second))
		require.NoError(t, err)
		require.Equal(t, 1, run.Completed)
		require.Equal(t, 3, reconcilerOrderStatus(t, db, 1))
	})
}

// A record that does not pay its order is refused as the callback refuses
// it: the order is left unchanged, the reason recorded and logged, and the
// record is not tried again.
func TestOrderPaymentReconcilerLeavesRecordsThatFailTheChecks(t *testing.T) {
	forEachReconcilerDatabase(t, func(t *testing.T, db *gorm.DB) {
		otherUsers := paidRecord("PAY2", 2)
		otherUsers.orderUser = 3
		short := paidRecord("PAY3", 3)
		short.amount = 99.99
		createStuckPayments(t, db, otherUsers, short)
		reconciler := NewOrderPaymentReconciler(db)
		ctx := context.Background()

		run, err := reconciler.RunOnce(ctx, reconcilerPaidAt.Add(time.Hour))
		require.NoError(t, err)
		require.Equal(t, OrderPaymentReconciliation{Examined: 2, Refused: 2}, run)
		for tradeNo, c := range map[string]struct {
			order  uint
			reason string
		}{
			"PAY2": {2, "the order is another user's"},
			"PAY3": {3, "the payment is below the order's total"},
		} {
			require.Equal(t, 0, reconcilerOrderStatus(t, db, c.order), tradeNo)
			row := reconcilerLedger(t, db, "payment:"+tradeNo)
			require.NotNil(t, row, tradeNo)
			require.JSONEq(t, fmt.Sprintf(`{"order_id":%d,"outcome":"refused","reason":%q}`, c.order, c.reason), row.Result, tradeNo)
			require.Nil(t, reconcilerLedger(t, db, fmt.Sprintf("order:%d", c.order)), tradeNo)
		}
		for _, id := range []uint{2, 3} {
			var buyer model.User
			require.NoError(t, db.Take(&buyer, id).Error)
			require.Nil(t, buyer.PlanID, "no refused payment grants a plan")
		}

		again, err := reconciler.RunOnce(ctx, reconcilerPaidAt.Add(2*time.Hour))
		require.NoError(t, err)
		require.Equal(t, OrderPaymentReconciliation{}, again, "a refused record is left alone")
		require.EqualValues(t, 2, reconcilerLedgerSize(t, db))
	})
}

// Only a paid record within the request ledger's retention whose order
// exists and is pending is a candidate.
func TestOrderPaymentReconcilerTouchesOnlyPaidRecordsOfPendingOrders(t *testing.T) {
	forEachReconcilerDatabase(t, func(t *testing.T, db *gorm.DB) {
		completed := paidRecord("PAY1", 1)
		completed.orderState = 3
		cancelled := paidRecord("PAY2", 2)
		cancelled.orderState = 2
		paidOnly := paidRecord("PAY3", 3)
		paidOnly.orderState = 1
		pending := paidRecord("PAY4", 4)
		pending.recordState = model.PaymentStatusPending
		noOrder := paidRecord("PAY5", 0)
		missingOrder := paidRecord("PAY6", 99)
		missingOrder.withOrder = false
		old := paidRecord("PAY7", 7)
		old.paidAt = reconcilerPaidAt.Add(-91 * 24 * time.Hour)
		createStuckPayments(t, db, completed, cancelled, paidOnly, pending, noOrder, missingOrder, old)

		run, err := NewOrderPaymentReconciler(db).RunOnce(context.Background(), reconcilerPaidAt.Add(time.Hour))
		require.NoError(t, err)
		require.Equal(t, OrderPaymentReconciliation{}, run)
		for id, status := range map[uint]int{1: 3, 2: 2, 3: 1, 4: 0, 7: 0} {
			require.Equal(t, status, reconcilerOrderStatus(t, db, id), "order %d", id)
		}
		require.Zero(t, reconcilerLedgerSize(t, db))
	})
}

// A grant that fails leaves the order paid for an administrator, as the
// callback does, and the order is no longer a candidate.
func TestOrderPaymentReconcilerKeepsTheOrderPaidWhenTheGrantFails(t *testing.T) {
	forEachReconcilerDatabase(t, func(t *testing.T, db *gorm.DB) {
		if db.Name() == "postgres" {
			require.NoError(t, db.Exec("ALTER TABLE v2_order DROP CONSTRAINT IF EXISTS fk_v2_order_plan").Error)
		}
		createStuckPayments(t, db, paidRecord("PAY1", 1))
		require.NoError(t, db.Model(&model.Order{}).Where("id = ?", 1).Update("plan_id", 404).Error)
		reconciler := NewOrderPaymentReconciler(db)

		run, err := reconciler.RunOnce(context.Background(), reconcilerPaidAt.Add(time.Hour))
		require.NoError(t, err)
		require.Equal(t, OrderPaymentReconciliation{Examined: 1, Paid: 1}, run)
		require.Equal(t, 1, reconcilerOrderStatus(t, db, 1))
		require.Nil(t, reconcilerLedger(t, db, "order:1"))

		again, err := reconciler.RunOnce(context.Background(), reconcilerPaidAt.Add(2*time.Hour))
		require.NoError(t, err)
		require.Equal(t, OrderPaymentReconciliation{}, again)
	})
}

// A run reads at most MaxBatches batches of Batch records; the rest waits
// for the next run.
func TestOrderPaymentReconcilerRunsInBoundedBatches(t *testing.T) {
	forEachReconcilerDatabase(t, func(t *testing.T, db *gorm.DB) {
		for id := uint(1); id <= 5; id++ {
			createStuckPayments(t, db, paidRecord(fmt.Sprintf("PAY%d", id), id))
		}
		reconciler := NewOrderPaymentReconciler(db)
		reconciler.Batch, reconciler.MaxBatches = 2, 2
		ctx := context.Background()
		now := reconcilerPaidAt.Add(time.Hour)

		first, err := reconciler.RunOnce(ctx, now)
		require.NoError(t, err)
		require.Equal(t, OrderPaymentReconciliation{Examined: 4, Completed: 4}, first)
		require.Equal(t, 0, reconcilerOrderStatus(t, db, 5))
		second, err := reconciler.RunOnce(ctx, now)
		require.NoError(t, err)
		require.Equal(t, OrderPaymentReconciliation{Examined: 1, Completed: 1}, second)
		third, err := reconciler.RunOnce(ctx, now)
		require.NoError(t, err)
		require.Equal(t, OrderPaymentReconciliation{}, third)
		for id := uint(1); id <= 5; id++ {
			require.Equal(t, 3, reconcilerOrderStatus(t, db, id))
		}
	})
}

// Two processes reconciling at once complete every order once.
func TestOrderPaymentReconcilersRunningAtOnceCompleteEachOrderOnce(t *testing.T) {
	forEachReconcilerDatabase(t, func(t *testing.T, db *gorm.DB) {
		const payments = 6
		for id := uint(1); id <= payments; id++ {
			createStuckPayments(t, db, paidRecord(fmt.Sprintf("PAY%d", id), id))
		}
		now := reconcilerPaidAt.Add(time.Hour)
		runs := make([]OrderPaymentReconciliation, 2)
		errs := make([]error, 2)
		var wg sync.WaitGroup
		for i := range runs {
			wg.Add(1)
			go func() {
				defer wg.Done()
				runs[i], errs[i] = NewOrderPaymentReconciler(db).RunOnce(context.Background(), now)
			}()
		}
		wg.Wait()
		require.NoError(t, errors.Join(errs...))
		require.Equal(t, payments, runs[0].Completed+runs[1].Completed)
		require.Zero(t, runs[0].Failed+runs[1].Failed)
		for id := uint(1); id <= payments; id++ {
			require.Equal(t, 3, reconcilerOrderStatus(t, db, id))
		}
		require.EqualValues(t, 2*payments, reconcilerLedgerSize(t, db), "payment:<trade_no> and order:<id> once each")
	})
}

// A cancelled run stops before the next record.
func TestOrderPaymentReconcilerStopsWhenCancelled(t *testing.T) {
	forEachReconcilerDatabase(t, func(t *testing.T, db *gorm.DB) {
		createStuckPayments(t, db, paidRecord("PAY1", 1))
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := NewOrderPaymentReconciler(db).RunOnce(ctx, reconcilerPaidAt.Add(time.Hour))
		require.ErrorIs(t, err, context.Canceled)
		require.Equal(t, 0, reconcilerOrderStatus(t, db, 1))
	})
}
