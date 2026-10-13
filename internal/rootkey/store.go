package rootkey

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// Where a resolved value comes from (Resolved.KeySource, JWTSource,
// IdentitySource). They are log text, never values.
const (
	// SourceFile: root.key in the data directory.
	SourceFile = "root.key"
	// SourceGenerated: a new random key, written to root.key at this start.
	SourceGenerated = "generated"
	// SourceLegacyCAKEK: module_runtime.ca_kek of the configuration.
	SourceLegacyCAKEK = "module_runtime.ca_kek"
	// SourceDerived: derived from the root key with HKDF-SHA256.
	SourceDerived = "derived from the root key"
	// SourceAdopted: a value an earlier configuration set, kept in root.key.
	SourceAdopted = "adopted (root.key)"
	// SourceConfigJWT and SourceConfigIdentity: set by the configuration.
	SourceConfigJWT      = "jwt.secret"
	SourceConfigIdentity = "identity.kek"
)

// Options of Resolve.
type Options struct {
	// DataDir is the directory of root.key (data_dir).
	DataDir string
	// Write lets Resolve create root.key (a new key, or the adopted
	// module_runtime.ca_kek) and record adopted values in it. Only the
	// server sets it: a command started by another user (sudo anix-control
	// ...) must never create the file, or the service could not read it.
	Write bool
	// LegacyCAKEK, LegacyJWTSecret and LegacyIdentityKEK are
	// module_runtime.ca_kek, jwt.secret and identity.kek as the
	// configuration sets them; empty when unset.
	LegacyCAKEK       string
	LegacyJWTSecret   string
	LegacyIdentityKEK string
}

// Resolved is the root key of a start and the values derived from or
// adopted with it. Its formatting never shows a secret.
type Resolved struct {
	Key Key
	// Path is root.key's path.
	Path string
	// KeySource is one of SourceFile, SourceGenerated, SourceLegacyCAKEK.
	KeySource string
	// Stored tells whether root.key holds the key (it may not when the
	// legacy key could not be written).
	Stored bool
	// JWTSecret and IdentityKEK are the effective values; their sources
	// are SourceConfigJWT or SourceConfigIdentity, SourceAdopted or
	// SourceDerived.
	JWTSecret      string
	JWTSource      string
	IdentityKEK    string
	IdentitySource string
	// Notes are the log lines of the resolution, without any secret.
	Notes []string
}

// String hides the secrets.
func (Resolved) String() string { return "[resolved root key]" }

// GoString hides the secrets.
func (Resolved) GoString() string { return "rootkey.Resolved{[resolved root key]}" }

// Path returns root.key's path in dataDir.
func Path(dataDir string) string { return filepath.Join(dataDir, FileName) }

// Resolve finds the root key of a start:
//
//   - root.key in the data directory, when it exists. A configured
//     module_runtime.ca_kek must then be the same key: a different one is
//     refused, since only one of them sealed the stored data.
//   - Otherwise a configured module_runtime.ca_kek is adopted: it becomes
//     the root key, so everything it sealed stays readable, and (with
//     Write) it is written to root.key. When the data directory cannot be
//     written, the key is used from the configuration and a note says so.
//   - Otherwise, with Write, a new random key is generated and written; a
//     data directory that cannot be written is refused. Without Write the
//     result is nil: there is no root key.
//
// The JWT secret and the identity KEK are the configured ones when set (and
// then recorded in root.key, with Write, so that they survive their removal
// from the configuration), else the ones root.key adopted earlier, else
// derived from the key.
func Resolve(opts Options) (*Resolved, error) {
	dataDir := strings.TrimSpace(opts.DataDir)
	if dataDir == "" {
		return nil, errors.New("data_dir is empty: Control keeps its root key there")
	}
	path := Path(dataDir)
	res := &Resolved{Path: path}

	var legacy *Key
	if raw := strings.TrimSpace(opts.LegacyCAKEK); raw != "" {
		key, err := ParseKEK(raw)
		if err != nil {
			return nil, fmt.Errorf("module_runtime.ca_kek: %w", err)
		}
		legacy = &key
	}
	identityKEK := ""
	if raw := strings.TrimSpace(opts.LegacyIdentityKEK); raw != "" {
		kek, err := ParseKEK(raw)
		if err != nil {
			return nil, fmt.Errorf("identity.kek: %w", err)
		}
		identityKEK = kek.Base64()
	}
	jwtSecret := strings.TrimSpace(opts.LegacyJWTSecret)

	file, err := resolveKey(opts.Write, dataDir, path, legacy, File{AdoptedJWTSecret: jwtSecret, AdoptedIdentityKEK: identityKEK}, res)
	if err != nil || res.Key.IsZero() {
		return nil, err
	}

	stored := File{Key: res.Key}
	if file != nil {
		stored = *file
	}
	updated := stored
	switch {
	case jwtSecret != "":
		res.JWTSecret, res.JWTSource = jwtSecret, SourceConfigJWT
		updated.AdoptedJWTSecret = jwtSecret
	case stored.AdoptedJWTSecret != "":
		res.JWTSecret, res.JWTSource = stored.AdoptedJWTSecret, SourceAdopted
	default:
		res.JWTSecret, res.JWTSource = res.Key.DerivedJWTSecret(), SourceDerived
	}
	switch {
	case identityKEK != "":
		res.IdentityKEK, res.IdentitySource = identityKEK, SourceConfigIdentity
		updated.AdoptedIdentityKEK = identityKEK
	case stored.AdoptedIdentityKEK != "":
		res.IdentityKEK, res.IdentitySource = stored.AdoptedIdentityKEK, SourceAdopted
	default:
		res.IdentityKEK, res.IdentitySource = res.Key.DerivedIdentityKEK(), SourceDerived
	}
	if res.Stored && file != nil && opts.Write && (updated.AdoptedJWTSecret != stored.AdoptedJWTSecret || updated.AdoptedIdentityKEK != stored.AdoptedIdentityKEK) {
		if err := replace(dataDir, path, updated); err != nil {
			res.Notes = append(res.Notes, fmt.Sprintf("WARNING: the configured jwt.secret or identity.kek cannot be recorded in %s (%v): keep it in the configuration until it can", path, err))
		} else {
			res.Notes = append(res.Notes, "Recorded the configured jwt.secret or identity.kek in "+path+": it stays in use when the configuration no longer sets it")
		}
	}
	return res, nil
}

