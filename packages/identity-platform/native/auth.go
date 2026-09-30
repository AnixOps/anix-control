package native

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/identity/account"
	"github.com/AnixOps/anix-control/identity/throttle"
	"github.com/AnixOps/anix-control/identity/token"
	kernelidentityv1 "github.com/AnixOps/anix-control/sdk/api/kernelidentity/v1"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/gin-gonic/gin/binding"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Control's identity tokens.
const (
	TokenIssuer   = "anixops-identity"
	TokenAudience = "anix-control"
)

// loginRequest and registerRequest bind exactly as the v2 requests do.
type loginRequest struct {
	Email     string `json:"email" binding:"required,email"`
	Password  string `json:"password" binding:"required"`
	MFACode   string `json:"mfa_code"`
	MFAMethod string `json:"mfa_method"`
}

type registerRequest struct {
	Email      string `json:"email" binding:"required,email"`
	Password   string `json:"password" binding:"required,min=6"`
	InviteCode string `json:"invite_code"`
}

func loginKey(email, ip string) string {
	normalizedIP := strings.TrimSpace(ip)
	if normalizedIP == "" {
		normalizedIP = "unknown"
	}
	return fmt.Sprintf("login|%s|%s", strings.ToLower(strings.TrimSpace(email)), normalizedIP)
}

func registerKey(ip string) string {
	normalizedIP := strings.TrimSpace(ip)
	if normalizedIP == "" {
		normalizedIP = "unknown"
	}
	return fmt.Sprintf("register|%s", normalizedIP)
}

func retryAfter(wait time.Duration) pluginhostsdk.Header {
	seconds := int(wait.Seconds())
	if seconds < 1 {
		seconds = 1
	}
	return pluginhostsdk.Header{Name: "Retry-After", Value: strconv.Itoa(seconds)}
}

// Login authenticates against identity's accounts, applies the admin MFA
// policy and issues an identity token for Control.
func (s *Service) Login(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	var req loginRequest
	if err := binding.JSON.BindBody(request.Body, &req); err != nil {
		return s.panelError("参数错误")
	}
	stores, err := s.Open(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	config, err := s.settings(ctx, stores)
	if err != nil {
		return s.panelError(err.Error())
	}
	limit := config.LoginRateLimit.options()
	key := loginKey(req.Email, request.Metadata.ClientIP)
	if blocked, wait, err := stores.Throttle.Check(ctx, key, limit); err != nil {
		return s.panelError(err.Error())
	} else if blocked {
		return s.panelError("too many login attempts, please try again later", retryAfter(wait))
	}

	user, err := s.authenticate(ctx, stores, req.Email, req.Password)
	if err != nil {
		if recordErr := stores.Throttle.RecordFailure(ctx, key, limit); recordErr != nil {
			return s.panelError(recordErr.Error())
		}
		return s.panelError(err.Error())
	}

	response, handled, err := s.loginMFA(ctx, stores, config, user, req, request.Metadata, key, limit)
	if err != nil {
		return s.panelError(err.Error())
	}
	if handled {
		return response, nil
	}

	signed, err := s.issue(ctx, config, user)
	if err != nil {
		return s.panelError(err.Error())
	}
	if err := stores.Throttle.RecordSuccess(ctx, key); err != nil {
		return s.panelError(err.Error())
	}
	return s.panel(s.sessionData(ctx, user, signed))
}

// authenticate reproduces the v2 checks in their order: account, password,
// ban, expiry.
func (s *Service) authenticate(ctx context.Context, stores *Stores, email, password string) (account.Account, error) {
	user, found, err := stores.Accounts.FindByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		return account.Account{}, err
	}
	if !found || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return account.Account{}, errors.New("用户不存在或密码错误")
	}
	if user.Banned {
		return account.Account{}, errors.New("用户已被封禁")
	}
	expiredAt, err := s.Directory.ExpiredAt(ctx, user.UserID)
	if err != nil {
		return account.Account{}, err
	}
	if expiredAt != nil && *expiredAt > 0 && *expiredAt <= s.now().Unix() {
		return account.Account{}, errors.New("用户已过期")
	}
	return user, nil
}

// mfaPolicy is the part of the admin MFA configuration login applies.
type mfaPolicy struct {
	Enabled         bool
	Required        bool
	EnforceForAdmin bool
}

func policyFrom(settings map[string]any) mfaPolicy {
	flag := func(name string) bool {
		value, _ := settings[name].(bool)
		return value
	}
	return mfaPolicy{Enabled: flag("enabled"), Required: flag("required"), EnforceForAdmin: flag("enforce_for_admin")}
}

