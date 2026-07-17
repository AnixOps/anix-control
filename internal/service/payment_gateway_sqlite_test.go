package service

import (
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMarkOrderPaidRetriesSQLiteWriterLockAtomically(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "payment-lock.db")+"?_pragma=busy_timeout(1)"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.PaymentGateway{}, &model.PaymentRecord{}, &model.Order{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(4)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })

	gateway := model.PaymentGateway{
		Name: "SQLite Retry Gateway", Type: model.PaymentGatewayEPay, Enabled: true,
		TotalOrders: 3, TotalAmount: 12.50,
	}
	require.NoError(t, db.Create(&gateway).Error)
	order := model.Order{TradeNo: "SQLITE-LOCK-PAYMENT", UserID: 7, TotalAmount: 1000}
	require.NoError(t, db.Create(&order).Error)
	record := model.PaymentRecord{
		GatewayID: gateway.ID, TradeNo: order.TradeNo, GatewayType: model.PaymentGatewayEPay,
		UserID: order.UserID, Amount: 10, ActualAmount: 10, Status: model.PaymentStatusPending, OrderID: &order.ID,
	}
	require.NoError(t, db.Create(&record).Error)

	holderReady := make(chan struct{})
	releaseHolder := make(chan struct{})
	var releaseHolderOnce sync.Once
	release := func() { releaseHolderOnce.Do(func() { close(releaseHolder) }) }
	t.Cleanup(release)
	holderDone := make(chan error, 1)
	go func() {
		holderDone <- db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(&model.PaymentGateway{}).Where("id = ?", gateway.ID).
				Update("description", "writer lock holder").Error; err != nil {
				return err
			}
			close(holderReady)
			<-releaseHolder
			return nil
		})
	}()
	select {
	case <-holderReady:
	case <-time.After(time.Second):
		t.Fatal("SQLite writer holder did not acquire the lock")
	}

	var paymentQueries atomic.Int32
	retryObserved := make(chan struct{})
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:observe_payment_retry", func(tx *gorm.DB) {
		if tx.Statement.Schema == nil || tx.Statement.Schema.Table != record.TableName() {
			return
		}
		if paymentQueries.Add(1) == 2 {
			close(retryObserved)
		}
	}))

	callbackAmount := 10.0
	paymentDone := make(chan error, 1)
	go func() {
		paymentDone <- NewPaymentGatewayService(db).MarkOrderPaidWithAmount(
			order.TradeNo,
			"EP-SQLITE-RETRY",
			`{"money":"10.00"}`,
			&callbackAmount,
		)
	}()

	select {
	case <-retryObserved:
	case <-time.After(time.Second):
		release()
		t.Fatal("payment transaction did not retry after SQLite writer contention")
	}

	var pendingRecord model.PaymentRecord
	require.NoError(t, db.First(&pendingRecord, record.ID).Error)
	require.Equal(t, model.PaymentStatusPending, pendingRecord.Status)
	require.Empty(t, pendingRecord.GatewayTradeNo)

	var pendingOrder model.Order
	require.NoError(t, db.First(&pendingOrder, order.ID).Error)
	require.Equal(t, 0, pendingOrder.Status)

	var pendingGateway model.PaymentGateway
	require.NoError(t, db.First(&pendingGateway, gateway.ID).Error)
	require.Equal(t, int64(3), pendingGateway.TotalOrders)
	require.InDelta(t, 12.50, pendingGateway.TotalAmount, 0.0001)

	release()
	select {
	case err := <-holderDone:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("SQLite writer holder did not release")
	}
	select {
	case err := <-paymentDone:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("payment transaction did not converge after writer lock release")
	}
	require.GreaterOrEqual(t, paymentQueries.Load(), int32(2))

	var paidRecord model.PaymentRecord
	require.NoError(t, db.First(&paidRecord, record.ID).Error)
	require.Equal(t, model.PaymentStatusPaid, paidRecord.Status)
	require.Equal(t, "EP-SQLITE-RETRY", paidRecord.GatewayTradeNo)
	require.Equal(t, `{"money":"10.00"}`, paidRecord.NotifyData)
	require.NotNil(t, paidRecord.PaidAt)

	var paidOrder model.Order
	require.NoError(t, db.First(&paidOrder, order.ID).Error)
	require.Equal(t, 1, paidOrder.Status)
	require.NotNil(t, paidOrder.PaidAt)

	var updatedGateway model.PaymentGateway
	require.NoError(t, db.First(&updatedGateway, gateway.ID).Error)
	require.Equal(t, int64(4), updatedGateway.TotalOrders)
	require.InDelta(t, 22.50, updatedGateway.TotalAmount, 0.0001)
}
