package main

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"

	"github.com/AnixOps/anix-control/v4/pkg/packagebridgesdk"
	"github.com/AnixOps/anix-control/v4/pkg/pluginhostsdk"
)

const (
	identityLoginRoute     = "identity.auth.login"
	identityRegisterRoute  = "identity.auth.register"
	identityMigrationRoute = "migration.identity-platform.001_identity_platform"
)

var identityRoutes = map[string]struct{}{
	"identity.admin.invite.config.get":                  {},
	"identity.admin.invite.config.put":                  {},
	"identity.admin.invite.stats.get":                   {},
	"identity.admin.invite.withdrawals.get":             {},
	"identity.admin.invite.withdrawals.id.process.post": {},
	"identity.admin.mfa.config.get":                     {},
	"identity.admin.mfa.config.put":                     {},
	"identity.admin.system.audit_logs.get":              {},
	"identity.admin.system.backup.config.get":           {},
	"identity.admin.system.backup.config.put":           {},
	"identity.admin.system.backup.post":                 {},
	"identity.admin.system.backup.stats.get":            {},
	"identity.admin.system.backups.get":                 {},
	"identity.admin.system.backups.id.delete":           {},
	"identity.admin.system.backups.id.restore.post":     {},
	"identity.admin.system.configs.get":                 {},
	"identity.admin.system.configs.key.delete":          {},
	"identity.admin.system.configs.key.get":             {},
	"identity.admin.system.configs.key.put":             {},
	"identity.admin.users.get":                          {},
	"identity.admin.users.id.ban.post":                  {},
	"identity.admin.users.id.delete":                    {},
	"identity.admin.users.id.get":                       {},
	"identity.admin.users.id.put":                       {},
	"identity.admin.users.id.reset_subscribe.post":      {},
	"identity.admin.users.id.reset_traffic.post":        {},
	"identity.admin.users.id.unban.post":                {},
	"identity.admin.users.post":                         {},
	"identity.admin.users.stats.get":                    {},
	"identity.auth.login":                               {},
	"identity.auth.register":                            {},
	"identity.user.dashboard.get":                       {},
	"identity.user.invite.commissions.get":              {},
	"identity.user.invite.generate.post":                {},
	"identity.user.invite.get":                          {},
	"identity.user.invite.withdraw.post":                {},
	"identity.user.invite.withdrawals.get":              {},
	"identity.user.mfa.backup_codes.regenerate.post":    {},
	"identity.user.mfa.disable.post":                    {},
	"identity.user.mfa.status.get":                      {},
	"identity.user.mfa.totp.enable.post":                {},
	"identity.user.mfa.totp.setup.post":                 {},
	"identity.user.mfa.verify.post":                     {},
	"identity.user.profile.get":                         {},
	"identity.user.reset.post":                          {},
}

type bridgeClient interface {
	Invoke(context.Context, []byte, string, []byte) (packagebridgesdk.Response, error)
}

type identityService struct {
	bridge   bridgeClient
	leaseID  string
	draining atomic.Bool
}

func newIdentityService(bridge bridgeClient, leaseID string) *identityService {
	return &identityService{bridge: bridge, leaseID: leaseID}
}

func (s *identityService) Dispatch(ctx context.Context, request pluginhostsdk.DispatchRequest) (pluginhostsdk.DispatchResponse, error) {
	if s == nil || s.bridge == nil || s.draining.Load() {
		return pluginhostsdk.DispatchResponse{}, errors.New("identity package is unavailable")
	}
	if _, exists := identityRoutes[request.RouteID]; !exists {
		return pluginhostsdk.DispatchResponse{}, errors.New("identity package route is unsupported")
	}
	if len(request.BridgeCapability) != 32 {
		return pluginhostsdk.DispatchResponse{}, errors.New("identity package bridge capability is required")
	}
	response, err := s.bridge.Invoke(ctx, request.BridgeCapability, request.RouteID, request.RequestBody)
	if err != nil {
		return pluginhostsdk.DispatchResponse{}, err
	}
	headers := make([]pluginhostsdk.Header, len(response.Headers))
	for index, header := range response.Headers {
		headers[index] = pluginhostsdk.Header{Name: header.Name, Value: header.Value}
	}
	return pluginhostsdk.DispatchResponse{
		StatusCode: response.StatusCode, ResponseBody: response.Body, Headers: headers,
	}, nil
}

func (s *identityService) Migrate(ctx context.Context, request pluginhostsdk.MigrationRequest) (pluginhostsdk.MigrationResponse, error) {
	if s == nil || s.bridge == nil || s.draining.Load() {
		return pluginhostsdk.MigrationResponse{}, errors.New("identity package is unavailable")
	}
	if request.MigrationID != "001_identity_platform" || len(request.BridgeCapability) != 32 {
		return pluginhostsdk.MigrationResponse{}, errors.New("identity package migration is unsupported")
	}
	response, err := s.bridge.Invoke(ctx, request.BridgeCapability, identityMigrationRoute, nil)
	if err != nil {
		return pluginhostsdk.MigrationResponse{}, err
	}
	var result struct {
		Checkpoint       string `json:"checkpoint"`
		ValidationDigest string `json:"validation_digest"`
		Complete         bool   `json:"complete"`
	}
	if response.StatusCode != 200 || json.Unmarshal(response.Body, &result) != nil || result.Checkpoint == "" || result.ValidationDigest == "" || !result.Complete {
		return pluginhostsdk.MigrationResponse{}, errors.New("identity package migration bridge response is invalid")
	}
	return pluginhostsdk.MigrationResponse{
		Checkpoint: result.Checkpoint, ValidationDigest: result.ValidationDigest, Complete: result.Complete,
	}, nil
}

func (s *identityService) Health(context.Context) (pluginhostsdk.HealthResponse, error) {
	if s == nil {
		return pluginhostsdk.HealthResponse{Healthy: false}, nil
	}
	if s.bridge == nil || s.draining.Load() {
		return pluginhostsdk.HealthResponse{Healthy: false, LeaseID: s.leaseID}, nil
	}
	return pluginhostsdk.HealthResponse{Healthy: true, LeaseID: s.leaseID, DetailsJSON: `{"bridge":"required"}`}, nil
}

func (s *identityService) Drain(context.Context) (pluginhostsdk.DrainResponse, error) {
	if s == nil {
		return pluginhostsdk.DrainResponse{}, errors.New("identity package is unavailable")
	}
	s.draining.Store(true)
	return pluginhostsdk.DrainResponse{Drained: true}, nil
}
