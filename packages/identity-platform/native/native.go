// Package native implements identity-platform's v2 routes in the identity
// module itself: accounts, credentials and MFA come from identity's
// storage, subscribers and permissions from Control through KernelIdentity.
// Responses are byte-compatible with the kernel's legacy handlers
// (internal/tests/identitycompat proves it route by route, on SQLite and
// PostgreSQL). A route uses its handler only once its mode is native:
//   - group A (login, registration, MFA, the administrator's account writes)
//     switches with the identity cutover;
//   - the account reads (the profile, the dashboard and the administrator's
//     user detail, user list and user statistics) take the account from
//     identity's store, so Control lets them leave legacy mode only while
//     identity is authoritative. The profile and the detail take their
//     subscriber fields, subscription token included, from
//     KernelIdentity.GetSubscriber for that one user, never from a view;
//     the list and the statistics search the user directory
//     (UserDirectory), which joins identity's accounts with Control's
//     subscriber views and shows no token;
//   - the administrator's traffic and subscription resets touch only the
//     subscriber, through KernelSubscriber (ResetTraffic, ResetCredentials),
//     and switch independently of the authority.
//
// The user's invite routes have no handler here and stay bridged (see the
// package's control host).
package native

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/AnixOps/anix-control/identity/account"
	"github.com/AnixOps/anix-control/identity/settings"
	"github.com/AnixOps/anix-control/identity/signingkey"
	"github.com/AnixOps/anix-control/identity/throttle"
	kernelidentityv1 "github.com/AnixOps/anix-control/sdk/api/kernelidentity/v1"
	kernelsubscriberv1 "github.com/AnixOps/anix-control/sdk/api/kernelsubscriber/v1"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/sdk/v2compat"
	"google.golang.org/grpc"
)

// Kernel is the part of KernelIdentity the native routes call.
type Kernel interface {
	CreateSubscriber(ctx context.Context, in *kernelidentityv1.CreateSubscriberRequest, opts ...grpc.CallOption) (*kernelidentityv1.CreateSubscriberResponse, error)
	ResolveActorAccess(ctx context.Context, in *kernelidentityv1.ResolveActorAccessRequest, opts ...grpc.CallOption) (*kernelidentityv1.ResolveActorAccessResponse, error)
	GetIdentitySettings(ctx context.Context, in *kernelidentityv1.GetIdentitySettingsRequest, opts ...grpc.CallOption) (*kernelidentityv1.GetIdentitySettingsResponse, error)
	UpdateSubscriber(ctx context.Context, in *kernelidentityv1.UpdateSubscriberRequest, opts ...grpc.CallOption) (*kernelidentityv1.UpdateSubscriberResponse, error)
	ApplyAccountProjection(ctx context.Context, in *kernelidentityv1.ApplyAccountProjectionRequest, opts ...grpc.CallOption) (*kernelidentityv1.ApplyAccountProjectionResponse, error)
	DeleteSubscriber(ctx context.Context, in *kernelidentityv1.DeleteSubscriberRequest, opts ...grpc.CallOption) (*kernelidentityv1.DeleteSubscriberResponse, error)
	GetSubscriber(ctx context.Context, in *kernelidentityv1.GetSubscriberRequest, opts ...grpc.CallOption) (*kernelidentityv1.GetSubscriberResponse, error)
}

// Subscriber is the part of KernelSubscriber the native routes call: the
// administrator's resets (kernel.subscriber.traffic.v1 and
// kernel.subscriber.credentials.v1).
type Subscriber interface {
	ResetTraffic(ctx context.Context, in *kernelsubscriberv1.ResetTrafficRequest, opts ...grpc.CallOption) (*kernelsubscriberv1.ResetTrafficResponse, error)
	ResetCredentials(ctx context.Context, in *kernelsubscriberv1.ResetCredentialsRequest, opts ...grpc.CallOption) (*kernelsubscriberv1.ResetCredentialsResponse, error)
}

// Directory reads subscriber fields Control owns, from the kernel API view
// kapi_user_directory_v1.
type Directory interface {
	// ExpiredAt returns the subscription end (Unix seconds), nil if none.
	ExpiredAt(ctx context.Context, userID uint64) (*int64, error)
}

// Stores are identity's storage-backed stores.
type Stores struct {
	Accounts *account.Store
	Throttle *throttle.Limiter
	Settings *settings.Store
}

// Service holds what the native routes need. Open returns the stores once
// identity storage is leased.
type Service struct {
	Open      func(ctx context.Context) (*Stores, error)
	Kernel    Kernel
	Directory Directory
	// Subscriber is Control's KernelSubscriber; without it the resets stay
	// legacy.
	Subscriber Subscriber
	// SigningKey returns the key that signs tokens now.
	SigningKey func(ctx context.Context) (signingkey.Key, error)
	// Now defaults to time.Now.
	Now func() time.Time
	// NewToken identifies a request without an Idempotency-Key or request
	// id; it defaults to a random UUID.
	NewToken func() string
}

