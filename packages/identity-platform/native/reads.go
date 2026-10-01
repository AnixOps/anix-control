package native

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/AnixOps/anix-control/identity/account"
	kernelidentityv1 "github.com/AnixOps/anix-control/sdk/api/kernelidentity/v1"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Route ids of the account reads and the administrator's resets.
const (
	ProfileRouteID        = "identity.user.profile.get"
	DashboardRouteID      = "identity.user.dashboard.get"
	AdminUserRouteID      = "identity.admin.users.id.get"
	ResetTrafficRouteID   = "identity.admin.users.id.reset_traffic.post"
	ResetSubscribeRouteID = "identity.admin.users.id.reset_subscribe.post"
)

// subscriberFields is what the account reads take from the subscriber
// KernelIdentity.GetSubscriber shows: the v2 user object, with its plan.
type subscriberFields struct {
	UUID           string  `json:"uuid"`
	Token          string  `json:"token"`
	PlanID         *uint64 `json:"plan_id"`
	TransferEnable int64   `json:"transfer_enable"`
	U              int64   `json:"u"`
	D              int64   `json:"d"`
	ExpiredAt      *int64  `json:"expired_at"`
	Plan           *struct {
		Name string `json:"name"`
	} `json:"plan"`
}

// account returns a user's account from identity's store; found is false
// when identity has none.
func (s *Service) account(ctx context.Context, userID uint64) (account.Account, bool, error) {
	stores, err := s.Open(ctx)
	if err != nil {
		return account.Account{}, false, err
	}
	accounts, err := stores.Accounts.Get(ctx, []uint64{userID})
	if err != nil || len(accounts) == 0 {
		return account.Account{}, false, err
	}
	return accounts[0], true, nil
}

// showSubscriber returns one subscriber as Control shows it, raw and decoded.
// It is the only way the reads see a subscription token: a contract call
// for that one user, never a view.
func (s *Service) showSubscriber(ctx context.Context, userID uint64) ([]byte, subscriberFields, error) {
	shown, err := s.Kernel.GetSubscriber(ctx, &kernelidentityv1.GetSubscriberRequest{UserId: userID})
	if err != nil {
		return nil, subscriberFields{}, err
	}
	var fields subscriberFields
	if err := json.Unmarshal(shown.GetSubscriberJson(), &fields); err != nil {
		return nil, subscriberFields{}, err
	}
	return shown.GetSubscriberJson(), fields, nil
}

// Profile is GET /api/v2/user/profile: the caller's account (email,
// administrator flag) from identity, the subscription token and proxy uuid
// of the caller's subscriber from Control, and the plugin permissions
// (KernelIdentity.ResolveActorAccess), as the v2 handler answers them.
func (s *Service) Profile(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	userID := uint64(request.Principal.ActorID)
	user, found, err := s.account(ctx, userID)
	if err != nil {
		return s.panelError("获取用户信息失败")
	}
	if !found {
		return s.panelError("用户不存在")
	}
	_, shown, err := s.showSubscriber(ctx, userID)
	if status.Code(err) == codes.NotFound {
		return s.panelError("用户不存在")
	}
	if err != nil {
		return s.panelError("获取用户信息失败")
	}
	access, err := s.Kernel.ResolveActorAccess(ctx, &kernelidentityv1.ResolveActorAccessRequest{UserId: user.UserID, IsAdmin: user.IsAdmin})
	if err != nil {
		return s.panelError("获取用户权限失败")
	}
	var permissions []string
	if !access.GetUnrestricted() {
		permissions = nonNil(access.GetPermissions())
	}
	return s.panel(map[string]any{
		"id":                 user.UserID,
		"email":              user.Email,
		"uuid":               shown.UUID,
		"token":              shown.Token,
		"is_admin":           user.IsAdmin,
		"permission_mode":    access.GetPermissionMode(),
		"permissions":        permissions,
		"restricted_plugins": nonNil(access.GetRestrictedPlugins()),
	})
}

// Subscription is the dashboard's subscription summary, with the fields and
// tags of the kernel's service.UserSubscription.
type Subscription struct {
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
	DaysRemaining    int       `json:"days_remaining"`
	UsagePercent     float64   `json:"usage_percent"`
	SubscribePath    string    `json:"subscribe_path,omitempty"`
	SubscribeDomains []string  `json:"subscribe_domains,omitempty"`
	CachedAt         time.Time `json:"cached_at"`
}

