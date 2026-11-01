package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/AnixOps/anix-control/v5/internal/agentpki"
	"github.com/AnixOps/anix-control/v5/internal/builtinpkg"
	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/database/migrations"
	"github.com/AnixOps/anix-control/v5/internal/modulepki"
	"github.com/AnixOps/anix-control/v5/internal/nodesecrets"
	"github.com/AnixOps/anix-control/v5/internal/packagestore"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"gorm.io/gorm"
)

// bootstrapLockKey is the PostgreSQL advisory lock that serializes schema and
// seed work across Control processes ("anixboot" as ASCII bytes).
const bootstrapLockKey int64 = 0x616e6978626f6f74

// bootstrapDatabase prepares the schema and seed data before the server (or
// the migrate command) uses the database. The schema comes from the ordered
// migrations (internal/database/migrations), in every environment: they
// create it in an empty database, adopt one an earlier Control created and
// refuse one a newer Control migrated. Before a pending migration runs on a
// database that is not empty, a pre-migration backup is taken
// (preMigrationBackup). On PostgreSQL it holds a session
// advisory lock, so concurrent Control processes (a rolling update, a
// migrate job next to a starting server) run it one at a time instead of
// racing on DDL and seed rows.
func bootstrapDatabase(ctx context.Context, cfg *config.Config) error {
	db := database.Get()
	release, err := acquireBootstrapLock(ctx, db, cfg.Database.Driver)
	if err != nil {
		return err
	}
	defer release()

	if _, err := migrations.Migrate(ctx, db, migrations.Options{ProgramVersion: version, Logf: log.Printf, BeforeMigrate: preMigrationBackup(log.Printf)}); err != nil {
		return fmt.Errorf("database schema: %w", err)
	}
	// The root key must open what the database holds sealed before anything
	// signs, seals or creates a CA with it.
	if !deferRootKeyCheck {
		if err := verifyRootKey(ctx, cfg, db, log.Printf); err != nil {
			return err
		}
	}
	// Seed rows that exist in every database, at every start as before.
	if err := service.SeedKernelDefaults(db); err != nil {
		return fmt.Errorf("seed the kernel defaults: %w", err)
	}
	if err := nodesecrets.EnsureStateRows(db); err != nil {
		return fmt.Errorf("seed the node secret split state: %w", err)
	}
	if err := ensureModulePKI(ctx, cfg, db); err != nil {
		return err
	}
	// Kernel API views only serve package storage leases: a failure must not
	// stop Control, it only fails leases that ask for the missing view.
	if err := packagestore.EnsureKernelAPIViews(db); err != nil {
		log.Printf("Kernel API views were not created: %v. Package storage leases that grant them will fail.", err)
	}
	if err := ensureConfiguredPluginTrustRoot(db, cfg.Plugins.OfficialPublicKey); err != nil {
		return fmt.Errorf("apply configured plugin trust root: %w", err)
	}
	// The official packages built into this binary. A package that fails is
	// reported and never stops the start. A binary without them (a
	// development build) imports identity-platform from
	// plugins.identity_bootstrap_package_dir, as before.
	if embedded := bootstrapBuiltinPackages(db, cfg, log.Printf); !embedded {
		if err := service.BootstrapIdentityPlatformPackage(db, cfg.Plugins.OfficialPublicKey, cfg.Plugins.IdentityBootstrapPackageDir); err != nil {
			return fmt.Errorf("bootstrap identity platform package: %w", err)
		}
	}

	// The default subscription group and plan. No
	// administrator is created: the first one claims the panel with a
	// one-time link (anix-control admin claim-link).
	service.InitSubscriptionDefaults()
	service.InitDefaultPlan()

	return nil
}

