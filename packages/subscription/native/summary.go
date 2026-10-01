package native

import (
	"context"
	"time"

	kernelsubscriberv1 "github.com/AnixOps/anix-control/sdk/api/kernelsubscriber/v1"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// SummaryRouteID is GET /api/v2/user/subscription; it has a native handler
// only with a Subscriber.
const SummaryRouteID = "subscription.user.subscription.get"

// Summary is the user's subscription summary, with the fields and tags of
// the kernel's service.UserSubscription.
type Summary struct {
	UserID           uint64    `json:"user_id"`
	Email            string    `json:"email"`
	PlanID           *uint64   `json:"plan_id"`
	PlanName         string    `json:"plan_name"`
	TransferEnable   int64     `json:"transfer_enable"`
	UsedTraffic      int64     `json:"used_traffic"`
	UploadTraffic    int64     `json:"upload_traffic"`
	DownloadTraffic  int64     `json:"download_traffic"`
	ExpiredAt        int64     `json:"expired_at"`
	IsExpired        bool      `json:"is_expired"`
	DaysRemaining    int64     `json:"days_remaining"`
	UsagePercent     float64   `json:"usage_percent"`
	SubscribePath    string    `json:"subscribe_path,omitempty"`
	SubscribeDomains []string  `json:"subscribe_domains,omitempty"`
	CachedAt         time.Time `json:"cached_at"`
}

// Summary is GET /api/v2/user/subscription: the caller's summary the kernel
// caches for 30 seconds, read through KernelSubscriber.GetSubscriptionSummary
// (kernel.subscriber.summary.v1), with the subscription link settings the
// kernel adds on every call. The kernel keeps the one cache both modes read,
// so a native answer carries the summary and cached_at the legacy handler
// would, and refresh=true (only that exact value, as gin's c.Query)
// rebuilds it.
func (s *Service) Summary(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	refresh := false
	if values := request.Metadata.Query["refresh"]; len(values) > 0 {
		refresh = values[0] == "true"
	}
	summary, err := s.Subscriber.GetSubscriptionSummary(ctx, &kernelsubscriberv1.GetSubscriptionSummaryRequest{
		UserId: uint64(request.Principal.ActorID), Refresh: refresh,
	})
	switch status.Code(err) {
	case codes.OK:
	case codes.NotFound, codes.InvalidArgument:
		// The kernel's handler looks the caller up; an unknown or invalid
		// id is not found.
		return s.panelError("用户不存在")
	default:
		return s.panelError("获取订阅信息失败")
	}
	cachedAt, err := time.Parse(time.RFC3339Nano, summary.GetCachedAt())
	if err != nil {
		return s.panelError("获取订阅信息失败")
	}
	return s.panel(Summary{
		UserID: summary.GetUserId(), Email: summary.GetEmail(), PlanID: summary.PlanId, PlanName: summary.GetPlanName(),
		TransferEnable: summary.GetTransferEnable(), UsedTraffic: summary.GetUsedTraffic(),
		UploadTraffic: summary.GetUploadTraffic(), DownloadTraffic: summary.GetDownloadTraffic(),
		ExpiredAt: summary.GetExpiredAt(), IsExpired: summary.GetIsExpired(), DaysRemaining: summary.GetDaysRemaining(),
		UsagePercent: summary.GetUsagePercent(), SubscribePath: summary.GetSubscribePath(), SubscribeDomains: summary.GetSubscribeDomains(),
		CachedAt: cachedAt,
	})
}