// Handlers returns the native handlers by route id.
func (s *Service) Handlers() map[string]pluginhostsdk.NativeHandler {
	handlers := map[string]pluginhostsdk.NativeHandler{
		"identity.auth.login":                            s.Login,
		"identity.auth.register":                         s.Register,
		"identity.user.mfa.status.get":                   s.MFAStatus,
		"identity.user.mfa.totp.setup.post":              s.SetupTOTP,
		"identity.user.mfa.totp.enable.post":             s.EnableTOTP,
		"identity.user.mfa.disable.post":                 s.DisableMFA,
		"identity.user.mfa.verify.post":                  s.VerifyMFA,
		"identity.user.mfa.backup_codes.regenerate.post": s.RegenerateBackupCodes,
		"identity.admin.mfa.config.get":                  s.AdminMFAConfig,
		"identity.admin.mfa.config.put":                  s.UpdateAdminMFAConfig,
		"identity.admin.users.post":                      s.CreateUser,
		"identity.admin.users.id.put":                    s.UpdateUser,
		"identity.admin.users.id.ban.post":               s.BanUser,
		"identity.admin.users.id.unban.post":             s.UnbanUser,
		"identity.admin.users.id.delete":                 s.DeleteUser,
		ProfileRouteID:                                   s.Profile,
		DashboardRouteID:                                 s.Dashboard,
		AdminUserRouteID:                                 s.AdminUser,
		AdminUsersRouteID:                                s.AdminUsers,
		AdminUserStatsRouteID:                            s.AdminUserStats,
	}
	if s.Subscriber != nil {
		handlers[ResetTrafficRouteID] = s.ResetTraffic
		handlers[ResetSubscribeRouteID] = s.ResetSubscribe
	}
	return handlers
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Settings is the configuration identity was seeded with from Control
// (KernelIdentity.GetIdentitySettings); the JSON matches it.
type Settings struct {
	Registration         RegistrationSettings `json:"registration"`
	LoginRateLimit       RateLimitSettings    `json:"login_rate_limit"`
	RegisterRateLimit    RateLimitSettings    `json:"register_rate_limit"`
	AdminMFA             map[string]any       `json:"admin_mfa"`
	TokenLifetimeSeconds int                  `json:"token_lifetime_seconds"`
	AuthorityState       string               `json:"authority_state"`
}

// RegistrationSettings is the registration policy.
type RegistrationSettings struct {
	Enabled             bool     `json:"enabled"`
	RequireInvite       bool     `json:"require_invite"`
	AllowedEmailDomains []string `json:"allowed_email_domains"`
	BlockedEmailDomains []string `json:"blocked_email_domains"`
}

// RateLimitSettings is one attempt limit.
type RateLimitSettings struct {
	Enabled        bool `json:"enabled"`
	MaxAttempts    int  `json:"max_attempts"`
	WindowSeconds  int  `json:"window_seconds"`
	LockoutSeconds int  `json:"lockout_seconds"`
}

func (r RateLimitSettings) options() throttle.Options {
	return throttle.Options{
		Enabled: r.Enabled, MaxAttempts: r.MaxAttempts,
		Window: time.Duration(r.WindowSeconds) * time.Second, Lockout: time.Duration(r.LockoutSeconds) * time.Second,
	}
}

// SettingsKey is the settings document the Control adapter keeps.
const SettingsKey = "control"

// settings returns the stored configuration, seeding it once from Control.
func (s *Service) settings(ctx context.Context, stores *Stores) (Settings, error) {
	raw, found, err := stores.Settings.Get(ctx, SettingsKey)
	if err != nil {
		return Settings{}, err
	}
	if !found {
		if s.Kernel == nil {
			return Settings{}, errors.New("identity settings are not seeded")
		}
		response, err := s.Kernel.GetIdentitySettings(ctx, &kernelidentityv1.GetIdentitySettingsRequest{})
		if err != nil {
			return Settings{}, fmt.Errorf("seed identity settings: %w", err)
		}
		raw = response.GetSettingsJson()
		if err := stores.Settings.Put(ctx, SettingsKey, raw); err != nil {
			return Settings{}, err
		}
	}
	var decoded Settings
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return Settings{}, fmt.Errorf("identity settings are malformed: %w", err)
	}
	return decoded, nil
}

func (s *Service) panel(data any) (pluginhostsdk.NativeResponse, error) {
	return pluginhostsdk.PanelJSON(v2compat.PanelSuccess(data, s.now()))
}

func (s *Service) panelError(message string, headers ...pluginhostsdk.Header) (pluginhostsdk.NativeResponse, error) {
	response, err := pluginhostsdk.PanelJSON(v2compat.PanelError(message, s.now()))
	response.Headers = append(response.Headers, headers...)
	return response, err
}
