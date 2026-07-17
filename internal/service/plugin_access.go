package service

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
)

const (
	PluginPermissionModeLegacy        = "legacy"
	PluginPermissionModeAuthoritative = "authoritative"
	PluginPermissionModeMixed         = "mixed"
)

// ActorPluginAccess is the kernel-authoritative plugin permission view for one
// actor. A legacy administrator remains unrestricted for plugins that have no
// explicit grant, while each plugin with at least one grant is evaluated
// fail-closed. This permits incremental migration of plugin permissions.
type ActorPluginAccess struct {
	Unrestricted      bool
	LegacyAdmin       bool
	Permissions       []string
	RestrictedPlugins []string
	byPlugin          map[string]map[string]struct{}
	restricted        map[string]struct{}
}

// NewFailClosedActorPluginAccess is used when authentication has already
// committed but the optional plugin authorization lookup is temporarily
// unavailable. It never grants access and lets the caller return a valid
// session that can retry its profile fetch.
func NewFailClosedActorPluginAccess() *ActorPluginAccess {
	return newEmptyActorPluginAccess(false)
}

func newEmptyActorPluginAccess(legacyAdmin bool) *ActorPluginAccess {
	access := &ActorPluginAccess{
		LegacyAdmin:       legacyAdmin,
		Permissions:       []string{},
		RestrictedPlugins: []string{},
		byPlugin:          make(map[string]map[string]struct{}),
		restricted:        make(map[string]struct{}),
	}
	access.Unrestricted = legacyAdmin
	return access
}

func (access ActorPluginAccess) PermissionMode() string {
	if access.Unrestricted {
		return PluginPermissionModeLegacy
	}
	if access.LegacyAdmin && len(access.restricted) > 0 {
		return PluginPermissionModeMixed
	}
	return PluginPermissionModeAuthoritative
}

func (access ActorPluginAccess) ProfilePermissions() []string {
	if access.Unrestricted {
		return nil
	}
	return append([]string{}, access.Permissions...)
}

func (access ActorPluginAccess) RestrictedPluginList() []string {
	return append([]string{}, access.RestrictedPlugins...)
}

// ProfileRestrictedPluginList intentionally hides the catalog of configured
// plugin grants from non-administrators. They still receive their effective
// permissions, but not a cross-tenant enumeration of restricted plugins.
func (access ActorPluginAccess) ProfileRestrictedPluginList() []string {
	if !access.LegacyAdmin {
		return []string{}
	}
	return access.RestrictedPluginList()
}

func (access ActorPluginAccess) Allows(pluginID, permission string) bool {
	if access.Unrestricted || (access.LegacyAdmin && !access.isRestricted(pluginID)) {
		return true
	}
	permissions := access.byPlugin[pluginID]
	if len(permissions) == 0 {
		return false
	}
	_, wildcard := permissions["*"]
	_, namespacedWildcard := permissions[pluginID+".*"]
	_, exact := permissions[permission]
	return wildcard || namespacedWildcard || exact
}

func (access ActorPluginAccess) HasAnyPermission(pluginID string) bool {
	if access.Unrestricted || (access.LegacyAdmin && !access.isRestricted(pluginID)) {
		return true
	}
	return len(access.byPlugin[pluginID]) > 0
}

func (access ActorPluginAccess) isRestricted(pluginID string) bool {
	_, ok := access.restricted[strings.TrimSpace(pluginID)]
	return ok
}

