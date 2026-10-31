package backup

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/AnixOps/anix-control/v5/internal/database/migrations"
	"github.com/AnixOps/anix-control/v5/internal/rootkey"
	"github.com/AnixOps/anix-control/v5/internal/rootkeycheck"
	"gorm.io/gorm"
)

// What a verification found wrong (VerifyError.Kind).
const (
	// ProblemArchive: the file is not a readable tar archive.
	ProblemArchive = "archive"
	// ProblemManifest: manifest.json is missing, unreadable or of another
	// format.
	ProblemManifest = "manifest"
	// ProblemMember: a member the manifest lists is missing, or the
	// bundle holds one it does not list.
	ProblemMember = "member"
	// ProblemChecksum: a member's size or SHA-256 is not the manifest's.
	ProblemChecksum = "checksum"
	// ProblemIntegrity: the snapshot cannot be opened or PRAGMA
	// integrity_check does not answer ok.
	ProblemIntegrity = "integrity"
	// ProblemMigrations: the snapshot's migration records are not the
	// manifest's, or this Control may not open the database (a newer
	// Control migrated it: the start's own message).
	ProblemMigrations = "migrations"
	// ProblemRootKey: root.key is unreadable, is not the manifest's, or
	// does not open what the snapshot holds sealed.
	ProblemRootKey = "root_key"
	// ProblemRowCounts: the snapshot's key tables do not have the row
	// counts the manifest recorded.
	ProblemRowCounts = "row_counts"
)

// VerifyError is a bundle that failed its verification.
type VerifyError struct {
	Kind string
	Err  error
}

func (e *VerifyError) Error() string { return e.Kind + ": " + e.Err.Error() }

func (e *VerifyError) Unwrap() error { return e.Err }

func problem(kind string, format string, args ...any) error {
	return &VerifyError{Kind: kind, Err: fmt.Errorf(format, args...)}
}

// Member size limits; the database has none beyond the manifest's size.
const (
	maxManifestBytes = 1 << 20
	maxRootKeyBytes  = 16 << 10
	maxConfigBytes   = 4 << 20
)

// Verified is a bundle that passed its verification.
type Verified struct {
	Manifest Manifest
	// Dir holds the extracted members while the caller keeps them
	// (VerifyOptions.Keep); "" otherwise.
	Dir string
}

// VerifyOptions configure VerifyFile.
type VerifyOptions struct {
	// ScratchDir is where the bundle is extracted (DataDir/tmp); a
	// directory verify-* is created in it and removed afterwards.
	ScratchDir string
	// ProgramVersion is this Control's version: the snapshot must be one
	// it may open (migrations.Check).
	ProgramVersion string
	// Postgres is how a PostgreSQL bundle is verified: its dump is restored
	// into a scratch database on this server. Nil verifies SQLite bundles
	// only.
	Postgres *Postgres
	// Keep leaves the extracted members in Verified.Dir for the caller
	// (a restore), who removes them.
	Keep bool
}

// VerifyFile verifies the bundle at path, the way a restore would use it:
//
//  1. the file is a tar archive of regular files with the members a
//     bundle has, manifest.json first, of format anixops.backup/v1;
//  2. every member it lists is there with its size and SHA-256, and no
//     other;
//  3. the snapshot, extracted to a scratch directory, opens and PRAGMA
//     integrity_check answers ok;
//  4. its migration records are consistent with the manifest and this
//     Control may open it (migrations.Check);
//  5. root.key parses, is the key the manifest names, and opens the CA
//     keys and sealed material of the snapshot (rootkeycheck.Check);
//  6. the key tables have the manifest's row counts.
//
// A failure is a *VerifyError naming the problem. The bundle is read under
// a shared lock, so the retention never removes it meanwhile.
func VerifyFile(ctx context.Context, path string, options VerifyOptions) (*Verified, error) {
	if err := ensureDir(options.ScratchDir); err != nil {
		return nil, fmt.Errorf("prepare %s: %w", options.ScratchDir, err)
	}
	file, err := openShared(path)
	if err != nil {
		return nil, fmt.Errorf("open the bundle: %w", err)
	}
	defer func() { _ = file.Close() }()
	work, err := os.MkdirTemp(options.ScratchDir, "verify-")
	if err != nil {
		return nil, fmt.Errorf("prepare the verification directory: %w", err)
	}
	keep := false
	defer func() {
		if !keep {
			_ = os.RemoveAll(work)
		}
	}()

	manifest, contents, err := extract(file, work)
	if err != nil {
		return nil, err
	}
	if manifest.Kind() == KindPostgres {
		if options.Postgres == nil {
			return nil, ErrPostgresBundle
		}
		if err := options.Postgres.CheckTools(); err != nil {
			return nil, err
		}
		if err := checkPostgresDump(ctx, options.Postgres, filepath.Join(work, MemberDatabasePG), manifest, contents[MemberRootKey], options.ProgramVersion); err != nil {
			return nil, err
		}
	} else if err := checkSnapshot(ctx, filepath.Join(work, MemberDatabase), manifest, contents[MemberRootKey], options.ProgramVersion); err != nil {
		return nil, err
	}
	verified := &Verified{Manifest: *manifest}
	if options.Keep {
		keep, verified.Dir = true, work
	}
	return verified, nil
}

