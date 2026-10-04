package forwardlegacy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
)

// ArchiveSchema is the archive file's schema version.
const ArchiveSchema = "anixops.forward.legacy-archive.v1"

// ArchiveDirName is the archive directory under Control's data directory.
const ArchiveDirName = "forward-legacy"

// Redacted replaces a secret value the archive leaves out.
const Redacted = "[redacted]"

// Archive is the archive file's document.
type Archive struct {
	Schema         string         `json:"schema"`
	CreatedAt      time.Time      `json:"created_at"`
	ControlVersion string         `json:"control_version,omitempty"`
	DatabaseDriver string         `json:"database_driver"`
	Note           string         `json:"note"`
	Tables         []ArchiveTable `json:"tables"`
}

// ArchiveTable is one table's rows.
type ArchiveTable struct {
	Name string `json:"name"`
	// Dropped tells whether the upgrade's drop removes the table.
	Dropped bool `json:"dropped"`
	// Present is false when the table did not exist (a release that
	// already removed it): Rows is then empty.
	Present  bool `json:"present"`
	RowCount int  `json:"row_count"`
	// RedactedColumns are columns whose values were left out.
	RedactedColumns []string         `json:"redacted_columns,omitempty"`
	Rows            []map[string]any `json:"rows"`
}

const archiveNote = "Flux forwarding data of anix-control 4.1, archived by the v4.2 upgrade " +
	"(forward-sdk.md section 10). It is not migrated: forwarding is reconfigured as v4 routes. " +
	"Node tokens and other secrets are left out."

// secretColumns are column names whose values never enter the archive.
var secretColumns = map[string]bool{
	"api_token": true, "token": true, "password": true, "secret": true, "private_key": true,
	"key_hash": true, "api_key": true, "shared_secret": true,
}

// secretColumn reports whether a column holds a secret by its name.
func secretColumn(name string) bool {
	lower := strings.ToLower(name)
	if secretColumns[lower] {
		return true
	}
	return strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "password")
}

