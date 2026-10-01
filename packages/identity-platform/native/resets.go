package native

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"

	kernelidentityv1 "github.com/AnixOps/anix-control/sdk/api/kernelidentity/v1"
	kernelsubscriberv1 "github.com/AnixOps/anix-control/sdk/api/kernelsubscriber/v1"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// The administrator's resets of one user, for ResetRequestID.
const (
	resetTraffic   = "reset_traffic"
	resetSubscribe = "reset_subscribe"
)

// ResetRequestID names an administrator's traffic or subscription reset of
// a user in Control's subscriber request ledger, so a retried request is
// applied once. token identifies the HTTP request: its Idempotency-Key,
// else its request id. The kernel's legacy handlers derive the same id
// (service.AdminUserResetRequestID), so a retry is recognized whichever side
// serves it; a new request is a new reset, as in v2.
func ResetRequestID(kind string, userID uint64, token string) string {
	sum := sha256.Sum256([]byte(token))
	return fmt.Sprintf("identity.%s:%d:%x", kind, userID, sum[:12])
}

// requestToken is what identifies the request for ResetRequestID.
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

// ResetTraffic is POST /api/v2/admin/users/:id/reset-traffic: Control zeroes
// the subscriber's counters (KernelSubscriber.ResetTraffic).
func (s *Service) ResetTraffic(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	id, ok := parseUserID(request.Metadata.PathParams["id"])
	if !ok {
		return s.panelError("无效的用户ID")
	}
	if id == 0 {
		return s.panelError("用户不存在")
	}
	// ResetTraffic resets whichever of its users exist; v2 refuses an
	// unknown one first.
	if _, err := s.Kernel.GetSubscriber(ctx, &kernelidentityv1.GetSubscriberRequest{UserId: id}); err != nil {
		return s.resetFailed("重置失败", err)
	}
	_, err := s.Subscriber.ResetTraffic(ctx, &kernelsubscriberv1.ResetTrafficRequest{
		RequestId: ResetRequestID(resetTraffic, id, s.requestToken(request)), UserIds: []uint64{id},
		Reason: "administrator reset",
	})
	if err != nil {
		return s.resetFailed("重置失败", err)
	}
	return s.panel(map[string]any{"message": "流量重置成功"})
}

// ResetSubscribe is POST /api/v2/admin/users/:id/reset-subscribe: Control
// issues a new subscription token (KernelSubscriber.ResetCredentials; the
// proxy uuid is kept), and the answer carries the subscriber's token, read
// for that one user (KernelIdentity.GetSubscriber). A retried request
// changes nothing and answers the token the first one issued.
func (s *Service) ResetSubscribe(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	id, ok := parseUserID(request.Metadata.PathParams["id"])
	if !ok {
		return s.panelError("无效的用户ID")
	}
	if id == 0 {
		return s.panelError("用户不存在")
	}
	_, err := s.Subscriber.ResetCredentials(ctx, &kernelsubscriberv1.ResetCredentialsRequest{
		RequestId: ResetRequestID(resetSubscribe, id, s.requestToken(request)), UserId: id, SubscriptionToken: true,
	})
	if err != nil {
		return s.resetFailed("重置订阅失败", err)
	}
	_, shown, err := s.showSubscriber(ctx, id)
	if err != nil {
		return s.resetFailed("重置订阅失败", err)
	}
	return s.panel(map[string]any{"token": shown.Token})
}

// resetFailed answers a failed reset as v2 does: an unknown user by name,
// anything else under the route's failure message.
func (s *Service) resetFailed(failure string, err error) (pluginhostsdk.NativeResponse, error) {
	if status.Code(err) == codes.NotFound {
		return s.panelError("用户不存在")
	}
	return s.panelError(failure + ": " + status.Convert(err).Message())
}