// Dashboard is GET /api/v2/user/dashboard: the caller's email from identity,
// plan, traffic and expiry from the caller's subscriber in Control. The
// kernel cached its answer for 30 seconds (cached_at is when it was
// computed); identity computes it on every request, so cached_at is the
// time of the answer.
func (s *Service) Dashboard(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	userID := uint64(request.Principal.ActorID)
	user, found, err := s.account(ctx, userID)
	if err != nil {
		return s.panelError("获取信息失败")
	}
	if !found {
		return s.panelError("用户不存在")
	}
	_, shown, err := s.showSubscriber(ctx, userID)
	if status.Code(err) == codes.NotFound {
		return s.panelError("用户不存在")
	}
	if err != nil {
		return s.panelError("获取信息失败")
	}
	now := s.now()
	expiredAt := int64(0)
	if shown.ExpiredAt != nil {
		expiredAt = *shown.ExpiredAt
	}
	subscription := Subscription{
		UserID: user.UserID, Email: user.Email, PlanID: shown.PlanID, PlanName: "无套餐",
		TransferEnable: shown.TransferEnable, UsedTraffic: shown.U + shown.D, UploadTraffic: shown.U, DownloadTraffic: shown.D,
		ExpiredAt: expiredAt, CachedAt: now,
	}
	if shown.Plan != nil {
		subscription.PlanName = shown.Plan.Name
	}
	if expiredAt > 0 {
		subscription.IsExpired = expiredAt <= now.Unix()
		if !subscription.IsExpired {
			subscription.DaysRemaining = int((expiredAt - now.Unix()) / 86400)
		}
	} else {
		subscription.DaysRemaining = -1
	}
	if shown.TransferEnable > 0 {
		subscription.UsagePercent = float64(subscription.UsedTraffic) / float64(shown.TransferEnable) * 100
	}
	return s.panel(map[string]any{"subscription": subscription})
}

// AdminUser is GET /api/v2/admin/users/:id: the subscriber as Control shows
// it, with its plan, and the identity fields (email, administrator, staff
// and ban flags) from identity's account. A subscriber identity has no
// account for keeps the fields Control holds. Every failure is "用户不存在",
// as in v2.
func (s *Service) AdminUser(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	id, ok := parseUserID(request.Metadata.PathParams["id"])
	if !ok {
		return s.panelError("无效的用户ID")
	}
	raw, _, err := s.showSubscriber(ctx, id)
	if err != nil {
		return s.panelError("用户不存在")
	}
	user, found, err := s.account(ctx, id)
	if err != nil {
		return s.panelError("用户不存在")
	}
	if found {
		if raw, err = withAccount(raw, user); err != nil {
			return s.panelError("用户不存在")
		}
	}
	return s.panel(json.RawMessage(raw))
}

// withAccount replaces the identity fields of a v2 user object with the
// account's, keeping the object's key order.
func withAccount(raw []byte, user account.Account) ([]byte, error) {
	fields := map[string]any{
		"email": user.Email, "is_admin": boolInt(user.IsAdmin), "is_staff": boolInt(user.IsStaff), "banned": boolInt(user.Banned),
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return nil, errors.New("subscriber is not a JSON object")
	}
	var out bytes.Buffer
	out.WriteByte('{')
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		key, ok := token.(string)
		if !ok {
			return nil, errors.New("subscriber has a malformed key")
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		if replacement, ok := fields[key]; ok {
			if value, err = json.Marshal(replacement); err != nil {
				return nil, err
			}
		}
		encodedKey, err := json.Marshal(key)
		if err != nil {
			return nil, err
		}
		if out.Len() > 1 {
			out.WriteByte(',')
		}
		out.Write(encodedKey)
		out.WriteByte(':')
		out.Write(value)
	}
	if token, err := decoder.Token(); err != nil || token != json.Delim('}') {
		return nil, errors.New("subscriber is not a JSON object")
	}
	out.WriteByte('}')
	return out.Bytes(), nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
