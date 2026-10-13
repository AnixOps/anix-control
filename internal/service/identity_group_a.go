package service

import (
	"errors"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/packagebridge"
	"gorm.io/gorm"
)

// IdentityPlatformPackageID is the package whose group A the identity
// module serves once it is authoritative.
const IdentityPlatformPackageID = "identity-platform"

// IdentityGroupARoutes are identity-platform's routes that move from the
// kernel's legacy handlers to identity together, with the authority state:
// login, registration, MFA, the admin MFA configuration, the admin
// account writes, and the user's own subscription reset, which checks the
// user's password or second factor before it resets.
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
	"identity.user.subscription.reset.post",
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

// ResolvePackageRouteModes returns the route modes identity group A has
// for packageID: identity-platform's group A routes are native while
// identity is authoritative (state identity or finalized), because group A
// is native exactly then, and legacy (not named) otherwise. Any other
// package, and any other route, gets nothing here
// (ResolveEffectivePackageRouteModes adds the rehearsed list). Nothing
// stores these modes: the identity cutover and rollback move the authority,
// and group A with it.
func ResolvePackageRouteModes(db *gorm.DB, packageID string) (map[string]string, error) {
	resolved := make(map[string]string, len(IdentityGroupARoutes))
	if packageID != IdentityPlatformPackageID {
		return resolved, nil
	}
	state, err := IdentityAuthorityState(db)
	if err != nil {
		return nil, err
	}
	if !IdentityAuthoritative(state) {
		return resolved, nil
	}
	for _, route := range IdentityGroupARoutes {
		resolved[route] = packagebridge.RouteModeNative
	}
	return resolved, nil
}