// ResolveActorPluginAccess reads enabled access-group memberships and their
// plugin_api grants. Permission arrays and truthy object entries are unioned,
// de-duplicated and sorted for deterministic API responses.
func ResolveActorPluginAccess(db *gorm.DB, actorID uint, legacyAdmin bool) (*ActorPluginAccess, error) {
	if db == nil {
		return nil, errors.New("database is not initialized")
	}
	var grantRows []model.ResourceGrant
	if err := db.Model(&model.ResourceGrant{}).Select("resource_id", "permissions").
		Where("resource_type = ?", PluginAPIGrantResourceType).
		Find(&grantRows).Error; err != nil {
		// The v2 compatibility surface can be exercised before the optional
		// Kernel migration (for example by migration/smoke tooling). With no
		// grant table, no plugin-specific authorization exists: regular users
		// remain fail-closed while legacy administrators retain their pre-Kernel
		// behavior. Other database failures must remain visible to the caller.
		if isMissingKernelGrantTableError(err) {
			return newEmptyActorPluginAccess(legacyAdmin), nil
		}
		return nil, err
	}

	access := &ActorPluginAccess{
		Permissions: make([]string, 0), byPlugin: make(map[string]map[string]struct{}),
		restricted: make(map[string]struct{}),
	}
	for _, row := range grantRows {
		pluginID := strings.TrimSpace(row.ResourceID)
		if pluginID != "" && len(validPluginGrantPermissions(pluginID, row.Permissions)) > 0 {
			access.restricted[pluginID] = struct{}{}
		}
	}
	access.LegacyAdmin = legacyAdmin
	for pluginID := range access.restricted {
		access.RestrictedPlugins = append(access.RestrictedPlugins, pluginID)
	}
	sort.Strings(access.RestrictedPlugins)
	if len(access.restricted) == 0 {
		access.Unrestricted = legacyAdmin
		return access, nil
	}
	if actorID == 0 {
		return access, nil
	}

	var grants []model.ResourceGrant
	if err := db.Model(&model.ResourceGrant{}).
		Joins("JOIN v3_kernel_access_group ON v3_kernel_access_group.id = v3_kernel_resource_grant.group_id AND v3_kernel_access_group.enabled = ?", true).
		Joins("JOIN v3_kernel_access_group_user ON v3_kernel_access_group_user.group_id = v3_kernel_access_group.id AND v3_kernel_access_group_user.user_id = ?", actorID).
		Where("v3_kernel_resource_grant.resource_type = ?", PluginAPIGrantResourceType).
		Order("v3_kernel_resource_grant.resource_id, v3_kernel_resource_grant.id").
		Find(&grants).Error; err != nil {
		return nil, err
	}

	allPermissions := make(map[string]struct{})
	for _, grant := range grants {
		pluginID := strings.TrimSpace(grant.ResourceID)
		if pluginID == "" {
			continue
		}
		permissions := validPluginGrantPermissions(pluginID, grant.Permissions)
		if len(permissions) == 0 {
			continue
		}
		pluginPermissions := access.byPlugin[pluginID]
		if pluginPermissions == nil {
			pluginPermissions = make(map[string]struct{})
			access.byPlugin[pluginID] = pluginPermissions
		}
		for _, permission := range permissions {
			if !pluginPermissionBelongsTo(pluginID, permission) {
				continue
			}
			pluginPermissions[permission] = struct{}{}
			profilePermission := permission
			if permission == "*" {
				profilePermission = pluginID + ".*"
			}
			allPermissions[profilePermission] = struct{}{}
		}
	}
	for permission := range allPermissions {
		access.Permissions = append(access.Permissions, permission)
	}
	sort.Strings(access.Permissions)
	return access, nil
}

func isMissingKernelGrantTableError(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "no such table") ||
		(strings.Contains(message, "42p01") && strings.Contains(message, "v3_kernel_resource_grant"))
}

// ValidatePluginAPIGrantPermissions rejects malformed or cross-plugin grants
// before they enter the kernel authorization table.
func ValidatePluginAPIGrantPermissions(pluginID, raw string) error {
	pluginID = strings.TrimSpace(pluginID)
	if pluginID == "" {
		return errors.New("plugin resource_id is required")
	}
	permissions := parseResourceGrantPermissions(raw)
	if len(permissions) == 0 {
		return errors.New("plugin_api grant must contain at least one enabled permission")
	}
	for _, permission := range permissions {
		if !pluginPermissionBelongsTo(pluginID, permission) {
			return errors.New("plugin_api permission must belong to its resource_id namespace")
		}
	}
	return nil
}

func validPluginGrantPermissions(pluginID, raw string) []string {
	permissions := parseResourceGrantPermissions(raw)
	valid := make([]string, 0, len(permissions))
	for _, permission := range permissions {
		if pluginPermissionBelongsTo(pluginID, permission) {
			valid = append(valid, permission)
		}
	}
	return valid
}

func pluginPermissionBelongsTo(pluginID, permission string) bool {
	permission = strings.TrimSpace(permission)
	if permission == "*" {
		return true
	}
	return strings.HasPrefix(permission, strings.TrimSpace(pluginID)+".")
}

func parseResourceGrantPermissions(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	permissions := make(map[string]struct{})
	var list []string
	if err := json.Unmarshal([]byte(raw), &list); err == nil {
		for _, permission := range list {
			if permission = strings.TrimSpace(permission); permission != "" {
				permissions[permission] = struct{}{}
			}
		}
		return sortedPermissionKeys(permissions)
	}

	var object map[string]any
	if err := json.Unmarshal([]byte(raw), &object); err != nil {
		return nil
	}
	for permission, value := range object {
		permission = strings.TrimSpace(permission)
		if permission != "" && resourceGrantValueEnabled(value) {
			permissions[permission] = struct{}{}
		}
	}
	return sortedPermissionKeys(permissions)
}

func sortedPermissionKeys(permissions map[string]struct{}) []string {
	result := make([]string, 0, len(permissions))
	for permission := range permissions {
		result = append(result, permission)
	}
	sort.Strings(result)
	return result
}

func resourceGrantValueEnabled(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		normalized := strings.ToLower(strings.TrimSpace(typed))
		return normalized == "true" || normalized == "1" || normalized == "yes"
	case float64:
		return typed != 0
	default:
		return value != nil
	}
}
