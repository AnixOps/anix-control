package packagestore

import (
	"context"
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// provisionLockKey serializes package storage provisioning across kernel
// processes; role names are cluster-wide.
const provisionLockKey int64 = 0x616e6978706b67 // "anixpkg"

const scramIterations = 4096

var grantTargetPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func (s Store) leasePostgres(ctx context.Context, holder Holder, grants Grants) (Lease, error) {
	for _, name := range append(append([]string(nil), grants.AdoptTables...), grants.Views...) {
		if !grantTargetPattern.MatchString(name) {
			return Lease{}, fmt.Errorf("invalid storage grant target %q", name)
		}
	}
	db := s.DB.WithContext(ctx)
	var privileges struct {
		UserName  string
		CanCreate bool
	}
	if err := db.Raw("SELECT current_user AS user_name, (rolcreaterole OR rolsuper) AS can_create FROM pg_roles WHERE rolname = current_user").
		Scan(&privileges).Error; err != nil {
		return Lease{}, err
	}
	if !privileges.CanCreate {
		return Lease{}, fmt.Errorf("%w: run ALTER ROLE %s CREATEROLE as a superuser", ErrCreateRoleRequired, quoteIdent(privileges.UserName))
	}
	password, verifier, err := newRolePassword()
	if err != nil {
		return Lease{}, err
	}
	role, schema := RoleName(holder.PackageID), SchemaName(holder.PackageID)
	var generation int64
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", provisionLockKey).Error; err != nil {
			return err
		}
		var kernelSchema string
		if err := tx.Raw("SELECT current_schema()").Scan(&kernelSchema).Error; err != nil {
			return err
		}
		if kernelSchema == "" {
			return errors.New("the kernel connection has no current schema")
		}
		if err := ensurePackageRole(tx, role, verifier); err != nil {
			return err
		}
		if err := ensurePackageSchema(tx, role, schema, kernelSchema); err != nil {
			return err
		}
		if err := applyPackageGrants(tx, role, kernelSchema, grants); err != nil {
			return err
		}
		generation, err = s.recordLease(tx, holder, grants, model.PackageStorage{
			Driver: DriverPostgres, RoleName: role, SchemaName: schema,
		})
		return err
	})
	if err != nil {
		return Lease{}, err
	}
	host := ""
	if holder.Remote {
		host = s.RemoteDatabaseHost
	}
	dsn, err := packageDSNWithHost(s.DSN, host, role, password, "anix-pkg-"+holder.PackageID)
	if err != nil {
		return Lease{}, err
	}
	return Lease{
		Driver: DriverPostgres, DSN: dsn, Schema: schema, LeaseGeneration: generation,
		AdoptedTables: grants.AdoptTables, Views: grants.Views,
	}, nil
}

func roleExists(tx *gorm.DB, role string) (bool, error) {
	var count int64
	err := tx.Raw("SELECT count(*) FROM pg_roles WHERE rolname = ?", role).Scan(&count).Error
	return count > 0, err
}

// ensurePackageRole creates or resets the package role and sets its password
// from a SCRAM verifier, so the password itself never reaches the server or a
// statement log.
func ensurePackageRole(tx *gorm.DB, role, verifier string) error {
	group, err := roleExists(tx, PackagesGroupRole)
	if err != nil {
		return err
	}
	if !group {
		if err := tx.Exec("CREATE ROLE " + quoteIdent(PackagesGroupRole) + " NOLOGIN").Error; err != nil {
			return err
		}
	}
	exists, err := roleExists(tx, role)
	if err != nil {
		return err
	}
	// A new role gets every restriction spelled out. PostgreSQL lets only a
	// superuser name SUPERUSER or REPLICATION in ALTER ROLE, so an existing
	// role keeps those and only has its login settings reset.
	statement := "ALTER ROLE " + quoteIdent(role) + " LOGIN NOINHERIT NOCREATEROLE"
	if !exists {
		statement = "CREATE ROLE " + quoteIdent(role) + " LOGIN NOINHERIT NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS"
	}
	statement += fmt.Sprintf(" CONNECTION LIMIT %d PASSWORD '%s'", ConnectionLimit, verifier)
	// The verifier is not the password, but keep it out of the kernel log.
	quiet := tx.Session(&gorm.Session{Logger: logger.Discard})
	if err := quiet.Exec(statement).Error; err != nil {
		return err
	}
	var elevated int64
	if err := tx.Raw(`SELECT count(*) FROM pg_roles WHERE rolname = ?
		AND (rolsuper OR rolcreatedb OR rolcreaterole OR rolreplication OR rolbypassrls OR rolinherit)`, role).Scan(&elevated).Error; err != nil {
		return err
	}
	if elevated > 0 {
		return fmt.Errorf("package role %s has elevated attributes; remove them or drop the role", role)
	}
	return tx.Exec("GRANT " + quoteIdent(PackagesGroupRole) + " TO " + quoteIdent(role)).Error
}

