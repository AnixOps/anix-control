package kernelorder

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	kernelorderv1 "github.com/AnixOps/anix-control/sdk/api/kernelorder/v1"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var paymentHost = packagebridge.HostIdentity{PackageID: "payment", Version: "4.1.0", Generation: 1}

// authorizer grants the capabilities it holds, or answers err.
type authorizer struct {
	granted map[string]bool
	err     error
}

func (a authorizer) AuthorizeCapability(_ context.Context, host packagebridge.HostIdentity, capability string) error {
	if a.err != nil {
		return a.err
	}
	if host == paymentHost && a.granted[capability] {
		return nil
	}
	return service.ErrCapabilityNotAuthorized
}

var completes = authorizer{granted: map[string]bool{service.CapabilityOrderComplete: true}}

func ptr[T any](value T) *T { return &value }

// fixture seeds a plan, buyers and orders, and a paid record per order.
//
//	order 1: buyer 2's pending order of 100.00, paid in full by PAY1;
//	order 2: buyer 3's order, paid by buyer 2's PAY2;
//	order 3: buyer 2's order of 100.00, paid 99.99 by PAY3;
//	order 4: buyer 2's completed order, paid again by PAY4;
//	order 5: buyer 2's cancelled order, paid by PAY5;
//	order 6: buyer 2's pending order of a deleted plan, paid by PAY6;
//	PAY7 names order 99, which does not exist; PAY8 is still pending;
//	PAY9 pays no order.
func fixture(t *testing.T, auth Authorizer) (*gorm.DB, kernelorderv1.KernelOrderServer) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.Plan{}, &model.PlanSubscriptionGroup{}, &model.User{}, &model.UserSubscriptionGroup{},
		&model.Order{}, &model.PaymentRecord{}, &model.SubscriberRequest{}, &model.SubscriberChange{}))
	require.NoError(t, db.Create(&model.Plan{ID: 1, Name: "Pro", GroupID: 4, TransferEnable: 10}).Error)
	require.NoError(t, db.Create(&model.PlanSubscriptionGroup{PlanID: 1, GroupID: 8}).Error)
	require.NoError(t, db.Create(&[]model.User{
		{ID: 2, Email: "buyer@x", Token: "t2", UUID: "u2"},
		{ID: 3, Email: "other@x", Token: "t3", UUID: "u3"},
	}).Error)
	order := func(id, user, plan uint, status int) model.Order {
		return model.Order{ID: id, UserID: user, PlanID: plan, Period: "month", TradeNo: fmt.Sprintf("T%d", id), TotalAmount: 10000, Status: status}
	}
	require.NoError(t, db.Create(&[]model.Order{
		order(1, 2, 1, 0), order(2, 3, 1, 0), order(3, 2, 1, 0), order(4, 2, 1, 3), order(5, 2, 1, 2), order(6, 2, 404, 0),
	}).Error)
	now := time.Now()
	record := func(tradeNo string, user uint, amount float64, orderID *uint, status int) model.PaymentRecord {
		r := model.PaymentRecord{TradeNo: tradeNo, UserID: user, Amount: amount, ActualAmount: amount, Status: status, OrderID: orderID}
		if status == model.PaymentStatusPaid {
			r.PaidAt = &now
		}
		return r
	}
	require.NoError(t, db.Create(&[]model.PaymentRecord{
		record("PAY1", 2, 100, ptr(uint(1)), model.PaymentStatusPaid),
		record("PAY2", 2, 100, ptr(uint(2)), model.PaymentStatusPaid),
		record("PAY3", 2, 99.99, ptr(uint(3)), model.PaymentStatusPaid),
		record("PAY4", 2, 100, ptr(uint(4)), model.PaymentStatusPaid),
		record("PAY5", 2, 100, ptr(uint(5)), model.PaymentStatusPaid),
		record("PAY6", 2, 100, ptr(uint(6)), model.PaymentStatusPaid),
		record("PAY7", 2, 100, ptr(uint(99)), model.PaymentStatusPaid),
		record("PAY8", 2, 100, ptr(uint(1)), model.PaymentStatusPending),
		record("PAY9", 2, 100, nil, model.PaymentStatusPaid),
	}).Error)
	server := &Server{DB: db, Authorizer: auth}
	return db, server.For(paymentHost)
}

func orderStatus(t *testing.T, db *gorm.DB, id uint) int {
	t.Helper()
	var order model.Order
	require.NoError(t, db.Take(&order, id).Error)
	return order.Status
}