func (p mfaPolicy) enforcedFor(user account.Account) bool {
	return p.Enabled && (p.Required || (p.EnforceForAdmin && user.IsAdmin))
}

func (s *Service) loginMFA(ctx context.Context, stores *Stores, config Settings, user account.Account, req loginRequest,
	metadata pluginhostsdk.RequestMetadata, key string, limit throttle.Options) (pluginhostsdk.NativeResponse, bool, error) {
	methods, enabled, err := stores.Accounts.MFAMethods(ctx, user.UserID)
	if err != nil {
		return pluginhostsdk.NativeResponse{}, false, err
	}
	if !enabled {
		if policyFrom(config.AdminMFA).enforcedFor(user) {
			enrollment := []string{account.MethodTOTP}
			response, err := s.panel(map[string]any{
				"mfa_enrollment_required": true, "mfa_setup_required": true,
				"methods": enrollment, "mfa_methods": enrollment, "user_id": user.UserID, "email": user.Email,
			})
			return response, true, err
		}
		return pluginhostsdk.NativeResponse{}, false, nil
	}
	code := strings.TrimSpace(req.MFACode)
	method := strings.ToLower(strings.TrimSpace(req.MFAMethod))
	if code == "" {
		response, err := s.panel(map[string]any{
			"mfa_required": true, "methods": methods, "mfa_methods": methods, "user_id": user.UserID, "email": user.Email,
		})
		return response, true, err
	}
	valid, err := stores.Accounts.VerifyMFA(ctx, user.UserID, code, method)
	if err != nil {
		return pluginhostsdk.NativeResponse{}, false, err
	}
	attemptMethod := method
	if attemptMethod == "" {
		attemptMethod = "auto"
	}
	if err := stores.Accounts.RecordMFAAttempt(ctx, user.UserID, metadata.ClientIP, metadata.UserAgent, valid, attemptMethod); err != nil {
		return pluginhostsdk.NativeResponse{}, false, err
	}
	if !valid {
		if err := stores.Throttle.RecordFailure(ctx, key, limit); err != nil {
			return pluginhostsdk.NativeResponse{}, false, err
		}
		response, err := s.panelError("invalid mfa code")
		return response, true, err
	}
	return pluginhostsdk.NativeResponse{}, false, nil
}

// issue signs a Control token for user with a new session.
func (s *Service) issue(ctx context.Context, config Settings, user account.Account) (string, error) {
	key, err := s.SigningKey(ctx)
	if err != nil {
		return "", err
	}
	session, err := token.NewSessionID()
	if err != nil {
		return "", err
	}
	lifetime := time.Duration(config.TokenLifetimeSeconds) * time.Second
	signed, _, err := token.Issuer{Issuer: TokenIssuer, Lifetime: lifetime, Now: s.now}.Issue(key, token.Subject{
		UserID: user.UserID, Email: user.Email, IsAdmin: user.IsAdmin, TokenVersion: max(user.TokenVersion, 1),
	}, session, TokenAudience)
	return signed, err
}

// sessionData is the v2 login and registration answer. Permission errors do
// not fail a committed login: the session gets empty, authoritative
// permissions, as in v2.
func (s *Service) sessionData(ctx context.Context, user account.Account, signed string) map[string]any {
	data := map[string]any{
		"token": signed, "is_admin": user.IsAdmin, "user_id": user.UserID, "email": user.Email,
		"permission_mode": "authoritative", "permissions": []string{}, "restricted_plugins": []string{},
	}
	access, err := s.Kernel.ResolveActorAccess(ctx, &kernelidentityv1.ResolveActorAccessRequest{UserId: user.UserID, IsAdmin: user.IsAdmin})
	if err != nil {
		return data
	}
	data["permission_mode"] = access.GetPermissionMode()
	if access.GetUnrestricted() {
		data["permissions"] = nil
	} else {
		data["permissions"] = nonNil(access.GetPermissions())
	}
	data["restricted_plugins"] = nonNil(access.GetRestrictedPlugins())
	return data
}

func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

