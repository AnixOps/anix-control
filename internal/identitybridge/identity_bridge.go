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
	"io"
	"net/http"
	"net/url"
	"strings"

	compatv2 "github.com/AnixOps/anix-control/v4/internal/compat/v2"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/handler"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/AnixOps/anix-control/v4/internal/packagestore"
	"github.com/AnixOps/anix-control/v4/internal/plugincontrol"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
)

const packageID = "identity-platform"

const machineTelemetryPackageID = "machine-telemetry"

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
	operations, err := newHostOperations(cfg)
	if err != nil {
		return nil, err
	}
	return packagebridge.NewFactory(allowlist, packagebridge.DefaultRouteRegistry()).WithSessionOptions(packagebridge.SessionOptions{
		MaxResponseBodyBytes: cfg.Plugins.ControlHostResponseBodyLimit(),
		MaxRequestBodyBytes:  cfg.Plugins.ControlHostRequestBodyLimit(),
		HostOperations:       operations,
	}), nil
}

// newHostOperations serves the session-scoped RPCs from the kernel database,
// including package storage leases derived from the kernel's connection.
func newHostOperations(cfg *config.Config) (service.PackageHostOperations, error) {
	db := database.Get()
	operations := service.PackageHostOperations{DB: db}
	if strings.TrimSpace(cfg.Plugins.OfficialPublicKey) != "" {
		publicKey, err := service.ParseOfficialPluginPublicKey(cfg.Plugins.OfficialPublicKey)
		if err != nil {
			return operations, err
		}
		operations.FallbackPublicKey = publicKey
	}
	source, err := database.PackageStorageSource(&cfg.Database)
	if err != nil {
		return operations, err
	}
	operations.Storage = &packagestore.Store{
		DB: db, Driver: cfg.Database.Driver, DSN: source,
		RemoteDatabaseHost: cfg.ModuleRuntime.DatabaseHost, Leases: packagestore.NewLeaseCache(),
	}
	return operations, nil
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
	handlers := map[string]gin.HandlerFunc{
		"identity.auth.login":                            authHandler.Login,
		"identity.auth.register":                         authHandler.Register,
		"identity.user.profile.get":                      userHandler.GetProfile,
		"identity.user.dashboard.get":                    userHandler.GetDashboard,
		"identity.admin.users.post":                      adminHandler.CreateUser,
		"identity.admin.users.get":                       adminHandler.GetUserList,
		"identity.admin.users.stats.get":                 adminHandler.GetUserStats,
		"identity.admin.users.id.get":                    adminHandler.GetUser,
		"identity.admin.users.id.put":                    adminHandler.UpdateUser,
		"identity.admin.users.id.delete":                 adminHandler.DeleteUser,
		"identity.admin.users.id.ban.post":               adminHandler.BanUser,
		"identity.admin.users.id.unban.post":             adminHandler.UnbanUser,
		"identity.admin.users.id.reset_traffic.post":     adminHandler.ResetUserTraffic,
		"identity.admin.users.id.reset_subscribe.post":   adminHandler.ResetUserSubscribe,
		"identity.user.mfa.status.get":                   mfaHandler.GetStatus,
		"identity.user.mfa.totp.setup.post":              mfaHandler.SetupTOTP,
		"identity.user.mfa.totp.enable.post":             mfaHandler.EnableTOTP,
		"identity.user.mfa.disable.post":                 mfaHandler.DisableMFA,
		"identity.user.mfa.verify.post":                  mfaHandler.VerifyMFA,
		"identity.user.mfa.backup_codes.regenerate.post": mfaHandler.RegenerateBackupCodes,
		"identity.admin.mfa.config.get":                  mfaHandler.GetAdminConfig,
		"identity.admin.mfa.config.put":                  mfaHandler.UpdateAdminConfig,
		"identity.user.invite.get":                       inviteHandler.GetInviteInfo,
		"identity.user.invite.generate.post":             inviteHandler.GenerateCode,
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
	machineTelemetryRouteID := service.PluginControlBridgeRouteID(machineTelemetryPackageID, plugincontrol.MachineTelemetryStatusRoute)
	operations = append(operations, packagebridge.Operation{
		PackageID: machineTelemetryPackageID, RouteID: machineTelemetryRouteID, Name: machineTelemetryRouteID,
		Handler: machineTelemetryStatusHandler,
	})
	routes := packagebridge.DefaultRouteRegistry()
	operations = append(operations, movedRouteOperations(routes)...)
	return packagebridge.NewAllowlistWithFallback(routes, operations...)
}

// movedRouteOperations keeps the old route ids of routes that moved to other
// packages callable by identity-platform releases that still declare them.
// Each call runs the new owner's handler, resolved when the call arrives
// because the router registers it.
func movedRouteOperations(routes packagebridge.OperationResolver) []packagebridge.Operation {
	var operations []packagebridge.Operation
	for _, moved := range compatv2.MovedRoutes() {
		if moved.FromPackage != packageID {
			continue
		}
		operations = append(operations, packagebridge.Operation{
			PackageID: packageID, RouteID: moved.FromRoute, Name: moved.FromRoute,
			Handler: func(ctx context.Context, call packagebridge.Call) (packagebridge.Response, error) {
				handler, ok := routes.Resolve(moved.ToPackage, moved.ToRoute, moved.ToRoute)
				if !ok {
					return packagebridge.Response{}, packagebridge.ErrCapabilityRejected
				}
				return handler(ctx, call)
			},
		})
	}
	return operations
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

type machineTelemetryRouteMetadata struct {
	Path  string              `json:"path"`
	Query map[string][]string `json:"query"`
}

func machineTelemetryStatusHandler(ctx context.Context, call packagebridge.Call) (packagebridge.Response, error) {
	routeID := service.PluginControlBridgeRouteID(machineTelemetryPackageID, plugincontrol.MachineTelemetryStatusRoute)
	if call.Host.PackageID != machineTelemetryPackageID || call.Request.RouteID != routeID || call.Operation != routeID || call.Request.Method != http.MethodGet {
		return packagebridge.Response{}, packagebridge.ErrCapabilityRejected
	}
	metadata, err := decodeMachineTelemetryRouteMetadata(call.Request.MetadataJSON)
	if err != nil {
		return packagebridge.Response{}, packagebridge.ErrCapabilityRejected
	}
	if metadata.Path != plugincontrol.MachineTelemetryStatusRoute {
		return packagebridge.Response{}, packagebridge.ErrCapabilityRejected
	}
	executor := plugincontrol.NewMachineTelemetryExecutorVersion(database.Get(), call.Host.Version)
	response, err := executor.HandleRoute(ctx, plugincontrol.RouteRequest{
		Method: call.Request.Method, Path: metadata.Path, Query: url.Values(metadata.Query),
	})
	if err != nil {
		return packagebridge.Response{}, err
	}
	body, err := json.Marshal(struct {
		Data any `json:"data"`
	}{Data: response.Data})
	if err != nil {
		return packagebridge.Response{}, err
	}
	statusCode, err := packageBridgeHTTPStatusCode(response.Status)
	if err != nil {
		return packagebridge.Response{}, err
	}
	return packagebridge.Response{
		StatusCode: statusCode, Body: body,
		Headers: []packagebridge.Header{{Name: "Content-Type", Value: "application/json; charset=utf-8"}},
	}, nil
}

func packageBridgeHTTPStatusCode(statusCode int) (uint32, error) {
	if statusCode < http.StatusContinue || statusCode > 599 {
		return 0, errors.New("package bridge response status is invalid")
	}
	return uint32(statusCode), nil
}

func decodeMachineTelemetryRouteMetadata(raw []byte) (machineTelemetryRouteMetadata, error) {
	var metadata machineTelemetryRouteMetadata
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&metadata); err != nil {
		return machineTelemetryRouteMetadata{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return machineTelemetryRouteMetadata{}, errors.New("machine telemetry route metadata is invalid")
	}
	if len(metadata.Query) > 64 {
		return machineTelemetryRouteMetadata{}, errors.New("machine telemetry route query is invalid")
	}
	for key, values := range metadata.Query {
		if key == "" || len(key) > 256 || len(values) > 32 {
			return machineTelemetryRouteMetadata{}, errors.New("machine telemetry route query is invalid")
		}
		for _, value := range values {
			if len(value) > 4096 || strings.ContainsAny(value, "\r\n\x00") {
				return machineTelemetryRouteMetadata{}, errors.New("machine telemetry route query is invalid")
			}
		}
	}
	return metadata, nil
}
