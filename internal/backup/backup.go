// Package backup writes, verifies, keeps and restores Control's backup
// bundles (docs/reference/backups.md; single-binary design, sections 3.8
// and 3.9).
//
// A bundle is one uncompressed tar file in <data_dir>/backups, named
// anix-control-<UTC time>-<type>.tar (mode 0600, the directory 0700), with
// four members in this order:
//
//	manifest.json    format, program and schema version, time, type, host,
//	                 domain, the root key's fingerprint, every other member's
//	                 size and SHA-256, and the row counts of key tables
//	database.sqlite  a consistent snapshot of the SQLite database
//	root.key         the root key file of the data directory
//	config.yaml      the configuration file Control loaded (only when it
//	                 loaded one)
//
// The bundle is not encrypted: it sits on the same disk as the database and
// the root key, so a key beside it would protect nothing (design B14). An
// off-site copy (M4) is encrypted with a key of its own.
//
// The snapshot is taken with VACUUM INTO on a dedicated short-lived
// connection to the database file (Snapshot): it runs in one read
// transaction, so it is consistent, and in WAL mode it neither waits for
// nor blocks the panel's single writer connection.
//
// Backups need SQLite. On PostgreSQL every operation answers ErrNeedsSQLite
// and nothing is written.
package backup

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Bundle types.
const (
	// TypeDaily is the daily backup of the backup worker.
	TypeDaily = "daily"
	// TypePreMigration is taken at a start, before a pending schema
	// migration runs on a database that is not empty.
	TypePreMigration = "pre-migration"
	// TypeManual is taken on request: the console's 立即备份 or
	// `anix-control backup create`.
	TypeManual = "manual"
	// TypeVerify is a bundle taken only to prove that a backup can be
	// written and verified end to end (`backup create -type verify`).
	TypeVerify = "verify"
)

// Types lists the bundle types.
var Types = []string{TypeDaily, TypePreMigration, TypeManual, TypeVerify}

// ValidType reports whether t is a bundle type.
func ValidType(t string) bool {
	for _, known := range Types {
		if t == known {
			return true
		}
	}
	return false
}

// The members of a bundle.
const (
	MemberManifest = "manifest.json"
	MemberDatabase = "database.sqlite"
	MemberRootKey  = "root.key"
	MemberConfig   = "config.yaml"
)

// ManifestFormat is the format of manifest.json this Control writes and
// reads.
const ManifestFormat = "anixops.backup/v1"

// Directory and file names in the data directory.
const (
	// DirName holds the bundles (0700).
	DirName = "backups"
	// ScratchDirName holds the snapshots being written and the bundles
	// being verified or restored (0700).
	ScratchDirName = "tmp"
	// LockFileName is the lock the running Control holds (Lock).
	LockFileName = "control.lock"
)

const (
	dirMode  os.FileMode = 0o700
	fileMode os.FileMode = 0o600
)

// RowCountTables are the tables whose row counts a manifest records and a
// verification compares.
var RowCountTables = []string{"v2_user", "v2_node", "v2_order"}

// ErrNeedsSQLite answers every backup operation on PostgreSQL.
var ErrNeedsSQLite = errors.New("backups need SQLite: this Control runs on PostgreSQL, whose data Control's backups do not cover. Back the database up with pg_dump and keep root.key with it")

// Manifest is manifest.json.
type Manifest struct {
	Format         string    `json:"format"`
	ProgramVersion string    `json:"program_version"`
	SchemaVersion  int       `json:"schema_version"`
	CreatedAt      time.Time `json:"created_at"`
	Type           string    `json:"type"`
	HostName       string    `json:"host_name"`
	// Domain is the panel's domain when the configuration set one.
	Domain string `json:"domain,omitempty"`
	// RootKeyFingerprint is rootkey.Key.Fingerprint of root.key: never a
	// digest of the key itself.
	RootKeyFingerprint string `json:"root_key_fingerprint"`
	// Members are the bundle's other members, in their order.
	Members []Member `json:"members"`
	// RowCounts are the row counts of RowCountTables the snapshot has.
	RowCounts map[string]int64 `json:"row_counts"`
}

// Member is a member of a bundle as the manifest records it.
type Member struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// member answers the manifest's record of name.
func (m *Manifest) member(name string) (Member, bool) {
	for _, member := range m.Members {
		if member.Name == name {
			return member, true
		}
	}
	return Member{}, false
}

// timeFormat is the time in a bundle's name.
const timeFormat = "20060102T150405Z"

var bundleName = regexp.MustCompile(`^anix-control-(\d{8}T\d{6}Z)-(daily|pre-migration|manual|verify)\.tar$`)

// FileName is the name of a bundle of type t taken at at.
func FileName(at time.Time, t string) string {
	return fmt.Sprintf("anix-control-%s-%s.tar", at.UTC().Format(timeFormat), t)
}

// ParseFileName reads the time and type of a bundle's file name; ok is
// false for any other file.
func ParseFileName(name string) (at time.Time, t string, ok bool) {
	match := bundleName.FindStringSubmatch(name)
	if match == nil {
		return time.Time{}, "", false
	}
	at, err := time.Parse(timeFormat, match[1])
	if err != nil {
		return time.Time{}, "", false
	}
	return at.UTC(), match[2], true
}

// Dir is the bundle directory of dataDir.
func Dir(dataDir string) string { return filepath.Join(dataDir, DirName) }

// ScratchDir is the scratch directory of dataDir.
func ScratchDir(dataDir string) string { return filepath.Join(dataDir, ScratchDirName) }

// ensureDir creates dir with mode 0700 and tightens an existing one.
func ensureDir(dir string) error {
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return err
	}
	return os.Chmod(dir, dirMode)
}

// syncDir makes a rename or a removal in dir durable.
func syncDir(dir string) error {
	handle, err := os.Open(dir) // #nosec G304 -- a directory of the data directory.
	if err != nil {
		return err
	}
	defer func() { _ = handle.Close() }()
	return handle.Sync()
}

// sqliteFile is the file part of a SQLite path that may carry query
// parameters.
func sqliteFile(path string) string {
	file, _, _ := strings.Cut(strings.TrimPrefix(strings.TrimSpace(path), "file:"), "?")
	return file
}

// inMemory reports whether a SQLite path names an in-memory database.
func inMemory(path string) bool {
	path = strings.TrimSpace(path)
	return path == "" || path == ":memory:" || strings.HasPrefix(path, "file::memory:") || strings.Contains(path, "mode=memory")
}
