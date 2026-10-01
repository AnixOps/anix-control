package native

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	kernelsubscriberv1 "github.com/AnixOps/anix-control/sdk/api/kernelsubscriber/v1"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/gin-gonic/gin/binding"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// bytesPerGiB converts a plan's transfer (GiB) to bytes.
const bytesPerGiB = 1 << 30

// maxTransferBytes is the kernel's ceiling for a snapshot's transfer.
const maxTransferBytes = 1 << 62

// AssignRequestID names an administrator's plan assignment in the kernel's
// subscriber request ledger, so a retried request is applied once. token
// identifies the HTTP request: its Idempotency-Key, else its X-Request-ID,
// else a fresh random value. A new request is a new grant, as in v2: an
// administrator who assigns the same plan again resets the traffic again.
// The kernel's legacy handler derives the same id
// (service.PlanAssignmentRequestID), so a retry is recognized whichever
// side serves it.
func AssignRequestID(planID uint64, userID uint, expireAt *int64, token string) string {
	expiry := "-"
	if expireAt != nil {
		expiry = strconv.FormatInt(*expireAt, 10)
	}
	sum := sha256.Sum256([]byte(token + "\x00" + expiry))
	return fmt.Sprintf("plan.assign:%d:%d:%x", planID, userID, sum[:12])
}

// requestToken is what identifies the request for AssignRequestID.
func (s *Service) requestToken(request pluginhostsdk.NativeRequest) string {
	for _, name := range []string{"Idempotency-Key", "X-Request-Id"} {
		if value := header(request, name); value != "" {
			return value
		}
	}
	if s.NewToken != nil {
		return s.NewToken()
	}
	return uuid.NewString()
}

// header is the request's first value of a forwarded header, trimmed.
func header(request pluginhostsdk.NativeRequest, name string) string {
	for key, values := range request.Metadata.Headers {
		if strings.EqualFold(key, name) && len(values) > 0 {
			return strings.TrimSpace(values[0])
		}
	}
	return ""
}

// snapshot is what an assignment grants, as the kernel's
// subscriber.PlanSnapshotFromPlan builds it without subscription groups
// (an assignment keeps the subscriber's).
//
// The contract carries the transfer as unsigned bytes: a plan with a
// negative transfer grants none, where v2 stored the negative value; both
// serve nothing. A device limit beyond int32 is capped.
func snapshot(plan Plan) *kernelsubscriberv1.PlanSnapshot {
	out := &kernelsubscriberv1.PlanSnapshot{PlanId: uint64(plan.ID), GroupId: uint64(plan.GroupID)}
	switch {
	case plan.TransferEnable <= 0:
	case plan.TransferEnable >= maxTransferBytes/bytesPerGiB:
		out.TransferBytes = maxTransferBytes
	default:
		out.TransferBytes = uint64(plan.TransferEnable) * bytesPerGiB
	}
	if plan.SpeedLimit != nil {
		speed := *plan.SpeedLimit
		out.SpeedLimitMbps = &speed
	}
	if plan.DeviceLimit != nil {
		devices := int32(max(min(*plan.DeviceLimit, math.MaxInt32), math.MinInt32)) // #nosec G115 -- clamped.
		out.DeviceLimit = &devices
	}
	return out
}

// AdminAssign is POST /api/v2/admin/plans/:id/assign: the plan's group,
// traffic and limits for the subscriber, the expiry when given, and the
// traffic counters reset; the subscriber's subscription groups are kept.
// The kernel applies it through KernelSubscriber.ApplyEntitlement.
func (s *Service) AdminAssign(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	planID, err := strconv.ParseUint(request.Metadata.PathParams["id"], 10, 32)
	if err != nil {
		return s.panelError("套餐ID 无效")
	}
	var req struct {
		UserID   uint   `json:"user_id" binding:"required,gt=0"`
		ExpireAt *int64 `json:"expire_at"`
	}
	if err := binding.JSON.BindBody(request.Body, &req); err != nil {
		return s.panelError("参数错误: " + err.Error())
	}
	if planID == 0 {
		return s.panelError(planNotFound)
	}
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError("分配失败: " + err.Error())
	}
	var plan Plan
	if err := db.First(&plan, planID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return s.panelError(planNotFound)
		}
		return s.panelError("分配失败: " + err.Error())
	}
	// Subscriber ids fit the contract's range; a larger id names no one.
	if uint64(req.UserID) > math.MaxUint32 {
		return s.panelError(planUserNotFound)
	}
	grant := &kernelsubscriberv1.ApplyEntitlementRequest{
		RequestId: AssignRequestID(planID, req.UserID, req.ExpireAt, s.requestToken(request)),
		UserId:    uint64(req.UserID), Plan: snapshot(plan),
		ResetTraffic: true, KeepSubscriptionGroups: true, Reason: "admin plan assignment",
	}
	if req.ExpireAt != nil {
		grant.Expiry = &kernelsubscriberv1.ApplyEntitlementRequest_ExpiresAtUnix{ExpiresAtUnix: *req.ExpireAt}
	}
	if _, err := s.Subscriber.ApplyEntitlement(ctx, grant); err != nil {
		if status.Code(err) == codes.NotFound {
			return s.panelError(planUserNotFound)
		}
		return s.panelError("分配失败: " + status.Convert(err).Message())
	}
	// The kernel wrote this event in the assignment's transaction; here it
	// follows the grant. A retry of a failed write replays the grant and
	// writes it then.
	payload := map[string]any{"plan_id": planID, "user_id": req.UserID}
	if req.ExpireAt != nil {
		payload["expired_at"] = *req.ExpireAt
	}
	if err := db.Create(s.event("plan.assigned", payload)).Error; err != nil {
		return s.panelError("分配失败: " + err.Error())
	}
	return s.panel(map[string]any{"message": "分配成功"})
}
