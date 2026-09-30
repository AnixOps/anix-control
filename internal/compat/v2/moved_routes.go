package v2

// MovedRoute is a v2 route whose owning package changed. The HTTP path and
// behaviour stay the same; only the package and its route id change.
//
// Old releases of the previous owner still declare the route, so while such
// a release is active:
//   - the kernel keeps accepting its old route id (see identitybridge), and
//   - route resolution prefers the new owner when both declare the route, so
//     the new owner can be installed before the old one is upgraded.
type MovedRoute struct {
	Method      string
	LegacyPath  string
	FromPackage string
	FromRoute   string
	ToPackage   string
	ToRoute     string
}

// movedRoutes: system configuration, audit and backup moved to platform;
// affiliate commissions, withdrawals and settings to affiliate; the Flux
// reset-flow endpoint to forward (4.1).
var movedRoutes = []MovedRoute{
	{Method: "GET", LegacyPath: "/api/v2/admin/invite/config", FromPackage: "identity-platform", FromRoute: "identity.admin.invite.config.get", ToPackage: "affiliate", ToRoute: "affiliate.admin.invite.config.get"},
	{Method: "PUT", LegacyPath: "/api/v2/admin/invite/config", FromPackage: "identity-platform", FromRoute: "identity.admin.invite.config.put", ToPackage: "affiliate", ToRoute: "affiliate.admin.invite.config.put"},
	{Method: "GET", LegacyPath: "/api/v2/admin/invite/stats", FromPackage: "identity-platform", FromRoute: "identity.admin.invite.stats.get", ToPackage: "affiliate", ToRoute: "affiliate.admin.invite.stats.get"},
	{Method: "GET", LegacyPath: "/api/v2/admin/invite/withdrawals", FromPackage: "identity-platform", FromRoute: "identity.admin.invite.withdrawals.get", ToPackage: "affiliate", ToRoute: "affiliate.admin.invite.withdrawals.get"},
	{Method: "POST", LegacyPath: "/api/v2/admin/invite/withdrawals/:id/process", FromPackage: "identity-platform", FromRoute: "identity.admin.invite.withdrawals.id.process.post", ToPackage: "affiliate", ToRoute: "affiliate.admin.invite.withdrawals.id.process.post"},
	{Method: "GET", LegacyPath: "/api/v2/user/invite/commissions", FromPackage: "identity-platform", FromRoute: "identity.user.invite.commissions.get", ToPackage: "affiliate", ToRoute: "affiliate.user.invite.commissions.get"},
	{Method: "POST", LegacyPath: "/api/v2/user/invite/withdraw", FromPackage: "identity-platform", FromRoute: "identity.user.invite.withdraw.post", ToPackage: "affiliate", ToRoute: "affiliate.user.invite.withdraw.post"},
	{Method: "GET", LegacyPath: "/api/v2/user/invite/withdrawals", FromPackage: "identity-platform", FromRoute: "identity.user.invite.withdrawals.get", ToPackage: "affiliate", ToRoute: "affiliate.user.invite.withdrawals.get"},
	{Method: "POST", LegacyPath: "/api/v2/user/reset", FromPackage: "identity-platform", FromRoute: "identity.user.reset.post", ToPackage: "forward", ToRoute: "forward.user.reset.post"},
	{Method: "GET", LegacyPath: "/api/v2/admin/system/audit-logs", FromPackage: "identity-platform", FromRoute: "identity.admin.system.audit_logs.get", ToPackage: "platform", ToRoute: "platform.admin.system.audit_logs.get"},
	{Method: "POST", LegacyPath: "/api/v2/admin/system/backup", FromPackage: "identity-platform", FromRoute: "identity.admin.system.backup.post", ToPackage: "platform", ToRoute: "platform.admin.system.backup.post"},
	{Method: "GET", LegacyPath: "/api/v2/admin/system/backup/config", FromPackage: "identity-platform", FromRoute: "identity.admin.system.backup.config.get", ToPackage: "platform", ToRoute: "platform.admin.system.backup.config.get"},
	{Method: "PUT", LegacyPath: "/api/v2/admin/system/backup/config", FromPackage: "identity-platform", FromRoute: "identity.admin.system.backup.config.put", ToPackage: "platform", ToRoute: "platform.admin.system.backup.config.put"},
	{Method: "GET", LegacyPath: "/api/v2/admin/system/backup/stats", FromPackage: "identity-platform", FromRoute: "identity.admin.system.backup.stats.get", ToPackage: "platform", ToRoute: "platform.admin.system.backup.stats.get"},
	{Method: "GET", LegacyPath: "/api/v2/admin/system/backups", FromPackage: "identity-platform", FromRoute: "identity.admin.system.backups.get", ToPackage: "platform", ToRoute: "platform.admin.system.backups.get"},
	{Method: "DELETE", LegacyPath: "/api/v2/admin/system/backups/:id", FromPackage: "identity-platform", FromRoute: "identity.admin.system.backups.id.delete", ToPackage: "platform", ToRoute: "platform.admin.system.backups.id.delete"},
	{Method: "POST", LegacyPath: "/api/v2/admin/system/backups/:id/restore", FromPackage: "identity-platform", FromRoute: "identity.admin.system.backups.id.restore.post", ToPackage: "platform", ToRoute: "platform.admin.system.backups.id.restore.post"},
	{Method: "GET", LegacyPath: "/api/v2/admin/system/configs", FromPackage: "identity-platform", FromRoute: "identity.admin.system.configs.get", ToPackage: "platform", ToRoute: "platform.admin.system.configs.get"},
	{Method: "DELETE", LegacyPath: "/api/v2/admin/system/configs/:key", FromPackage: "identity-platform", FromRoute: "identity.admin.system.configs.key.delete", ToPackage: "platform", ToRoute: "platform.admin.system.configs.key.delete"},
	{Method: "GET", LegacyPath: "/api/v2/admin/system/configs/:key", FromPackage: "identity-platform", FromRoute: "identity.admin.system.configs.key.get", ToPackage: "platform", ToRoute: "platform.admin.system.configs.key.get"},
	{Method: "PUT", LegacyPath: "/api/v2/admin/system/configs/:key", FromPackage: "identity-platform", FromRoute: "identity.admin.system.configs.key.put", ToPackage: "platform", ToRoute: "platform.admin.system.configs.key.put"},
}

// MovedRoutes returns a copy of the route moves the kernel still honours.
func MovedRoutes() []MovedRoute {
	return append([]MovedRoute(nil), movedRoutes...)
}

// preferMovedRouteOwners drops a match that is an old owner's declaration of a
// moved route when the new owner also declares it.
func preferMovedRouteOwners(matches []Route) []Route {
	if len(matches) < 2 {
		return matches
	}
	kept := matches[:0:0]
	for _, match := range matches {
		if superseded(match, matches) {
			continue
		}
		kept = append(kept, match)
	}
	return kept
}

func superseded(match Route, matches []Route) bool {
	for _, moved := range movedRoutes {
		if moved.FromPackage != match.PackageID || moved.FromRoute != match.PackageRoute {
			continue
		}
		for _, other := range matches {
			if other.PackageID == moved.ToPackage && other.PackageRoute == moved.ToRoute {
				return true
			}
		}
	}
	return false
}