// extract reads the archive into work: the manifest, and every member it
// lists, checked against its size and SHA-256. The small members are also
// answered in memory.
func extract(file io.Reader, work string) (*Manifest, map[string][]byte, error) {
	reader := tar.NewReader(file)
	var manifest *Manifest
	seen := map[string]bool{}
	contents := map[string][]byte{}
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, nil, problem(ProblemArchive, "the bundle is not a readable tar archive: %v", err)
		}
		name := header.Name
		if header.Typeflag != tar.TypeReg {
			return nil, nil, problem(ProblemArchive, "member %q is not a regular file", name)
		}
		if seen[name] {
			return nil, nil, problem(ProblemMember, "%s is in the bundle twice", name)
		}
		seen[name] = true
		if manifest == nil {
			if name != MemberManifest {
				return nil, nil, problem(ProblemManifest, "the bundle does not start with %s (its first member is %q)", MemberManifest, name)
			}
			manifest, err = readManifest(reader, header.Size)
			if err != nil {
				return nil, nil, err
			}
			continue
		}
		want, listed := manifest.member(name)
		if !listed {
			return nil, nil, problem(ProblemMember, "%q is in the bundle but not in the manifest", name)
		}
		if header.Size != want.Size {
			return nil, nil, problem(ProblemChecksum, "%s is %d bytes, the manifest says %d", name, header.Size, want.Size)
		}
		limit := map[string]int64{MemberRootKey: maxRootKeyBytes, MemberConfig: maxConfigBytes}[name]
		if limit > 0 && header.Size > limit {
			return nil, nil, problem(ProblemMember, "%s is %d bytes, more than a bundle's %s can be", name, header.Size, name)
		}
		target, err := os.OpenFile(filepath.Join(work, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, fileMode) // #nosec G304 G305 -- name is one of the manifest's members, checked above (a fixed set of names, never a path).
		if err != nil {
			return nil, nil, err
		}
		sum := sha256.New()
		var memory bytes.Buffer
		writers := []io.Writer{target, sum}
		if limit > 0 {
			writers = append(writers, &memory)
		}
		// Bounded by the size the manifest declares (and the tar header, which
		// was compared with it above): a member cannot expand past it.
		written, copyErr := io.CopyN(io.MultiWriter(writers...), reader, want.Size)
		closeErr := target.Close()
		if copyErr != nil {
			return nil, nil, problem(ProblemArchive, "read %s: %v", name, copyErr)
		}
		if closeErr != nil {
			return nil, nil, closeErr
		}
		if written != want.Size {
			return nil, nil, problem(ProblemChecksum, "%s is %d bytes, the manifest says %d", name, written, want.Size)
		}
		if got := hex.EncodeToString(sum.Sum(nil)); got != want.SHA256 {
			return nil, nil, problem(ProblemChecksum, "%s has SHA-256 %s, the manifest says %s", name, got, want.SHA256)
		}
		if limit > 0 {
			contents[name] = memory.Bytes()
		}
	}
	if manifest == nil {
		return nil, nil, problem(ProblemManifest, "the bundle has no %s", MemberManifest)
	}
	for _, member := range manifest.Members {
		if !seen[member.Name] {
			return nil, nil, problem(ProblemMember, "%s is missing from the bundle", member.Name)
		}
	}
	return manifest, contents, nil
}

func readManifest(reader io.Reader, size int64) (*Manifest, error) {
	if size > maxManifestBytes {
		return nil, problem(ProblemManifest, "%s is %d bytes, more than a manifest can be", MemberManifest, size)
	}
	data, err := io.ReadAll(io.LimitReader(reader, maxManifestBytes))
	if err != nil {
		return nil, problem(ProblemArchive, "read %s: %v", MemberManifest, err)
	}
	var manifest Manifest
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&manifest); err != nil {
		return nil, problem(ProblemManifest, "%s is not valid JSON: %v", MemberManifest, err)
	}
	if manifest.Format != ManifestFormat {
		return nil, problem(ProblemManifest, "%s has format %q, this Control reads %s", MemberManifest, manifest.Format, ManifestFormat)
	}
	if !ValidType(manifest.Type) {
		return nil, problem(ProblemManifest, "%s has the unknown type %q", MemberManifest, manifest.Type)
	}
	if kind := manifest.Kind(); kind != KindSQLite && kind != KindPostgres {
		return nil, problem(ProblemManifest, "%s has the unknown database kind %q", MemberManifest, manifest.DatabaseKind)
	}
	required := map[string]bool{DatabaseMemberName(manifest.Kind()): true, MemberRootKey: true}
	for _, member := range manifest.Members {
		switch member.Name {
		case DatabaseMemberName(manifest.Kind()), MemberRootKey, MemberConfig:
		default:
			return nil, problem(ProblemManifest, "%s lists the unknown member %q", MemberManifest, member.Name)
		}
		if member.Size < 0 || len(member.SHA256) != sha256.Size*2 {
			return nil, problem(ProblemManifest, "%s has no valid size and SHA-256 for %s", MemberManifest, member.Name)
		}
		delete(required, member.Name)
	}
	for name := range required {
		return nil, problem(ProblemMember, "%s does not list %s, which every bundle has", MemberManifest, name)
	}
	return &manifest, nil
}

