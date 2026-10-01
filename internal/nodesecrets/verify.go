package nodesecrets

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
)

// DefaultSamples is how many mismatches a verification names per table.
const DefaultSamples = 20

// VerifyOptions selects the tables of a verification.
type VerifyOptions struct {
	Tables []string
	// Samples is how many mismatching secrets to name per table.
	Samples int
}

// Mismatch names one secret whose old and new forms differ: where it is,
// never its value or hash.
type Mismatch struct {
	// Problem is missing (in the legacy table only), extra (in the new
	// table only) or different.
	Problem string `json:"problem"`
	// Secret is the subject and kind ("proxy 12 node_api_key"), or the
	// owner, column and JSON pointer ("node_protocol 5 reality_settings
	// /private_key").
	Secret string `json:"secret"`
}

// TableVerify is the outcome of one table's verification.
type TableVerify struct {
	Table string `json:"table"`
	Phase string `json:"phase"`
	// LegacyRows counts the legacy rows read; LegacySecrets the secrets
	// they hold, and NewSecrets the current rows of the new tables.
	LegacyRows    int64 `json:"legacy_rows"`
	LegacySecrets int64 `json:"legacy_secrets"`
	NewSecrets    int64 `json:"new_secrets"`
	// LegacyDigest and NewDigest are the digests of every secret of each
	// form. They are equal when the forms match.
	LegacyDigest string     `json:"legacy_digest"`
	NewDigest    string     `json:"new_digest"`
	Match        bool       `json:"match"`
	Missing      int64      `json:"missing"`
	Extra        int64      `json:"extra"`
	Different    int64      `json:"different"`
	Samples      []Mismatch `json:"samples,omitempty"`
}

// ErrMismatch is returned by Verify when a table's forms differ.
var ErrMismatch = errors.New("nodesecrets: the old and new forms of the node secrets differ")

// Verify compares every secret of the legacy tables with the new tables
// (section 4.3). Each form is reduced to one entry per secret: the subject
// or owner, the kind or position, and a SHA-256 over the hash of the value
// and the credential's metadata (key hash, endpoint, status, expiry and
// revocation). A table's digest is the SHA-256 of its sorted entries. Both
// forms are read in one transaction (a repeatable-read snapshot on
// PostgreSQL), so concurrent writers cannot make it report a difference.
//
// The outcome is recorded in v4_kernel_node_secret_split: a match records
// the digest and verified_at. The phase is left as it is; the readers move
// in a later release (NO-3). Verify answers ErrMismatch, with the results,
// when a table differs.
func Verify(ctx context.Context, db *gorm.DB, options VerifyOptions) ([]TableVerify, error) {
	tables, err := selectTables(options.Tables)
	if err != nil {
		return nil, err
	}
	if !installed(db) {
		return nil, errors.New("nodesecrets: the split tables do not exist; start Control or run its migrate command first")
	}
	samples := options.Samples
	if samples <= 0 {
		samples = DefaultSamples
	}
	results := make([]TableVerify, 0, len(tables))
	mismatch := false
	for _, table := range tables {
		result, err := verifyTable(ctx, db, specs[table], samples)
		if err != nil {
			return results, fmt.Errorf("verify %s: %w", table, err)
		}
		results = append(results, result)
		mismatch = mismatch || !result.Match
	}
	if mismatch {
		return results, ErrMismatch
	}
	return results, nil
}