// secretKey reports whether a JSON key inside a stored document names a
// secret (a runtime job payload written before NO-7 carried node tokens).
func secretKey(key string) bool {
	lower := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "_", ""), "-", ""))
	for _, marker := range []string{"token", "secret", "password", "apikey", "authorization", "privatekey", "credential"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// minSecretLength is the shortest known secret value the archive scrubs
// from free text: shorter values would match ordinary words.
const minSecretLength = 8

// BuildArchive reads the flux tables and the kept reference tables into
// an archive with every secret left out.
func BuildArchive(ctx context.Context, db *gorm.DB, now time.Time, controlVersion string) (*Archive, error) {
	db = db.WithContext(ctx)
	secrets, err := knownSecrets(db)
	if err != nil {
		return nil, err
	}
	archive := &Archive{
		Schema: ArchiveSchema, CreatedAt: now.UTC(), ControlVersion: controlVersion,
		DatabaseDriver: db.Name(), Note: archiveNote,
	}
	tables := append(append([]string{}, DropTables...), KeptTables...)
	for _, table := range tables {
		entry := ArchiveTable{Name: table, Dropped: isDropTable(table), Rows: []map[string]any{}}
		if db.Migrator().HasTable(table) {
			entry.Present = true
			rows, redacted, err := readTable(db, table, secrets)
			if err != nil {
				return nil, fmt.Errorf("archive %s: %w", table, err)
			}
			entry.Rows, entry.RowCount, entry.RedactedColumns = rows, len(rows), redacted
		}
		archive.Tables = append(archive.Tables, entry)
	}
	return archive, nil
}

// RowCounts answers the archive's row count per table, -1 for a table
// that did not exist.
func (a *Archive) RowCounts() map[string]int64 {
	counts := map[string]int64{}
	for _, table := range a.Tables {
		if !table.Present {
			counts[table.Name] = -1
			continue
		}
		counts[table.Name] = int64(table.RowCount)
	}
	return counts
}

// knownSecrets collects the secret values stored next to the flux data
// (node and clean agent tokens, the node credential split's values), so
// any copy of one inside free text is scrubbed too.
func knownSecrets(db *gorm.DB) ([]string, error) {
	var values []string
	collect := func(table, column string) error {
		if !db.Migrator().HasTable(table) || !db.Migrator().HasColumn(table, column) {
			return nil
		}
		var found []string
		if err := db.Table(table).Where(column+" IS NOT NULL").Pluck(column, &found).Error; err != nil {
			return fmt.Errorf("read %s.%s: %w", table, column, err)
		}
		for _, value := range found {
			if len(strings.TrimSpace(value)) >= minSecretLength {
				values = append(values, value)
			}
		}
		return nil
	}
	for _, source := range [][2]string{
		{"v2_forward_node", "api_token"}, {"v2_forward_clean_agent", "token"}, {"v4_kernel_node_credential", "value"},
	} {
		if err := collect(source[0], source[1]); err != nil {
			return nil, err
		}
	}
	// Longest first, so a secret containing another is replaced whole.
	sort.Slice(values, func(i, j int) bool { return len(values[i]) > len(values[j]) })
	return values, nil
}

// readTable reads every row of table, ordered by id, as column → value.
func readTable(db *gorm.DB, table string, secrets []string) ([]map[string]any, []string, error) {
	query := db.Table(table)
	if db.Migrator().HasColumn(table, "id") {
		query = query.Order("id")
	}
	rows, err := query.Rows()
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return nil, nil, err
	}
	redacted := map[string]bool{}
	result := []map[string]any{}
	for rows.Next() {
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err := rows.Scan(pointers...); err != nil {
			return nil, nil, err
		}
		row := make(map[string]any, len(columns))
		for i, column := range columns {
			if secretColumn(column) {
				redacted[column] = true
				continue
			}
			row[column] = scrubValue(normalizeValue(values[i]), secrets)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	names := make([]string, 0, len(redacted))
	for column := range redacted {
		names = append(names, column)
	}
	sort.Strings(names)
	return result, names, nil
}

// normalizeValue makes a scanned value JSON-friendly.
func normalizeValue(value any) any {
	switch typed := value.(type) {
	case []byte:
		return string(typed)
	case time.Time:
		return typed.UTC().Format(time.RFC3339Nano)
	}
	return value
}

// scrubValue removes secrets from a string value: a JSON document loses
// every key naming a secret, and any known secret value is replaced.
func scrubValue(value any, secrets []string) any {
	text, ok := value.(string)
	if !ok {
		return value
	}
	trimmed := strings.TrimSpace(text)
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		var document any
		if err := json.Unmarshal([]byte(trimmed), &document); err == nil {
			if encoded, err := json.Marshal(scrubDocument(document)); err == nil {
				text = string(encoded)
			}
		}
	}
	for _, secret := range secrets {
		text = strings.ReplaceAll(text, secret, Redacted)
	}
	return text
}

func scrubDocument(document any) any {
	switch typed := document.(type) {
	case map[string]any:
		for key, value := range typed {
			if secretKey(key) {
				delete(typed, key)
				continue
			}
			typed[key] = scrubDocument(value)
		}
		return typed
	case []any:
		for i, value := range typed {
			typed[i] = scrubDocument(value)
		}
		return typed
	}
	return document
}

// ArchiveFileName is the timestamped name of an archive written at now.
func ArchiveFileName(now time.Time) string {
	return "forward-legacy-archive-" + now.UTC().Format("20060102T150405Z") + ".json"
}

// WriteResult is a written and recorded archive.
type WriteResult struct {
	Record model.ForwardLegacyArchive `json:"record"`
	// Tables is each archived table's row count, -1 when it did not exist.
	Tables map[string]int64 `json:"tables"`
}

// WriteOptions say where an archive goes and who wrote it.
type WriteOptions struct {
	// Output is a file that must not exist yet, or an existing directory
	// that receives a timestamped file. Empty means Dir.
	Output string
	// Dir is the archive directory (created 0700 when missing).
	Dir            string
	Trigger        string
	Actor          string
	ControlVersion string
	Now            time.Time
}

// WriteArchive builds the archive, writes it (mode 0600, never over an
// existing file) and records it.
func WriteArchive(ctx context.Context, db *gorm.DB, options WriteOptions) (*WriteResult, error) {
	if err := EnsureSchema(db); err != nil {
		return nil, err
	}
	now := options.Now
	if now.IsZero() {
		now = time.Now()
	}
	path, err := archivePath(options, now)
	if err != nil {
		return nil, err
	}
	archive, err := BuildArchive(ctx, db, now, options.ControlVersion)
	if err != nil {
		return nil, err
	}
	encoded, err := json.MarshalIndent(archive, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := writeExclusive(path, append(encoded, '\n')); err != nil {
		return nil, err
	}
	digest := sha256.Sum256(append(encoded, '\n'))
	counts := archive.RowCounts()
	countsJSON, _ := json.Marshal(counts)
	record := model.ForwardLegacyArchive{
		Path: path, SHA256: hex.EncodeToString(digest[:]), SizeBytes: int64(len(encoded) + 1),
		Schema: ArchiveSchema, Trigger: options.Trigger, Actor: options.Actor, RowCounts: string(countsJSON),
		CreatedAt: now.UTC(),
	}
	if err := db.WithContext(ctx).Create(&record).Error; err != nil {
		return nil, fmt.Errorf("record the archive %s (the file was written): %w", path, err)
	}
	return &WriteResult{Record: record, Tables: counts}, nil
}

// archivePath resolves where the archive goes: never an existing file.
func archivePath(options WriteOptions, now time.Time) (string, error) {
	target := strings.TrimSpace(options.Output)
	if target == "" {
		if strings.TrimSpace(options.Dir) == "" {
			return "", errors.New("no archive directory")
		}
		if err := os.MkdirAll(options.Dir, 0o700); err != nil {
			return "", fmt.Errorf("create the archive directory: %w", err)
		}
		target = options.Dir
	}
	absolute, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(absolute)
	switch {
	case err == nil && info.IsDir():
		return filepath.Join(absolute, ArchiveFileName(now)), nil
	case err == nil:
		return "", fmt.Errorf("%s exists: an archive never overwrites a file", absolute)
	case errors.Is(err, os.ErrNotExist):
		return absolute, nil
	default:
		return "", err
	}
}

// writeExclusive writes a new file with mode 0600, failing when it exists.
func writeExclusive(path string, content []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) // #nosec G304 -- the administrator chose the path.
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("%s exists: an archive never overwrites a file", path)
		}
		return err
	}
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

