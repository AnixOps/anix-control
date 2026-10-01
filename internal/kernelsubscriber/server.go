// Package kernelsubscriber serves the KernelSubscriber contract
// (sdk/api/kernelsubscriber/v1, docs/architecture/subscriber-service.md) to
// official packages: shared subscriber state behind internal/subscriber and
// the kernel's user update path. Every call is authorized for its method
// family's capability against the calling host's current generation.
package kernelsubscriber

import (
	"context"
	"errors"
	"strings"
	"time"

	kernelsubscriberv1 "github.com/AnixOps/anix-control/sdk/api/kernelsubscriber/v1"
	"github.com/AnixOps/anix-control/v4/internal/authn"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/AnixOps/anix-control/v4/internal/subscriber"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

const (
	maxRequestID     = 128
	maxBatchEntries  = 5000
	maxLookupIDs     = 500
	defaultListLimit = 500
	maxListLimit     = 1000
	watchBatch       = 500
	watchPoll        = time.Second
	// watchReauthorize bounds how long a watch keeps streaming after its
	// package lost the capability or its generation was fenced.
	watchReauthorize = 30 * time.Second
)

// Authorizer admits a calling host to a method family.
type Authorizer interface {
	AuthorizeCapability(ctx context.Context, host packagebridge.HostIdentity, capability string) error
}

// Server holds what every host's KernelSubscriber calls share.
type Server struct {
	DB         *gorm.DB
	Authorizer Authorizer
	// Now defaults to time.Now.
	Now func() time.Time
}

// For returns the KernelSubscriber server that host reaches; it has the
// shape of packagebridge.KernelSubscriberProvider.
func (s *Server) For(host packagebridge.HostIdentity) kernelsubscriberv1.KernelSubscriberServer {
	return &hostServer{server: s, host: host}
}

type hostServer struct {
	kernelsubscriberv1.UnimplementedKernelSubscriberServer
	server *Server
	host   packagebridge.HostIdentity
}

func (h *hostServer) now() time.Time {
	if h.server.Now != nil {
		return h.server.Now()
	}
	return time.Now()
}

func (h *hostServer) begin(ctx context.Context, capability string) (*gorm.DB, error) {
	if h.server == nil || h.server.DB == nil || h.server.Authorizer == nil {
		return nil, status.Error(codes.Unavailable, "kernel subscriber is not configured")
	}
	err := h.server.Authorizer.AuthorizeCapability(ctx, h.host, capability)
	switch {
	case err == nil:
		return h.server.DB.WithContext(ctx), nil
	case errors.Is(err, packagebridge.ErrHostFenced):
		return nil, status.Error(codes.PermissionDenied, "package host generation is fenced")
	case errors.Is(err, service.ErrCapabilityNotAuthorized):
		return nil, status.Errorf(codes.PermissionDenied, "package is not authorized for %s", capability)
	default:
		return nil, status.Error(codes.Unavailable, "kernel subscriber authorization failed")
	}
}

func requestID(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > maxRequestID {
		return "", status.Errorf(codes.InvalidArgument, "request_id is required and at most %d bytes", maxRequestID)
	}
	return value, nil
}

func userID(value uint64) (uint, error) {
	if value == 0 || value > uint64(^uint32(0)) {
		return 0, status.Error(codes.InvalidArgument, "user_id is invalid")
	}
	return uint(value), nil
}

func failure(operation string, err error) error {
	switch {
	case errors.Is(err, subscriber.ErrSubscriberNotFound), errors.Is(err, service.ErrUserNotFound):
		return status.Error(codes.NotFound, "subscriber not found")
	case errors.Is(err, subscriber.ErrInsufficientBalance):
		return status.Error(codes.FailedPrecondition, "insufficient balance")
	case errors.Is(err, subscriber.ErrNegativeTraffic):
		return status.Error(codes.InvalidArgument, err.Error())
	case status.Code(err) != codes.Unknown:
		return err
	default:
		return status.Errorf(codes.Internal, "%s failed", operation)
	}
}

func (h *hostServer) ApplyEntitlement(ctx context.Context, request *kernelsubscriberv1.ApplyEntitlementRequest) (*kernelsubscriberv1.ApplyEntitlementResponse, error) {
	db, err := h.begin(ctx, service.CapabilitySubscriberEntitlements)
	if err != nil {
		return nil, err
	}
	id, err := requestID(request.GetRequestId())
	if err != nil {
		return nil, err
	}
	user, err := userID(request.GetUserId())
	if err != nil {
		return nil, err
	}
	plan := request.GetPlan()
	if plan == nil || plan.GetPlanId() == 0 {
		return nil, status.Error(codes.InvalidArgument, "plan with plan_id is required")
	}
	entitlement := subscriber.Entitlement{
		RequestID: id, UserID: user, RenewSamePlan: request.GetRenewSamePlan(), ResetTraffic: request.GetResetTraffic(),
		KeepSubscriptionGroups: request.GetKeepSubscriptionGroups(),
		Plan: subscriber.PlanSnapshot{
			PlanID: uint(plan.GetPlanId()), GroupID: uint(plan.GetGroupId()), // #nosec G115 -- ids are bounded by the v2 schema.
			TransferBytes: int64(min(plan.GetTransferBytes(), uint64(1<<62))), // #nosec G115 -- clamped.
		},
	}
	for _, group := range plan.GetSubscriptionGroupIds() {
		entitlement.Plan.SubscriptionGroupIDs = append(entitlement.Plan.SubscriptionGroupIDs, uint(group)) // #nosec G115 -- ids are bounded by the v2 schema.
	}
	if plan.SpeedLimitMbps != nil {
		speed := plan.GetSpeedLimitMbps()
		entitlement.Plan.SpeedLimit = &speed
	}
	if plan.DeviceLimit != nil {
		devices := int(plan.GetDeviceLimit())
		entitlement.Plan.DeviceLimit = &devices
	}
	switch expiry := request.GetExpiry().(type) {
	case *kernelsubscriberv1.ApplyEntitlementRequest_Period:
		if expiry.Period.GetMonths() < 0 || expiry.Period.GetDays() < 0 {
			return nil, status.Error(codes.InvalidArgument, "period must not be negative")
		}
		entitlement.Period = &subscriber.Period{Months: int(expiry.Period.GetMonths()), Days: int(expiry.Period.GetDays())}
	case *kernelsubscriberv1.ApplyEntitlementRequest_ExpiresAtUnix:
		expires := expiry.ExpiresAtUnix
		entitlement.ExpiresAt = &expires
	}
	var result subscriber.EntitlementResult
	err = db.Transaction(func(tx *gorm.DB) error {
		var err error
		result, err = subscriber.ApplyEntitlementTx(tx, entitlement, h.now())
		return err
	})
	if err != nil {
		return nil, failure("apply entitlement", err)
	}
	response := &kernelsubscriberv1.ApplyEntitlementResponse{Applied: result.Applied}
	if result.ExpiresAt != nil {
		response.ExpiresAtUnix = *result.ExpiresAt
	}
	return response, nil
}

// updateOnce applies v2_user updates through the kernel's user update path
// (revocations, change log) once per request id.
func (h *hostServer) updateOnce(db *gorm.DB, id, method string, user uint, updates map[string]any) (bool, error) {
	applied := false
	err := db.Transaction(func(tx *gorm.DB) error {
		var previous struct{}
		seen, err := subscriber.Replay(tx, id, &previous)
		if err != nil || seen {
			return err
		}
		revocation, err := service.UpdateUserTx(tx, user, updates)
		if err != nil {
			return err
		}
		if revocation != nil {
			defer authn.Remember(*revocation)
		}
		applied = true
		return subscriber.Record(tx, id, method, user, struct{}{}, h.now())
	})
	return applied, err
}

func (h *hostServer) AdjustEntitlement(ctx context.Context, request *kernelsubscriberv1.AdjustEntitlementRequest) (*kernelsubscriberv1.AdjustEntitlementResponse, error) {
	db, err := h.begin(ctx, service.CapabilitySubscriberEntitlements)
	if err != nil {
		return nil, err
	}
	id, err := requestID(request.GetRequestId())
	if err != nil {
		return nil, err
	}
	user, err := userID(request.GetUserId())
	if err != nil {
		return nil, err
	}
	updates := map[string]any{}
	if request.PlanId != nil {
		updates["plan_id"] = request.GetPlanId()
	}
	if request.GroupId != nil {
		updates["group_id"] = request.GetGroupId()
	}
	if request.GetClearExpiry() {
		updates["expired_at"] = nil
	} else if request.ExpiresAtUnix != nil {
		updates["expired_at"] = request.GetExpiresAtUnix()
	}
	if request.TransferBytes != nil {
		updates["transfer_enable"] = request.GetTransferBytes()
	}
	if request.SpeedLimitMbps != nil {
		updates["speed_limit"] = request.GetSpeedLimitMbps()
	}
	if request.DeviceLimit != nil {
		updates["device_limit"] = request.GetDeviceLimit()
	}
	if request.FlowResetDay != nil {
		updates["flow_reset_time"] = request.GetFlowResetDay()
	}
	if request.Remark != nil {
		updates["remark_content"] = request.GetRemark()
	}
	if len(updates) == 0 {
		return nil, status.Error(codes.InvalidArgument, "no entitlement field to change")
	}
	applied, err := h.updateOnce(db, id, "adjust_entitlement", user, updates)
	if err != nil {
		return nil, failure("adjust entitlement", err)
	}
	return &kernelsubscriberv1.AdjustEntitlementResponse{Applied: applied}, nil
}

func (h *hostServer) RecordTraffic(ctx context.Context, request *kernelsubscriberv1.RecordTrafficRequest) (*kernelsubscriberv1.RecordTrafficResponse, error) {
	db, err := h.begin(ctx, service.CapabilitySubscriberTraffic)
	if err != nil {
		return nil, err
	}
	batch, err := requestID(request.GetBatchId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "batch_id is required and at most 128 bytes")
	}
	if len(request.GetEntries()) > maxBatchEntries {
		return nil, status.Errorf(codes.InvalidArgument, "a batch holds at most %d entries", maxBatchEntries)
	}
	rate := request.GetRate()
	if rate == 0 {
		rate = 1
	}
	if rate < 0 || rate > 1000 {
		return nil, status.Error(codes.InvalidArgument, "rate must be between 0 and 1000")
	}
	entries := make([]subscriber.TrafficEntry, 0, len(request.GetEntries()))
	for _, entry := range request.GetEntries() {
		user, err := userID(entry.GetUserId())
		if err != nil {
			return nil, err
		}
		upload, download := scaled(entry.GetUploadBytes(), rate), scaled(entry.GetDownloadBytes(), rate)
		entries = append(entries, subscriber.TrafficEntry{UserID: user, Upload: upload, Download: download})
	}
	var result subscriber.TrafficResult
	err = db.Transaction(func(tx *gorm.DB) error {
		var err error
		result, err = subscriber.RecordTrafficTx(tx, batch, entries, h.now())
		return err
	})
	if err != nil {
		return nil, failure("record traffic", err)
	}
	response := &kernelsubscriberv1.RecordTrafficResponse{Applied: result.Applied}
	for _, id := range result.Exhausted {
		response.ExhaustedUserIds = append(response.ExhaustedUserIds, uint64(id))
	}
	return response, nil
}

