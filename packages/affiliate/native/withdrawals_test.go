package native

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	kernelsubscriberv1 "github.com/AnixOps/anix-control/sdk/api/kernelsubscriber/v1"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// kernel answers AdjustBalance with err, or applies it.
type kernel struct {
	err   error
	calls []*kernelsubscriberv1.AdjustBalanceRequest
}

func (k *kernel) AdjustBalance(_ context.Context, in *kernelsubscriberv1.AdjustBalanceRequest, _ ...grpc.CallOption) (*kernelsubscriberv1.AdjustBalanceResponse, error) {
	k.calls = append(k.calls, in)
	if k.err != nil {
		return nil, k.err
	}
	return &kernelsubscriberv1.AdjustBalanceResponse{Applied: true}, nil
}

// fixture is a package database with the adopted tables, a table standing
// in for the entitlement view, a configuration and a pending withdrawal of
// 40 by user 2, whose view balance is 100.
func fixture(t *testing.T, k *kernel) (*Service, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "affiliate.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(&Withdrawal{}, &InviteConfig{}, &Entitlement{}))
	cfg := defaultInviteConfig()
	require.NoError(t, db.Create(&cfg).Error)
	require.NoError(t, db.Create(&Entitlement{ID: 2, CommissionBalance: 100}).Error)
	require.NoError(t, db.Create(&Withdrawal{ID: 1, UserID: 2, Amount: 40, Method: "bank", Remark: "before"}).Error)
	return &Service{Open: func(ctx context.Context) (*gorm.DB, error) { return db.WithContext(ctx), nil }, Subscriber: k}, db
}

func call(t *testing.T, handler pluginhostsdk.NativeHandler, request pluginhostsdk.NativeRequest) (uint32, map[string]any) {
	t.Helper()
	response, err := handler(context.Background(), request)
	require.NoError(t, err)
	var body map[string]any
	require.NoError(t, json.Unmarshal(response.Body, &body), "%s", response.Body)
	return response.StatusCode, body
}

func process(id, body string) pluginhostsdk.NativeRequest {
	return pluginhostsdk.NativeRequest{
		Body: []byte(body), Principal: pluginhostsdk.Principal{ActorID: 1, Admin: true},
		Metadata: pluginhostsdk.RequestMetadata{PathParams: map[string]string{"id": id}},
	}
}

func withdrawRequest(amount string) pluginhostsdk.NativeRequest {
	return pluginhostsdk.NativeRequest{
		Body: []byte(`{"amount":` + amount + `,"method":"bank","account":"a","name":"n"}`), Principal: pluginhostsdk.Principal{ActorID: 2},
	}
}

func stored(t *testing.T, db *gorm.DB, id uint) Withdrawal {
	t.Helper()
	var row Withdrawal
	require.NoError(t, db.Take(&row, id).Error)
	return row
}

// A decision that commits after this one read the withdrawal wins; this
// one neither overwrites it nor refunds.
func TestAConcurrentDecisionAppliesOnce(t *testing.T) {
	k := &kernel{}
	service, db := fixture(t, k)
	fired := false
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:concurrent", func(tx *gorm.DB) {
		if fired || tx.Statement.Table != "v2_commission_withdraw" {
			return
		}
		fired = true
		tx.Session(&gorm.Session{NewDB: true}).Exec("UPDATE v2_commission_withdraw SET status = 1, remark = 'first' WHERE id = 1")
	}))
	code, body := call(t, service.AdminProcessWithdrawal, process("1", `{"approve":false,"remark":"second"}`))
	require.EqualValues(t, 400, code)
	require.Equal(t, "withdrawal already processed", body["error"])
	require.Empty(t, k.calls, "no refund")
	row := stored(t, db, 1)
	require.Equal(t, 1, row.Status)
	require.Equal(t, "first", row.Remark)
}

