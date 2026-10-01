// Package kernelorder serves the KernelOrder contract
// (sdk/api/kernelorder/v1, docs/architecture/order-service.md) to official
// packages: the order writes that span domains, which the kernel performs
// in one transaction with the same code its legacy handlers use. Every call
// is authorized for its capability against the calling host's current
// generation.
package kernelorder

import (
	"context"
	"errors"
	"time"

	kernelorderv1 "github.com/AnixOps/anix-control/sdk/api/kernelorder/v1"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// Authorizer admits a calling host to a capability.
type Authorizer interface {
	AuthorizeCapability(ctx context.Context, host packagebridge.HostIdentity, capability string) error
}

// Server holds what every host's KernelOrder calls share.
type Server struct {
	DB         *gorm.DB
	Authorizer Authorizer
	// Now defaults to time.Now.
	Now func() time.Time
}

// For returns the KernelOrder server that host reaches; it has the shape of
// packagebridge.KernelOrderProvider.
func (s *Server) For(host packagebridge.HostIdentity) kernelorderv1.KernelOrderServer {
	return &hostServer{server: s, host: host}
}

type hostServer struct {
	kernelorderv1.UnimplementedKernelOrderServer
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
		return nil, status.Error(codes.Unavailable, "kernel order is not configured")
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
		return nil, status.Error(codes.Unavailable, "kernel order authorization failed")
	}
}

var outcomes = map[string]kernelorderv1.OrderPaymentOutcome{
	service.OrderPaymentCompleted: kernelorderv1.OrderPaymentOutcome_ORDER_PAYMENT_OUTCOME_COMPLETED,
	service.OrderPaymentPaid:      kernelorderv1.OrderPaymentOutcome_ORDER_PAYMENT_OUTCOME_PAID,
	service.OrderPaymentRefused:   kernelorderv1.OrderPaymentOutcome_ORDER_PAYMENT_OUTCOME_REFUSED,
}

// CompleteOrderPayment applies a paid payment record to its order through
// service.CompleteOrderPaymentTx, the function the kernel's own payment
// callbacks call in their transaction.
func (h *hostServer) CompleteOrderPayment(ctx context.Context, request *kernelorderv1.CompleteOrderPaymentRequest) (*kernelorderv1.CompleteOrderPaymentResponse, error) {
	db, err := h.begin(ctx, service.CapabilityOrderComplete)
	if err != nil {
		return nil, err
	}
	tradeNo := request.GetTradeNo()
	if tradeNo == "" || len(tradeNo) > service.MaxOrderPaymentTradeNo {
		return nil, status.Errorf(codes.InvalidArgument, "trade_no is required and at most %d bytes", service.MaxOrderPaymentTradeNo)
	}
	var result service.OrderPaymentResult
	err = service.WithRetryableTransaction(db, func(tx *gorm.DB) error {
		var err error
		result, err = service.CompleteOrderPaymentTx(tx, tradeNo, request.GetOrderId(), h.now())
		return err
	})
	switch {
	case err == nil:
	case errors.Is(err, service.ErrOrderPaymentRecordNotFound):
		return nil, status.Error(codes.NotFound, "payment record not found")
	case errors.Is(err, service.ErrOrderPaymentNotPaid), errors.Is(err, service.ErrOrderPaymentOtherOrder),
		errors.Is(err, service.ErrOrderPaymentRequestTaken):
		return nil, status.Error(codes.FailedPrecondition, err.Error())
	default:
		return nil, status.Error(codes.Internal, "complete order payment failed")
	}
	return &kernelorderv1.CompleteOrderPaymentResponse{
		Applied: result.Applied, OrderId: result.OrderID, Outcome: outcomes[result.Outcome], Reason: result.Reason,
	}, nil
}