// scaled multiplies bytes by rate, capped far below int64 overflow.
func scaled(bytes uint64, rate float64) int64 {
	const ceiling = float64(1 << 60)
	value := float64(bytes) * rate
	if value > ceiling {
		value = ceiling
	}
	return int64(value)
}

func (h *hostServer) ResetTraffic(ctx context.Context, request *kernelsubscriberv1.ResetTrafficRequest) (*kernelsubscriberv1.ResetTrafficResponse, error) {
	db, err := h.begin(ctx, service.CapabilitySubscriberTraffic)
	if err != nil {
		return nil, err
	}
	id, err := requestID(request.GetRequestId())
	if err != nil {
		return nil, err
	}
	if len(request.GetUserIds()) > maxBatchEntries {
		return nil, status.Errorf(codes.InvalidArgument, "at most %d users per reset", maxBatchEntries)
	}
	users := make([]uint, 0, len(request.GetUserIds()))
	for _, raw := range request.GetUserIds() {
		user, err := userID(raw)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	var result subscriber.ResetResult
	err = db.Transaction(func(tx *gorm.DB) error {
		var err error
		result, err = subscriber.ResetTrafficTx(tx, id, users, h.now())
		return err
	})
	if err != nil {
		return nil, failure("reset traffic", err)
	}
	return &kernelsubscriberv1.ResetTrafficResponse{Applied: result.Applied, ResetUsers: uint64(max(result.Reset, 0))}, nil
}

func (h *hostServer) ResetCredentials(ctx context.Context, request *kernelsubscriberv1.ResetCredentialsRequest) (*kernelsubscriberv1.ResetCredentialsResponse, error) {
	db, err := h.begin(ctx, service.CapabilitySubscriberCredentials)
	if err != nil {
		return nil, err
	}
	id, err := requestID(request.GetRequestId())
	if err != nil {
		return nil, err
	}
	user, err := userID(request.GetUserId())
	if err != nil {
		return nil, err
	}
	updates := map[string]any{}
	if request.GetSubscriptionToken() {
		updates["token"] = uuid.NewString()
	}
	if request.GetProxyUuid() {
		updates["uuid"] = uuid.NewString()
	}
	if len(updates) == 0 {
		return nil, status.Error(codes.InvalidArgument, "choose subscription_token and/or proxy_uuid")
	}
	applied, err := h.updateOnce(db, id, "reset_credentials", user, updates)
	if err != nil {
		return nil, failure("reset credentials", err)
	}
	return &kernelsubscriberv1.ResetCredentialsResponse{Applied: applied}, nil
}

func (h *hostServer) AdjustBalance(ctx context.Context, request *kernelsubscriberv1.AdjustBalanceRequest) (*kernelsubscriberv1.AdjustBalanceResponse, error) {
	db, err := h.begin(ctx, service.CapabilitySubscriberBalance)
	if err != nil {
		return nil, err
	}
	id, err := requestID(request.GetRequestId())
	if err != nil {
		return nil, err
	}
	user, err := userID(request.GetUserId())
	if err != nil {
		return nil, err
	}
	kind := subscriber.BalanceAccount
	switch request.GetKind() {
	case kernelsubscriberv1.BalanceKind_BALANCE_KIND_ACCOUNT:
	case kernelsubscriberv1.BalanceKind_BALANCE_KIND_COMMISSION:
		kind = subscriber.BalanceCommission
	default:
		return nil, status.Error(codes.InvalidArgument, "kind is required")
	}
	var result subscriber.BalanceResult
	err = db.Transaction(func(tx *gorm.DB) error {
		var err error
		result, err = subscriber.AdjustBalanceTx(tx, id, user, kind, request.GetAmountCents(), h.now())
		return err
	})
	if err != nil {
		return nil, failure("adjust balance", err)
	}
	return &kernelsubscriberv1.AdjustBalanceResponse{Applied: result.Applied, BalanceCents: result.Balance}, nil
}
