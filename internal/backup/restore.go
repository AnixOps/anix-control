package backup

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/database/migrations"
	"github.com/AnixOps/anix-control/v5/internal/rootkey"
)

// RestoreOptions configure Restore. The targets are the files a Control
// started with the same configuration uses.
type RestoreOptions struct {
	// Bundle is the bundle's path.
	Bundle string
	// Driver is the database driver; anything but SQLite is refused.
	Driver string
	// DataDir receives root.key; DataDir/tmp is the scratch directory and
	// DataDir/replaced-<UTC time> takes what --replace moves aside.
	DataDir string
	// DatabasePath is the SQLite database file to restore.
	DatabasePath string
	// ConfigPath is the configuration file Control loads. "" when it
	// loads none: the bundle's configuration is then written to
	// DataDir/config.yaml.restored for the operator to use.
	ConfigPath string
	// Replace moves an existing database, root key and configuration
	// aside instead of refusing.
	Replace bool
	// ProgramVersion is this Control's version: a bundle whose database a
	// newer Control migrated is refused (migrations.Check).
	ProgramVersion string
	// Now defaults to time.Now.
	Now func() time.Time
}

// RestoreResult is what Restore did.
type RestoreResult struct {
	Manifest Manifest
	// Written are the files Restore wrote.
	Written []string
	// ConfigUnchanged is true when the configuration file already was the
	// bundle's.
	ConfigUnchanged bool
	// ConfigNote says where the configuration went when Control loads no
	// configuration file; "" otherwise.
	ConfigNote string
	// ReplacedDir holds what Replace moved aside, Moved the moved files;
	// "" and nil when nothing was moved.
	ReplacedDir string
	Moved       []string
}

// RefusedError is a restore that was refused before anything changed.
type RefusedError struct {
	Reason string
	Err    error
}

func (e *RefusedError) Error() string {
	if e.Err != nil {
		return e.Reason + ": " + e.Err.Error()
	}
	return e.Reason
}

func (e *RefusedError) Unwrap() error { return e.Err }

func refused(reason string, err error) error { return &RefusedError{Reason: reason, Err: err} }

