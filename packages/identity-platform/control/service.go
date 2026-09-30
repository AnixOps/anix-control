package main

import (
	"log"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
)

const identityMigrationRoute = "migration.identity-platform.001_identity_platform"

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

// newIdentityService returns the identity host's router: only the declared
// identity routes and the 001_identity_platform migration are accepted, and
// every route passes through the bridge (identity stays kernel-owned).
func newIdentityService(bridge pluginhostsdk.RouterBridge, leaseID string) (*pluginhostsdk.Router, error) {
	return pluginhostsdk.NewRouter(pluginhostsdk.RouterConfig{
		PackageID: "identity-platform", LeaseID: leaseID, Bridge: bridge, Logf: log.Printf,
		AllowRoute: func(routeID string) bool {
			_, allowed := identityRoutes[routeID]
			return allowed
		},
		MigrationOperation: func(migrationID string) (string, bool) {
			return identityMigrationRoute, migrationID == "001_identity_platform"
		},
	})
}
