package service

import (
	"sort"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

// adminCapabilityRoutes are the capabilities the admin app reads: each is
// a package's control route. forward.v4 is the forward package's v4 API,
// which the forwarding pages need.
var adminCapabilityRoutes = map[string]struct{ pluginID, route string }{
	"forward.v4": {pluginID: "forward", route: "/api/v4/plugins/forward/*"},
}

// AdminCapabilities answers the capabilities whose package is enabled and
// declares the route, and whose api permission the actor holds.
func AdminCapabilities(db *gorm.DB, actorID uint, legacyAdmin bool) ([]string, error) {
	access, err := ResolveActorPluginAccess(db, actorID, legacyAdmin)
	if err != nil {
		return nil, err
	}
	out := []string{}
	for name, wanted := range adminCapabilityRoutes {
		if !access.Allows(wanted.pluginID, PluginAPIPermission(wanted.pluginID)) {
			continue
		}
		var installation model.PluginInstallation
		err := db.Where("plugin_id = ? AND target = ? AND enabled = ? AND state IN ?", wanted.pluginID, "control", true, []string{"enabled", "healthy"}).
			Limit(1).Find(&installation).Error
		if err != nil {
			return nil, err
		}
		if installation.ID == 0 {
			continue
		}
		var release model.PluginRelease
		if err := db.Where("plugin_id = ? AND version = ?", wanted.pluginID, installation.ObservedVersion).Limit(1).Find(&release).Error; err != nil {
			return nil, err
		}
		if release.ID == 0 {
			continue
		}
		manifest, err := DecodePluginReleaseManifest(release)
		if err != nil {
			return nil, err
		}
		if containsPluginString(manifest.ControlRoutes, wanted.route) {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out, nil
}
