package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
)

// WithdrawSecurityTestSuite holds the regression tests for commission
// withdrawals: whole amounts, debits under the subscriber's row lock, one
// decision per withdrawal and atomic refunds.
type WithdrawSecurityTestSuite struct {
	HandlerTestSuite
	user *model.User
}

func (s *WithdrawSecurityTestSuite) SetupTest() {
	s.HandlerTestSuite.SetupTest()
	s.user = &model.User{Email: "inviter@example.test", Token: "inviter-token", UUID: "inviter-uuid", CommissionBalance: 100}
	s.Require().NoError(s.db.Create(s.user).Error)
}

// serve runs one request through handler as the inviter and returns the
// status code and decoded body.
func (s *WithdrawSecurityTestSuite) serve(method, pattern, path, body string, handler gin.HandlerFunc) (int, map[string]any) {
	router := gin.New()
	router.Handle(method, pattern, func(c *gin.Context) {
		c.Set("user_id", s.user.ID)
		handler(c)
	})
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	var decoded map[string]any
	s.Require().NoError(json.Unmarshal(w.Body.Bytes(), &decoded), w.Body.String())
	return w.Code, decoded
}

func (s *WithdrawSecurityTestSuite) withdraw(body string) (int, map[string]any) {
	return s.serve("POST", "/user/invite/withdraw", "/user/invite/withdraw", body, NewInviteHandler().RequestWithdraw)
}

func (s *WithdrawSecurityTestSuite) process(id uint, body string) (int, map[string]any) {
	path := "/admin/invite/withdrawals/" + strconv.FormatUint(uint64(id), 10) + "/process"
	return s.serve("POST", "/admin/invite/withdrawals/:id/process", path, body, NewInviteHandler().ProcessWithdraw)
}

func (s *WithdrawSecurityTestSuite) balance() int64 {
	var user model.User
	s.Require().NoError(s.db.Select("commission_balance").First(&user, s.user.ID).Error)
	return user.CommissionBalance
}

func (s *WithdrawSecurityTestSuite) withdrawals() []model.CommissionWithdraw {
	var rows []model.CommissionWithdraw
	s.Require().NoError(s.db.Order("id").Find(&rows).Error)
	return rows
}

func (s *WithdrawSecurityTestSuite) ledger() []string {
	var ids []string
	s.Require().NoError(s.db.Model(&model.SubscriberRequest{}).Order("request_id").Pluck("request_id", &ids).Error)
	return ids
}

func (s *WithdrawSecurityTestSuite) TestAFractionalAmountIsRefused() {
	// On PostgreSQL the driver truncated the amount, so a withdrawal of 1.99
	// debited 1 and its approval paid out 1.99; SQLite stored a fraction in
	// the integer balance.
	code, body := s.withdraw(`{"amount":1.99,"method":"alipay","account":"a","name":"n"}`)
	s.Equal(http.StatusBadRequest, code)
	s.Equal("amount must be a whole number", body["error"])
	s.Equal(int64(100), s.balance())
	s.Empty(s.withdrawals())
}

func (s *WithdrawSecurityTestSuite) TestAWithdrawalDebitsOnceInTheLedger() {
	code, body := s.withdraw(`{"amount":40,"method":"alipay","account":"a","name":"n"}`)
	s.Require().Equal(http.StatusOK, code, "%v", body)
	s.Equal(int64(60), s.balance())
	rows := s.withdrawals()
	s.Require().Len(rows, 1)
	s.Equal([]string{service.WithdrawDebitRequestID(rows[0].ID)}, s.ledger())
}

// A concurrent withdrawal that commits after this request's balance check
// must not take the balance below zero: the debit re-reads the balance
// under the subscriber's row lock.
func (s *WithdrawSecurityTestSuite) TestAConcurrentWithdrawalCannotOverdraw() {
	userID := s.user.ID
	creates := s.db.Callback().Create()
	s.Require().NoError(creates.After("gorm:create").Register("test:concurrent-debit", func(tx *gorm.DB) {
		if tx.Statement.Table == "v2_commission_withdraw" {
			tx.Session(&gorm.Session{NewDB: true}).Exec("UPDATE v2_user SET commission_balance = 30 WHERE id = ?", userID)
		}
	}))
	s.T().Cleanup(func() { _ = s.db.Callback().Create().Remove("test:concurrent-debit") })

	code, body := s.withdraw(`{"amount":50,"method":"alipay","account":"a","name":"n"}`)
	s.Equal(http.StatusBadRequest, code)
	s.Equal("insufficient balance", body["error"])
	s.Empty(s.withdrawals(), "the withdrawal rolls back with its debit")
	s.Empty(s.ledger())
}

// An administrator's change to the minimum applies to the next request; the
// handler serving users kept the value it first read until a restart.
func (s *WithdrawSecurityTestSuite) TestTheMinimumIsReadForEachRequest() {
	s.Require().NoError(s.db.Create(&model.InviteConfig{Enabled: true, CommissionType: 1, CommissionMinAmount: 10}).Error)
	handler := NewInviteHandler()
	serve := func(amount string) (int, map[string]any) {
		return s.serve("POST", "/user/invite/withdraw", "/user/invite/withdraw",
			`{"amount":`+amount+`,"method":"alipay","account":"a","name":"n"}`, handler.RequestWithdraw)
	}
	code, _ := serve("10")
	s.Require().Equal(http.StatusOK, code)
	s.Require().NoError(s.db.Model(&model.InviteConfig{}).Where("1 = 1").Update("commission_min_amount", 50).Error)
	code, body := serve("20")
	s.Equal(http.StatusBadRequest, code)
	s.Equal("amount below minimum", body["error"])
	s.Equal(int64(90), s.balance())
}

