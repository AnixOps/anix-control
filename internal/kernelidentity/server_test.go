package kernelidentity

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	kernelidentityv1 "github.com/AnixOps/anix-control/sdk/api/kernelidentity/v1"
	"github.com/AnixOps/anix-control/v4/internal/authn"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/AnixOps/anix-control/v4/internal/utils"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

type authorizerStub struct {
	seen []packagebridge.HostIdentity
	err  error
}

func (a *authorizerStub) AuthorizeIdentity(_ context.Context, host packagebridge.HostIdentity) error {
	a.seen = append(a.seen, host)
	return a.err
}

var identityHost = packagebridge.HostIdentity{PackageID: "identity-platform", Version: "4.1.0", Generation: 3}

const testSecret = "kernel-identity-test-secret"

type fixture struct {
	db         *gorm.DB
	authorizer *authorizerStub
	server     kernelidentityv1.KernelIdentityServer
	store      *authn.Store
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	require.NoError(t, database.Close())
	require.NoError(t, database.Init(&config.DatabaseConfig{Driver: "sqlite", Database: ":memory:"}))
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	db := database.Get()
	require.NoError(t, service.EnsureKernelSchema(db))
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserMFA{}, &model.Plan{}, &model.WireGuardPeer{}, &model.SystemConfig{}))
	cfg := &config.Config{JWT: config.JWTConfig{Secret: testSecret, Expire: 3600}}
	config.Set(cfg)
	store := authn.NewStore(db, time.Hour)
	authn.SetDefaultStore(store)
	t.Cleanup(func() { authn.SetDefaultStore(nil) })
	authorizer := &authorizerStub{}
	server := (&Server{DB: db, Authorizer: authorizer, Config: config.Get}).For(identityHost)
	return &fixture{db: db, authorizer: authorizer, server: server, store: store}
}

func requireCode(t *testing.T, err error, code codes.Code) {
	t.Helper()
	require.Error(t, err)
	require.Equal(t, code, status.Code(err), err.Error())
}

const accountA = "0b6f1c3e-8d5a-4a6e-9a0c-6b2f1d9e4a11"

func (f *fixture) createSubscriber(t *testing.T, account, email string) uint {
	t.Helper()
	response, err := f.server.CreateSubscriber(context.Background(), &kernelidentityv1.CreateSubscriberRequest{AccountUuid: account, Email: email})
	require.NoError(t, err)
	return uint(response.GetUserId())
}

func TestCreateSubscriberIsIdempotentOnTheAccount(t *testing.T) {
	f := newFixture(t)
	inviter := model.User{Email: "inviter@example.test", Password: "x", UUID: "u-inviter", Token: "t-inviter"}
	require.NoError(t, f.db.Create(&inviter).Error)

	first, err := f.server.CreateSubscriber(context.Background(), &kernelidentityv1.CreateSubscriberRequest{
		AccountUuid: accountA, Email: " New@Example.test ", InviteUserId: uint64(inviter.ID),
	})
	require.NoError(t, err)
	require.True(t, first.GetCreated())

	var user model.User
	require.NoError(t, f.db.Take(&user, first.GetUserId()).Error)
	require.Equal(t, "new@example.test", user.Email)
	require.Equal(t, UnusableLegacyPassword, user.Password, "identity owns the credentials")
	require.NotEmpty(t, user.UUID)
	require.NotEmpty(t, user.Token)
	require.Equal(t, inviter.ID, *user.InviteUserID)

	again, err := f.server.CreateSubscriber(context.Background(), &kernelidentityv1.CreateSubscriberRequest{AccountUuid: accountA, Email: "new@example.test"})
	require.NoError(t, err)
	require.False(t, again.GetCreated())
	require.Equal(t, first.GetUserId(), again.GetUserId())

	_, err = f.server.CreateSubscriber(context.Background(), &kernelidentityv1.CreateSubscriberRequest{
		AccountUuid: "5d1c7a2b-3e4f-4a5b-8c6d-7e8f9a0b1c2d", Email: "new@example.test",
	})
	requireCode(t, err, codes.AlreadyExists)
	_, err = f.server.CreateSubscriber(context.Background(), &kernelidentityv1.CreateSubscriberRequest{AccountUuid: "not-a-uuid", Email: "x@example.test"})
	requireCode(t, err, codes.InvalidArgument)
	require.Len(t, f.authorizer.seen, 4)
	require.Equal(t, identityHost, f.authorizer.seen[0])
}

func TestEveryCallIsAuthorized(t *testing.T) {
	f := newFixture(t)
	f.authorizer.err = packagebridge.ErrHostFenced
	_, err := f.server.ResolveActorAccess(context.Background(), &kernelidentityv1.ResolveActorAccessRequest{UserId: 1})
	requireCode(t, err, codes.PermissionDenied)
	f.authorizer.err = service.ErrIdentityNotAuthorized
	_, err = f.server.CreateSubscriber(context.Background(), &kernelidentityv1.CreateSubscriberRequest{AccountUuid: accountA, Email: "a@example.test"})
	requireCode(t, err, codes.PermissionDenied)
	var users int64
	require.NoError(t, f.db.Model(&model.User{}).Count(&users).Error)
	require.Zero(t, users)
}

