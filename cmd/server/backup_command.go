package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/backup"
	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

const backupCommandUsage = `usage:
  anix-control backup create [-type manual|daily|pre-migration|verify] [-config <path>]
  anix-control backup list [-json] [-config <path>]
  anix-control backup verify [<bundle>] [-config <path>]
  anix-control backup restore <bundle> [--replace] [--data-dir <dir>] [-config <path>]

create takes a bundle of the database, root.key and the configuration file
into data_dir/backups and verifies it. list prints the bundles with their
verification. verify checks a bundle the way a restore would use it (the
newest bundle without <bundle>); a bundle in data_dir/backups gets the
result recorded. restore installs a bundle while Control is stopped: it
refuses a data directory or database that is not empty unless --replace
moves them aside into data_dir/replaced-<UTC time>/, and refuses while
Control runs. Control migrates the restored database at its next start.

The commands read the configuration the way Control does (-config, else
ANIX_CONTROL_CONFIG, else config/config.yaml, plus the ANIX_CONTROL_*
variables). Backups need SQLite.

Exit status: 0 done; 1 the backup or its verification failed; 2 a usage or
configuration error; 3 refused (PostgreSQL, Control running, a restore
running, a target that is not empty, a bundle of a newer Control).`

// Exit statuses of `backup` (backupCommandUsage).
const (
	backupExitOK      = 0
	backupExitFailed  = 1
	backupExitUsage   = 2
	backupExitRefused = 3
)

// runBackupCommand runs `backup ...`; handled is false for any other
// command. It never migrates the database: a bundle copies the database as
// it is.
func runBackupCommand(arguments []string, stdout, stderr io.Writer) (code int, handled bool) {
	if len(arguments) == 0 || arguments[0] != "backup" {
		return 0, false
	}
	if len(arguments) < 2 {
		_, _ = fmt.Fprintln(stderr, backupCommandUsage)
		return backupExitUsage, true
	}
	command := &backupCommand{stdout: stdout, stderr: stderr, environ: os.Environ()}
	switch arguments[1] {
	case "create":
		return command.create(arguments[2:]), true
	case "list":
		return command.list(arguments[2:]), true
	case "verify":
		return command.verify(arguments[2:]), true
	case "restore":
		return command.restore(arguments[2:]), true
	}
	_, _ = fmt.Fprintln(stderr, backupCommandUsage)
	return backupExitUsage, true
}

type backupCommand struct {
	stdout, stderr io.Writer
	environ        []string
	// configPath is the file the configuration came from; "" for none.
	configPath string
}

func (c *backupCommand) printf(format string, args ...any) {
	_, _ = fmt.Fprintf(c.stdout, format, args...)
}

// fail prints err and answers its exit status.
func (c *backupCommand) fail(err error) int {
	_, _ = fmt.Fprintf(c.stderr, "backup: %v\n", err)
	var restoreRefusal *backup.RefusedError
	switch {
	case errors.Is(err, backup.ErrNeedsSQLite), errors.Is(err, backup.ErrRestoreRunning), errors.Is(err, backup.ErrServiceRunning), errors.As(err, &restoreRefusal):
		return backupExitRefused
	case errors.Is(err, errBackupUsage), errors.Is(err, errBackupConfig):
		return backupExitUsage
	}
	return backupExitFailed
}

var (
	errBackupUsage  = errors.New("usage")
	errBackupConfig = errors.New("configuration")
)

// parseInterleaved parses flags that may follow the positional arguments
// (restore <bundle> --replace) and answers the positional ones.
func parseInterleaved(flags *flag.FlagSet, arguments []string) ([]string, error) {
	var positional []string
	for {
		if err := flags.Parse(arguments); err != nil {
			return nil, err
		}
		arguments = flags.Args()
		if len(arguments) == 0 {
			return positional, nil
		}
		positional = append(positional, arguments[0])
		arguments = arguments[1:]
	}
}