func (s *WithdrawSecurityTestSuite) pending(amount float64) *model.CommissionWithdraw {
	withdraw := &model.CommissionWithdraw{UserID: s.user.ID, Amount: amount, Method: "alipay", Account: "a", Name: "n"}
	s.Require().NoError(s.db.Create(withdraw).Error)
	return withdraw
}

func (s *WithdrawSecurityTestSuite) TestARejectionRefundsOnce() {
	withdraw := s.pending(40)
	code, body := s.process(withdraw.ID, `{"approve":false}`)
	s.Require().Equal(http.StatusOK, code, "%v", body)
	s.Equal(int64(140), s.balance())
	code, body = s.process(withdraw.ID, `{"approve":false}`)
	s.Equal(http.StatusBadRequest, code)
	s.Equal("withdrawal already processed", body["error"])
	s.Equal(int64(140), s.balance())
	s.Equal([]string{service.WithdrawRefundRequestID(withdraw.ID)}, s.ledger())
}

// A decision that commits after this one read the withdrawal wins; this one
// neither overwrites it nor refunds again.
func (s *WithdrawSecurityTestSuite) TestAConcurrentDecisionAppliesOnce() {
	withdraw := s.pending(40)
	userID := s.user.ID
	fired := false
	queries := s.db.Callback().Query()
	s.Require().NoError(queries.After("gorm:query").Register("test:concurrent-rejection", func(tx *gorm.DB) {
		if fired || tx.Statement.Table != "v2_commission_withdraw" {
			return
		}
		fired = true
		other := tx.Session(&gorm.Session{NewDB: true})
		other.Exec("UPDATE v2_commission_withdraw SET status = 2, remark = 'first' WHERE id = ?", withdraw.ID)
		other.Exec("UPDATE v2_user SET commission_balance = commission_balance + 40 WHERE id = ?", userID)
	}))
	s.T().Cleanup(func() { _ = s.db.Callback().Query().Remove("test:concurrent-rejection") })

	code, body := s.process(withdraw.ID, `{"approve":true,"remark":"second"}`)
	s.Equal(http.StatusBadRequest, code)
	s.Equal("withdrawal already processed", body["error"])
	rows := s.withdrawals()
	s.Require().Len(rows, 1)
	s.Equal(2, rows[0].Status)
	s.Equal("first", rows[0].Remark)
	s.Equal(int64(140), s.balance(), "refunded once, by the first decision")
}

// A refund that fails leaves the withdrawal pending; it was marked rejected
// with the amount lost.
func (s *WithdrawSecurityTestSuite) TestAFailedRefundLeavesTheWithdrawalPending() {
	withdraw := s.pending(40)
	updates := s.db.Callback().Update()
	s.Require().NoError(updates.Before("gorm:update").Register("test:refund-fails", func(tx *gorm.DB) {
		if tx.Statement.Table == "v2_user" {
			_ = tx.AddError(errors.New("refund refused"))
		}
	}))
	s.T().Cleanup(func() { _ = s.db.Callback().Update().Remove("test:refund-fails") })

	code, body := s.process(withdraw.ID, `{"status":"rejected"}`)
	s.Equal(http.StatusInternalServerError, code)
	s.Equal("failed to process withdrawal", body["error"])
	rows := s.withdrawals()
	s.Require().Len(rows, 1)
	s.Equal(0, rows[0].Status)
	s.Nil(rows[0].ProcessedAt)
	s.Equal(int64(100), s.balance())
	s.Empty(s.ledger())
}

// A withdrawal stored with a fraction before amounts had to be whole is
// refunded its whole part, which is what PostgreSQL debited, and never
// leaves a fraction in the integer balance.
func (s *WithdrawSecurityTestSuite) TestAFractionalWithdrawalIsRefundedItsWholePart() {
	withdraw := s.pending(1.99)
	code, body := s.process(withdraw.ID, `{"status":2}`)
	s.Require().Equal(http.StatusOK, code, "%v", body)
	s.Equal(int64(101), s.balance())
}

// A rejected withdrawal whose user no longer exists has no one to refund.
func (s *WithdrawSecurityTestSuite) TestARejectionWithoutItsUserStillApplies() {
	withdraw := &model.CommissionWithdraw{UserID: 999, Amount: 40}
	s.Require().NoError(s.db.Create(withdraw).Error)
	code, body := s.process(withdraw.ID, `{"approve":false}`)
	s.Require().Equal(http.StatusOK, code, "%v", body)
	s.Equal("rejected", body["data"].(map[string]any)["status"])
	s.Equal(int64(100), s.balance())
}

func (s *WithdrawSecurityTestSuite) TestAnApprovalLeavesTheBalance() {
	withdraw := s.pending(40)
	code, body := s.process(withdraw.ID, `{"approved":true}`)
	s.Require().Equal(http.StatusOK, code, "%v", body)
	s.Equal("approved", body["data"].(map[string]any)["status"])
	s.Equal(int64(100), s.balance())
	s.Empty(s.ledger())
}

func TestWithdrawSecurity(t *testing.T) {
	suite.Run(t, new(WithdrawSecurityTestSuite))
}
