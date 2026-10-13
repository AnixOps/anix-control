package backup

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

// Manager runs the backups of one Control: it writes bundles, verifies
// them, keeps v5_backup in step with the directory and applies the
// retention. One operation runs at a time per Manager.
type Manager struct {
	// DataDir is the data directory.
	DataDir string
	// Driver is the database driver; anything but SQLite answers
	// ErrNeedsSQLite.
	Driver string
	// DatabasePath is the SQLite database file.
	DatabasePath string
	// ConfigPath is the configuration file Control loaded; "" for none.
	ConfigPath string
	// RootKey answers the content of root.key.
	RootKey func() ([]byte, error)
	// Domain is the panel's domain (the manifest, the daily time).
	Domain string
	// ProgramVersion is this Control's version.
	ProgramVersion string
	// DB is the live database, where v5_backup is; nil works on the
	// directory alone.
	DB *gorm.DB
	// Now defaults to time.Now; HostName to os.Hostname.
	Now      func() time.Time
	HostName func() (string, error)
	// Logf receives one line per backup, verification and removal; nil
	// discards them.
	Logf func(format string, args ...any)

	mu sync.Mutex
}

var defaultManager struct {
	sync.RWMutex
	manager *Manager
}

// SetDefault makes m the process's Manager, which the API and the alert
// scan read (Default).
func SetDefault(m *Manager) {
	defaultManager.Lock()
	defer defaultManager.Unlock()
	defaultManager.manager = m
}

// Default is the process's Manager; nil before SetDefault.
func Default() *Manager {
	defaultManager.RLock()
	defer defaultManager.RUnlock()
	return defaultManager.manager
}

func (m *Manager) now() time.Time {
	if m.Now != nil {
		return m.Now().UTC()
	}
	return time.Now().UTC()
}

func (m *Manager) logf(format string, args ...any) {
	if m.Logf != nil {
		m.Logf(format, args...)
	}
}

// Supported answers ErrNeedsSQLite on PostgreSQL, and why backups cannot
// run otherwise (no data directory, an in-memory database).
func (m *Manager) Supported() error {
	switch strings.ToLower(strings.TrimSpace(m.Driver)) {
	case "", "sqlite", "sqlite3":
	default:
		return ErrNeedsSQLite
	}
	if strings.TrimSpace(m.DataDir) == "" {
		return errors.New("data_dir is empty: backups are kept in data_dir/backups")
	}
	if inMemory(m.DatabasePath) {
		return errors.New("the database is in memory: there is nothing to back up")
	}
	return nil
}

// Dir is the bundle directory.
func (m *Manager) Dir() string { return Dir(m.DataDir) }

// ErrBusy answers a request while another backup operation of this
// Control runs.
var ErrBusy = errors.New("another backup or verification is running; try again when it finished")

// tryLock takes the Manager's lock without waiting.
func (m *Manager) tryLock() error {
	if !m.mu.TryLock() {
		return ErrBusy
	}
	return nil
}

// Create writes a bundle of type t and records it in v5_backup (pending
// verification). It answers ErrBusy while another operation runs.
func (m *Manager) Create(ctx context.Context, t string) (*Bundle, *model.Backup, error) {
	if err := m.Supported(); err != nil {
		return nil, nil, err
	}
	if err := m.tryLock(); err != nil {
		return nil, nil, err
	}
	defer m.mu.Unlock()
	return m.create(ctx, t)
}