func newBackupFlags(name string) (*flag.FlagSet, *string) {
	flags := flag.NewFlagSet("backup "+name, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	configFlag := flags.String("config", "", "the configuration file")
	return flags, configFlag
}

// load reads the configuration as Control does and resolves the SQLite
// path and, with withKey, the root key (read only: a command never creates
// root.key). A restore needs no key: it brings its own.
func (c *backupCommand) load(configFlag string, dataDir string, withKey bool) (*config.Config, error) {
	configured := ""
	for _, entry := range c.environ {
		if name, value, ok := strings.Cut(entry, "="); ok && name == config.ConfigPathEnv {
			configured = strings.TrimSpace(value)
		}
	}
	path, err := selectConfigPath(configFlag, strings.TrimSpace(configFlag) != "", configured)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errBackupConfig, err)
	}
	cfg, _, err := config.LoadFile(path, c.environ)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errBackupConfig, err)
	}
	if strings.TrimSpace(dataDir) != "" {
		if !filepath.IsAbs(dataDir) {
			return nil, fmt.Errorf("%w: --data-dir %q is not an absolute path", errBackupConfig, dataDir)
		}
		cfg.DataDir = filepath.Clean(dataDir)
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Database.Driver)) {
	case "", "sqlite", "sqlite3":
		cfg.Database.Database = resolveSQLitePath(cfg.Database.Database, path)
	default:
		return cfg, backup.ErrNeedsSQLite
	}
	if config.PlaceholderJWTSecret(cfg.JWT.Secret) {
		cfg.JWT.Secret = ""
	}
	if withKey {
		if _, err := resolveRootKey(cfg, false, func(string, ...any) {}); err != nil {
			return nil, fmt.Errorf("root key: %w", err)
		}
	}
	cfg.Database.LogLevel = "silent"
	c.configPath = path
	return cfg, nil
}

// open opens the existing database without migrating it, under the
// service lock (a restore must not run meanwhile).
func (c *backupCommand) open(cfg *config.Config) (*backup.Manager, func(), error) {
	if _, err := os.Stat(cfg.Database.Database); err != nil {
		return nil, nil, fmt.Errorf("%w: the database %s: %v", errBackupConfig, cfg.Database.Database, err)
	}
	lock, err := backup.LockService(cfg.DataDir)
	if err != nil {
		return nil, nil, err
	}
	db, err := database.Open(&cfg.Database)
	if err != nil {
		lock.Release()
		return nil, nil, err
	}
	manager := newBackupManager(cfg, c.configPath, func() *gorm.DB { return db })
	manager.Logf = nil
	return manager, func() {
		_ = database.CloseHandle(db)
		lock.Release()
	}, nil
}

func (c *backupCommand) create(arguments []string) int {
	flags, configFlag := newBackupFlags("create")
	typ := flags.String("type", backup.TypeManual, "the bundle type")
	if positional, err := parseInterleaved(flags, arguments); err != nil || len(positional) != 0 {
		return c.usage(err)
	}
	if !backup.ValidType(*typ) {
		return c.usage(fmt.Errorf("unknown -type %q (one of %s)", *typ, strings.Join(backup.Types, ", ")))
	}
	cfg, err := c.load(*configFlag, "", true)
	if err != nil {
		return c.fail(err)
	}
	manager, closeAll, err := c.open(cfg)
	if err != nil {
		return c.fail(err)
	}
	defer closeAll()
	ctx := context.Background()
	bundle, _, err := manager.CreateWait(ctx, *typ)
	if err != nil {
		return c.fail(err)
	}
	c.printf("Wrote %s (%s, %s, schema %04d).\n", bundle.Path, bundle.Manifest.Type, humanBytes(bundle.Size), bundle.Manifest.SchemaVersion)
	row, err := manager.Verify(ctx, bundle.Name)
	if err != nil {
		return c.fail(fmt.Errorf("the bundle %s was written, but its verification failed: %w", bundle.Name, err))
	}
	c.printf("Verified %s: it restores (%s).\n", bundle.Name, row.VerifyStatus)
	return backupExitOK
}

