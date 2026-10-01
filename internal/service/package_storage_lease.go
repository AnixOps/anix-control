package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/AnixOps/anix-control/v4/internal/packagestore"
	"gorm.io/gorm"
)

// LeaseStorage provisions the calling host's storage from the storage
// capabilities of its signed release and returns fresh credentials. The host
// must be the current installation generation.
func (o PackageHostOperations) LeaseStorage(ctx context.Context, host packagebridge.HostIdentity) (packagebridge.StorageLease, error) {
	if o.Storage == nil {
		return packagebridge.StorageLease{}, fmt.Errorf("%w: the kernel has no package storage configured", packagebridge.ErrStorageUnavailable)
	}
	if _, err := o.currentInstallation(ctx, host); err != nil {
		return packagebridge.StorageLease{}, err
	}
	var release model.PluginRelease
	if err := o.DB.WithContext(ctx).First(&release, "plugin_id = ? AND version = ?", host.PackageID, host.Version).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return packagebridge.StorageLease{}, packagebridge.ErrHostFenced
		}
		return packagebridge.StorageLease{}, err
	}
	// Grants come only from the signed manifest, verified again here.
	manifest, err := VerifyStoredPluginRelease(o.DB.WithContext(ctx), release, o.FallbackPublicKey)
	if err != nil {
		return packagebridge.StorageLease{}, fmt.Errorf("verify package release for storage: %w", err)
	}
	// A grant the installation cannot honour yet (a table whose credential
	// split is not finalized, a view that exists only after) is left out.
	grants, err := EffectiveStorageGrants(o.DB.WithContext(ctx), StorageGrants(*manifest))
	if err != nil {
		return packagebridge.StorageLease{}, err
	}
	lease, err := o.Storage.Lease(ctx, packagestore.Holder{
		PackageID: host.PackageID, Version: host.Version, Generation: host.Generation, Remote: host.Remote,
	}, packagestore.Grants{Storage: grants.Storage, AdoptTables: grants.AdoptTables, Views: grants.Views})
	switch {
	case errors.Is(err, packagestore.ErrStorageNotDeclared), errors.Is(err, packagestore.ErrPackageNotEligible),
		errors.Is(err, packagestore.ErrCreateRoleRequired), errors.Is(err, packagestore.ErrGrantTargetMissing):
		return packagebridge.StorageLease{}, fmt.Errorf("%w: %w", packagebridge.ErrStorageUnavailable, err)
	case err != nil:
		return packagebridge.StorageLease{}, err
	}
	return packagebridge.StorageLease{
		Driver: lease.Driver, DSN: lease.DSN, Schema: lease.Schema, TablePrefix: lease.TablePrefix,
		LeaseGeneration: lease.LeaseGeneration, AdoptedTables: lease.AdoptedTables, Views: lease.Views,
	}, nil
}
