package service

import (
	"fmt"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func paymentActivationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.PaymentGateway{}, &model.PaymentRecord{}, &model.Order{}, &model.Plan{},
		&model.PlanSubscriptionGroup{}, &model.User{}, &model.UserSubscriptionGroup{}, &model.SubscriberRequest{}, &model.SubscriberChange{}))
	return db
}

func pendingPayment(t *testing.T, db *gorm.DB, tradeNo string, planID uint) model.Order {
	t.Helper()
	order := model.Order{TradeNo: tradeNo, UserID: 1, PlanID: planID, Period: "month", TotalAmount: 1000}
	require.NoError(t, db.Create(&order).Error)
	require.NoError(t, db.Create(&model.PaymentRecord{
		TradeNo: tradeNo, UserID: 1, Amount: 10, ActualAmount: 10, Status: model.PaymentStatusPending, OrderID: &order.ID,
	}).Error)
	return order
}

// A successful payment callback activates the order's plan at once
// (owner decision, 2026-09-30).
func TestPaymentCallbackActivatesThePaidOrder(t *testing.T) {
	db := paymentActivationDB(t)
	require.NoError(t, db.Create(&model.User{ID: 1, Email: "buyer@example.test", Token: "t", UUID: "u"}).Error)
	plan := model.Plan{Name: "Pro", GroupID: 4, TransferEnable: 50}
	require.NoError(t, db.Create(&plan).Error)
	require.NoError(t, db.Create(&model.PlanSubscriptionGroup{PlanID: plan.ID, GroupID: 4}).Error)
	order := pendingPayment(t, db, "PAY-ACTIVATE", plan.ID)

	require.NoError(t, NewPaymentGatewayService(db).MarkOrderPaid("PAY-ACTIVATE", "GW-1", "{}"))
	require.NoError(t, db.Take(&order, order.ID).Error)
	require.Equal(t, 3, order.Status, "the paid order is completed")
	var user model.User
	require.NoError(t, db.Take(&user, 1).Error)
	require.Equal(t, plan.ID, *user.PlanID)
	require.Equal(t, int64(50*1073741824), user.TransferEnable)
	require.NotNil(t, user.ExpiredAt)
}

// If activation fails the payment is still recorded, and the order stays
// paid for an administrator to complete.
func TestPaymentCallbackKeepsThePaymentWhenActivationFails(t *testing.T) {
	db := paymentActivationDB(t)
	require.NoError(t, db.Create(&model.User{ID: 1, Email: "buyer@example.test", Token: "t", UUID: "u"}).Error)
	order := pendingPayment(t, db, "PAY-NO-PLAN", 404)

	require.NoError(t, NewPaymentGatewayService(db).MarkOrderPaid("PAY-NO-PLAN", "GW-2", "{}"))
	require.NoError(t, db.Take(&order, order.ID).Error)
	require.Equal(t, 1, order.Status)
	var record model.PaymentRecord
	require.NoError(t, db.Take(&record, "trade_no = ?", "PAY-NO-PLAN").Error)
	require.Equal(t, model.PaymentStatusPaid, record.Status)
}

// MarkOrderPaidIfCovered checks the pending record inside the transaction:
// a refusal (or no check) writes nothing, an acceptance pays as
// MarkOrderPaid does.
func TestMarkOrderPaidIfCoveredChecksTheRecord(t *testing.T) {
	db := paymentActivationDB(t)
	require.NoError(t, db.Create(&model.User{ID: 1, Email: "buyer@example.test", Token: "t", UUID: "u"}).Error)
	order := pendingPayment(t, db, "PAY-COVERED", 404)
	svc := NewPaymentGatewayService(db)

	refuse := func(record model.PaymentRecord) error {
		require.Equal(t, model.PaymentStatusPending, record.Status)
		require.Equal(t, 10.0, record.ActualAmount)
		return fmt.Errorf("%w: short", ErrPaymentNotCovered)
	}
	require.ErrorIs(t, svc.MarkOrderPaidIfCovered("PAY-COVERED", "GW-1", "{}", refuse), ErrPaymentNotCovered)
	require.ErrorIs(t, svc.MarkOrderPaidIfCovered("PAY-COVERED", "GW-1", "{}", nil), ErrPaymentNotCovered)
	var record model.PaymentRecord
	require.NoError(t, db.Take(&record, "trade_no = ?", "PAY-COVERED").Error)
	require.Equal(t, model.PaymentStatusPending, record.Status)
	require.Empty(t, record.GatewayTradeNo)
	require.NoError(t, db.Take(&order, order.ID).Error)
	require.Equal(t, 0, order.Status)

	require.NoError(t, svc.MarkOrderPaidIfCovered("PAY-COVERED", "GW-2", "{}", func(model.PaymentRecord) error { return nil }))
	require.NoError(t, db.Take(&record, "trade_no = ?", "PAY-COVERED").Error)
	require.Equal(t, model.PaymentStatusPaid, record.Status)
	require.Equal(t, "GW-2", record.GatewayTradeNo)
	require.NoError(t, db.Take(&order, order.ID).Error)
	require.Equal(t, 1, order.Status)
}