// resolveKey finds the root key (Resolve) and fills res's key fields. It
// returns the file root.key holds, nil when it holds none; with no key at
// all res.Key stays zero. adopted carries the configured secrets a new
// root.key records.
func resolveKey(write bool, dataDir, path string, legacy *Key, adopted File, res *Resolved) (*File, error) {
	file, err := readExisting(path, write, res)
	if errors.Is(err, errUnreadable) && legacy != nil {
		// The configured key still opens what it sealed; the start checks
		// that against the database.
		res.Key, res.KeySource = *legacy, SourceLegacyCAKEK
		res.Notes = append(res.Notes, fmt.Sprintf("WARNING: %v. Control uses module_runtime.ca_kek from the configuration", err))
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	switch {
	case file != nil:
		if legacy != nil && !legacy.Equal(file.Key) {
			return nil, fmt.Errorf("module_runtime.ca_kek and %s hold different keys, and only one of them sealed this database. "+
				"If root.key is the right one, remove module_runtime.ca_kek from the configuration (anix-control config rewrite drops it); "+
				"if the configured key is, move %s out of the data directory and start again so that Control adopts the configured key", path, path)
		}
		res.Key, res.KeySource, res.Stored = file.Key, SourceFile, true
		if legacy != nil {
			res.Notes = append(res.Notes, "module_runtime.ca_kek is the key in "+path+": remove it from the configuration (anix-control config rewrite)")
		}
		return file, nil
	case legacy != nil:
		res.Key, res.KeySource = *legacy, SourceLegacyCAKEK
		if !write {
			return nil, nil
		}
		created := adopted
		created.Key = *legacy
		existing, err := createExclusive(dataDir, path, created)
		switch {
		case err != nil:
			res.Notes = append(res.Notes, fmt.Sprintf("WARNING: the root key cannot be written to %s (%v): Control uses module_runtime.ca_kek from the configuration. "+
				"Give Control a data_dir it can write (ANIX_CONTROL_DATA_DIR) so that the key is kept there", path, err))
			return nil, nil
		case existing != nil:
			// Another process wrote it first.
			if !existing.Key.Equal(*legacy) {
				return nil, fmt.Errorf("%s appeared while Control adopted module_runtime.ca_kek and holds another key: stop the other Control process and start again", path)
			}
			res.Stored = true
			return existing, nil
		}
		res.Stored = true
		res.Notes = append(res.Notes, "Adopted module_runtime.ca_kek as the root key in "+path+" (mode 0600); it can now be removed from the configuration")
		return &created, nil
	case !write:
		return nil, nil
	}
	key, err := Generate()
	if err != nil {
		return nil, err
	}
	created := adopted
	created.Key = key
	existing, err := createExclusive(dataDir, path, created)
	switch {
	case err != nil:
		return nil, fmt.Errorf("Control keeps its root key in %s and cannot create it: %v. "+
			"Make data_dir (ANIX_CONTROL_DATA_DIR) a directory the user Control runs as can write, for example install -d -m 0700 -o <user> %s", path, err, dataDir)
	case existing != nil:
		res.Key, res.KeySource, res.Stored = existing.Key, SourceFile, true
		return existing, nil
	}
	res.Key, res.KeySource, res.Stored = key, SourceGenerated, true
	res.Notes = append(res.Notes, "Generated the root key "+path+" (mode 0600): back it up with the database")
	return &created, nil
}

// errUnreadable marks a root.key that exists, or may exist, but that this
// process may not read.
var errUnreadable = errors.New("the root key cannot be read")

// ReadFile reads and checks root.key at path (fs.ErrNotExist when absent).
func ReadFile(path string) (File, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- root.key in the operator's data directory.
	if err != nil {
		return File{}, err
	}
	file, err := Parse(data)
	if err != nil {
		return File{}, fmt.Errorf("%s is damaged: %v", path, err)
	}
	return file, nil
}

// readExisting reads root.key when it exists, nil when it does not. A file
// other users can read is tightened to 0600 (with fix) or noted.
func readExisting(path string, fix bool, res *Resolved) (*File, error) {
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
		return nil, nil
	}
	if errors.Is(err, fs.ErrPermission) {
		return nil, fmt.Errorf("%w: %s cannot be reached (%v): data_dir must belong to the user Control runs as", errUnreadable, path, err)
	}
	if err != nil {
		return nil, fmt.Errorf("the root key %s cannot be read: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("the root key %s is not a regular file", path)
	}
	if mode := info.Mode().Perm(); mode&0o077 != 0 {
		if !fix {
			res.Notes = append(res.Notes, fmt.Sprintf("WARNING: the root key %s is readable by other users (mode %04o): chmod 600 %s", path, mode, path))
		} else if err := os.Chmod(path, 0o600); err != nil {
			return nil, fmt.Errorf("the root key %s is readable by other users (mode %04o) and cannot be restricted (%v): chmod 600 %s", path, mode, err, path)
		} else {
			res.Notes = append(res.Notes, fmt.Sprintf("WARNING: the root key %s was readable by other users (mode %04o); it is 0600 now", path, mode))
		}
	}
	file, err := ReadFile(path)
	if errors.Is(err, fs.ErrPermission) {
		return nil, fmt.Errorf("%w: %s cannot be read (%v): it must belong to the user Control runs as", errUnreadable, path, err)
	}
	if err != nil {
		return nil, fmt.Errorf("the root key cannot be used: %w. Restore root.key from the backup taken with this database", err)
	}
	return &file, nil
}

// prepareDir creates the data directory with mode 0700 and restricts an
// existing one other users can enter.
func prepareDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	info, err := os.Stat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", dir)
	}
	if info.Mode().Perm()&0o077 != 0 {
		// #nosec G302 -- 0700 is the private directory mode.
		if err := os.Chmod(dir, 0o700); err != nil {
			return fmt.Errorf("%s is open to other users (mode %04o) and cannot be restricted: %w", dir, info.Mode().Perm(), err)
		}
	}
	return nil
}

