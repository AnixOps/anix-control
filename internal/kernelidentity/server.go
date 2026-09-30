// Package kernelidentity serves the KernelIdentity contract: the kernel side
// of the identity module. Identity owns accounts and credentials; the kernel
// owns subscribers (v2_user rows and their entitlements), keeps the identity
// columns of v2_user as a versioned read projection, and enforces
// revocations. Only the official identity package, whose signed release
// declares kernel.identity.v1, may call it.
package kernelidentity

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	kernelidentityv1 "github.com/AnixOps/anix-control/sdk/api/kernelidentity/v1"
	"github.com/AnixOps/anix-control/v4/internal/authn"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/handler"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// UnusableLegacyPassword is stored in v2_user.password for accounts whose
// credentials live in identity. It never matches a password.
const UnusableLegacyPassword = "!identity"

// Authorizer admits a calling host to KernelIdentity.
type Authorizer interface {
	AuthorizeIdentity(ctx context.Context, host packagebridge.HostIdentity) error
}

// Server holds what every host's KernelIdentity calls share.
type Server struct {
	DB         *gorm.DB
	Authorizer Authorizer
	// Config returns the current configuration (settings, token lifetime).
	Config func() *config.Config
}

// For returns the KernelIdentity server that host reaches; it has the shape
// of packagebridge.KernelIdentityProvider.
func (s *Server) For(host packagebridge.HostIdentity) kernelidentityv1.KernelIdentityServer {
	return &hostServer{server: s, host: host}
}

type hostServer struct {
	kernelidentityv1.UnimplementedKernelIdentityServer
	server *Server
	host   packagebridge.HostIdentity
}

func (h *hostServer) begin(ctx context.Context) (*gorm.DB, error) {
	if h.server == nil || h.server.DB == nil || h.server.Authorizer == nil {
		return nil, status.Error(codes.Unavailable, "kernel identity is not configured")
	}
	err := h.server.Authorizer.AuthorizeIdentity(ctx, h.host)
	switch {
	case err == nil:
		return h.server.DB.WithContext(ctx), nil
	case errors.Is(err, packagebridge.ErrHostFenced):
		return nil, status.Error(codes.PermissionDenied, "package host generation is fenced")
	case errors.Is(err, service.ErrIdentityNotAuthorized):
		return nil, status.Error(codes.PermissionDenied, "package is not authorized for kernel.identity.v1")
	default:
		return nil, status.Error(codes.Unavailable, "kernel identity authorization failed")
	}
}

var errEmailTaken = errors.New("email belongs to another user")

func (h *hostServer) CreateSubscriber(ctx context.Context, request *kernelidentityv1.CreateSubscriberRequest) (*kernelidentityv1.CreateSubscriberResponse, error) {
	db, err := h.begin(ctx)
	if err != nil {
		return nil, err
	}
	accountID, err := uuid.Parse(request.GetAccountUuid())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "account_uuid must be a UUID")
	}
	accountUUID := accountID.String()
	email := normalizeEmail(request.GetEmail())
	if email == "" {
		return nil, status.Error(codes.InvalidArgument, "email is required")
	}
	var inviteUserID *uint
	if request.GetInviteUserId() != 0 {
		id, err := userID(request.GetInviteUserId())
		if err != nil {
			return nil, err
		}
		inviteUserID = &id
	}

	var response *kernelidentityv1.CreateSubscriberResponse
	err = db.Transaction(func(tx *gorm.DB) error {
		if link, found, err := linkByAccount(tx, accountUUID); err != nil || found {
			if found {
				response = &kernelidentityv1.CreateSubscriberResponse{UserId: uint64(link.UserID)}
			}
			return err
		}
		var taken int64
		if err := tx.Model(&model.User{}).Where("email = ?", email).Count(&taken).Error; err != nil {
			return err
		}
		if taken > 0 {
			return errEmailTaken
		}
		if inviteUserID != nil {
			var inviter int64
			if err := tx.Model(&model.User{}).Where("id = ?", *inviteUserID).Count(&inviter).Error; err != nil {
				return err
			}
			if inviter == 0 {
				inviteUserID = nil
			}
		}
		user := service.NewSubscriberUser(email, UnusableLegacyPassword)
		user.InviteUserID = inviteUserID
		if err := tx.Create(user).Error; err != nil {
			return err
		}
		if err := tx.Create(&model.IdentityAccountLink{UserID: user.ID, AccountUUID: accountUUID}).Error; err != nil {
			return err
		}
		response = &kernelidentityv1.CreateSubscriberResponse{UserId: uint64(user.ID), Created: true}
		return nil
	})
	if err != nil {
		// A concurrent call for the same account may have won the race.
		if link, found, lookupErr := linkByAccount(db, accountUUID); lookupErr == nil && found {
			return &kernelidentityv1.CreateSubscriberResponse{UserId: uint64(link.UserID)}, nil
		}
		if errors.Is(err, errEmailTaken) {
			return nil, status.Error(codes.AlreadyExists, "email belongs to another subscriber")
		}
		return nil, internalError("create subscriber", err)
	}
	return response, nil
}