// A payment for an order that is no longer pending (cancelled, or paid by
// another payment) is recorded, and the order is left unchanged; the
// refusal is recorded once under the payment's request id.
func TestPaymentCallbackLeavesAnOrderThatIsNoLongerPending(t *testing.T) {
	db := paymentActivationDB(t)
	require.NoError(t, db.Create(&model.User{ID: 1, Email: "buyer@example.test", Token: "t", UUID: "u"}).Error)
	plan := model.Plan{Name: "Pro", GroupID: 4, TransferEnable: 50}
	require.NoError(t, db.Create(&plan).Error)
	order := pendingPayment(t, db, "PAY-CANCELLED", plan.ID)
	require.NoError(t, db.Model(&order).Update("status", 2).Error)

	require.NoError(t, NewPaymentGatewayService(db).MarkOrderPaid("PAY-CANCELLED", "GW-1", "{}"))
	var record model.PaymentRecord
	require.NoError(t, db.Take(&record, "trade_no = ?", "PAY-CANCELLED").Error)
	require.Equal(t, model.PaymentStatusPaid, record.Status)
	require.NoError(t, db.Take(&order, order.ID).Error)
	require.Equal(t, 2, order.Status)
	require.Nil(t, order.PaidAt)
	var user model.User
	require.NoError(t, db.Take(&user, 1).Error)
	require.Nil(t, user.PlanID)
	var ledger model.SubscriberRequest
	require.NoError(t, db.Take(&ledger, "request_id = ?", OrderPaymentRequestID("PAY-CANCELLED")).Error)
	require.JSONEq(t, fmt.Sprintf(`{"order_id":%d,"outcome":"refused","reason":"the order is not pending (status 2)"}`, order.ID), ledger.Result)
}

// The payment module records a payment, then completes its order through
// KernelOrder. A failure in between leaves a paid record whose order is
// pending; the provider's repeat of the callback completes the order, here
// in the kernel's legacy path, while answering as before.
func TestRepeatedCallbackFinishesAnOrderLeftPending(t *testing.T) {
	db := paymentActivationDB(t)
	require.NoError(t, db.Create(&model.User{ID: 1, Email: "buyer@example.test", Token: "t", UUID: "u"}).Error)
	plan := model.Plan{Name: "Pro", GroupID: 4, TransferEnable: 50}
	require.NoError(t, db.Create(&plan).Error)
	order := pendingPayment(t, db, "PAY-HALFWAY", plan.ID)
	require.NoError(t, db.Model(&model.PaymentRecord{}).Where("trade_no = ?", "PAY-HALFWAY").Update("status", model.PaymentStatusPaid).Error)

	svc := NewPaymentGatewayService(db)
	require.EqualError(t, svc.MarkOrderPaid("PAY-HALFWAY", "GW-1", "{}"), "payment already processed")
	require.NoError(t, db.Take(&order, order.ID).Error)
	require.Equal(t, 3, order.Status, "the repeat completes the order")
	var requests int64
	require.NoError(t, db.Model(&model.SubscriberRequest{}).Count(&requests).Error)
	require.EqualValues(t, 2, requests, "payment:PAY-HALFWAY and order:<id>")

	require.EqualError(t, svc.MarkOrderPaid("PAY-HALFWAY", "GW-1", "{}"), "payment already processed")
	require.NoError(t, db.Model(&model.SubscriberRequest{}).Count(&requests).Error)
	require.EqualValues(t, 2, requests, "a further repeat applies nothing")
}