// Restore installs a bundle while Control is stopped (docs/reference/
// backups.md, "Restore"):
//
//  1. It takes DataDir/control.lock exclusively and checks that no process
//     has the database open; a running Control is refused.
//  2. It verifies the bundle (VerifyFile) into DataDir/tmp. A bundle whose
//     database a newer Control migrated is refused with the start's own
//     message.
//  3. An existing database (with its -wal and -shm), root.key, or a
//     configuration file other than the bundle's is refused, or with
//     Replace moved into DataDir/replaced-<UTC time>/; nothing is ever
//     deleted.
//  4. It writes the database, root.key and the configuration, each synced
//     and renamed into place, mode 0600, owned like its directory when run
//     as root. It starts nothing: the next start migrates the restored
//     database as usual, after a pre-migration backup.
func Restore(ctx context.Context, options RestoreOptions) (*RestoreResult, error) {
	switch strings.ToLower(strings.TrimSpace(options.Driver)) {
	case "", "sqlite", "sqlite3":
	default:
		return nil, refused("nothing was restored", ErrNeedsSQLite)
	}
	if strings.TrimSpace(options.DataDir) == "" {
		return nil, errors.New("data_dir is empty")
	}
	if inMemory(options.DatabasePath) {
		return nil, refused("nothing was restored", errors.New("the configuration names an in-memory database"))
	}
	now := time.Now
	if options.Now != nil {
		now = options.Now
	}
	databasePath, err := filepath.Abs(sqliteFile(options.DatabasePath))
	if err != nil {
		return nil, err
	}
	if err := ensureDir(options.DataDir); err != nil {
		return nil, err
	}

	lock, err := LockExclusive(options.DataDir)
	if err != nil {
		if errors.Is(err, ErrServiceRunning) {
			return nil, refused("nothing was restored", err)
		}
		return nil, err
	}
	defer lock.Release()
	if inUse, err := databaseInUse(databasePath); err != nil {
		return nil, fmt.Errorf("check whether %s is in use: %w", databasePath, err)
	} else if inUse {
		return nil, refused("nothing was restored", fmt.Errorf("another process has the database %s open (Control or one of its package hosts is still running): stop Control first, for example with `sudo systemctl stop anix-control`", databasePath))
	}

	verified, err := VerifyFile(ctx, options.Bundle, VerifyOptions{ScratchDir: ScratchDir(options.DataDir), ProgramVersion: options.ProgramVersion, Keep: true})
	if err != nil {
		var newer *migrations.RefusedError
		if errors.As(err, &newer) {
			return nil, refused("nothing was restored: this Control cannot open the bundle's database", newer)
		}
		var failed *VerifyError
		if errors.As(err, &failed) {
			return nil, refused("nothing was restored: the bundle failed its verification", failed)
		}
		return nil, err
	}
	defer func() { _ = os.RemoveAll(verified.Dir) }()
	result := &RestoreResult{Manifest: verified.Manifest}

	var bundleConfig []byte
	_, hasConfig := verified.Manifest.member(MemberConfig)
	if hasConfig {
		bundleConfig, err = os.ReadFile(filepath.Join(verified.Dir, MemberConfig)) // #nosec G304 -- extracted by the verification.
		if err != nil {
			return nil, err
		}
	}
	configTarget := strings.TrimSpace(options.ConfigPath)
	if hasConfig && configTarget == "" {
		configTarget = filepath.Join(options.DataDir, "config.yaml.restored")
		result.ConfigNote = fmt.Sprintf("Control loaded no configuration file, so the bundle's configuration was written to %s: start Control with -config %s (or ANIX_CONTROL_CONFIG)", configTarget, configTarget)
	}

	// What is in the way.
	var existing []string
	for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
		if exists(databasePath + suffix) {
			existing = append(existing, databasePath+suffix)
		}
	}
	rootKeyPath := rootkey.Path(options.DataDir)
	if exists(rootKeyPath) {
		existing = append(existing, rootKeyPath)
	}
	if hasConfig && exists(configTarget) {
		current, err := os.ReadFile(configTarget) // #nosec G304 -- the configuration file Control loads.
		if err != nil {
			return nil, err
		}
		if bytes.Equal(current, bundleConfig) {
			result.ConfigUnchanged = true
		} else {
			existing = append(existing, configTarget)
		}
	}
	if len(existing) > 0 && !options.Replace {
		return nil, refused("nothing was restored", fmt.Errorf("the target is not empty: %s exist. Restore into an empty data directory and database, or pass --replace to move them aside into %s first (nothing is deleted)",
			strings.Join(existing, ", "), filepath.Join(options.DataDir, "replaced-<UTC time>")))
	}
	if len(existing) > 0 {
		result.ReplacedDir = filepath.Join(options.DataDir, "replaced-"+now().UTC().Format(timeFormat))
		if err := os.Mkdir(result.ReplacedDir, dirMode); err != nil {
			return nil, fmt.Errorf("prepare %s: %w", result.ReplacedDir, err)
		}
		for _, path := range existing {
			name := filepath.Base(path)
			if path == configTarget {
				name = "config.yaml"
			}
			target := filepath.Join(result.ReplacedDir, name)
			if err := move(path, target); err != nil {
				return result, fmt.Errorf("move %s aside: %w (what was moved so far is in %s)", path, err, result.ReplacedDir)
			}
			result.Moved = append(result.Moved, target)
		}
	}

	install := func(from, to string, mode os.FileMode) error {
		if err := installFile(from, to, mode); err != nil {
			return fmt.Errorf("write %s: %w", to, err)
		}
		result.Written = append(result.Written, to)
		return nil
	}
	if err := ensureDirLoose(filepath.Dir(databasePath)); err != nil {
		return result, err
	}
	if err := install(filepath.Join(verified.Dir, MemberDatabase), databasePath, fileMode); err != nil {
		return result, err
	}
	if err := install(filepath.Join(verified.Dir, MemberRootKey), rootKeyPath, fileMode); err != nil {
		return result, err
	}
	if hasConfig && !result.ConfigUnchanged {
		if err := ensureDirLoose(filepath.Dir(configTarget)); err != nil {
			return result, err
		}
		if err := install(filepath.Join(verified.Dir, MemberConfig), configTarget, fileMode); err != nil {
			return result, err
		}
	}
	return result, nil
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// ensureDirLoose creates a directory outside the data directory (the
// database's, the configuration's) without changing an existing one.
func ensureDirLoose(dir string) error {
	if exists(dir) {
		return nil
	}
	return os.MkdirAll(dir, dirMode)
}

// installFile copies from to a temporary file beside to, syncs it, renames
// it into place and syncs the directory.
func installFile(from, to string, mode os.FileMode) error {
	source, err := os.Open(from) // #nosec G304 -- a member the verification extracted.
	if err != nil {
		return err
	}
	defer func() { _ = source.Close() }()
	temporary, err := os.CreateTemp(filepath.Dir(to), "."+filepath.Base(to)+".restoring-*")
	if err != nil {
		return err
	}
	done := false
	defer func() {
		if !done {
			_ = temporary.Close()
			_ = os.Remove(temporary.Name())
		}
	}()
	if err := temporary.Chmod(mode); err != nil {
		return err
	}
	if _, err := io.Copy(temporary, source); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := ownTo(temporary.Name()); err != nil {
		return err
	}
	if err := os.Rename(temporary.Name(), to); err != nil {
		return err
	}
	done = true
	return syncDir(filepath.Dir(to))
}

// move renames from to to; across file systems it copies, syncs and then
// removes the original, so the file is never lost.
func move(from, to string) error {
	err := os.Rename(from, to)
	if err == nil || !errors.Is(err, syscall.EXDEV) {
		return err
	}
	info, err := os.Stat(from)
	if err != nil {
		return err
	}
	if err := installFile(from, to, info.Mode().Perm()); err != nil {
		return err
	}
	return os.Remove(from)
}
