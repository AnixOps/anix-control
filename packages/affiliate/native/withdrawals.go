package native

import (
	"context"
	"fmt"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"

	kernelsubscriberv1 "github.com/AnixOps/anix-control/sdk/api/kernelsubscriber/v1"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/gin-gonic/gin/binding"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// DebitRequestID names a withdrawal's debit of the commission balance in
// the kernel's subscriber request ledger, as the kernel's legacy handler
// does (service.WithdrawDebitRequestID).
func DebitRequestID(withdrawalID uint) string {
	return fmt.Sprintf("affiliate.withdraw:%d", withdrawalID)
}

// RefundRequestID names a rejected withdrawal's refund, as the kernel's
// legacy handler does (service.WithdrawRefundRequestID).
func RefundRequestID(withdrawalID uint) string {
	return fmt.Sprintf("affiliate.withdraw.refund:%d", withdrawalID)
}

// maxWithdrawCents bounds an amount to integers a float64 holds exactly.
const maxWithdrawCents = 1 << 53

// RefundCents is what rejecting a withdrawal returns to the commission
// balance, as the kernel's service.WithdrawRefundCents: a stored fraction is
// refunded its whole part, which is what its debit took.
func RefundCents(amount float64) (int64, bool) {
	whole := math.Trunc(amount)
	if math.IsNaN(whole) || whole < 0 || whole > maxWithdrawCents {
		return 0, false
	}
	return int64(whole), true
}

// Answers of the kernel's withdrawal routes.
const (
	errAmountNotWhole       = "amount must be a whole number"
	errBelowMinimum         = "amount below minimum"
	errInsufficientBalance  = "insufficient balance"
	errSubscriberNotFound   = "subscriber not found"
	errWithdrawalNotFound   = "withdrawal not found"
	errWithdrawalProcessed  = "withdrawal already processed"
	errProcessingFailed     = "failed to process withdrawal"
	errStatusMustBeDecision = "status must be approved/rejected or 1/2"
)

// subscriberID is a user id as the contract carries it; a user id outside
// its range names no subscriber.
func subscriberID(userID uint) (uint64, bool) {
	if userID == 0 || uint64(userID) > math.MaxUint32 {
		return 0, false
	}
	return uint64(userID), true
}

// refused reports whether the kernel refused a call without applying it.
// Any other failure (unavailable, deadline, internal) leaves the outcome
// unknown.
func refused(err error) bool {
	switch status.Code(err) {
	case codes.FailedPrecondition, codes.NotFound, codes.InvalidArgument, codes.PermissionDenied, codes.Unimplemented:
		return true
	}
	return false
}

func withdrawalStatusText(code int) string {
	switch code {
	case withdrawApproved:
		return "approved"
	case withdrawRejected:
		return "rejected"
	default:
		return "pending"
	}
}

func withdrawalStatusFromText(text string) (int, bool) {
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "pending":
		return withdrawPending, true
	case "approved":
		return withdrawApproved, true
	case "rejected":
		return withdrawRejected, true
	default:
		return 0, false
	}
}

// withdrawalResponse is the kernel's inviteWithdrawalResponse.
func withdrawalResponse(w Withdrawal) map[string]any {
	return map[string]any{
		"id":           w.ID,
		"user_id":      w.UserID,
		"amount":       w.Amount,
		"method":       w.Method,
		"account":      w.Account,
		"name":         w.Name,
		"remark":       w.Remark,
		"status":       withdrawalStatusText(w.Status),
		"status_code":  w.Status,
		"processed_at": w.ProcessedAt,
		"created_at":   w.CreatedAt,
		"updated_at":   w.UpdatedAt,
	}
}

// UserCommissions is GET /api/v2/user/invite/commissions: the caller's
// commission records, newest first.
func (s *Service) UserCommissions(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	page, _ := strconv.Atoi(defaultQuery(request, "page", "1"))
	pageSize, _ := strconv.Atoi(defaultQuery(request, "page_size", "20"))
	db, err := s.Open(ctx)
	if err != nil {
		return errorAnswer(http.StatusInternalServerError, err.Error())
	}
	var records []Commission
	var total int64
	// The kernel's query, the count's error ignored as there.
	list := db.Model(&Commission{}).Where("user_id = ?", request.Principal.ActorID)
	list.Count(&total)
	if err := list.Order("created_at DESC").Limit(pageSize).Offset((page - 1) * pageSize).Find(&records).Error; err != nil {
		return errorAnswer(http.StatusInternalServerError, err.Error())
	}
	return s.panel(map[string]any{"list": records, "total": total, "page": page, "page_size": pageSize})
}