func ledger(t *testing.T, db *gorm.DB, requestID string) *model.SubscriberRequest {
	t.Helper()
	var row model.SubscriberRequest
	err := db.Take(&row, "request_id = ?", requestID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	require.NoError(t, err)
	return &row
}

func TestCompleteOrderPaymentNeedsItsCapability(t *testing.T) {
	ctx := context.Background()
	request := &kernelorderv1.CompleteOrderPaymentRequest{TradeNo: "PAY1", OrderId: 1}
	for name, auth := range map[string]authorizer{
		"no capability":      {},
		"another capability": {granted: map[string]bool{service.CapabilitySubscriberEntitlements: true}},
		"a fenced host":      {err: packagebridge.ErrHostFenced},
	} {
		db, server := fixture(t, auth)
		_, err := server.CompleteOrderPayment(ctx, request)
		require.Equal(t, codes.PermissionDenied, status.Code(err), name)
		require.Equal(t, 0, orderStatus(t, db, 1), name)
	}
	unconfigured := (&Server{}).For(paymentHost)
	_, err := unconfigured.CompleteOrderPayment(ctx, request)
	require.Equal(t, codes.Unavailable, status.Code(err))
}

// A paid record pays its order, which is completed and its plan granted,
// once per trade number.
func TestCompleteOrderPaymentCompletesTheOrderOnce(t *testing.T) {
	db, server := fixture(t, completes)
	ctx := context.Background()
	request := &kernelorderv1.CompleteOrderPaymentRequest{TradeNo: "PAY1", OrderId: 1}

	first, err := server.CompleteOrderPayment(ctx, request)
	require.NoError(t, err)
	require.True(t, first.GetApplied())
	require.Equal(t, kernelorderv1.OrderPaymentOutcome_ORDER_PAYMENT_OUTCOME_COMPLETED, first.GetOutcome())
	require.EqualValues(t, 1, first.GetOrderId())
	require.Empty(t, first.GetReason())
	var order model.Order
	require.NoError(t, db.Take(&order, 1).Error)
	require.Equal(t, 3, order.Status)
	require.NotNil(t, order.PaidAt)
	var buyer model.User
	require.NoError(t, db.Take(&buyer, 2).Error)
	require.Equal(t, uint(1), *buyer.PlanID)
	require.Equal(t, int64(10<<30), buyer.TransferEnable)
	payment := ledger(t, db, "payment:PAY1")
	require.NotNil(t, payment)
	require.Equal(t, service.OrderPaymentMethod, payment.Method)
	require.EqualValues(t, 2, payment.UserID)
	require.JSONEq(t, `{"order_id":1,"outcome":"completed"}`, payment.Result)
	require.NotNil(t, ledger(t, db, "order:1"), "the plan is granted with the order's request id")

	// The buyer's plan changes after the grant; a repeat grants nothing.
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", 2).Update("transfer_enable", 7).Error)
	again, err := server.CompleteOrderPayment(ctx, request)
	require.NoError(t, err)
	require.False(t, again.GetApplied())
	require.Equal(t, first.GetOutcome(), again.GetOutcome())
	require.NoError(t, db.Take(&buyer, 2).Error)
	require.Equal(t, int64(7), buyer.TransferEnable)

	_, err = server.CompleteOrderPayment(ctx, &kernelorderv1.CompleteOrderPaymentRequest{TradeNo: "PAY1", OrderId: 3})
	require.Equal(t, codes.FailedPrecondition, status.Code(err), "a repeat that names another order")
}

// The kernel re-checks that the record pays its order (#73): a refusal
// leaves the order unchanged and is recorded, so a repeat answers it again.
func TestCompleteOrderPaymentRefusesRecordsThatDoNotPayTheirOrder(t *testing.T) {
	db, server := fixture(t, completes)
	ctx := context.Background()
	for _, c := range []struct {
		tradeNo string
		order   uint64
		reason  string
		status  int
	}{
		{"PAY2", 2, "the order is another user's", 0},
		{"PAY3", 3, "the payment is below the order's total", 0},
		{"PAY4", 4, "the order is not pending (status 3)", 3},
		{"PAY5", 5, "the order is not pending (status 2)", 2},
		{"PAY7", 99, "the order does not exist", -1},
	} {
		response, err := server.CompleteOrderPayment(ctx, &kernelorderv1.CompleteOrderPaymentRequest{TradeNo: c.tradeNo, OrderId: c.order})
		require.NoError(t, err, c.tradeNo)
		require.True(t, response.GetApplied(), c.tradeNo)
		require.Equal(t, kernelorderv1.OrderPaymentOutcome_ORDER_PAYMENT_OUTCOME_REFUSED, response.GetOutcome(), c.tradeNo)
		require.Equal(t, c.reason, response.GetReason(), c.tradeNo)
		if c.status >= 0 {
			require.Equal(t, c.status, orderStatus(t, db, uint(c.order)), c.tradeNo)
		}
		var result service.OrderPaymentResult
		row := ledger(t, db, "payment:"+c.tradeNo)
		require.NotNil(t, row, c.tradeNo)
		require.NoError(t, json.Unmarshal([]byte(row.Result), &result))
		require.Equal(t, c.reason, result.Reason)

		again, err := server.CompleteOrderPayment(ctx, &kernelorderv1.CompleteOrderPaymentRequest{TradeNo: c.tradeNo, OrderId: c.order})
		require.NoError(t, err, c.tradeNo)
		require.False(t, again.GetApplied(), c.tradeNo)
		require.Equal(t, c.reason, again.GetReason(), c.tradeNo)
	}
	var buyer model.User
	require.NoError(t, db.Take(&buyer, 2).Error)
	require.Nil(t, buyer.PlanID, "no refused payment grants a plan")
	require.Nil(t, ledger(t, db, "order:2"))
}

// A grant that fails leaves the order paid for an administrator.
func TestCompleteOrderPaymentKeepsTheOrderPaidWhenTheGrantFails(t *testing.T) {
	db, server := fixture(t, completes)
	response, err := server.CompleteOrderPayment(context.Background(), &kernelorderv1.CompleteOrderPaymentRequest{TradeNo: "PAY6", OrderId: 6})
	require.NoError(t, err)
	require.Equal(t, kernelorderv1.OrderPaymentOutcome_ORDER_PAYMENT_OUTCOME_PAID, response.GetOutcome())
	require.Equal(t, 1, orderStatus(t, db, 6))
	require.Nil(t, ledger(t, db, "order:6"))
}

// Only a paid record that names the order is applied; anything else is an
// error that records nothing, so a later call may apply.
func TestCompleteOrderPaymentNeedsAPaidRecordOfTheOrder(t *testing.T) {
	db, server := fixture(t, completes)
	ctx := context.Background()
	for _, c := range []struct {
		request *kernelorderv1.CompleteOrderPaymentRequest
		code    codes.Code
	}{
		{&kernelorderv1.CompleteOrderPaymentRequest{OrderId: 1}, codes.InvalidArgument},
		{&kernelorderv1.CompleteOrderPaymentRequest{TradeNo: string(make([]byte, 121)), OrderId: 1}, codes.InvalidArgument},
		{&kernelorderv1.CompleteOrderPaymentRequest{TradeNo: "NOPE", OrderId: 1}, codes.NotFound},
		{&kernelorderv1.CompleteOrderPaymentRequest{TradeNo: "PAY8", OrderId: 1}, codes.FailedPrecondition},
		{&kernelorderv1.CompleteOrderPaymentRequest{TradeNo: "PAY1", OrderId: 3}, codes.FailedPrecondition},
		{&kernelorderv1.CompleteOrderPaymentRequest{TradeNo: "PAY9", OrderId: 1}, codes.FailedPrecondition},
	} {
		_, err := server.CompleteOrderPayment(ctx, c.request)
		require.Equal(t, c.code, status.Code(err), "%s", c.request.GetTradeNo())
	}
	var recorded int64
	require.NoError(t, db.Model(&model.SubscriberRequest{}).Count(&recorded).Error)
	require.Zero(t, recorded)
	require.Equal(t, 0, orderStatus(t, db, 1))

	require.NoError(t, db.Model(&model.PaymentRecord{}).Where("trade_no = ?", "PAY8").Update("status", model.PaymentStatusPaid).Error)
	response, err := server.CompleteOrderPayment(ctx, &kernelorderv1.CompleteOrderPaymentRequest{TradeNo: "PAY8", OrderId: 1})
	require.NoError(t, err)
	require.Equal(t, kernelorderv1.OrderPaymentOutcome_ORDER_PAYMENT_OUTCOME_COMPLETED, response.GetOutcome())
}

// Another write that took the payment's request id blocks the completion
// instead of being answered as its first result.
func TestCompleteOrderPaymentRefusesARequestIDOfAnotherWrite(t *testing.T) {
	db, server := fixture(t, completes)
	require.NoError(t, db.Create(&model.SubscriberRequest{
		RequestID: "payment:PAY1", Method: "apply_entitlement", UserID: 2, Result: `{}`, CreatedAt: time.Now(),
	}).Error)
	_, err := server.CompleteOrderPayment(context.Background(), &kernelorderv1.CompleteOrderPaymentRequest{TradeNo: "PAY1", OrderId: 1})
	require.Equal(t, codes.FailedPrecondition, status.Code(err))
	require.Equal(t, 0, orderStatus(t, db, 1))
}