// LatestArchive answers the newest recorded archive, nil when none.
func LatestArchive(db *gorm.DB) (*model.ForwardLegacyArchive, error) {
	if !db.Migrator().HasTable(&model.ForwardLegacyArchive{}) {
		return nil, nil
	}
	var record model.ForwardLegacyArchive
	err := db.Order("id DESC").First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &record, nil
}

// VerifyArchive reads a recorded archive's file and checks it: readable,
// the recorded SHA-256, and the archive schema. It answers the archive.
func VerifyArchive(record *model.ForwardLegacyArchive) (*Archive, error) {
	file, err := os.Open(record.Path) // #nosec G304 -- the path is the recorded archive's.
	if err != nil {
		return nil, fmt.Errorf("archive %s is not readable: %w", record.Path, err)
	}
	defer file.Close()
	content, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("archive %s is not readable: %w", record.Path, err)
	}
	digest := sha256.Sum256(content)
	if hex.EncodeToString(digest[:]) != record.SHA256 {
		return nil, fmt.Errorf("archive %s changed since it was written (SHA-256 mismatch)", record.Path)
	}
	var archive Archive
	if err := json.Unmarshal(content, &archive); err != nil {
		return nil, fmt.Errorf("archive %s is not valid JSON: %w", record.Path, err)
	}
	if archive.Schema != ArchiveSchema {
		return nil, fmt.Errorf("archive %s has schema %q, want %q", record.Path, archive.Schema, ArchiveSchema)
	}
	return &archive, nil
}

// StartupArchive writes the archive at Control's first v4.2 start: when a
// flux table exists, the tables were not dropped, and no recorded archive
// file is still there. It answers nil when there was nothing to do.
func StartupArchive(ctx context.Context, db *gorm.DB, dir, controlVersion string, now time.Time) (*WriteResult, error) {
	if err := EnsureSchema(db); err != nil {
		return nil, err
	}
	if Dropped(db) || !AnyDropTablePresent(db) {
		return nil, nil
	}
	var records []model.ForwardLegacyArchive
	if err := db.WithContext(ctx).Order("id DESC").Find(&records).Error; err != nil {
		return nil, err
	}
	for _, record := range records {
		if _, err := os.Stat(record.Path); err == nil {
			return nil, nil
		}
	}
	return WriteArchive(ctx, db, WriteOptions{Dir: dir, Trigger: "startup", Actor: "system/startup", ControlVersion: controlVersion, Now: now})
}
