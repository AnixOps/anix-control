// Package packagestore provisions least-privilege database storage for
// package hosts.
//
// On PostgreSQL every package gets its own login role anix_pkg_<id> and
// schema pkg_<id>. The role owns only what it creates in its schema; tables
// it adopts and kernel API views it reads are granted from signed manifest
// capabilities, and everything else stays out of reach. The kernel's own
// credentials never leave the kernel. SQLite has no roles: packages share the
// kernel's database file and keep their tables apart by a pkg_<id>_ prefix,
// which isolates nothing and is meant for development and tests.
package packagestore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	DriverPostgres = "postgres"
	DriverSQLite   = "sqlite"

	// RolePrefix and SchemaPrefix name a package's PostgreSQL role and schema.
	RolePrefix   = "anix_pkg_"
	SchemaPrefix = "pkg_"
	// PackagesGroupRole is a NOLOGIN group every package role belongs to, so
	// pg_hba.conf can admit all package roles with "+anix_packages". Package
	// roles are NOINHERIT and the group holds no privileges.
	PackagesGroupRole = "anix_packages"
	// ConnectionLimit caps the connections of one package role.
	ConnectionLimit = 4
)

var (
	// ErrStorageNotDeclared means the package's signed manifest does not
	// declare kernel.storage.v1.
	ErrStorageNotDeclared = errors.New("package does not declare kernel.storage.v1")
	// ErrPackageNotEligible means the package id cannot name a database role.
	ErrPackageNotEligible = errors.New("package id cannot name package storage")
	// ErrCreateRoleRequired means the kernel's PostgreSQL role cannot create
	// package roles.
	ErrCreateRoleRequired = errors.New("the kernel database role needs CREATEROLE to provision package storage")
	// ErrGrantTargetMissing means an adopted table or a kernel API view does
	// not exist in the kernel database.
	ErrGrantTargetMissing = errors.New("adopted table or kernel API view does not exist")
)