// acquireBootstrapLock takes the bootstrap advisory lock on a dedicated
// PostgreSQL connection. The lock belongs to that session, so it is released
// by the returned function or, if the process dies, when the connection
// closes. SQLite needs no lock: it has a single writer.
func acquireBootstrapLock(ctx context.Context, db *gorm.DB, driver string) (func(), error) {
	switch strings.ToLower(strings.TrimSpace(driver)) {
	case "postgres", "postgresql":
	default:
		return func() {}, nil
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("bootstrap lock: %w", err)
	}
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("bootstrap lock connection: %w", err)
	}
	log.Println("Waiting for the database bootstrap lock...")
	if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", bootstrapLockKey); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("acquire bootstrap lock: %w", err)
	}
	return func() { releaseBootstrapLock(conn) }, nil
}

func releaseBootstrapLock(conn *sql.Conn) {
	if _, err := conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", bootstrapLockKey); err != nil {
		log.Printf("release bootstrap lock: %v", err)
	}
	_ = conn.Close()
}

// ensureModulePKI creates the built-in module CA and the forward link CA on
// first start. It runs
// under the bootstrap lock, so concurrent kernels create one CA.
func ensureModulePKI(ctx context.Context, cfg *config.Config, db *gorm.DB) error {
	authority, err := modulepki.FromConfig(cfg.ModuleRuntime, db)
	if errors.Is(err, modulepki.ErrBuiltinPKIDisabled) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("module PKI: %w", err)
	}
	if err := authority.Ensure(ctx); err != nil {
		return fmt.Errorf("create the module CA: %w", err)
	}
	// The forward link CA (H28) is a separate root sealed with the same
	// key-encryption key.
	link, err := agentpki.LinkAuthorityFromConfig(cfg, db)
	if err != nil {
		return fmt.Errorf("forward link PKI: %w", err)
	}
	if err := link.Ensure(ctx); err != nil {
		return fmt.Errorf("create the forward link CA: %w", err)
	}
	return nil
}

// bootstrapBuiltinPackages installs, upgrades and enables the official
// packages embedded in this binary (service.BootstrapBuiltinPackages) and
// logs what happened to each. It runs under the bootstrap lock at every
// start; a package that fails is logged and raised as a kernel alert
// (builtin_package_invalid, builtin_package_failed), and Control starts.
// It reports whether the binary embeds packages (an unreadable set counts:
// it is a broken release binary, not a development build).
func bootstrapBuiltinPackages(db *gorm.DB, cfg *config.Config, logf func(string, ...any)) bool {
	set, err := builtinpkg.Default()
	if err != nil {
		logf("WARNING: the built-in packages of this binary cannot be read: %v. No official package was installed or upgraded; reinstall a verified release binary.", err)
		return true
	}
	if set.Empty() {
		logf("This binary embeds no official packages (a development build without `make build`).")
		return false
	}
	if strings.TrimSpace(cfg.Plugins.IdentityBootstrapPackageDir) != "" {
		logf("plugins.identity_bootstrap_package_dir is ignored: the official packages, identity-platform included, are built into Control")
	}
	failed := 0
	for _, result := range service.BootstrapBuiltinPackages(db, cfg.Plugins.OfficialPublicKey, set) {
		switch result.Action {
		case service.BuiltinActionInvalid, service.BuiltinActionFailed:
			failed++
			logf("WARNING: built-in package %s %s %s: %v. Control starts without it; its routes answer as unavailable.", result.ID, result.Version, result.Action, result.Err)
		case service.BuiltinActionUnchanged:
		default:
			logf("Built-in package %s %s %s.", result.ID, result.Version, result.Action)
		}
	}
	logf("Built-in packages: %d embedded, %d failed (signing key %s).", len(set.Packages), failed, builtinSigningKeyID(set))
	return true
}

// builtinSigningKeyID names the key the embedded packages were signed with,
// for the log line that compares it with plugins.official_public_key.
func builtinSigningKeyID(set *builtinpkg.Set) string {
	publicKey, err := builtinpkg.PublicKey(set)
	if err != nil {
		return "unknown"
	}
	return service.PluginTrustRootKeyID(publicKey)
}