// checkSnapshot runs the database checks of a verification on the
// extracted snapshot.
func checkSnapshot(ctx context.Context, path string, manifest *Manifest, rootKeyFile []byte, programVersion string) error {
	db, closeDB, err := openSnapshot(path)
	if err != nil {
		return problem(ProblemIntegrity, "the snapshot does not open: %v", err)
	}
	defer closeDB()
	if err := integrityCheck(ctx, db); err != nil {
		return err
	}
	return checkDatabase(ctx, db, manifest, rootKeyFile, programVersion)
}

// checkPostgresDump restores the dump into a scratch database and runs the
// database checks of a verification on it; the scratch database is dropped
// afterwards.
func checkPostgresDump(ctx context.Context, pg *Postgres, dump string, manifest *Manifest, rootKeyFile []byte, programVersion string) error {
	name, drop, err := pg.scratchDatabase(ctx)
	if err != nil {
		return err
	}
	defer drop()
	if err := pg.restoreInto(ctx, name, dump); err != nil {
		return problem(ProblemIntegrity, "the dump does not restore: %v", err)
	}
	db, closeDB, err := pg.openGorm(name)
	if err != nil {
		return problem(ProblemIntegrity, "the restored dump does not open: %v", err)
	}
	defer closeDB()
	return checkDatabase(ctx, db, manifest, rootKeyFile, programVersion)
}

// checkDatabase is what a verification checks in a restored database
// (SQLite snapshot or PostgreSQL scratch database): migrations, the root
// key and the row counts.
func checkDatabase(ctx context.Context, db *gorm.DB, manifest *Manifest, rootKeyFile []byte, programVersion string) error {
	status, err := migrations.Check(ctx, db, programVersion)
	if err != nil {
		return &VerifyError{Kind: ProblemMigrations, Err: err}
	}
	if status.SchemaVersion != manifest.SchemaVersion {
		return problem(ProblemMigrations, "the snapshot's newest migration is %04d, the manifest says %04d", status.SchemaVersion, manifest.SchemaVersion)
	}
	meta, err := migrations.Meta(ctx, db)
	if err != nil {
		return problem(ProblemMigrations, "read %s: %v", migrations.MetaTable, err)
	}
	if recorded, ok := meta[migrations.MetaSchemaVersion]; ok && recorded != fmt.Sprint(status.SchemaVersion) {
		return problem(ProblemMigrations, "%s says schema_version %s, but the newest recorded migration is %04d", migrations.MetaTable, recorded, status.SchemaVersion)
	}

	file, err := rootkey.Parse(rootKeyFile)
	if err != nil {
		return problem(ProblemRootKey, "root.key does not parse: %v", err)
	}
	if fingerprint := file.Key.Fingerprint(); fingerprint != manifest.RootKeyFingerprint {
		return problem(ProblemRootKey, "root.key has the fingerprint %s, the manifest says %s", fingerprint, manifest.RootKeyFingerprint)
	}
	if err := rootkeycheck.Check(ctx, db, file.Key.Bytes(), nil); err != nil {
		return problem(ProblemRootKey, "%v: the bundle's root.key and database are not a pair", err)
	}

	facts, err := factsOf(ctx, db)
	if err != nil {
		return problem(ProblemIntegrity, "%v", err)
	}
	for _, table := range RowCountTables {
		got, has := facts.RowCounts[table]
		want, recorded := manifest.RowCounts[table]
		if has != recorded || got != want {
			return problem(ProblemRowCounts, "%s has %d rows, the manifest says %d", table, got, want)
		}
	}
	return nil
}

// integrityCheck runs PRAGMA integrity_check, which reads every page.
func integrityCheck(ctx context.Context, db *gorm.DB) error {
	rows, err := db.WithContext(ctx).Raw("PRAGMA integrity_check").Rows()
	if err != nil {
		return problem(ProblemIntegrity, "PRAGMA integrity_check failed: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var messages []string
	for rows.Next() {
		var message string
		if err := rows.Scan(&message); err != nil {
			return problem(ProblemIntegrity, "PRAGMA integrity_check failed: %v", err)
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return problem(ProblemIntegrity, "PRAGMA integrity_check failed: %v", err)
	}
	if len(messages) == 1 && messages[0] == "ok" {
		return nil
	}
	if len(messages) > 5 {
		messages = append(messages[:5], fmt.Sprintf("and %d more", len(messages)-5))
	}
	return problem(ProblemIntegrity, "PRAGMA integrity_check: %s", strings.Join(messages, "; "))
}
