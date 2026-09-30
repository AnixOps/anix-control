//go:build unix

package integration

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/packagebridgesdk"
	"github.com/AnixOps/anix-control/sdk/packagestoresdk"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/AnixOps/anix-control/v4/internal/packagestore"
	"github.com/AnixOps/anix-control/v4/internal/pluginhost"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const storageFixtureID = "storage-fixture"

// A real host process leases its storage over the package bridge and applies
// its embedded migrations when the kernel runs them through the ledger.
func TestStorageHostMigratesThroughTheLedgerOnSQLite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kernel.db")
	db, err := gorm.Open(sqlite.Open(path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	runStorageHostMigration(t, db, "sqlite", path)
}

func TestStorageHostMigratesThroughTheLedgerOnPostgres(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("ANIX_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("ANIX_TEST_POSTGRES_DSN is not set")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	var databaseName string
	require.NoError(t, db.Raw("SELECT current_database()").Scan(&databaseName).Error)
	if !strings.Contains(strings.ToLower(databaseName), "test") && os.Getenv("ANIX_TEST_POSTGRES_ALLOW_UNSAFE") != "1" {
		t.Skipf("refusing to run destructive postgres test against database %q", databaseName)
	}
	role := packagestore.RoleName(storageFixtureID)
	cleanup := func() {
		_ = db.Exec("DROP SCHEMA IF EXISTS " + packagestore.SchemaName(storageFixtureID) + " CASCADE").Error
		_ = db.Exec("DROP ROLE IF EXISTS " + role).Error
		_ = db.Migrator().DropTable(model.KernelModels()...)
	}
	cleanup()
	t.Cleanup(cleanup)
	runStorageHostMigration(t, db, "postgres", dsn)
}

func runStorageHostMigration(t *testing.T, db *gorm.DB, driver, storageSource string) {
	t.Helper()
	require.NoError(t, service.EnsureKernelSchema(db))
	publicKey := seedStorageFixtureRelease(t, db)
	operations := service.PackageHostOperations{
		DB: db, FallbackPublicKey: publicKey,
		Storage: &packagestore.Store{DB: db, Driver: driver, DSN: storageSource},
	}
	allowlist, err := packagebridge.NewAllowlistWithFallback(nil)
	require.NoError(t, err)
	manager, err := pluginhost.NewManager(pluginhost.ManagerConfig{
		RuntimeDir:     shortRuntimeDir(t),
		BridgeFactory:  packagebridge.NewFactory(allowlist).WithSessionOptions(packagebridge.SessionOptions{HostOperations: operations}),
		StartupTimeout: 10 * time.Second,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, manager.Shutdown(context.Background())) })

	ref := buildStorageFixtureRef(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	require.NoError(t, manager.Start(ctx, ref, 7))
	migrator := service.PackageHostMigrator{DB: db, Hosts: manager}
	require.NoError(t, migrator.MigrateStartedHost(ctx, ref, 7))

	require.Equal(t, []string{"first", "second"}, readFixtureNotes(t, ctx, operations))
	if driver == "postgres" {
		var count int64
		err := db.Table(packagestore.SchemaName(storageFixtureID) + ".notes").Count(&count).Error
		require.ErrorContains(t, err, "permission denied", "the package's own tables are not the kernel's")
	}
	var run model.PackageMigrationRun
	require.NoError(t, db.First(&run, "package_id = ? AND generation = ?", storageFixtureID, 7).Error)
	require.Equal(t, model.PackageMigrationStateCompleted, run.State)
	require.Equal(t, service.PackageMigrationStepsDigest(ref.Migrations), run.ValidationDigest)
	require.NotNil(t, run.HealthVerifiedAt)
	var storage model.PackageStorage
	require.NoError(t, db.First(&storage, "package_id = ?", storageFixtureID).Error)
	require.Equal(t, driver, storage.Driver)

	// Starting the same generation again, as after a Control restart, does
	// not run the migrations a second time.
	require.NoError(t, migrator.MigrateStartedHost(ctx, ref, 7))
	var runs, validations int64
	require.NoError(t, db.Model(&model.PackageMigrationRun{}).Where("package_id = ?", storageFixtureID).Count(&runs).Error)
	require.NoError(t, db.Model(&model.PackageValidationResult{}).Where("package_id = ?", storageFixtureID).Count(&validations).Error)
	require.EqualValues(t, 1, runs)
	require.EqualValues(t, 1, validations)
	require.Len(t, readFixtureNotes(t, ctx, operations), 2)

	// A host whose embedded scripts differ from the verified index is caught.
	tampered := ref
	tampered.Migrations = append([]pluginhost.MigrationStep(nil), ref.Migrations...)
	tampered.Migrations[1].SHA256 = strings.Repeat("0", 64)
	require.NoError(t, manager.Start(ctx, tampered, 8))
	require.NoError(t, db.Model(&model.PluginInstallation{}).Where("plugin_id = ?", storageFixtureID).Update("lifecycle_generation", 8).Error)
	require.ErrorContains(t, migrator.MigrateStartedHost(ctx, tampered, 8), "differ from the verified index")
}

// readFixtureNotes reads the fixture's own table the way the package does:
// through a storage lease for the current generation.
func readFixtureNotes(t *testing.T, ctx context.Context, operations service.PackageHostOperations) []string {
	t.Helper()
	store, err := packagestoresdk.Open(ctx, packagestoresdk.LeaserFunc(func(ctx context.Context) (packagebridgesdk.StorageLease, error) {
		lease, err := operations.LeaseStorage(ctx, packagebridge.HostIdentity{PackageID: storageFixtureID, Version: "4.1.0", Generation: 7})
		return packagebridgesdk.StorageLease{
			Driver: lease.Driver, DSN: lease.DSN, Schema: lease.Schema, TablePrefix: lease.TablePrefix, LeaseGeneration: lease.LeaseGeneration,
		}, err
	}))
	require.NoError(t, err)
	defer func() { require.NoError(t, store.Close()) }()
	var bodies []string
	require.NoError(t, store.DB.Table(store.Table("notes")).Order("id").Pluck("body", &bodies).Error)
	return bodies
}

func seedStorageFixtureRelease(t *testing.T, db *gorm.DB) ed25519.PublicKey {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	digest := strings.Repeat("a", 64)
	manifest := service.PluginManifest{
		ID: storageFixtureID, Name: "Storage fixture", Version: "4.1.0", APIVersion: "v2", Publisher: "AnixOps",
		Targets: []string{"control"}, ArtifactSHA256: digest,
		ControlEntrypoint:   &service.PluginEntrypoint{Path: "bin/control-host", SHA256: digest},
		Migrations:          &service.PluginMigrations{Index: "migrations/index.json", SHA256: digest},
		CompatibilityRoutes: &service.PluginCompatibilityRoutes{Path: "compat/v2-routes.json", SHA256: digest},
		RouteContractDigest: digest,
		Capabilities:        []string{"kernel.storage.v1"},
	}
	canonical, err := service.CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	_, err = service.RegisterPluginRelease(db, string(canonical), base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical)), publicKey)
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.PluginInstallation{
		PluginID: storageFixtureID, Target: "control", DesiredVersion: "4.1.0", ObservedVersion: "4.1.0",
		State: "healthy", Enabled: true, LifecycleGeneration: 7,
	}).Error)
	return publicKey
}

