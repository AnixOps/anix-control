package service

import (
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"gorm.io/gorm"
)

// IdentityPlatformPackageID is the package whose group A the identity
// module serves once it is authoritative.
const IdentityPlatformPackageID = "identity-platform"

// IdentityGroupARoutes are identity-platform's routes that move from the
// kernel's legacy handlers to identity together, with the authority state:
// login, registration, MFA, the admin MFA configuration and the admin
// account writes.
var IdentityGroupARoutes = []string{
	"identity.auth.login",
	"identity.auth.register",
	"identity.user.mfa.status.get",
	"identity.user.mfa.totp.setup.post",
	"identity.user.mfa.totp.enable.post",
	"identity.user.mfa.disable.post",
	"identity.user.mfa.verify.post",
	"identity.user.mfa.backup_codes.regenerate.post",
	"identity.admin.mfa.config.get",
	"identity.admin.mfa.config.put",
	"identity.admin.users.post",
	"identity.admin.users.id.put",
	"identity.admin.users.id.ban.post",
	"identity.admin.users.id.unban.post",
	"identity.admin.users.id.delete",
}

// IdentityAuthorityState returns the identity authority state; no row
// means the kernel.
func IdentityAuthorityState(db *gorm.DB) (string, error) {
	var authority model.IdentityAuthority
	err := db.Select("state").Where("id = ?", 1).Take(&authority).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.IdentityAuthorityKernel, nil
	}
	return authority.State, err
}

// IdentityAuthoritative reports whether identity owns credentials in state.
func IdentityAuthoritative(state string) bool {
	return state == model.IdentityAuthorityIdentity || state == model.IdentityAuthorityFinalized
}

// validateIdentityGroupA keeps identity-platform's group A consistent with
// the authority: its routes are native together, and exactly while identity
// is authoritative. Anything else would let the legacy handlers and identity
// both change credentials. The cutover and rollback change both at once.
func validateIdentityGroupA(tx *gorm.DB, packageID string, modes map[string]string) error {
	if packageID != IdentityPlatformPackageID {
		return nil
	}
	native := 0
	for _, route := range IdentityGroupARoutes {
		if modes[route] == packagebridge.RouteModeNative {
			native++
		}
	}
	state, err := IdentityAuthorityState(tx)
	if err != nil {
		return err
	}
	switch {
	case native != 0 && native != len(IdentityGroupARoutes):
		return fmt.Errorf("identity group A routes switch together: %d of %d are native", native, len(IdentityGroupARoutes))
	case native != 0 && !IdentityAuthoritative(state):
		return fmt.Errorf("identity group A is served natively only once identity is authoritative (state %q): use the identity cutover", state)
	case native == 0 && IdentityAuthoritative(state):
		return fmt.Errorf("identity is authoritative (state %q): group A stays native; roll back with the identity rollback", state)
	}
	return nil
}

// SetPackageRouteModesTx sets route modes in a Control installation's
// configuration inside tx, keeping the rest of the document. It fails with
// ErrPluginConfigurationConflict if the document changes concurrently.
func SetPackageRouteModesTx(tx *gorm.DB, publicKey ed25519.PublicKey, installationID uint, modes map[string]string, actorID uint) (*model.PluginConfiguration, error) {
	if len(publicKey) != ed25519.PublicKeySize {
		return nil, ErrPluginTrustRootRequired
	}
	current, err := GetPluginConfiguration(tx, installationID)
	if err != nil {
		return nil, err
	}
	document := map[string]json.RawMessage{}
	if err := json.Unmarshal([]byte(current.ConfigJSON), &document); err != nil {
		return nil, fmt.Errorf("plugin configuration is not a JSON object: %w", err)
	}
	routes := map[string]string{}
	if raw, ok := document[PackageRouteModesConfigKey]; ok {
		if err := json.Unmarshal(raw, &routes); err != nil || routes == nil {
			return nil, fmt.Errorf("plugin configuration %q must map route ids to modes", PackageRouteModesConfigKey)
		}
	}
	for route, mode := range modes {
		if mode == packagebridge.RouteModeLegacy {
			delete(routes, route)
			continue
		}
		routes[route] = mode
	}
	encodedRoutes, err := json.Marshal(routes)
	if err != nil {
		return nil, err
	}
	document[PackageRouteModesConfigKey] = encodedRoutes
	encoded, err := json.Marshal(document)
	if err != nil {
		return nil, err
	}
	canonical, err := CanonicalKernelOperationConfig(string(encoded))
	if err != nil {
		return nil, err
	}
	revision := current.Revision
	return updatePluginConfigurationTx(tx, publicKey, installationID, canonical, &revision, actorID, nil, nil)
}
