package service

import (
	"errors"
	"fmt"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// IdentityPlatformPackageID is the identity module's package.
const IdentityPlatformPackageID = "identity-platform"

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

// RequireIdentityFinalized refuses a database whose accounts identity does
// not own yet: Control keeps no login of its own, so a database with users
// must have been cut over to identity and finalized by an earlier release.
// It only reads, before any migration runs; a database without users
// passes, and EnsureIdentityFinalized then records it as finalized.
func RequireIdentityFinalized(db *gorm.DB) error {
	migrator := db.Migrator()
	if !migrator.HasTable(&model.User{}) {
		return nil
	}
	state := model.IdentityAuthorityKernel
	if migrator.HasTable(&model.IdentityAuthority{}) {
		var err error
		if state, err = IdentityAuthorityState(db); err != nil {
			return fmt.Errorf("read the identity authority: %w", err)
		}
	}
	if state == model.IdentityAuthorityFinalized {
		return nil
	}
	var users int64
	if err := db.Model(&model.User{}).Limit(1).Count(&users).Error; err != nil {
		return fmt.Errorf("count users: %w", err)
	}
	if users == 0 {
		return nil
	}
	return fmt.Errorf("identity: the accounts are in state %s (finalize not completed). This release serves logins through identity only. "+
		"On 5.0.3, import the accounts, cut over and finalize (POST /api/v4/kernel/identity/import, /cutover, /finalize), "+
		"then start this release (docs/UPGRADE.md, \"Upgrading To The Release After 5.0.3\")", state)
}

// EnsureIdentityFinalized records identity as finalized. Control runs it at
// every start after RequireIdentityFinalized, which leaves only a finalized
// database or one without users.
func EnsureIdentityFinalized(db *gorm.DB) error {
	authority := model.IdentityAuthority{ID: 1, State: model.IdentityAuthorityFinalized, UpdatedAt: time.Now().UTC()}
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.Assignments(map[string]any{"state": model.IdentityAuthorityFinalized}),
	}).Create(&authority).Error
}
