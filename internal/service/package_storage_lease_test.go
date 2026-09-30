package service

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/AnixOps/anix-control/v4/internal/packagestore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLeaseStorageGrantsOnlySignedCapabilitiesToTheCurrentGeneration(t *testing.T) {
	db := newKernelTestDB(t)
	require.NoError(t, db.Exec("CREATE TABLE v2_knowledge (id INTEGER PRIMARY KEY)").Error)
	_, installation := seedKnowledgeRelease(t, db, "", []string{"kernel.storage.v1", "kernel.storage.adopt:v2_knowledge"})
	path := filepath.Join(t.TempDir(), "kernel.db")
	operations := PackageHostOperations{DB: db, Storage: &packagestore.Store{DB: db, Driver: "sqlite", DSN: path}}
	current := packagebridge.HostIdentity{PackageID: "knowledge", Version: "4.0.1", Generation: 7}

	lease, err := operations.LeaseStorage(context.Background(), current)
	require.NoError(t, err)
	assert.Equal(t, packagebridge.StorageLease{
		Driver: "sqlite", DSN: path, TablePrefix: "pkg_knowledge_", LeaseGeneration: 1,
		AdoptedTables: []string{"v2_knowledge"}, Views: []string{},
	}, lease)
	var row model.PackageStorage
	require.NoError(t, db.First(&row, "package_id = ?", "knowledge").Error)
	assert.EqualValues(t, 7, row.HostGeneration)

	for name, host := range map[string]packagebridge.HostIdentity{
		"stale generation": {PackageID: "knowledge", Version: "4.0.1", Generation: 6},
		"other version":    {PackageID: "knowledge", Version: "4.0.0", Generation: 7},
		"unknown package":  {PackageID: "ticket", Version: "4.0.1", Generation: 7},
	} {
		_, err := operations.LeaseStorage(context.Background(), host)
		assert.ErrorIs(t, err, packagebridge.ErrHostFenced, name)
	}
	require.NoError(t, db.Model(&model.PluginInstallation{}).Where("id = ?", installation.ID).Update("enabled", false).Error)
	_, err = operations.LeaseStorage(context.Background(), current)
	assert.ErrorIs(t, err, packagebridge.ErrHostFenced, "a disabled package is fenced")
}

func TestLeaseStorageRefusesPackagesWithoutTheStorageCapability(t *testing.T) {
	db := newKernelTestDB(t)
	seedKnowledgeRelease(t, db, "", nil)
	current := packagebridge.HostIdentity{PackageID: "knowledge", Version: "4.0.1", Generation: 7}

	operations := PackageHostOperations{DB: db, Storage: &packagestore.Store{DB: db, Driver: "sqlite", DSN: "kernel.db"}}
	_, err := operations.LeaseStorage(context.Background(), current)
	assert.ErrorIs(t, err, packagebridge.ErrStorageUnavailable)
	assert.ErrorIs(t, err, packagestore.ErrStorageNotDeclared)

	_, err = PackageHostOperations{DB: db}.LeaseStorage(context.Background(), current)
	assert.ErrorIs(t, err, packagebridge.ErrStorageUnavailable, "a kernel without storage refuses leases")
}

func TestLeaseStorageRejectsATamperedRelease(t *testing.T) {
	db := newKernelTestDB(t)
	seedKnowledgeRelease(t, db, "", nil)
	// Granting storage by editing the stored manifest breaks its signature.
	var release model.PluginRelease
	require.NoError(t, db.First(&release, "plugin_id = ?", "knowledge").Error)
	tampered := release.ManifestJSON[:len(release.ManifestJSON)-1] + `,"capabilities":["kernel.storage.v1"]}`
	require.NoError(t, db.Model(&release).Update("manifest_json", tampered).Error)

	operations := PackageHostOperations{DB: db, Storage: &packagestore.Store{DB: db, Driver: "sqlite", DSN: "kernel.db"}}
	_, err := operations.LeaseStorage(context.Background(), packagebridge.HostIdentity{PackageID: "knowledge", Version: "4.0.1", Generation: 7})
	require.Error(t, err)
	assert.NotErrorIs(t, err, packagebridge.ErrStorageUnavailable)
	var count int64
	require.NoError(t, db.Model(&model.PackageStorage{}).Count(&count).Error)
	assert.Zero(t, count)
}
