package kernelsubscriber

import (
	"context"
	"errors"
	"time"

	kernelsubscriberv1 "github.com/AnixOps/anix-control/sdk/api/kernelsubscriber/v1"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// GetSubscriptionSummary answers one subscriber's subscription summary
// (kernel.subscriber.summary.v1) through
// service.StatsService.UserSubscriptionSummary, the function and cache the
// kernel's legacy handler uses (docs/architecture/kernel-caches.md).
func (h *hostServer) GetSubscriptionSummary(ctx context.Context, request *kernelsubscriberv1.GetSubscriptionSummaryRequest) (*kernelsubscriberv1.GetSubscriptionSummaryResponse, error) {
	db, err := h.begin(ctx, service.CapabilitySubscriberSummary)
	if err != nil {
		return nil, err
	}
	user, err := userID(request.GetUserId())
	if err != nil {
		return nil, err
	}
	summary, err := service.NewStatsServiceOn(db).UserSubscriptionSummary(user, request.GetRefresh(), service.NewSystemConfigService(db), h.config())
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return nil, status.Error(codes.NotFound, "subscriber not found")
	case err != nil:
		return nil, status.Error(codes.Internal, "subscription summary failed")
	}
	response := &kernelsubscriberv1.GetSubscriptionSummaryResponse{
		UserId: uint64(summary.UserID), Email: summary.Email, PlanName: summary.PlanName,
		TransferEnable: summary.TransferEnable, UsedTraffic: summary.UsedTraffic,
		UploadTraffic: summary.UploadTraffic, DownloadTraffic: summary.DownloadTraffic,
		ExpiredAt: summary.ExpiredAt, IsExpired: summary.IsExpired, DaysRemaining: int64(summary.DaysRemaining),
		UsagePercent: summary.UsagePercent, SubscribePath: summary.SubscribePath, SubscribeDomains: summary.SubscribeDomains,
		CachedAt: summary.CachedAt.Format(time.RFC3339Nano),
	}
	if summary.PlanID != nil {
		plan := uint64(*summary.PlanID)
		response.PlanId = &plan
	}
	return response, nil
}

// config is the kernel's configuration the subscription link settings
// read: Server.Config, else the current one.
func (h *hostServer) config() *config.Config {
	if h.server.Config != nil {
		return h.server.Config()
	}
	return config.Get()
}