// Register creates the identity account and, through Control, its
// subscriber; the invite code is consumed by Control with it.
func (s *Service) Register(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	var req registerRequest
	if err := binding.JSON.BindBody(request.Body, &req); err != nil {
		return s.panelError("参数错误")
	}
	stores, err := s.Open(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	config, err := s.settings(ctx, stores)
	if err != nil {
		return s.panelError(err.Error())
	}
	policy := newPolicy(config.Registration)
	if !policy.Enabled {
		return s.panelError("registration is disabled")
	}
	limit := config.RegisterRateLimit.options()
	key := registerKey(request.Metadata.ClientIP)
	if blocked, wait, err := stores.Throttle.Check(ctx, key, limit); err != nil {
		return s.panelError(err.Error())
	} else if blocked {
		return s.panelError("too many registration attempts, please try again later", retryAfter(wait))
	}
	// Every registration counts against the address, successful or not.
	if err := stores.Throttle.RecordFailure(ctx, key, limit); err != nil {
		return s.panelError(err.Error())
	}
	if len(req.Password) < 6 {
		return s.panelError("密码长度至少6位")
	}
	if len(req.Email) < 5 {
		return s.panelError("请输入有效的邮箱地址")
	}
	if err := policy.validate(req.Email); err != nil {
		return s.panelError(err.Error())
	}
	inviteCode := strings.TrimSpace(req.InviteCode)
	if policy.RequireInvite && inviteCode == "" {
		return s.panelError("invite code is required")
	}

	user, err := s.register(ctx, stores, strings.ToLower(strings.TrimSpace(req.Email)), req.Password, inviteCode)
	if err != nil {
		return s.panelError(err.Error())
	}
	signed, err := s.issue(ctx, config, user)
	if err != nil {
		return s.panelError(err.Error())
	}
	return s.panel(s.sessionData(ctx, user, signed))
}

func (s *Service) register(ctx context.Context, stores *Stores, email, password, inviteCode string) (account.Account, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return account.Account{}, errors.New("密码加密失败")
	}
	taken, err := stores.Accounts.EmailTaken(ctx, email)
	if err != nil {
		return account.Account{}, err
	}
	if taken {
		return account.Account{}, errors.New("该邮箱已被注册")
	}
	accountUUID := uuid.NewString()
	created, err := s.Kernel.CreateSubscriber(ctx, &kernelidentityv1.CreateSubscriberRequest{
		AccountUuid: accountUUID, Email: email, InviteCode: inviteCode,
	})
	switch status.Code(err) {
	case codes.OK:
	case codes.AlreadyExists:
		return account.Account{}, errors.New("该邮箱已被注册")
	case codes.FailedPrecondition:
		return account.Account{}, errors.New(status.Convert(err).Message())
	default:
		return account.Account{}, errors.New("注册失败，请稍后重试")
	}
	user := account.Account{
		UserID: created.GetUserId(), AccountUUID: accountUUID, Email: email, PasswordHash: string(hash), TokenVersion: 1,
	}
	if err := stores.Accounts.Create(ctx, user); err != nil {
		if errors.Is(err, account.ErrEmailTaken) {
			return account.Account{}, errors.New("该邮箱已被注册")
		}
		return account.Account{}, errors.New("注册失败，请稍后重试")
	}
	return user, nil
}

// registrationPolicy reproduces the v2 registration checks.
type registrationPolicy struct {
	Enabled       bool
	RequireInvite bool
	Allowed       []string
	Blocked       []string
}

func newPolicy(settings RegistrationSettings) registrationPolicy {
	return registrationPolicy{
		Enabled: settings.Enabled, RequireInvite: settings.RequireInvite,
		Allowed: normalizeDomains(settings.AllowedEmailDomains), Blocked: normalizeDomains(settings.BlockedEmailDomains),
	}
}

func (p registrationPolicy) validate(email string) error {
	normalized := strings.ToLower(strings.TrimSpace(email))
	parts := strings.Split(normalized, "@")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return errors.New("invalid email address")
	}
	domain := strings.TrimPrefix(parts[1], ".")
	if len(p.Blocked) > 0 && domainMatches(domain, p.Blocked) {
		return fmt.Errorf("email domain %s is not allowed", domain)
	}
	if len(p.Allowed) > 0 && !domainMatches(domain, p.Allowed) {
		return fmt.Errorf("email domain %s is not in the allowlist", domain)
	}
	return nil
}

func normalizeDomains(items []string) []string {
	out := make([]string, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		domain := strings.TrimPrefix(strings.TrimPrefix(strings.ToLower(strings.TrimSpace(item)), "@"), ".")
		if domain == "" || seen[domain] {
			continue
		}
		seen[domain] = true
		out = append(out, domain)
	}
	return out
}

func domainMatches(domain string, patterns []string) bool {
	for _, pattern := range patterns {
		if domain == pattern || strings.HasSuffix(domain, "."+pattern) {
			return true
		}
	}
	return false
}