func verifyTable(ctx context.Context, db *gorm.DB, spec tableSpec, samples int) (TableVerify, error) {
	result := TableVerify{Table: spec.table}
	split, err := splitRow(db.WithContext(ctx), spec.table)
	if err != nil {
		return result, err
	}
	result.Phase = split.Phase

	var legacy, current map[string]string
	err = readSnapshot(db.WithContext(ctx), func(tx *gorm.DB) error {
		var err error
		legacy, result.LegacyRows, err = legacyEntries(tx, spec)
		if err != nil {
			return err
		}
		current, err = newEntries(tx, spec)
		return err
	})
	if err != nil {
		return result, err
	}

	result.LegacySecrets = int64(len(legacy))
	result.NewSecrets = int64(len(current))
	result.LegacyDigest = digest(legacy)
	result.NewDigest = digest(current)
	keys := make([]string, 0, len(legacy)+len(current))
	for key := range legacy {
		keys = append(keys, key)
	}
	for key := range current {
		if _, ok := legacy[key]; !ok {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		old, inLegacy := legacy[key]
		now, inNew := current[key]
		problem := ""
		switch {
		case !inNew:
			problem = "missing"
			result.Missing++
		case !inLegacy:
			problem = "extra"
			result.Extra++
		case old != now:
			problem = "different"
			result.Different++
		default:
			continue
		}
		if len(result.Samples) < samples {
			result.Samples = append(result.Samples, Mismatch{Problem: problem, Secret: key})
		}
	}
	result.Match = result.Missing == 0 && result.Extra == 0 && result.Different == 0 && result.LegacyDigest == result.NewDigest

	now := time.Now().UTC()
	updates := map[string]any{"checked_at": now, "mismatches": result.Missing + result.Extra + result.Different, "updated_at": now}
	if result.Match {
		updates["digest"] = result.LegacyDigest
		updates["verified_at"] = now
	}
	if err := db.WithContext(ctx).Model(&model.NodeSecretSplit{}).Where("table_name = ?", spec.table).Updates(updates).Error; err != nil {
		return result, err
	}
	return result, nil
}

// readSnapshot runs fn in a read-only transaction: a repeatable-read
// snapshot on PostgreSQL; SQLite reads one snapshot per transaction anyway.
func readSnapshot(db *gorm.DB, fn func(tx *gorm.DB) error) error {
	if db.Name() == "postgres" {
		return db.Transaction(fn, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	}
	return db.Transaction(fn)
}

// legacyEntries reads every legacy row of spec's table, in batches by id,
// and answers its secrets as entries.
func legacyEntries(tx *gorm.DB, spec tableSpec) (map[string]string, int64, error) {
	entries := make(map[string]string)
	var rows int64
	cursor := uint64(0)
	for {
		var ids []uint64
		if err := tx.Table(spec.table).Where("id > ?", cursor).Order("id").Limit(DefaultBatchSize).Pluck("id", &ids).Error; err != nil {
			return nil, 0, err
		}
		if len(ids) == 0 {
			return entries, rows, nil
		}
		derived, err := spec.derive(tx, ids, false)
		if err != nil {
			return nil, 0, err
		}
		for _, entry := range derived.credentials {
			entries[credentialEntryKey(spec.subjectKind, entry.SubjectID, entry.Kind)] = credentialFingerprint(
				entry.KeyHash, entry.Value, entry.Endpoint, entry.Status, entry.ExpiresAt, entry.RevokedAt)
		}
		for _, entry := range derived.secrets {
			entries[secretEntryKey(spec.scope, entry.OwnerID, entry.Column, entry.Pointer)] = secretFingerprint(entry.Value)
		}
		rows += int64(len(ids))
		cursor = ids[len(ids)-1]
	}
}

// newEntries reads the current rows the new tables hold for spec's table.
// Retired versions are history, not secrets: they hold no value.
func newEntries(tx *gorm.DB, spec tableSpec) (map[string]string, error) {
	entries := make(map[string]string)
	if spec.subjectKind != "" {
		var batch []model.NodeCredential
		err := tx.Where("subject_kind = ? AND kind IN ? AND status <> ?", spec.subjectKind, spec.kinds, StatusRetired).
			FindInBatches(&batch, DefaultBatchSize, func(*gorm.DB, int) error {
				for _, row := range batch {
					key := credentialEntryKey(row.SubjectKind, row.SubjectID, row.Kind)
					if _, duplicate := entries[key]; duplicate {
						// Two current versions of one credential: never equal
						// to the single legacy value.
						entries[key] = "duplicate"
						continue
					}
					entries[key] = credentialFingerprint(row.KeyHash, row.Value, row.Endpoint, row.Status, row.ExpiresAt, row.RevokedAt)
				}
				return nil
			}).Error
		if err != nil {
			return nil, err
		}
	}
	if spec.scope != "" {
		var batch []model.ProtocolSecret
		err := tx.Where("scope = ?", spec.scope).FindInBatches(&batch, DefaultBatchSize, func(*gorm.DB, int) error {
			for _, row := range batch {
				entries[secretEntryKey(row.Scope, row.OwnerID, row.ColumnName, row.JSONPointer)] = secretFingerprint(row.Value)
			}
			return nil
		}).Error
		if err != nil {
			return nil, err
		}
	}
	return entries, nil
}

func credentialEntryKey(subjectKind string, subjectID uint64, kind string) string {
	return subjectKind + " " + strconv.FormatUint(subjectID, 10) + " " + kind
}

func secretEntryKey(scope string, ownerID uint64, column, pointer string) string {
	return scope + " " + strconv.FormatUint(ownerID, 10) + " " + column + " " + pointer
}

func credentialFingerprint(keyHash, value, endpoint, status string, expiresAt, revokedAt *time.Time) string {
	return fingerprint(keyHash, hashSecret(value), endpoint, status, unixOrEmpty(expiresAt), unixOrEmpty(revokedAt))
}

func secretFingerprint(value string) string {
	return fingerprint(hashSecret(value))
}

func fingerprint(fields ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(fields, "\x00")))
	return hex.EncodeToString(sum[:])
}

func unixOrEmpty(value *time.Time) string {
	if value == nil {
		return ""
	}
	return strconv.FormatInt(value.Unix(), 10)
}

// digest is the SHA-256 of the sorted entries.
func digest(entries map[string]string) string {
	keys := make([]string, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	hash := sha256.New()
	for _, key := range keys {
		hash.Write([]byte(key))
		hash.Write([]byte{0})
		hash.Write([]byte(entries[key]))
		hash.Write([]byte{'\n'})
	}
	return hex.EncodeToString(hash.Sum(nil))
}