func (h *hostServer) UpdateSubscriber(ctx context.Context, request *kernelidentityv1.UpdateSubscriberRequest) (*kernelidentityv1.UpdateSubscriberResponse, error) {
	db, err := h.begin(ctx)
	if err != nil {
		return nil, err
	}
	id, err := userID(request.GetUserId())
	if err != nil {
		return nil, err
	}
	var entitlements service.SubscriberEntitlements
	decoder := json.NewDecoder(bytes.NewReader(request.GetEntitlementsJson()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&entitlements); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "entitlements_json: %v", err)
	}
	if entitlements.PlanID != nil && *entitlements.PlanID == 0 {
		return nil, status.Error(codes.InvalidArgument, "entitlements_json: plan_id must be positive")
	}
	updates := entitlements.Updates(service.NewPlanService())
	err = db.Transaction(func(tx *gorm.DB) error {
		if _, err := linkByUser(tx, id); err != nil {
			return err
		}
		if len(updates) == 0 {
			return nil
		}
		// Entitlements never end sessions, so there is no revocation to apply.
		_, err := service.UpdateUserTx(tx, id, updates)
		return err
	})
	if err != nil {
		return nil, statusFor("update subscriber", err)
	}
	return &kernelidentityv1.UpdateSubscriberResponse{}, nil
}

func (h *hostServer) ApplyAccountProjection(ctx context.Context, request *kernelidentityv1.ApplyAccountProjectionRequest) (*kernelidentityv1.ApplyAccountProjectionResponse, error) {
	db, err := h.begin(ctx)
	if err != nil {
		return nil, err
	}
	id, err := userID(request.GetUserId())
	if err != nil {
		return nil, err
	}
	if request.GetVersion() == 0 {
		return nil, status.Error(codes.InvalidArgument, "version must be positive")
	}
	email := normalizeEmail(request.GetEmail())
	if email == "" {
		return nil, status.Error(codes.InvalidArgument, "email is required")
	}

	var (
		response   *kernelidentityv1.ApplyAccountProjectionResponse
		revocation *authn.Revocation
	)
	err = db.Transaction(func(tx *gorm.DB) error {
		var link model.IdentityAccountLink
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ?", id).Take(&link).Error; err != nil {
			return err
		}
		if request.GetVersion() <= link.ProjectionVersion {
			response = &kernelidentityv1.ApplyAccountProjectionResponse{StoredVersion: link.ProjectionVersion}
			return nil
		}
		updates := map[string]any{
			"email":    email,
			"is_admin": boolInt(request.GetIsAdmin()),
			"is_staff": boolInt(request.GetIsStaff()),
			"banned":   boolInt(request.GetBanned()),
		}
		if mirror := request.GetLegacyMirror(); mirror != nil {
			state, err := authorityState(tx)
			if err != nil {
				return err
			}
			// After finalize the kernel keeps no credentials.
			if state != model.IdentityAuthorityFinalized {
				if err := mirrorCredentials(tx, id, mirror, updates); err != nil {
					return err
				}
			}
		}
		var err error
		if revocation, err = service.UpdateUserTx(tx, id, updates); err != nil {
			return err
		}
		if err := tx.Model(&model.IdentityAccountLink{}).Where("user_id = ?", id).
			Updates(map[string]any{"projection_version": request.GetVersion(), "updated_at": time.Now()}).Error; err != nil {
			return err
		}
		response = &kernelidentityv1.ApplyAccountProjectionResponse{Applied: true, StoredVersion: request.GetVersion()}
		return nil
	})
	if err != nil {
		return nil, statusFor("apply account projection", err)
	}
	if revocation != nil {
		authn.Remember(*revocation)
	}
	return response, nil
}