func TestUpdateSubscriberAppliesEntitlementsLikeTheAdminAPI(t *testing.T) {
	f := newFixture(t)
	id := f.createSubscriber(t, accountA, "sub@example.test")
	speed := int64(100)
	group := uint(4)
	plan := model.Plan{Name: "Pro", GroupID: group, TransferEnable: 50, SpeedLimit: &speed}
	require.NoError(t, f.db.Create(&plan).Error)

	entitlements, err := json.Marshal(map[string]any{"plan_id": plan.ID, "balance": 700, "expired_at": nil})
	require.NoError(t, err)
	_, err = f.server.UpdateSubscriber(context.Background(), &kernelidentityv1.UpdateSubscriberRequest{UserId: uint64(id), EntitlementsJson: entitlements})
	require.NoError(t, err)
	var user model.User
	require.NoError(t, f.db.Take(&user, id).Error)
	require.Equal(t, plan.ID, *user.PlanID)
	require.Equal(t, group, *user.GroupID)
	require.Equal(t, int64(50*1073741824), user.TransferEnable)
	require.Equal(t, speed, *user.SpeedLimit)
	require.Equal(t, int64(700), user.Balance)
	require.Nil(t, user.ExpiredAt, "null leaves the field as it is")

	_, err = f.server.UpdateSubscriber(context.Background(), &kernelidentityv1.UpdateSubscriberRequest{UserId: uint64(id), EntitlementsJson: []byte(`{"email":"x@example.test"}`)})
	requireCode(t, err, codes.InvalidArgument)

	legacy := model.User{Email: "legacy@example.test", Password: "x", UUID: "u-legacy", Token: "t-legacy"}
	require.NoError(t, f.db.Create(&legacy).Error)
	_, err = f.server.UpdateSubscriber(context.Background(), &kernelidentityv1.UpdateSubscriberRequest{UserId: uint64(legacy.ID), EntitlementsJson: []byte(`{"balance":1}`)})
	requireCode(t, err, codes.NotFound)
}

func TestApplyAccountProjectionIsMonotonicAndRevokes(t *testing.T) {
	f := newFixture(t)
	id := f.createSubscriber(t, accountA, "proj@example.test")
	token, err := utils.GenerateToken(id, "proj@example.test", false, testSecret, 3600)
	require.NoError(t, err)
	verify := func() error { _, err := authn.Default().Verify(context.Background(), token); return err }
	require.NoError(t, verify())

	apply := func(version uint64, email string, banned bool) *kernelidentityv1.ApplyAccountProjectionResponse {
		response, err := f.server.ApplyAccountProjection(context.Background(), &kernelidentityv1.ApplyAccountProjectionRequest{
			UserId: uint64(id), Version: version, Email: email, IsStaff: true, Banned: banned,
		})
		require.NoError(t, err)
		return response
	}

	response := apply(2, "proj@example.test", false)
	require.True(t, response.GetApplied())
	require.NoError(t, verify(), "an unchanged email and ban flag keep the session")
	var user model.User
	require.NoError(t, f.db.Take(&user, id).Error)
	require.Equal(t, 1, user.IsStaff)

	response = apply(1, "old@example.test", true)
	require.False(t, response.GetApplied())
	require.Equal(t, uint64(2), response.GetStoredVersion())
	require.NoError(t, f.db.Take(&user, id).Error)
	require.Equal(t, "proj@example.test", user.Email, "an older version is ignored")
	require.Equal(t, 0, user.Banned)

	apply(3, "proj@example.test", true)
	require.ErrorIs(t, verify(), authn.ErrRevokedToken, "a ban projected into the kernel revokes at once")
}