func buildStorageFixtureRef(t *testing.T) pluginhost.ArtifactRef {
	t.Helper()
	repoRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	require.NoError(t, err)
	directory := t.TempDir()
	entrypointPath := filepath.Join(directory, "storage-fixture-host")
	command := exec.Command("go", "build", "-trimpath", "-buildvcs=false", "-o", entrypointPath, "./internal/tests/packagefixture/storagehost")
	command.Dir = repoRoot
	command.Env = append(os.Environ(), "GOWORK=off", "CGO_ENABLED=0")
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
	write := func(name string, content []byte) (string, string) {
		path := filepath.Join(directory, name)
		require.NoError(t, os.WriteFile(path, content, 0o600))
		return path, sha256Hex(content)
	}
	entrypoint, err := os.ReadFile(entrypointPath)
	require.NoError(t, err)
	artifactPath, artifactDigest := write("package.anxp", []byte("storage fixture artifact"))
	manifestPath, manifestDigest := write("manifest.json", []byte(`{"id":"storage-fixture","version":"4.1.0","targets":["control"]}`))
	steps := make([]pluginhost.MigrationStep, 0, 2)
	for _, id := range []string{"001_notes", "002_seed"} {
		script, err := os.ReadFile(filepath.Join(repoRoot, "internal", "tests", "packagefixture", "storagehost", "migrations", id+".sql"))
		require.NoError(t, err)
		steps = append(steps, pluginhost.MigrationStep{ID: id, Path: "migrations/" + id + ".sql", SHA256: sha256Hex(script)})
	}
	return pluginhost.ArtifactRef{
		PackageID: storageFixtureID, Version: "4.1.0",
		ArtifactPath: artifactPath, ArtifactSHA256: artifactDigest,
		EntrypointPath: entrypointPath, EntrypointSHA256: sha256Hex(entrypoint),
		ManifestPath: manifestPath, ManifestSHA256: manifestDigest,
		MigrationIndexSHA256: strings.Repeat("b", 64), Migrations: steps, Storage: true,
	}
}

func sha256Hex(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

// shortRuntimeDir keeps host socket paths below the Unix socket limit.
func shortRuntimeDir(t *testing.T) string {
	t.Helper()
	parent, err := filepath.Abs(".")
	require.NoError(t, err)
	directory, err := os.MkdirTemp(parent, ".anix-host-")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(directory)) })
	return directory
}