// UserWithdrawals is GET /api/v2/user/invite/withdrawals: the caller's
// withdrawals, newest first, as stored.
func (s *Service) UserWithdrawals(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	page, _ := strconv.Atoi(defaultQuery(request, "page", "1"))
	pageSize, _ := strconv.Atoi(defaultQuery(request, "page_size", "20"))
	db, err := s.Open(ctx)
	if err != nil {
		return errorAnswer(http.StatusInternalServerError, err.Error())
	}
	var records []Withdrawal
	var total int64
	list := db.Model(&Withdrawal{}).Where("user_id = ?", request.Principal.ActorID)
	list.Count(&total)
	if err := list.Order("created_at DESC").Limit(pageSize).Offset((page - 1) * pageSize).Find(&records).Error; err != nil {
		return errorAnswer(http.StatusInternalServerError, err.Error())
	}
	return s.panel(map[string]any{"list": records, "total": total, "page": page, "page_size": pageSize})
}

// AdminWithdrawals is GET /api/v2/admin/invite/withdrawals: every
// withdrawal, newest first, optionally of one status (a number, or
// pending, approved or rejected); pages of 1 to 100.
func (s *Service) AdminWithdrawals(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	statusFilter := query(request, "status")
	page, _ := strconv.Atoi(defaultQuery(request, "page", "1"))
	pageSize, _ := strconv.Atoi(defaultQuery(request, "page_size", "20"))
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	db, err := s.Open(ctx)
	if err != nil {
		return errorAnswer(http.StatusInternalServerError, "failed to count withdrawals")
	}
	list := db.Model(&Withdrawal{})
	if statusFilter != "" {
		if numeric, err := strconv.Atoi(statusFilter); err == nil {
			list = list.Where("status = ?", numeric)
		} else if mapped, ok := withdrawalStatusFromText(statusFilter); ok {
			list = list.Where("status = ?", mapped)
		}
	}
	var total int64
	if err := list.Count(&total).Error; err != nil {
		return errorAnswer(http.StatusInternalServerError, "failed to count withdrawals")
	}
	var records []Withdrawal
	if err := list.Order("created_at DESC").Limit(pageSize).Offset((page - 1) * pageSize).Find(&records).Error; err != nil {
		return errorAnswer(http.StatusInternalServerError, "failed to load withdrawals")
	}
	answers := make([]map[string]any, 0, len(records))
	for _, record := range records {
		answers = append(answers, withdrawalResponse(record))
	}
	return s.panel(map[string]any{"list": answers, "total": total, "page": page, "page_size": pageSize})
}

