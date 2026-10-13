package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/AnixOps/anix-control/v5/internal/backup"
	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/database/migrations"
	"github.com/AnixOps/anix-control/v5/internal/rootkey"
	"gorm.io/gorm"
)

// setupBackups prepares the backups of this process before it opens the
// database: with lock (the server and `migrate`) it takes the shared lock
// on data_dir/control.lock that a restore needs exclusively, so a restore
// never runs under a Control and a Control never starts during a restore
// (backup.LockService); and it makes the process's backup manager
// (backup.Default), which the pre-migration backup, the backup worker and
// the API use. The returned function releases the lock.
func setupBackups(cfg *config.Config, configPath string, lock bool) func() {
	release := func() {}
	if lock && strings.TrimSpace(cfg.DataDir) != "" {
		held, err := backup.LockService(cfg.DataDir)
		if err != nil {
			log.Fatalf("Data directory %s: %v", cfg.DataDir, err)
		}
		release = held.Release
	}
	backup.SetDefault(newBackupManager(cfg, configPath, database.Get))
	return release
}

// newBackupManager is the backup manager of a configuration: data_dir, the
// SQLite database, the configuration file Control loaded and its root key.
func newBackupManager(cfg *config.Config, configPath string, db func() *gorm.DB) *backup.Manager {
	return &backup.Manager{
		DataDir: cfg.DataDir, Driver: cfg.Database.Driver, DatabasePath: cfg.Database.Database, ConfigPath: configPath,
		RootKey: backupRootKey(cfg), Domain: cfg.PanelDomain(), ProgramVersion: version, DB: db, Logf: log.Printf,
	}
}

// backupRootKey answers the content of root.key for a bundle. Without the
// file (a key Control could only use from module_runtime.ca_kek, because
// data_dir was not writable) the bundle gets a root.key written from the
// key this process resolved, with the values it adopted, so that a restore
// still has the key.
func backupRootKey(cfg *config.Config) func() ([]byte, error) {
	return func() ([]byte, error) {
		data, err := os.ReadFile(rootkey.Path(cfg.DataDir))
		if err == nil || !errors.Is(err, os.ErrNotExist) {
			return data, err
		}
		raw := strings.TrimSpace(cfg.ModuleRuntime.CAKEK)
		if raw == "" {
			return nil, fmt.Errorf("there is no root key: %s does not exist and the configuration sets none. Start Control once, which creates it", rootkey.Path(cfg.DataDir))
		}
		key, err := rootkey.ParseKEK(raw)
		if err != nil {
			return nil, err
		}
		file := rootkey.File{Key: key}
		if secret := strings.TrimSpace(cfg.JWT.Secret); secret != "" && secret != key.DerivedJWTSecret() {
			file.AdoptedJWTSecret = secret
		}
		if raw := strings.TrimSpace(cfg.Identity.KEK); raw != "" {
			if kek, err := rootkey.ParseKEK(raw); err == nil && kek.Base64() != key.DerivedIdentityKEK() {
				file.AdoptedIdentityKEK = kek.Base64()
			}
		}
		return file.Marshal(), nil
	}
}

// preMigrationBackup is the migration runner's BeforeMigrate hook: before a
// pending migration runs on a database that is not empty, it takes a
// pre-migration bundle. When that fails, nothing is migrated and the start
// stops with the reason. A new database, PostgreSQL and an in-memory
// database go on without one.
func preMigrationBackup(logf func(string, ...any)) func(context.Context, migrations.Pending) error {
	return func(ctx context.Context, pending migrations.Pending) error {
		if pending.Empty {
			return nil
		}
		steps := strings.Join(pending.Steps, ", ")
		manager := backup.Default()
		if manager == nil {
			logf("Database schema: this process has no backup manager; %s run without a pre-migration backup.", steps)
			return nil
		}
		if err := manager.Supported(); err != nil {
			if errors.Is(err, backup.ErrNeedsSQLite) || errors.Is(err, backup.ErrInMemory) {
				logf("Database schema: no pre-migration backup before %s: %v", steps, err)
				return nil
			}
			return fmt.Errorf("no pre-migration backup can be taken, so nothing was migrated: %w", err)
		}
		bundle, _, err := manager.CreateWait(ctx, backup.TypePreMigration)
		if err != nil {
			return fmt.Errorf("the backup taken before migrating the database failed, so nothing was migrated: %w. Fix the cause (free space or permissions in %s) and start again", err, manager.Dir())
		}
		logf("Database schema: took the pre-migration backup %s before %s; it is verified once Control runs.", bundle.Path, steps)
		return nil
	}
}

// backupWorker is the singleton worker of the daily backup.
func backupWorker(cfg *config.Config) *backup.Worker {
	seed := ""
	if cfg != nil {
		seed = cfg.PanelDomain()
	}
	if seed == "" {
		seed, _ = os.Hostname()
	}
	return &backup.Worker{Manager: backup.Default(), Seed: seed}
}
