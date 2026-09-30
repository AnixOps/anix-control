package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"gorm.io/gorm"
)

// ErrIdentityNotAuthorized means the calling host may not use KernelIdentity.
var ErrIdentityNotAuthorized = errors.New("package is not authorized for kernel.identity.v1")

// AuthorizeIdentity admits a host to KernelIdentity only when it is the
// current generation of an official AnixOps package whose signed release,
// verified again here, declares kernel.identity.v1.
func (o PackageHostOperations) AuthorizeIdentity(ctx context.Context, host packagebridge.HostIdentity) error {
	if _, err := o.currentInstallation(ctx, host); err != nil {
		return err
	}
	db := o.DB.WithContext(ctx)
	var plugin model.Plugin
	if err := db.First(&plugin, "id = ? AND official = ? AND publisher = ?", host.PackageID, true, "AnixOps").Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrIdentityNotAuthorized
		}
		return err
	}
	var release model.PluginRelease
	if err := db.First(&release, "plugin_id = ? AND version = ?", host.PackageID, host.Version).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return packagebridge.ErrHostFenced
		}
		return err
	}
	manifest, err := VerifyStoredPluginRelease(db, release, o.FallbackPublicKey)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrIdentityNotAuthorized, err)
	}
	if !containsPluginCapability(manifest.Capabilities, CapabilityIdentity) {
		return ErrIdentityNotAuthorized
	}
	return nil
}