// writeTemp writes file to a new 0600 file in dir and returns its path.
func writeTemp(dir string, file File) (string, error) {
	temporary, err := os.CreateTemp(dir, ".root.key-*")
	if err != nil {
		return "", err
	}
	name := temporary.Name()
	fail := func(err error) (string, error) {
		_ = temporary.Close()
		_ = os.Remove(name)
		return "", err
	}
	if err := temporary.Chmod(0o600); err != nil {
		return fail(err)
	}
	if _, err := temporary.Write(file.Marshal()); err != nil {
		return fail(err)
	}
	if err := temporary.Sync(); err != nil {
		return fail(err)
	}
	if err := temporary.Close(); err != nil {
		_ = os.Remove(name)
		return "", err
	}
	return name, nil
}

// createExclusive writes file to path unless path exists; then it returns
// the existing file (another process created it first).
func createExclusive(dir, path string, file File) (*File, error) {
	if err := prepareDir(dir); err != nil {
		return nil, err
	}
	temporary, err := writeTemp(dir, file)
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.Remove(temporary) }()
	if err := os.Link(temporary, path); err != nil {
		if errors.Is(err, fs.ErrExist) {
			existing, readErr := ReadFile(path)
			if readErr != nil {
				return nil, readErr
			}
			return &existing, nil
		}
		return nil, err
	}
	syncDir(dir)
	return nil, nil
}

// replace rewrites path with file atomically.
func replace(dir, path string, file File) error {
	temporary, err := writeTemp(dir, file)
	if err != nil {
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	syncDir(dir)
	return nil
}

// syncDir makes a new directory entry durable; a failure only loses
// durability, which the next start notices.
func syncDir(dir string) {
	handle, err := os.Open(dir) // #nosec G304 -- the data directory.
	if err != nil {
		return
	}
	_ = handle.Sync()
	_ = handle.Close()
}