func TestLegacyMirrorKeepsCredentialsUntilFinalize(t *testing.T) {
	f := newFixture(t)
	id := f.createSubscriber(t, accountA, "mirror@example.test")
	mirror := &kernelidentityv1.LegacyCredentialMirror{
		PasswordHash: "$2a$10$abcdefghijklmnopqrstuvabcdefghijklmnopqrstuvwxyzABCDE", MfaEnabled: true, TotpSecret: "JBSWY3DPEHPK3PXP",
	}
	_, err := f.server.ApplyAccountProjection(context.Background(), &kernelidentityv1.ApplyAccountProjectionRequest{
		UserId: uint64(id), Version: 1, Email: "mirror@example.test", LegacyMirror: mirror,
	})
	require.NoError(t, err)
	var user model.User
	require.NoError(t, f.db.Take(&user, id).Error)
	require.Equal(t, mirror.GetPasswordHash(), user.Password)
	var mfa model.UserMFA
	require.NoError(t, f.db.Take(&mfa, "user_id = ?", id).Error)
	require.True(t, mfa.Enabled)
	require.Equal(t, "JBSWY3DPEHPK3PXP", mfa.TOTPSecret)

	require.NoError(t, f.db.Create(&model.IdentityAuthority{ID: 1, State: model.IdentityAuthorityFinalized, UpdatedAt: time.Now()}).Error)
	_, err = f.server.ApplyAccountProjection(context.Background(), &kernelidentityv1.ApplyAccountProjectionRequest{
		UserId: uint64(id), Version: 2, Email: "mirror@example.test",
		LegacyMirror: &kernelidentityv1.LegacyCredentialMirror{PasswordHash: "$2a$10$changed"},
	})
	require.NoError(t, err)
	require.NoError(t, f.db.Take(&user, id).Error)
	require.Equal(t, mirror.GetPasswordHash(), user.Password, "after finalize nothing is mirrored")
}

func TestDeleteSubscriberRemovesTheLinkAndRevokes(t *testing.T) {
	f := newFixture(t)
	id := f.createSubscriber(t, accountA, "delete@example.test")
	token, err := utils.GenerateToken(id, "delete@example.test", false, testSecret, 3600)
	require.NoError(t, err)
	time.Sleep(time.Until(time.Now().Truncate(time.Second).Add(time.Second)))

	_, err = f.server.DeleteSubscriber(context.Background(), &kernelidentityv1.DeleteSubscriberRequest{UserId: uint64(id)})
	require.NoError(t, err)
	var count int64
	require.NoError(t, f.db.Model(&model.User{}).Where("id = ?", id).Count(&count).Error)
	require.Zero(t, count)
	require.NoError(t, f.db.Model(&model.IdentityAccountLink{}).Count(&count).Error)
	require.Zero(t, count)
	_, err = authn.Default().Verify(context.Background(), token)
	require.ErrorIs(t, err, authn.ErrRevokedToken)

	_, err = f.server.DeleteSubscriber(context.Background(), &kernelidentityv1.DeleteSubscriberRequest{UserId: uint64(id)})
	requireCode(t, err, codes.NotFound)
}

func TestPublishRevocationValidatesAndApplies(t *testing.T) {
	f := newFixture(t)
	token, err := utils.GenerateToken(9, "rev@example.test", false, testSecret, 3600)
	require.NoError(t, err)
	claims, err := utils.ParseTokenWithSecret(token, testSecret)
	require.NoError(t, err)

	_, err = f.server.PublishRevocation(context.Background(), &kernelidentityv1.PublishRevocationRequest{UserId: 9})
	requireCode(t, err, codes.InvalidArgument)

	_, err = f.server.PublishRevocation(context.Background(), &kernelidentityv1.PublishRevocationRequest{
		UserId: 9, SessionId: claims.SessionID, SessionExpiresAtUnix: time.Now().Add(time.Hour).Unix(), Reason: "logout",
	})
	require.NoError(t, err)
	_, err = authn.Default().Verify(context.Background(), token)
	require.ErrorIs(t, err, authn.ErrRevokedToken)

	require.NoError(t, f.store.Reload(context.Background()))
	_, err = authn.Default().Verify(context.Background(), token)
	require.ErrorIs(t, err, authn.ErrRevokedToken, "the session revocation is stored")
}

func TestResolveActorAccessAndSettings(t *testing.T) {
	f := newFixture(t)
	access, err := f.server.ResolveActorAccess(context.Background(), &kernelidentityv1.ResolveActorAccessRequest{UserId: 1, IsAdmin: true})
	require.NoError(t, err)
	require.True(t, access.GetUnrestricted())
	require.Equal(t, service.PluginPermissionModeLegacy, access.GetPermissionMode())

	access, err = f.server.ResolveActorAccess(context.Background(), &kernelidentityv1.ResolveActorAccessRequest{UserId: 2})
	require.NoError(t, err)
	require.False(t, access.GetUnrestricted())
	require.Empty(t, access.GetRestrictedPlugins(), "non-administrators never see the restricted catalog")

	response, err := f.server.GetIdentitySettings(context.Background(), &kernelidentityv1.GetIdentitySettingsRequest{})
	require.NoError(t, err)
	var settings IdentitySettings
	require.NoError(t, json.Unmarshal(response.GetSettingsJson(), &settings))
	require.True(t, settings.Registration.Enabled)
	require.Equal(t, 3600, settings.TokenLifetimeSeconds)
	require.Equal(t, model.IdentityAuthorityKernel, settings.AuthorityState)
	require.Contains(t, settings.AdminMFA, "enabled")
	require.Positive(t, settings.LoginRateLimit.MaxAttempts)
}