// A refund the kernel refuses undoes the rejection: the withdrawal is
// pending again, as it was.
func TestARefusedRefundLeavesTheWithdrawalPending(t *testing.T) {
	k := &kernel{err: status.Error(codes.PermissionDenied, "package is not authorized for kernel.subscriber.balance.v1")}
	service, db := fixture(t, k)
	before := stored(t, db, 1)
	code, body := call(t, service.AdminProcessWithdrawal, process("1", `{"approve":false,"remark":"no"}`))
	require.EqualValues(t, 500, code)
	require.Equal(t, "failed to process withdrawal", body["error"])
	require.Len(t, k.calls, 1)
	require.Equal(t, "affiliate.withdraw.refund:1", k.calls[0].GetRequestId())
	require.EqualValues(t, 40, k.calls[0].GetAmountCents())
	after := stored(t, db, 1)
	require.Equal(t, 0, after.Status)
	require.Equal(t, "before", after.Remark)
	require.Nil(t, after.ProcessedAt)
	require.True(t, before.UpdatedAt.Equal(after.UpdatedAt))
}

// A refund whose outcome is unknown may have been applied: the withdrawal
// stays rejected, so it cannot also be approved and paid out.
func TestAnUnknownRefundOutcomeLeavesTheWithdrawalRejected(t *testing.T) {
	k := &kernel{err: status.Error(codes.Unavailable, "connection reset")}
	service, db := fixture(t, k)
	code, body := call(t, service.AdminProcessWithdrawal, process("1", `{"approve":false}`))
	require.EqualValues(t, 500, code)
	require.Equal(t, "failed to process withdrawal", body["error"])
	require.Equal(t, 2, stored(t, db, 1).Status)
	code, body = call(t, service.AdminProcessWithdrawal, process("1", `{"approve":true}`))
	require.EqualValues(t, 400, code)
	require.Equal(t, "withdrawal already processed", body["error"])
}

// A debit the kernel refuses leaves no withdrawal.
func TestARefusedDebitRemovesTheReservation(t *testing.T) {
	k := &kernel{err: status.Error(codes.FailedPrecondition, "insufficient balance")}
	service, db := fixture(t, k)
	code, body := call(t, service.UserWithdraw, withdrawRequest("60"))
	require.EqualValues(t, 400, code)
	require.Equal(t, "insufficient balance", body["error"])
	require.Len(t, k.calls, 1)
	require.Equal(t, "affiliate.withdraw:2", k.calls[0].GetRequestId())
	require.EqualValues(t, -60, k.calls[0].GetAmountCents())
	require.Equal(t, kernelsubscriberv1.BalanceKind_BALANCE_KIND_COMMISSION, k.calls[0].GetKind())
	var count int64
	require.NoError(t, db.Model(&Withdrawal{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
}

// A debit whose outcome is unknown keeps the reservation, which no
// decision can approve or reject: it is never paid out undebited.
func TestAnUnknownDebitOutcomeKeepsTheReservation(t *testing.T) {
	k := &kernel{err: status.Error(codes.DeadlineExceeded, "deadline exceeded")}
	service, db := fixture(t, k)
	code, body := call(t, service.UserWithdraw, withdrawRequest("60"))
	require.EqualValues(t, 400, code)
	require.Equal(t, "deadline exceeded", body["error"])
	require.Equal(t, withdrawReserving, stored(t, db, 2).Status)
	k.err = nil
	code, body = call(t, service.AdminProcessWithdrawal, process("2", `{"approve":true}`))
	require.EqualValues(t, 400, code)
	require.Equal(t, "withdrawal already processed", body["error"])
	require.Len(t, k.calls, 1, "no refund either")
}

// A withdrawal ends as the kernel writes it: pending, updated when created.
func TestAWithdrawalEndsPending(t *testing.T) {
	k := &kernel{}
	service, db := fixture(t, k)
	service.Now = func() time.Time { return time.Unix(1_790_000_000, 0) }
	code, body := call(t, service.UserWithdraw, withdrawRequest("60"))
	require.EqualValues(t, 200, code, "%v", body)
	row := stored(t, db, 2)
	require.Equal(t, withdrawPending, row.Status)
	require.True(t, row.CreatedAt.Equal(row.UpdatedAt))
	require.EqualValues(t, 0, body["data"].(map[string]any)["status"])
}

func TestRefundCents(t *testing.T) {
	for _, c := range []struct {
		amount float64
		cents  int64
		ok     bool
	}{
		{40, 40, true}, {1.5, 1, true}, {1.99, 1, true}, {0, 0, true}, {-1, 0, false}, {1 << 60, 0, false},
	} {
		cents, ok := RefundCents(c.amount)
		require.Equal(t, c.ok, ok, "%v", c.amount)
		require.Equal(t, c.cents, cents, "%v", c.amount)
	}
}