// UserWithdraw is POST /api/v2/user/invite/withdraw: the caller withdraws a
// whole amount of at least the configured minimum from their commission
// balance.
//
// The withdrawal is recorded as a reservation, the balance is debited
// through KernelSubscriber.AdjustBalance (refused below zero under the
// subscriber's row lock), and the reservation then becomes pending. A
// refused debit removes the reservation; a debit whose outcome is unknown
// leaves it, never approvable, for an administrator to reconcile with the
// ledger.
func (s *Service) UserWithdraw(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	var req struct {
		Amount  float64 `json:"amount" binding:"required,min=1"`
		Method  string  `json:"method" binding:"required,oneof=alipay wechat bank"`
		Account string  `json:"account" binding:"required"`
		Name    string  `json:"name" binding:"required"`
	}
	if err := binding.JSON.BindBody(request.Body, &req); err != nil {
		return badRequest(err.Error())
	}
	if req.Amount != math.Trunc(req.Amount) {
		return badRequest(errAmountNotWhole)
	}
	db, err := s.Open(ctx)
	if err != nil {
		return badRequest(err.Error())
	}
	userID := request.Principal.ActorID
	// The kernel reads the configuration for each withdrawal and skips the
	// minimum when it cannot.
	if cfg, err := loadConfig(db); err == nil && req.Amount < cfg.CommissionMinAmount {
		return badRequest(errBelowMinimum)
	}
	var balance float64
	db.Model(&Entitlement{}).Where("id = ?", userID).Select("commission_balance").Scan(&balance)
	if req.Amount > balance || req.Amount > maxWithdrawCents {
		return badRequest(errInsufficientBalance)
	}
	user, ok := subscriberID(userID)
	if !ok {
		return badRequest(errSubscriberNotFound)
	}

	withdrawal := Withdrawal{
		UserID: userID, Amount: req.Amount, Method: req.Method, Account: req.Account, Name: req.Name, Status: withdrawReserving,
	}
	if err := db.Create(&withdrawal).Error; err != nil {
		return badRequest(err.Error())
	}
	debit, err := s.Subscriber.AdjustBalance(ctx, &kernelsubscriberv1.AdjustBalanceRequest{
		RequestId: DebitRequestID(withdrawal.ID), UserId: user, Kind: kernelsubscriberv1.BalanceKind_BALANCE_KIND_COMMISSION,
		AmountCents: -int64(req.Amount), Reason: "commission withdrawal",
	})
	switch {
	case err != nil && refused(err):
		s.release(db, withdrawal.ID)
		return badRequest(status.Convert(err).Message())
	case err != nil:
		log.Printf("affiliate: withdrawal %d: debit outcome unknown, reservation kept (ledger id %s): %v",
			withdrawal.ID, DebitRequestID(withdrawal.ID), err)
		return badRequest(status.Convert(err).Message())
	case !debit.GetApplied():
		// The ledger already holds this id for a withdrawal that no longer
		// exists; this reservation was not debited.
		s.release(db, withdrawal.ID)
		return badRequest(fmt.Sprintf("withdrawal %d was debited before", withdrawal.ID))
	}
	// The row ends as the kernel writes it: pending, created and updated at
	// its creation.
	confirmed := db.Model(&Withdrawal{}).Where("id = ? AND status = ?", withdrawal.ID, withdrawReserving).
		UpdateColumn("status", withdrawPending)
	if confirmed.Error != nil || confirmed.RowsAffected != 1 {
		log.Printf("affiliate: withdrawal %d: debited but still reserved (ledger id %s): %v",
			withdrawal.ID, DebitRequestID(withdrawal.ID), confirmed.Error)
		return badRequest(errProcessingFailed)
	}
	withdrawal.Status = withdrawPending
	return s.panel(withdrawal)
}

// release removes a reservation the kernel did not debit.
func (s *Service) release(db *gorm.DB, id uint) {
	if err := db.Where("id = ? AND status = ?", id, withdrawReserving).Delete(&Withdrawal{}).Error; err != nil {
		log.Printf("affiliate: withdrawal %d: reservation not debited and not removed: %v", id, err)
	}
}

// decision parses an administrator's decision as the kernel does: approve
// or approved (booleans), else status (1, 2, "1", "2", "approved",
// "rejected"). Anything else is 0.
func decision(req map[string]any) (int, string) {
	for _, key := range []string{"approve", "approved"} {
		raw, ok := req[key]
		if !ok {
			continue
		}
		approve, valid := boolFromAny(raw)
		if !valid {
			return 0, key + " must be boolean"
		}
		if approve {
			return withdrawApproved, ""
		}
		return withdrawRejected, ""
	}
	if raw, ok := req["status"]; ok {
		switch v := raw.(type) {
		case float64:
			return int(v), ""
		case string:
			if parsed, err := strconv.Atoi(v); err == nil {
				return parsed, ""
			}
			if mapped, ok := withdrawalStatusFromText(v); ok {
				return mapped, ""
			}
		}
	}
	return 0, ""
}

// boolFromAny is the kernel's inviteBoolFromAny.
func boolFromAny(v any) (bool, bool) {
	switch raw := v.(type) {
	case bool:
		return raw, true
	case float64:
		if raw == 1 {
			return true, true
		}
		if raw == 0 {
			return false, true
		}
	case string:
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case "1", "true", "yes", "y", "on":
			return true, true
		case "0", "false", "no", "n", "off":
			return false, true
		}
	}
	return false, false
}