// mirrorCredentials copies identity's credentials into the legacy columns so
// that switching the identity routes back to legacy logs users in.
func mirrorCredentials(tx *gorm.DB, id uint, mirror *kernelidentityv1.LegacyCredentialMirror, updates map[string]any) error {
	if mirror.GetPasswordHash() != "" {
		updates["password"] = mirror.GetPasswordHash()
		updates["password_algo"] = nullableString(mirror.GetPasswordAlgo())
		updates["password_salt"] = nullableString(mirror.GetPasswordSalt())
	}
	if !mirror.GetMfaEnabled() {
		return tx.Model(&model.UserMFA{}).Where("user_id = ?", id).
			Updates(map[string]any{"enabled": false, "totp_secret": "", "backup_codes": ""}).Error
	}
	// Legacy compares backup codes in plain text while identity keeps only
	// hashes, so the legacy backup codes are left as they are; TOTP is kept
	// in step.
	var existing model.UserMFA
	err := tx.Where("user_id = ?", id).Take(&existing).Error
	switch {
	case err == nil:
		return tx.Model(&model.UserMFA{}).Where("user_id = ?", id).
			Updates(map[string]any{"enabled": true, "totp_secret": mirror.GetTotpSecret()}).Error
	case errors.Is(err, gorm.ErrRecordNotFound):
		return tx.Create(&model.UserMFA{UserID: id, Enabled: true, TOTPSecret: mirror.GetTotpSecret(), BackupCodes: "[]"}).Error
	default:
		return err
	}
}

func (h *hostServer) DeleteSubscriber(ctx context.Context, request *kernelidentityv1.DeleteSubscriberRequest) (*kernelidentityv1.DeleteSubscriberResponse, error) {
	db, err := h.begin(ctx)
	if err != nil {
		return nil, err
	}
	id, err := userID(request.GetUserId())
	if err != nil {
		return nil, err
	}
	var revocation authn.Revocation
	err = db.Transaction(func(tx *gorm.DB) error {
		if _, err := linkByUser(tx, id); err != nil {
			return err
		}
		var err error
		if revocation, err = service.DeleteUserTx(tx, id); err != nil {
			return err
		}
		return tx.Where("user_id = ?", id).Delete(&model.IdentityAccountLink{}).Error
	})
	if err != nil {
		return nil, statusFor("delete subscriber", err)
	}
	authn.Remember(revocation)
	return &kernelidentityv1.DeleteSubscriberResponse{}, nil
}