func (c *backupCommand) list(arguments []string) int {
	flags, configFlag := newBackupFlags("list")
	asJSON := flags.Bool("json", false, "print JSON")
	if positional, err := parseInterleaved(flags, arguments); err != nil || len(positional) != 0 {
		return c.usage(err)
	}
	cfg, err := c.load(*configFlag, "", true)
	if err != nil {
		return c.fail(err)
	}
	manager, closeAll, err := c.open(cfg)
	if err != nil {
		return c.fail(err)
	}
	defer closeAll()
	rows, err := manager.List(context.Background())
	if err != nil {
		return c.fail(err)
	}
	if *asJSON {
		if rows == nil {
			rows = []model.Backup{}
		}
		encoder := json.NewEncoder(c.stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(rows); err != nil {
			return c.fail(err)
		}
		return backupExitOK
	}
	if len(rows) == 0 {
		c.printf("No backups in %s.\n", manager.Dir())
		return backupExitOK
	}
	table := tabwriter.NewWriter(c.stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(table, "BUNDLE\tTYPE\tSIZE\tCREATED (UTC)\tVERIFIED")
	for _, row := range rows {
		verified := row.VerifyStatus
		if row.VerifiedAt != nil {
			verified += " " + row.VerifiedAt.UTC().Format("2006-01-02 15:04")
		}
		if row.VerifyError != "" {
			verified += ": " + row.VerifyError
		}
		_, _ = fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\n", row.Path, row.Type, humanBytes(row.Size), row.CreatedAt.UTC().Format("2006-01-02 15:04:05"), verified)
	}
	_ = table.Flush()
	c.printf("Directory: %s\n", manager.Dir())
	return backupExitOK
}

func (c *backupCommand) verify(arguments []string) int {
	flags, configFlag := newBackupFlags("verify")
	positional, err := parseInterleaved(flags, arguments)
	if err != nil || len(positional) > 1 {
		return c.usage(err)
	}
	cfg, err := c.load(*configFlag, "", true)
	if err != nil {
		return c.fail(err)
	}
	manager, closeAll, err := c.open(cfg)
	if err != nil {
		return c.fail(err)
	}
	defer closeAll()
	ctx := context.Background()
	name := ""
	if len(positional) == 1 {
		path, err := filepath.Abs(positional[0])
		if err != nil {
			return c.fail(err)
		}
		if filepath.Dir(path) != filepath.Clean(manager.Dir()) {
			// A bundle elsewhere (copied from another machine): verified,
			// nothing recorded.
			verified, err := backup.VerifyFile(ctx, path, backup.VerifyOptions{ScratchDir: backup.ScratchDir(cfg.DataDir), ProgramVersion: version})
			if err != nil {
				return c.fail(fmt.Errorf("%s: %w", path, err))
			}
			c.printf("Verified %s (%s of %s, Control %s, schema %04d): it restores.\n", path, verified.Manifest.Type,
				verified.Manifest.CreatedAt.Format(time.RFC3339), verified.Manifest.ProgramVersion, verified.Manifest.SchemaVersion)
			return backupExitOK
		}
		name = filepath.Base(path)
	} else {
		rows, err := manager.List(ctx)
		if err != nil {
			return c.fail(err)
		}
		if len(rows) == 0 {
			return c.fail(fmt.Errorf("there is no bundle in %s to verify", manager.Dir()))
		}
		name = rows[0].Path
	}
	row, err := manager.Verify(ctx, name)
	if err != nil {
		return c.fail(fmt.Errorf("%s: %w", name, err))
	}
	c.printf("Verified %s: it restores (%s, recorded).\n", name, row.VerifyStatus)
	return backupExitOK
}

func (c *backupCommand) restore(arguments []string) int {
	flags, configFlag := newBackupFlags("restore")
	replace := flags.Bool("replace", false, "move an existing database, root.key and configuration aside")
	dataDir := flags.String("data-dir", "", "the data directory (default: data_dir of the configuration)")
	positional, err := parseInterleaved(flags, arguments)
	if err != nil || len(positional) != 1 {
		return c.usage(err)
	}
	cfg, err := c.load(*configFlag, *dataDir, false)
	if err != nil {
		return c.fail(err)
	}
	result, err := backup.Restore(context.Background(), backup.RestoreOptions{
		Bundle: positional[0], Driver: cfg.Database.Driver, DataDir: cfg.DataDir, DatabasePath: cfg.Database.Database,
		ConfigPath: c.configPath, Replace: *replace, ProgramVersion: version,
	})
	if err != nil {
		return c.fail(err)
	}
	manifest := result.Manifest
	c.printf("Restored %s (%s of %s, Control %s, schema %04d, from %s).\n", positional[0], manifest.Type,
		manifest.CreatedAt.Format(time.RFC3339), manifest.ProgramVersion, manifest.SchemaVersion, manifest.HostName)
	for _, path := range result.Written {
		c.printf("  wrote %s\n", path)
	}
	if result.ConfigUnchanged {
		c.printf("  the configuration %s already was the bundle's\n", c.configPath)
	}
	if result.ReplacedDir != "" {
		c.printf("Moved aside into %s (nothing was deleted):\n", result.ReplacedDir)
		for _, path := range result.Moved {
			c.printf("  %s\n", path)
		}
	}
	if result.ConfigNote != "" {
		c.printf("%s.\n", result.ConfigNote)
	}
	c.printf("Start Control (for example `sudo systemctl start anix-control`): it migrates the restored database if it is older, after a pre-migration backup.\n")
	return backupExitOK
}

func (c *backupCommand) usage(err error) int {
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "backup: %v\n", err)
	}
	_, _ = fmt.Fprintln(c.stderr, backupCommandUsage)
	return backupExitUsage
}

// humanBytes prints a size in KiB, MiB or GiB.
func humanBytes(size int64) string {
	switch {
	case size >= 1<<30:
		return fmt.Sprintf("%.1f GiB", float64(size)/(1<<30))
	case size >= 1<<20:
		return fmt.Sprintf("%.1f MiB", float64(size)/(1<<20))
	default:
		return fmt.Sprintf("%.1f KiB", float64(size)/(1<<10))
	}
}
