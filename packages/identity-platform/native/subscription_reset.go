package native

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/identity/throttle"
	kernelsubscriberv1 "github.com/AnixOps/anix-control/sdk/api/kernelsubscriber/v1"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/gin-gonic/gin/binding"
	"golang.org/x/crypto/bcrypt"
)

// UserSubscriptionResetRouteID is a user's reset of their own subscription
// link, POST /api/v2/user/subscription/reset.
const UserSubscriptionResetRouteID = "identity.user.subscription.reset.post"

// userResetSubscribe is the user's own subscription reset, for
// ResetRequestID: identity.user_reset_subscribe:<user>:<digest>, as the
// kernel's legacy handler derives it (service.UserSubscriptionResetRequestID).
const userResetSubscribe = "user_reset_subscribe"

// subscriptionResetLimit bounds a user's subscription resets as the kernel
// does (service.SubscriptionResetRateLimit): every attempt that checks a
// credential counts, and the third in an hour locks the user's resets for
// an hour.
var subscriptionResetLimit = throttle.Options{Enabled: true, MaxAttempts: 3, Window: time.Hour, Lockout: time.Hour}

// The answers of a subscription reset that does not reset, as the kernel
// answers them.
const (
	subscriptionResetLimited = "too many subscription reset attempts, please try again later"
	stepUpPasswordRequired   = "password required"
	stepUpInvalidPassword    = "invalid password"
	stepUpMFARequired        = "mfa code required"
	stepUpInvalidMFACode     = "invalid mfa code"
)

func subscriptionResetKey(userID uint64) string {
	return fmt.Sprintf("subscription_reset|%d", userID)
}

// subscriptionResetRequest binds as the kernel's request does.
type subscriptionResetRequest struct {
	Password string `json:"password"`
	Code     string `json:"code"`
	Method   string `json:"method"`
}

// ResetOwnSubscription is POST /api/v2/user/subscription/reset: the signed-in
// user re-authenticates (stepUp) and Control issues their subscriber a new
// subscription token, exactly as the administrator's reset does
// (KernelSubscriber.ResetCredentials; the proxy uuid is kept). The answer
// carries the new token, read for that one user. Attempts are limited per
// user in identity's throttle table.
func (s *Service) ResetOwnSubscription(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	var req subscriptionResetRequest
	if err := binding.JSON.BindBody(request.Body, &req); err != nil {
		return s.panelError("参数错误")
	}
	userID := uint64(request.Principal.ActorID)
	stores, err := s.Open(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	key := subscriptionResetKey(userID)
	// The attempt is counted before the credential check; one that checked
	// no credential is given back.
	if allowed, wait, err := stores.Throttle.Attempt(ctx, key, subscriptionResetLimit); err != nil {
		return s.panelError(err.Error())
	} else if !allowed {
		return s.panelError(subscriptionResetLimited, retryAfter(wait))
	}
	checked, refusal, err := s.stepUp(ctx, stores, userID, req)
	if !checked {
		if releaseErr := stores.Throttle.Release(ctx, key, subscriptionResetLimit); releaseErr != nil {
			return s.panelError(releaseErr.Error())
		}
	}
	if err != nil {
		return s.panelError(err.Error())
	}
	if refusal != "" {
		return s.panelError(refusal)
	}
	_, err = s.Subscriber.ResetCredentials(ctx, &kernelsubscriberv1.ResetCredentialsRequest{
		RequestId: ResetRequestID(userResetSubscribe, userID, s.requestToken(request)), UserId: userID, SubscriptionToken: true,
	})
	if err != nil {
		return s.resetFailed("重置订阅失败", err)
	}
	_, shown, err := s.showSubscriber(ctx, userID)
	if err != nil {
		return s.resetFailed("重置订阅失败", err)
	}
	return s.panel(map[string]any{"token": shown.Token})
}

// stepUp re-authenticates a signed-in user as the kernel does
// (service.UserService.VerifyStepUp): with a second factor, a TOTP or
// recovery code (Accounts.VerifyMFA, which consumes a used recovery code);
// otherwise the current password. checked reports whether a credential was
// checked; refusal is the answer when the check failed or a credential is
// missing.
func (s *Service) stepUp(ctx context.Context, stores *Stores, userID uint64, req subscriptionResetRequest) (bool, string, error) {
	users, err := stores.Accounts.Get(ctx, []uint64{userID})
	if err != nil {
		return false, "", err
	}
	if len(users) == 0 {
		return false, "用户不存在", nil
	}
	_, enabled, err := stores.Accounts.MFAMethods(ctx, userID)
	if err != nil {
		return false, "", err
	}
	if enabled {
		code := strings.TrimSpace(req.Code)
		if code == "" {
			return false, stepUpMFARequired, nil
		}
		valid, err := stores.Accounts.VerifyMFA(ctx, userID, code, req.Method)
		if err != nil {
			return true, "", err
		}
		if !valid {
			return true, stepUpInvalidMFACode, nil
		}
		return true, "", nil
	}
	if req.Password == "" {
		return false, stepUpPasswordRequired, nil
	}
	if bcrypt.CompareHashAndPassword([]byte(users[0].PasswordHash), []byte(req.Password)) != nil {
		return true, stepUpInvalidPassword, nil
	}
	return true, "", nil
}
