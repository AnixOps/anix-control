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

// ErrCapabilityNotAuthorized means the calling host may not use a kernel
// contract method family.
var ErrCapabilityNotAuthorized = errors.New("package is not authorized for the kernel capability")

// AuthorizeIdentity admits a host to KernelIdentity only when it is the
// current generation of an official AnixOps package whose signed release,
// verified again here, declares kernel.identity.v1.
func (o PackageHostOperations) AuthorizeIdentity(ctx context.Context, host packagebridge.HostIdentity) error {
	return o.authorizeCapability(ctx, host, CapabilityIdentity, ErrIdentityNotAuthorized)
}

// AuthorizeCapability admits a host to a kernel contract method family on
// the same terms: the current generation of an official AnixOps package
// whose verified signed release declares capability.
func (o PackageHostOperations) AuthorizeCapability(ctx context.Context, host packagebridge.HostIdentity, capability string) error {
	return o.authorizeCapability(ctx, host, capability, ErrCapabilityNotAuthorized)
}

func (o PackageHostOperations) authorizeCapability(ctx context.Context, host packagebridge.HostIdentity, capability string, notAuthorized error) error {
	if _, err := o.currentInstallation(ctx, host); err != nil {
		return err
	}
	db := o.DB.WithContext(ctx)
	var plugin model.Plugin
	if err := db.First(&plugin, "id = ? AND official = ? AND publisher = ?", host.PackageID, true, "AnixOps").Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return notAuthorized
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
		return fmt.Errorf("%w: %w", notAuthorized, err)
	}
	if !containsPluginCapability(manifest.Capabilities, capability) {
		return notAuthorized
	}
	return nil
}