func (h *hostServer) PublishRevocation(ctx context.Context, request *kernelidentityv1.PublishRevocationRequest) (*kernelidentityv1.PublishRevocationResponse, error) {
	db, err := h.begin(ctx)
	if err != nil {
		return nil, err
	}
	id, err := userID(request.GetUserId())
	if err != nil {
		return nil, err
	}
	revocation := authn.Revocation{
		UserID: id, TokenVersion: request.GetTokenVersion(), SessionID: request.GetSessionId(), Reason: truncate(request.GetReason(), 64),
	}
	if request.GetNotBeforeUnix() > 0 {
		revocation.NotBefore = time.Unix(request.GetNotBeforeUnix(), 0)
	}
	if request.GetSessionExpiresAtUnix() > 0 {
		revocation.SessionExpiresAt = time.Unix(request.GetSessionExpiresAtUnix(), 0)
	}
	if err := authn.Write(db, revocation); err != nil {
		if errors.Is(err, authn.ErrInvalidRevocation) {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		return nil, internalError("publish revocation", err)
	}
	authn.Remember(revocation)
	return &kernelidentityv1.PublishRevocationResponse{}, nil
}

func (h *hostServer) ResolveActorAccess(ctx context.Context, request *kernelidentityv1.ResolveActorAccessRequest) (*kernelidentityv1.ResolveActorAccessResponse, error) {
	db, err := h.begin(ctx)
	if err != nil {
		return nil, err
	}
	id, err := userID(request.GetUserId())
	if err != nil {
		return nil, err
	}
	access, err := service.ResolveActorPluginAccess(db, id, request.GetIsAdmin())
	if err != nil {
		return nil, internalError("resolve actor access", err)
	}
	permissions := access.ProfilePermissions()
	return &kernelidentityv1.ResolveActorAccessResponse{
		PermissionMode:    access.PermissionMode(),
		Permissions:       permissions,
		RestrictedPlugins: access.ProfileRestrictedPluginList(),
		Unrestricted:      permissions == nil,
	}, nil
}

// IdentitySettings is the settings_json document of GetIdentitySettings.
type IdentitySettings struct {
	Registration          RegistrationSettings `json:"registration"`
	LoginRateLimit        RateLimitSettings    `json:"login_rate_limit"`
	RegisterRateLimit     RateLimitSettings    `json:"register_rate_limit"`
	AdminMFA              map[string]any       `json:"admin_mfa"`
	TokenLifetimeSeconds  int                  `json:"token_lifetime_seconds"`
	LegacyTokenIssuer     string               `json:"legacy_token_issuer"`
	AuthorityState        string               `json:"authority_state"`
	SettingsSchemaVersion int                  `json:"settings_schema_version"`
}

// RegistrationSettings mirrors the kernel's registration policy.
type RegistrationSettings struct {
	Enabled             bool     `json:"enabled"`
	RequireInvite       bool     `json:"require_invite"`
	AllowedEmailDomains []string `json:"allowed_email_domains"`
	BlockedEmailDomains []string `json:"blocked_email_domains"`
}

// RateLimitSettings mirrors one login or registration rate limit.
type RateLimitSettings struct {
	Enabled        bool `json:"enabled"`
	MaxAttempts    int  `json:"max_attempts"`
	WindowSeconds  int  `json:"window_seconds"`
	LockoutSeconds int  `json:"lockout_seconds"`
}

func (h *hostServer) GetIdentitySettings(ctx context.Context, _ *kernelidentityv1.GetIdentitySettingsRequest) (*kernelidentityv1.GetIdentitySettingsResponse, error) {
	db, err := h.begin(ctx)
	if err != nil {
		return nil, err
	}
	var cfg *config.Config
	if h.server.Config != nil {
		cfg = h.server.Config()
	}
	if cfg == nil {
		return nil, status.Error(codes.Unavailable, "kernel configuration is not loaded")
	}
	mfa, err := handler.AdminMFASettings(db)
	if err != nil {
		return nil, internalError("load admin MFA settings", err)
	}
	state, err := authorityState(db)
	if err != nil {
		return nil, internalError("load identity authority", err)
	}
	policy := service.ResolveRegistrationPolicy(cfg)
	settings := IdentitySettings{
		Registration: RegistrationSettings{
			Enabled: policy.Enabled, RequireInvite: policy.RequireInvite,
			AllowedEmailDomains: nonNil(policy.AllowedEmailDomains), BlockedEmailDomains: nonNil(policy.BlockedEmailDomains),
		},
		LoginRateLimit:        rateLimit(service.ResolveLoginRateLimitOptions(cfg)),
		RegisterRateLimit:     rateLimit(service.ResolveRegisterRateLimitOptions(cfg)),
		AdminMFA:              mfa,
		TokenLifetimeSeconds:  cfg.JWT.Expire,
		LegacyTokenIssuer:     "v2board",
		AuthorityState:        state,
		SettingsSchemaVersion: 1,
	}
	raw, err := json.Marshal(settings)
	if err != nil {
		return nil, internalError("encode identity settings", err)
	}
	return &kernelidentityv1.GetIdentitySettingsResponse{SettingsJson: raw}, nil
}

func rateLimit(options service.LoginRateLimitOptions) RateLimitSettings {
	return RateLimitSettings{
		Enabled: options.Enabled, MaxAttempts: options.MaxAttempts,
		WindowSeconds: int(options.Window / time.Second), LockoutSeconds: int(options.Lockout / time.Second),
	}
}

// AuthorityState returns the identity authority state; no row means the
// kernel still owns logins.
func AuthorityState(db *gorm.DB) (string, error) { return authorityState(db) }

func authorityState(db *gorm.DB) (string, error) {
	var authority model.IdentityAuthority
	err := db.Where("id = ?", 1).Take(&authority).Error
	switch {
	case err == nil:
		return authority.State, nil
	case errors.Is(err, gorm.ErrRecordNotFound):
		return model.IdentityAuthorityKernel, nil
	default:
		return "", err
	}
}

var errNotLinked = errors.New("user is not linked to an identity account")

func linkByAccount(db *gorm.DB, accountUUID string) (model.IdentityAccountLink, bool, error) {
	var link model.IdentityAccountLink
	err := db.Where("account_uuid = ?", accountUUID).Take(&link).Error
	switch {
	case err == nil:
		return link, true, nil
	case errors.Is(err, gorm.ErrRecordNotFound):
		return link, false, nil
	default:
		return link, false, err
	}
}

func linkByUser(db *gorm.DB, id uint) (model.IdentityAccountLink, error) {
	var link model.IdentityAccountLink
	err := db.Where("user_id = ?", id).Take(&link).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return link, errNotLinked
	}
	return link, err
}

func statusFor(operation string, err error) error {
	switch {
	case errors.Is(err, errNotLinked), errors.Is(err, gorm.ErrRecordNotFound), errors.Is(err, service.ErrUserNotFound):
		return status.Error(codes.NotFound, "subscriber is not linked to an identity account")
	case isUniqueViolation(err):
		return status.Error(codes.AlreadyExists, "email belongs to another subscriber")
	default:
		return internalError(operation, err)
	}
}

func isUniqueViolation(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique constraint") || strings.Contains(message, "duplicate key")
}

func internalError(operation string, err error) error {
	return status.Error(codes.Internal, fmt.Sprintf("%s failed: %v", operation, err))
}

func userID(value uint64) (uint, error) {
	if value == 0 || value > math.MaxUint32 {
		return 0, status.Error(codes.InvalidArgument, "user_id is out of range")
	}
	return uint(value), nil // #nosec G115 -- range is checked immediately above.
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}