// storagePackageIDPattern keeps role and schema names short, lowercase and
// collision-free: '-' maps to '_', and ids cannot contain '_' themselves.
var storagePackageIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,39}$`)

// Grants are the storage capabilities a signed manifest declares.
type Grants struct {
	Storage     bool
	AdoptTables []string
	Views       []string
}

// Holder identifies the package host a lease is issued to.
type Holder struct {
	PackageID  string
	Version    string
	Generation uint64
	// Remote holders are network module instances: they need PostgreSQL and
	// connect through Store.RemoteDatabaseHost when it is set.
	Remote bool
}

// Lease is what a package host receives. DSN connects as the package role on
// PostgreSQL and names the shared database file on SQLite.
type Lease struct {
	Driver          string
	DSN             string
	Schema          string
	TablePrefix     string
	LeaseGeneration int64
	AdoptedTables   []string
	Views           []string
}

// Store provisions package storage in the kernel database.
type Store struct {
	// DB is the kernel's connection.
	DB *gorm.DB
	// Driver is the kernel database driver, as configured.
	Driver string
	// DSN is the kernel's PostgreSQL connection string, or the absolute path
	// of the SQLite database file. Credentials in it are never passed on.
	DSN string
	// Now defaults to time.Now.
	Now func() time.Time
	// RemoteDatabaseHost replaces the host (and port, as host:port) of the
	// kernel DSN in leases for remote holders, which reach the database from
	// another container or pod.
	RemoteDatabaseHost string
	// Leases, when set, reuses one lease per holder generation and grants:
	// every lease rotates the role password, so the replicas of one remote
	// generation must share a lease instead of invalidating each other's.
	Leases *LeaseCache
}

// LeaseCache keeps the latest lease of each package generation.
type LeaseCache struct {
	mu      sync.Mutex
	entries map[string]cachedLease
}

type cachedLease struct {
	key   string
	lease Lease
}

// NewLeaseCache returns an empty cache.
func NewLeaseCache() *LeaseCache {
	return &LeaseCache{entries: make(map[string]cachedLease)}
}

func (c *LeaseCache) get(holder Holder, grants Grants) (Lease, bool) {
	if c == nil {
		return Lease{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[holder.PackageID]
	if !ok || entry.key != leaseCacheKey(holder, grants) {
		return Lease{}, false
	}
	return entry.lease, true
}

func (c *LeaseCache) put(holder Holder, grants Grants, lease Lease) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	// One entry per package: a newer generation or other grants replace it,
	// and the previous password is rotated away anyway.
	c.entries[holder.PackageID] = cachedLease{key: leaseCacheKey(holder, grants), lease: lease}
}

func leaseCacheKey(holder Holder, grants Grants) string {
	return fmt.Sprintf("%s\x00%d\x00%t\x00%s\x00%s", holder.Version, holder.Generation, holder.Remote,
		strings.Join(grants.AdoptTables, ","), strings.Join(grants.Views, ","))
}

// RoleName returns the PostgreSQL role of a package.
func RoleName(packageID string) string {
	return RolePrefix + strings.ReplaceAll(packageID, "-", "_")
}

// SchemaName returns the PostgreSQL schema of a package.
func SchemaName(packageID string) string {
	return SchemaPrefix + strings.ReplaceAll(packageID, "-", "_")
}

// TablePrefix returns the SQLite table prefix of a package.
func TablePrefix(packageID string) string {
	return SchemaName(packageID) + "_"
}

// Lease provisions the holder's storage for grants and returns fresh
// credentials. On PostgreSQL each lease rotates the role password, so the
// previous lease can no longer open new connections; connections it already
// holds stay open.
func (s Store) Lease(ctx context.Context, holder Holder, grants Grants) (Lease, error) {
	if s.DB == nil {
		return Lease{}, errors.New("database is not initialized")
	}
	if !storagePackageIDPattern.MatchString(holder.PackageID) {
		return Lease{}, fmt.Errorf("%w: %q", ErrPackageNotEligible, holder.PackageID)
	}
	if !grants.Storage {
		return Lease{}, ErrStorageNotDeclared
	}
	grants = normalizeGrants(grants)
	if cached, ok := s.Leases.get(holder, grants); ok {
		return cached, nil
	}
	var (
		lease Lease
		err   error
	)
	switch normalizeDriver(s.Driver) {
	case DriverPostgres:
		lease, err = s.leasePostgres(ctx, holder, grants)
	case DriverSQLite:
		if holder.Remote {
			return Lease{}, errors.New("remote package hosts need PostgreSQL storage; they cannot share the kernel's SQLite file")
		}
		lease, err = s.leaseSQLite(ctx, holder, grants)
	default:
		return Lease{}, fmt.Errorf("package storage does not support database driver %q", s.Driver)
	}
	if err != nil {
		return Lease{}, err
	}
	s.Leases.put(holder, grants, lease)
	return lease, nil
}

func normalizeDriver(driver string) string {
	switch strings.ToLower(strings.TrimSpace(driver)) {
	case "postgres", "postgresql":
		return DriverPostgres
	case "sqlite", "sqlite3", "":
		return DriverSQLite
	default:
		return driver
	}
}

func normalizeGrants(grants Grants) Grants {
	unique := func(values []string) []string {
		set := make(map[string]struct{}, len(values))
		for _, value := range values {
			set[value] = struct{}{}
		}
		out := make([]string, 0, len(set))
		for value := range set {
			out = append(out, value)
		}
		sort.Strings(out)
		return out
	}
	return Grants{Storage: grants.Storage, AdoptTables: unique(grants.AdoptTables), Views: unique(grants.Views)}
}

func (s Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// recordLease upserts the package's storage row and returns the new lease
// generation. It runs inside the provisioning transaction.
func (s Store) recordLease(tx *gorm.DB, holder Holder, grants Grants, row model.PackageStorage) (int64, error) {
	grantsJSON, err := json.Marshal(struct {
		AdoptTables []string `json:"adopt_tables"`
		Views       []string `json:"views"`
	}{grants.AdoptTables, grants.Views})
	if err != nil {
		return 0, err
	}
	var existing model.PackageStorage
	err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&existing, "package_id = ?", holder.PackageID).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		existing = model.PackageStorage{PackageID: holder.PackageID, CreatedAt: s.now()}
	case err != nil:
		return 0, err
	}
	row.PackageID = holder.PackageID
	row.GrantsJSON = string(grantsJSON)
	row.PackageVersion = holder.Version
	row.HostGeneration = holder.Generation
	row.LeaseGeneration = existing.LeaseGeneration + 1
	row.CreatedAt = existing.CreatedAt
	row.UpdatedAt = s.now()
	if err := tx.Save(&row).Error; err != nil {
		return 0, err
	}
	return row.LeaseGeneration, nil
}

func (s Store) leaseSQLite(ctx context.Context, holder Holder, grants Grants) (Lease, error) {
	if strings.TrimSpace(s.DSN) == "" {
		return Lease{}, errors.New("sqlite package storage needs the database file path")
	}
	db := s.DB.WithContext(ctx)
	for _, name := range append(append([]string(nil), grants.AdoptTables...), grants.Views...) {
		var count int64
		if err := db.Raw("SELECT count(*) FROM sqlite_master WHERE type IN ('table', 'view') AND name = ?", name).Scan(&count).Error; err != nil {
			return Lease{}, err
		}
		if count == 0 {
			return Lease{}, fmt.Errorf("%w: %s", ErrGrantTargetMissing, name)
		}
	}
	var generation int64
	err := db.Transaction(func(tx *gorm.DB) error {
		var err error
		generation, err = s.recordLease(tx, holder, grants, model.PackageStorage{
			Driver: DriverSQLite, TablePrefix: TablePrefix(holder.PackageID),
		})
		return err
	})
	if err != nil {
		return Lease{}, err
	}
	return Lease{
		Driver: DriverSQLite, DSN: s.DSN, TablePrefix: TablePrefix(holder.PackageID), LeaseGeneration: generation,
		AdoptedTables: grants.AdoptTables, Views: grants.Views,
	}, nil
}