// AdminProcessWithdrawal is POST /api/v2/admin/invite/withdrawals/:id/process:
// an administrator approves or rejects a pending withdrawal; a rejection
// returns the amount to the commission balance.
//
// The decision claims the withdrawal while it is pending, so of two
// concurrent decisions one is "already processed". A rejection's refund
// then goes through KernelSubscriber.AdjustBalance, once per withdrawal. If
// the kernel refuses it, the claim is undone and the withdrawal is pending
// again; if its outcome is unknown, the withdrawal stays rejected and the
// answer is a failure, for an administrator to reconcile with the ledger.
// A withdrawal whose user does not exist has no one to refund and is
// rejected all the same, as in the kernel.
func (s *Service) AdminProcessWithdrawal(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	var req map[string]any
	if err := binding.JSON.BindBody(request.Body, &req); err != nil {
		return badRequest(err.Error())
	}
	decided, invalid := decision(req)
	if invalid != "" {
		return badRequest(invalid)
	}
	if decided != withdrawApproved && decided != withdrawRejected {
		return badRequest(errStatusMustBeDecision)
	}
	remark, _ := req["remark"].(string)

	id, err := strconv.ParseUint(request.Metadata.PathParams["id"], 10, 32)
	if err != nil {
		return errorAnswer(http.StatusNotFound, errWithdrawalNotFound)
	}
	db, err := s.Open(ctx)
	if err != nil {
		return errorAnswer(http.StatusNotFound, errWithdrawalNotFound)
	}
	var withdrawal Withdrawal
	if err := db.First(&withdrawal, uint(id)).Error; err != nil {
		return errorAnswer(http.StatusNotFound, errWithdrawalNotFound)
	}
	if withdrawal.Status != withdrawPending {
		return badRequest(errWithdrawalProcessed)
	}
	var refund int64
	if decided == withdrawRejected {
		cents, ok := RefundCents(withdrawal.Amount)
		if !ok {
			log.Printf("affiliate: withdrawal %d: amount %v cannot be refunded", withdrawal.ID, withdrawal.Amount)
			return errorAnswer(http.StatusInternalServerError, errProcessingFailed)
		}
		refund = cents
	}

	now := s.now()
	claimed := db.Model(&Withdrawal{}).Where("id = ? AND status = ?", withdrawal.ID, withdrawPending).
		Updates(map[string]any{"status": decided, "remark": remark, "processed_at": now, "updated_at": now})
	if claimed.Error != nil {
		return errorAnswer(http.StatusInternalServerError, errProcessingFailed)
	}
	if claimed.RowsAffected == 0 {
		return badRequest(errWithdrawalProcessed)
	}
	if decided == withdrawRejected {
		if err := s.refund(ctx, withdrawal, refund); err != nil {
			if refused(err) {
				s.unclaim(db, withdrawal)
			} else {
				log.Printf("affiliate: withdrawal %d: rejected, refund outcome unknown (ledger id %s): %v",
					withdrawal.ID, RefundRequestID(withdrawal.ID), err)
			}
			return errorAnswer(http.StatusInternalServerError, errProcessingFailed)
		}
	}
	withdrawal.Status = decided
	withdrawal.Remark = remark
	withdrawal.ProcessedAt = &now
	withdrawal.UpdatedAt = now
	return s.panel(withdrawalResponse(withdrawal))
}

// refund returns a rejected withdrawal's amount to its user's commission
// balance. A user who does not exist has nothing to refund.
func (s *Service) refund(ctx context.Context, withdrawal Withdrawal, cents int64) error {
	user, ok := subscriberID(withdrawal.UserID)
	if !ok {
		return nil
	}
	_, err := s.Subscriber.AdjustBalance(ctx, &kernelsubscriberv1.AdjustBalanceRequest{
		RequestId: RefundRequestID(withdrawal.ID), UserId: user, Kind: kernelsubscriberv1.BalanceKind_BALANCE_KIND_COMMISSION,
		AmountCents: cents, Reason: "commission withdrawal rejected",
	})
	if status.Code(err) == codes.NotFound {
		return nil
	}
	return err
}

// unclaim puts a rejection the kernel refused to refund back to pending.
func (s *Service) unclaim(db *gorm.DB, withdrawal Withdrawal) {
	err := db.Model(&Withdrawal{}).Where("id = ? AND status = ?", withdrawal.ID, withdrawRejected).
		Updates(map[string]any{
			"status": withdrawPending, "remark": withdrawal.Remark, "processed_at": withdrawal.ProcessedAt, "updated_at": withdrawal.UpdatedAt,
		}).Error
	if err != nil {
		log.Printf("affiliate: withdrawal %d: refund refused and rejection not undone: %v", withdrawal.ID, err)
	}
}
