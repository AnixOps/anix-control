// Package identitybridge defines the narrow, kernel-owned operations exposed
// to the identity-platform package host. Package code never receives database
// or signing credentials; each operation is bound to one v2 route.
package identitybridge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/handler"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/gin-gonic/gin"
)

const packageID = "identity-platform"

const (
	identityMigrationRoute = "migration.identity-platform.001_identity_platform"
	identityMigrationSQL   = "CREATE TABLE IF NOT EXISTS identity_platform_projection (\n" +
		"  projection_key TEXT PRIMARY KEY,\n" +
		"  projection_value TEXT NOT NULL,\n" +
		"  updated_at BIGINT NOT NULL\n" +
		");\n"
)

// NewFactory creates isolated, generation-bound bridge sessions for
// identity-platform hosts.
func NewFactory(cfg *config.Config) (packagebridge.SessionFactory, error) {
	allowlist, err := NewAllowlist(cfg)
	if err != nil {
		return nil, err
	}
	return packagebridge.NewFactory(allowlist, packagebridge.DefaultRouteRegistry()), nil
}

// NewAllowlist returns the complete set of legacy-compatible operations that
// the identity package may invoke through a one-shot capability.
func NewAllowlist(cfg *config.Config) (*packagebridge.Allowlist, error) {
	if cfg == nil {
		return nil, errors.New("identity package bridge configuration is required")
	}
	authHandler := handler.NewAuthHandler(cfg)
	userHandler := handler.NewUserHandler()
	adminHandler := handler.NewAdminHandler()
	mfaHandler := handler.NewMFAHandler()
	inviteHandler := handler.NewInviteHandler()
	systemHandler := handler.NewSystemHandler()
	handlers := map[string]gin.HandlerFunc{
		"identity.auth.login":                               authHandler.Login,
		"identity.auth.register":                            authHandler.Register,
		"identity.user.profile.get":                         userHandler.GetProfile,
		"identity.user.dashboard.get":                       userHandler.GetDashboard,
		"identity.user.reset.post":                          adminHandler.ResetCompatFlow,
		"identity.admin.users.post":                         adminHandler.CreateUser,
		"identity.admin.users.get":                          adminHandler.GetUserList,
		"identity.admin.users.stats.get":                    adminHandler.GetUserStats,
		"identity.admin.users.id.get":                       adminHandler.GetUser,
		"identity.admin.users.id.put":                       adminHandler.UpdateUser,
		"identity.admin.users.id.delete":                    adminHandler.DeleteUser,
		"identity.admin.users.id.ban.post":                  adminHandler.BanUser,
		"identity.admin.users.id.unban.post":                adminHandler.UnbanUser,
		"identity.admin.users.id.reset_traffic.post":        adminHandler.ResetUserTraffic,
		"identity.admin.users.id.reset_subscribe.post":      adminHandler.ResetUserSubscribe,
		"identity.user.mfa.status.get":                      mfaHandler.GetStatus,
		"identity.user.mfa.totp.setup.post":                 mfaHandler.SetupTOTP,
		"identity.user.mfa.totp.enable.post":                mfaHandler.EnableTOTP,
		"identity.user.mfa.disable.post":                    mfaHandler.DisableMFA,
		"identity.user.mfa.verify.post":                     mfaHandler.VerifyMFA,
		"identity.user.mfa.backup_codes.regenerate.post":    mfaHandler.RegenerateBackupCodes,
		"identity.admin.mfa.config.get":                     mfaHandler.GetAdminConfig,
		"identity.admin.mfa.config.put":                     mfaHandler.UpdateAdminConfig,
		"identity.user.invite.get":                          inviteHandler.GetInviteInfo,
		"identity.user.invite.generate.post":                inviteHandler.GenerateCode,
		"identity.user.invite.commissions.get":              inviteHandler.GetCommissionRecords,
		"identity.user.invite.withdraw.post":                inviteHandler.RequestWithdraw,
		"identity.user.invite.withdrawals.get":              inviteHandler.GetWithdrawRecords,
		"identity.admin.invite.config.get":                  inviteHandler.GetConfig,
		"identity.admin.invite.config.put":                  inviteHandler.UpdateConfig,
		"identity.admin.invite.stats.get":                   inviteHandler.GetInviteStats,
		"identity.admin.invite.withdrawals.get":             inviteHandler.GetWithdrawals,
		"identity.admin.invite.withdrawals.id.process.post": inviteHandler.ProcessWithdraw,
		"identity.admin.system.configs.get":                 systemHandler.GetConfigs,
		"identity.admin.system.configs.key.get":             systemHandler.GetConfig,
		"identity.admin.system.configs.key.put":             systemHandler.SetConfig,
		"identity.admin.system.configs.key.delete":          systemHandler.DeleteConfig,
		"identity.admin.system.audit_logs.get":              systemHandler.GetAuditLogs,
		"identity.admin.system.backup.config.get":           systemHandler.GetBackupConfig,
		"identity.admin.system.backup.config.put":           systemHandler.UpdateBackupConfig,
		"identity.admin.system.backup.post":                 systemHandler.CreateBackup,
		"identity.admin.system.backups.get":                 systemHandler.ListBackups,
		"identity.admin.system.backup.stats.get":            systemHandler.GetBackupStats,
		"identity.admin.system.backups.id.delete":           systemHandler.DeleteBackup,
		"identity.admin.system.backups.id.restore.post":     systemHandler.RestoreBackup,
	}
	operations := make([]packagebridge.Operation, 0, len(handlers))
	for routeID, routeHandler := range handlers {
		operations = append(operations, packagebridge.Operation{
			PackageID: packageID, RouteID: routeID, Name: routeID,
			Handler: packagebridge.NewHTTPAdapter(routeHandler),
		})
	}
	operations = append(operations, packagebridge.Operation{
		PackageID: packageID, RouteID: identityMigrationRoute, Name: identityMigrationRoute,
		Handler: identityMigrationHandler,
	})
	return packagebridge.NewAllowlistWithFallback(packagebridge.DefaultRouteRegistry(), operations...)
}

func identityMigrationHandler(ctx context.Context, call packagebridge.Call) (packagebridge.Response, error) {
	if call.Operation != identityMigrationRoute || call.Request.RouteID != identityMigrationRoute || len(call.Payload) != 0 {
		return packagebridge.Response{}, packagebridge.ErrCapabilityRejected
	}
	db := database.Get()
	if db == nil {
		return packagebridge.Response{}, errors.New("identity migration database is unavailable")
	}
	if err := db.WithContext(ctx).Exec(identityMigrationSQL).Error; err != nil {
		return packagebridge.Response{}, err
	}
	digest := sha256.Sum256([]byte(identityMigrationSQL))
	body, err := json.Marshal(struct {
		Checkpoint       string `json:"checkpoint"`
		ValidationDigest string `json:"validation_digest"`
		Complete         bool   `json:"complete"`
	}{
		Checkpoint: "identity-platform/001", ValidationDigest: hex.EncodeToString(digest[:]), Complete: true,
	})
	if err != nil {
		return packagebridge.Response{}, err
	}
	return packagebridge.Response{StatusCode: 200, Body: body}, nil
}