// CreateWait is Create waiting for another operation to end instead of
// answering ErrBusy: the pre-migration backup and the daily one.
func (m *Manager) CreateWait(ctx context.Context, t string) (*Bundle, *model.Backup, error) {
	if err := m.Supported(); err != nil {
		return nil, nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.create(ctx, t)
}

func (m *Manager) create(ctx context.Context, t string) (*Bundle, *model.Backup, error) {
	source, err := m.source()
	if err != nil {
		return nil, nil, err
	}
	started := time.Now()
	bundle, err := Write(ctx, source, t, m.now())
	if err != nil {
		return nil, nil, err
	}
	m.logf("Backup: wrote %s (%s, %d MiB, schema %04d) in %s.", bundle.Path, t, bundle.Size>>20, bundle.Manifest.SchemaVersion, time.Since(started).Round(time.Millisecond))
	row, err := m.record(ctx, bundle)
	if err != nil {
		m.logf("Backup: %s was written but not recorded in v5_backup: %v", bundle.Name, err)
	}
	return bundle, row, nil
}

func (m *Manager) source() (Source, error) {
	if m.RootKey == nil {
		return Source{}, errors.New("no root key: start Control once so that it creates root.key in data_dir")
	}
	rootKey, err := m.RootKey()
	if err != nil {
		return Source{}, fmt.Errorf("root.key: %w", err)
	}
	var config []byte
	if strings.TrimSpace(m.ConfigPath) != "" {
		config, err = os.ReadFile(m.ConfigPath) // #nosec G304 -- the configuration file Control loaded.
		if err != nil {
			return Source{}, fmt.Errorf("read the configuration file: %w", err)
		}
		if config == nil {
			config = []byte{}
		}
	}
	hostName := ""
	lookup := m.HostName
	if lookup == nil {
		lookup = os.Hostname
	}
	if name, err := lookup(); err == nil {
		hostName = name
	}
	return Source{
		DataDir: m.DataDir, DatabasePath: m.DatabasePath, RootKey: rootKey, Config: config,
		Domain: m.Domain, ProgramVersion: m.ProgramVersion, HostName: hostName,
	}, nil
}

// hasTable reports whether the live database has v5_backup: not before the
// migration that creates it, never without a database.
func (m *Manager) hasTable(ctx context.Context) bool {
	return m.DB != nil && m.DB.WithContext(ctx).Migrator().HasTable(&model.Backup{})
}

// record adds a written bundle to v5_backup.
func (m *Manager) record(ctx context.Context, bundle *Bundle) (*model.Backup, error) {
	row := &model.Backup{
		Path: bundle.Name, Type: bundle.Manifest.Type, Size: bundle.Size, SHA256: bundle.SHA256,
		CreatedAt: bundle.Manifest.CreatedAt, VerifyStatus: model.BackupVerifyPending,
		ProgramVersion: bundle.Manifest.ProgramVersion, SchemaVersion: bundle.Manifest.SchemaVersion,
	}
	if !m.hasTable(ctx) {
		return row, nil
	}
	if err := m.DB.WithContext(ctx).Create(row).Error; err != nil {
		return row, err
	}
	return row, nil
}

// Verify verifies the bundle named name in the bundle directory and
// records the result. It answers the row and the verification's error (a
// *VerifyError when the bundle failed); err is nil when it passed.
func (m *Manager) Verify(ctx context.Context, name string) (*model.Backup, error) {
	if err := m.Supported(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.sync(ctx); err != nil {
		return nil, err
	}
	return m.verify(ctx, name)
}

func (m *Manager) verify(ctx context.Context, name string) (*model.Backup, error) {
	if _, _, ok := ParseFileName(name); !ok {
		return nil, fmt.Errorf("%q is not the name of a backup bundle", name)
	}
	started := time.Now()
	_, verifyErr := VerifyFile(ctx, filepath.Join(m.Dir(), name), VerifyOptions{ScratchDir: ScratchDir(m.DataDir), ProgramVersion: m.ProgramVersion})
	var failed *VerifyError
	if verifyErr != nil && !errors.As(verifyErr, &failed) {
		// The verification could not run (the file went away, the
		// scratch directory cannot be written): nothing is recorded.
		return nil, verifyErr
	}
	now := m.now()
	row := &model.Backup{Path: name}
	if m.hasTable(ctx) {
		if err := m.DB.WithContext(ctx).Where("path = ?", name).Limit(1).Find(row).Error; err != nil {
			return nil, err
		}
	}
	row.VerifiedAt = &now
	row.VerifyStatus, row.VerifyError = model.BackupVerifyPassed, ""
	if failed != nil {
		row.VerifyStatus, row.VerifyError = model.BackupVerifyFailed, truncate(failed.Error(), 2000)
		m.logf("Backup: verification of %s FAILED: %v", name, failed)
	} else {
		m.logf("Backup: verified %s in %s: the bundle restores.", name, time.Since(started).Round(time.Millisecond))
	}
	if m.hasTable(ctx) && row.ID != 0 {
		if err := m.DB.WithContext(ctx).Model(&model.Backup{}).Where("id = ?", row.ID).Updates(map[string]any{
			"verified_at": row.VerifiedAt, "verify_status": row.VerifyStatus, "verify_error": row.VerifyError,
		}).Error; err != nil {
			return row, err
		}
	}
	return row, verifyErr
}

// VerifyNewestUnverified verifies the newest bundle that was not verified
// yet, if there is one: after the daily backup, after a pre-migration or
// manual backup, and when the backup worker starts. found is false when
// every bundle was verified.
func (m *Manager) VerifyNewestUnverified(ctx context.Context) (row *model.Backup, found bool, err error) {
	if err := m.Supported(); err != nil {
		return nil, false, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.sync(ctx); err != nil {
		return nil, false, err
	}
	rows, err := m.list(ctx)
	if err != nil {
		return nil, false, err
	}
	for _, candidate := range rows {
		if candidate.VerifyStatus == model.BackupVerifyPending {
			row, err := m.verify(ctx, candidate.Path)
			return row, true, err
		}
	}
	return nil, false, nil
}

// List answers the bundles, newest first, with their verification. With
// v5_backup it brings the table in step with the directory first.
func (m *Manager) List(ctx context.Context) ([]model.Backup, error) {
	if err := m.Supported(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.sync(ctx); err != nil {
		return nil, err
	}
	return m.list(ctx)
}

func (m *Manager) list(ctx context.Context) ([]model.Backup, error) {
	if m.hasTable(ctx) {
		var rows []model.Backup
		if err := m.DB.WithContext(ctx).Order("created_at DESC").Order("id DESC").Find(&rows).Error; err != nil {
			return nil, err
		}
		return rows, nil
	}
	files, err := m.files()
	if err != nil {
		return nil, err
	}
	rows := make([]model.Backup, 0, len(files))
	for _, file := range files {
		rows = append(rows, model.Backup{Path: file.name, Type: file.typ, Size: file.size, CreatedAt: file.at, VerifyStatus: model.BackupVerifyPending})
	}
	return rows, nil
}

// Find answers the bundle row id (v5_backup).
func (m *Manager) Find(ctx context.Context, id uint) (*model.Backup, error) {
	if err := m.Supported(); err != nil {
		return nil, err
	}
	if !m.hasTable(ctx) {
		return nil, gorm.ErrRecordNotFound
	}
	var row model.Backup
	if err := m.DB.WithContext(ctx).First(&row, id).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

// Open opens the bundle of row for reading, under the shared lock that
// keeps the retention from removing it. The caller closes the file.
func (m *Manager) Open(row *model.Backup) (*os.File, error) {
	if _, _, ok := ParseFileName(row.Path); !ok {
		return nil, fmt.Errorf("%q is not the name of a backup bundle", row.Path)
	}
	return openShared(filepath.Join(m.Dir(), row.Path))
}

// bundleFile is a bundle in the directory.
type bundleFile struct {
	name string
	typ  string
	at   time.Time
	size int64
}

// files answers the bundles in the directory, newest first. Other files
// (partial bundles, anything an operator put there) are left out.
func (m *Manager) files() ([]bundleFile, error) {
	entries, err := os.ReadDir(m.Dir())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var files []bundleFile
	for _, entry := range entries {
		at, typ, ok := ParseFileName(entry.Name())
		if !ok || !entry.Type().IsRegular() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, bundleFile{name: entry.Name(), typ: typ, at: at, size: info.Size()})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].at.After(files[j].at) })
	return files, nil
}

// sync brings v5_backup in step with the directory: a bundle without a row
// (written before the table existed, or by another process) gets one from
// its manifest, pending verification; the row of a bundle that is gone is
// removed.
func (m *Manager) sync(ctx context.Context) error {
	if !m.hasTable(ctx) {
		return nil
	}
	files, err := m.files()
	if err != nil {
		return err
	}
	var rows []model.Backup
	if err := m.DB.WithContext(ctx).Find(&rows).Error; err != nil {
		return err
	}
	known := map[string]bool{}
	for _, row := range rows {
		known[row.Path] = true
	}
	present := map[string]bool{}
	for _, file := range files {
		present[file.name] = true
		if known[file.name] {
			continue
		}
		row := model.Backup{Path: file.name, Type: file.typ, Size: file.size, CreatedAt: file.at, VerifyStatus: model.BackupVerifyPending}
		if manifest, err := ReadManifest(filepath.Join(m.Dir(), file.name)); err == nil {
			row.CreatedAt, row.ProgramVersion, row.SchemaVersion = manifest.CreatedAt, manifest.ProgramVersion, manifest.SchemaVersion
		}
		if err := m.DB.WithContext(ctx).Create(&row).Error; err != nil {
			return fmt.Errorf("record %s: %w", file.name, err)
		}
	}
	for _, row := range rows {
		if !present[row.Path] {
			if err := m.DB.WithContext(ctx).Delete(&model.Backup{}, row.ID).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

// ReadManifest reads the manifest of the bundle at path without verifying
// the bundle.
func ReadManifest(path string) (*Manifest, error) {
	file, err := openShared(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	reader := tar.NewReader(file)
	header, err := reader.Next()
	if err != nil {
		return nil, problem(ProblemArchive, "the bundle is not a readable tar archive: %v", err)
	}
	if header.Name != MemberManifest {
		return nil, problem(ProblemManifest, "the bundle does not start with %s", MemberManifest)
	}
	return readManifest(reader, header.Size)
}

// staleScratchAfter: a scratch directory or partial bundle older than this
// is left from a process that died, and the retention removes it.
const staleScratchAfter = 24 * time.Hour

// Prune applies the retention: it removes the bundles Plan does not keep,
// unless one is being read (verified, downloaded, restored), and the
// scratch files a crashed process left. It answers the decisions.
func (m *Manager) Prune(ctx context.Context) ([]Decision, error) {
	if err := m.Supported(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.sync(ctx); err != nil {
		return nil, err
	}
	rows, err := m.list(ctx)
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(rows))
	ids := map[string]uint{}
	for _, row := range rows {
		entries = append(entries, Entry{Name: row.Path, Type: row.Type, CreatedAt: row.CreatedAt, Verified: row.VerifyStatus == model.BackupVerifyPassed})
		ids[row.Path] = row.ID
	}
	decisions := Plan(entries, m.now())
	for _, decision := range decisions {
		if decision.Keep {
			continue
		}
		removed, err := removeUnlessInUse(filepath.Join(m.Dir(), decision.Name))
		if err != nil {
			m.logf("Backup: could not remove %s: %v", decision.Name, err)
			continue
		}
		if !removed {
			m.logf("Backup: %s is in use; the retention removes it next time.", decision.Name)
			continue
		}
		m.logf("Backup: removed %s (%s).", decision.Name, decision.Reason)
		if id := ids[decision.Name]; id != 0 && m.hasTable(ctx) {
			if err := m.DB.WithContext(ctx).Delete(&model.Backup{}, id).Error; err != nil {
				return decisions, err
			}
		}
	}
	m.removeStale()
	return decisions, nil
}

// removeStale removes the partial bundles and scratch directories of
// Control's backups that are older than staleScratchAfter.
func (m *Manager) removeStale() {
	cutoff := time.Now().Add(-staleScratchAfter)
	for _, dir := range []struct {
		path     string
		prefixes []string
	}{
		{m.Dir(), []string{".partial-"}},
		{ScratchDir(m.DataDir), []string{"backup-", "verify-", "restore-"}},
	} {
		entries, err := os.ReadDir(dir.path)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			matches := false
			for _, prefix := range dir.prefixes {
				matches = matches || strings.HasPrefix(entry.Name(), prefix)
			}
			info, err := entry.Info()
			if !matches || err != nil || info.ModTime().After(cutoff) {
				continue
			}
			if err := os.RemoveAll(filepath.Join(dir.path, entry.Name())); err == nil {
				m.logf("Backup: removed %s, left by an interrupted backup.", filepath.Join(dir.path, entry.Name()))
			}
		}
	}
}

// truncate cuts value to at most limit bytes, at a character boundary.
func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(value[cut]) {
		cut--
	}
	return value[:cut]
}