// ensurePackageSchema creates the package schema, owned by the kernel, in
// which the package role may create and own its tables.
func ensurePackageSchema(tx *gorm.DB, role, schema, kernelSchema string) error {
	for _, statement := range []string{
		"CREATE SCHEMA IF NOT EXISTS " + quoteIdent(schema),
		"REVOKE ALL ON SCHEMA " + quoteIdent(schema) + " FROM PUBLIC",
		"GRANT USAGE, CREATE ON SCHEMA " + quoteIdent(schema) + " TO " + quoteIdent(role),
		"ALTER ROLE " + quoteIdent(role) + " SET search_path = " + quoteIdent(schema) + ", " + quoteIdent(kernelSchema),
	} {
		if err := tx.Exec(statement).Error; err != nil {
			return err
		}
	}
	return nil
}

// applyPackageGrants replaces every privilege the role holds in the kernel
// schema with exactly the declared ones: DML on adopted tables and their
// sequences, and SELECT on kernel API views.
func applyPackageGrants(tx *gorm.DB, role, kernelSchema string, grants Grants) error {
	for _, statement := range []string{
		"REVOKE ALL ON ALL TABLES IN SCHEMA " + quoteIdent(kernelSchema) + " FROM " + quoteIdent(role),
		"REVOKE ALL ON ALL SEQUENCES IN SCHEMA " + quoteIdent(kernelSchema) + " FROM " + quoteIdent(role),
	} {
		if err := tx.Exec(statement).Error; err != nil {
			return err
		}
	}
	for _, table := range grants.AdoptTables {
		if err := requireRelation(tx, kernelSchema, table, "r", "p"); err != nil {
			return err
		}
		if err := tx.Exec("GRANT SELECT, INSERT, UPDATE, DELETE ON " + quoteIdent(kernelSchema) + "." + quoteIdent(table) + " TO " + quoteIdent(role)).Error; err != nil {
			return err
		}
		var sequences []string
		if err := tx.Raw(`SELECT s.relname FROM pg_class s
			JOIN pg_namespace n ON n.oid = s.relnamespace
			JOIN pg_depend d ON d.objid = s.oid AND d.classid = 'pg_class'::regclass AND d.refclassid = 'pg_class'::regclass
			JOIN pg_class t ON t.oid = d.refobjid
			WHERE s.relkind = 'S' AND d.deptype IN ('a', 'i') AND n.nspname = ? AND t.relname = ? AND t.relnamespace = n.oid
			ORDER BY s.relname`, kernelSchema, table).Scan(&sequences).Error; err != nil {
			return err
		}
		for _, sequence := range sequences {
			if err := tx.Exec("GRANT USAGE, SELECT ON SEQUENCE " + quoteIdent(kernelSchema) + "." + quoteIdent(sequence) + " TO " + quoteIdent(role)).Error; err != nil {
				return err
			}
		}
	}
	for _, view := range grants.Views {
		if err := requireRelation(tx, kernelSchema, view, "v", "m"); err != nil {
			return err
		}
		if err := tx.Exec("GRANT SELECT ON " + quoteIdent(kernelSchema) + "." + quoteIdent(view) + " TO " + quoteIdent(role)).Error; err != nil {
			return err
		}
	}
	return nil
}

func requireRelation(tx *gorm.DB, schema, name string, kinds ...string) error {
	var count int64
	if err := tx.Raw(`SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = ? AND c.relname = ? AND c.relkind::text IN ?`, schema, name, kinds).Scan(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("%w: %s", ErrGrantTargetMissing, name)
	}
	return nil
}

// newRolePassword returns a random password and its SCRAM-SHA-256 verifier in
// the format PostgreSQL stores (RFC 5803).
func newRolePassword() (password, verifier string, err error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	password = base64.RawURLEncoding.EncodeToString(raw)
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", "", err
	}
	verifier, err = scramVerifier(password, salt, scramIterations)
	return password, verifier, err
}

func scramVerifier(password string, salt []byte, iterations int) (string, error) {
	salted, err := pbkdf2.Key(sha256.New, password, salt, iterations, sha256.Size)
	if err != nil {
		return "", err
	}
	mac := func(key []byte, message string) []byte {
		h := hmac.New(sha256.New, key)
		h.Write([]byte(message))
		return h.Sum(nil)
	}
	storedKey := sha256.Sum256(mac(salted, "Client Key"))
	serverKey := mac(salted, "Server Key")
	encode := base64.StdEncoding.EncodeToString
	return fmt.Sprintf("SCRAM-SHA-256$%d:%s$%s:%s", iterations, encode(salt), encode(storedKey[:]), encode(serverKey)), nil
}
