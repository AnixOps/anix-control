package nodesecrets

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Phase P3 (section 4.3): Finalize moves a table from dual_read to
// finalized, writing the tombstones of section 4.4 into its legacy secret
// columns; Unsplit is the way back, writing the secrets back from the new
// tables. Neither runs by itself: an operator runs them, with a
// confirmation, after a staging rehearsal and, on production, with the
// owner's approval (decision D6).

// ErrFinalizeRefused is returned by Finalize when a table may not be
// finalized; nothing is changed.
var ErrFinalizeRefused = errors.New("nodesecrets: finalize refused")

// ErrUnsplitRefused is returned by Unsplit when a table may not be
// unsplit; nothing is changed.
var ErrUnsplitRefused = errors.New("nodesecrets: unsplit refused")

// finalizePasses bounds the tombstone passes of one finalize: the first
// pass writes the tombstones, the next one finds none left. A writer of an
// older binary (or a stale phase cache) that writes a secret back in
// between is caught by another pass.
const finalizePasses = 3

// FinalizeOptions selects a finalize.
type FinalizeOptions struct {
	// Tables are the split tables to finalize; empty is all of them.
	Tables []string
	// Confirm must be set: finalize writes tombstones over the legacy
	// columns, and only unsplit (or a backup) writes them back.
	Confirm bool
	// Actor names who asked, for the audit entries and finalized_by.
	Actor string
	// BatchSize is the number of legacy rows per transaction.
	BatchSize int
	// Now is the time the verification's age is measured at; zero is the
	// current time.
	Now time.Time

	// afterBatch, in tests, runs after each committed batch; an error stops
	// the finalize there, as an interruption would.
	afterBatch func(table string) error
}

// TableFinalize is the outcome of one table's finalize. Counts only.
type TableFinalize struct {
	Table string `json:"table"`
	From  string `json:"from"`
	// Refused says why the table may not be finalized; empty when it may.
	Refused string `json:"refused,omitempty"`
	// Resumed: an interrupted finalize of the table was carried on.
	Resumed bool `json:"resumed,omitempty"`
	// Rows counts the legacy rows of the last pass; Tombstoned the rows
	// whose columns were rewritten, over every pass.
	Rows        int64      `json:"rows"`
	Tombstoned  int64      `json:"tombstoned"`
	Passes      int        `json:"passes"`
	FinalizedAt *time.Time `json:"finalized_at,omitempty"`
	// VerifiedAt and Digest are the verification the finalize relied on.
	VerifiedAt *time.Time `json:"verified_at,omitempty"`
	Digest     string     `json:"digest,omitempty"`
}

// Finalize moves the given tables to phase finalized (section 4.3, P3).
//
//   - It needs Confirm, and each table in phase dual_read with a recent
//     clean verification (the dual_read gate: the latest verify matched,
//     with no mismatch, within VerifyMaxAge). A table already finalized is
//     left as it is. Either every table may be finalized or none is:
//     ErrFinalizeRefused, with the reasons.
//   - The phase moves to finalized first, with finalized_at empty: from
//     then on the readers read the new tables only, and every writer's
//     Sync writes tombstones into the legacy columns it writes.
//   - Then the legacy rows are rewritten in batches by id, one transaction
//     each, which locks its rows, brings the new tables in line with them
//     (as backfill does) and writes the tombstones: unique per row, so no
//     unique index is violated; a value that would collide is refused
//     (ErrTombstoneCollision) and the batch rolled back.
//   - finalized_at is recorded once a pass finds nothing left to rewrite.
//     An interrupted finalize (finalized, finalized_at empty) is resumed by
//     running it again, without a new verification.
//
// Each step is audited (v2_operation_log, module node_secrets). No secret
// is logged or answered.
func Finalize(ctx context.Context, db *gorm.DB, options FinalizeOptions) ([]TableFinalize, error) {
	tables, err := selectTables(options.Tables)
	if err != nil {
		return nil, err
	}
	if err := requireSplitSchema(db); err != nil {
		return nil, err
	}
	now := options.Now
	if now.IsZero() {
		now = time.Now()
	}
	now = now.UTC()
	actor := actorName(options.Actor)
	batchSize := options.BatchSize
	if batchSize <= 0 {
		batchSize = DefaultBatchSize
	}

	results := make([]TableFinalize, 0, len(tables))
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		results = results[:0]
		refused := false
		for _, table := range tables {
			row, err := lockSplitRow(tx, table)
			if err != nil {
				return err
			}
			result := TableFinalize{Table: table, From: row.Phase}
			switch {
			case !options.Confirm:
				result.Refused = "finalize writes tombstones over the legacy columns; confirm it (-confirm)"
			case row.Phase == PhaseFinalized && row.FinalizedAt != nil:
				result.FinalizedAt = row.FinalizedAt
			case row.Phase == PhaseFinalized:
				result.Resumed = true
			case row.Phase != PhaseDualRead:
				result.Refused = fmt.Sprintf("finalize needs phase dual_read; the table is in phase %q", row.Phase)
			default:
				result.Refused = verificationRefusal(row, now)
				if result.Refused == "" {
					result.VerifiedAt, result.Digest = row.VerifiedAt, row.Digest
				}
			}
			refused = refused || result.Refused != ""
			results = append(results, result)
		}
		if refused {
			return ErrFinalizeRefused
		}
		for _, result := range results {
			if result.From != PhaseDualRead {
				continue
			}
			if err := tx.Model(&model.NodeSecretSplit{}).Where("table_name = ?", result.Table).Updates(map[string]any{
				"phase": PhaseFinalized, "finalized_at": nil, "finalized_by": truncate(actor, 128), "updated_at": now,
			}).Error; err != nil {
				return err
			}
			if err := tx.Create(splitAudit(result.Table, "finalize_started", map[string]any{
				"table": result.Table, "from": result.From, "verified_at": result.VerifiedAt, "digest": result.Digest,
			}, actor, now)).Error; err != nil {
				return err
			}
		}
		return nil
	})
	invalidatePhases(db)
	if errors.Is(err, ErrFinalizeRefused) {
		return results, err
	}
	if err != nil {
		return nil, err
	}

	for i := range results {
		result := &results[i]
		if result.FinalizedAt != nil {
			continue
		}
		if err := finalizeTable(ctx, db, specs[result.Table], result, batchSize, actor, options.afterBatch); err != nil {
			return results, fmt.Errorf("finalize %s: %w", result.Table, err)
		}
	}
	return results, nil
}

// finalizeTable rewrites the legacy rows of a table being finalized, pass
// after pass until one finds nothing left, then records finalized_at.
func finalizeTable(ctx context.Context, db *gorm.DB, spec tableSpec, result *TableFinalize, batchSize int, actor string, afterBatch func(string) error) error {
	for result.Passes < finalizePasses {
		rows, tombstoned, err := tombstonePass(ctx, db, spec, batchSize, afterBatch)
		result.Passes++
		result.Rows = rows
		result.Tombstoned += tombstoned
		if err != nil {
			return err
		}
		if tombstoned == 0 {
			return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				row, err := lockSplitRow(tx, spec.table)
				if err != nil {
					return err
				}
				if row.Phase != PhaseFinalized {
					return fmt.Errorf("the table left phase finalized (now %q) while it was being finalized", row.Phase)
				}
				now := time.Now().UTC()
				if err := tx.Model(&model.NodeSecretSplit{}).Where("table_name = ?", spec.table).Updates(map[string]any{
					"finalized_at": now, "updated_at": now,
				}).Error; err != nil {
					return err
				}
				result.FinalizedAt = &now
				return tx.Create(splitAudit(spec.table, "finalized", map[string]any{
					"table": spec.table, "rows": result.Rows, "tombstoned": result.Tombstoned, "passes": result.Passes,
				}, actor, now)).Error
			})
		}
	}
	return fmt.Errorf("legacy rows still held secrets after %d passes: a writer older than this release may be running; "+
		"stop it and run finalize again", finalizePasses)
}

// tombstonePass rewrites every legacy row of spec's table, in batches by
// id. It answers the rows read and the rows rewritten.
func tombstonePass(ctx context.Context, db *gorm.DB, spec tableSpec, batchSize int, afterBatch func(string) error) (int64, int64, error) {
	var rows, tombstoned int64
	cursor := uint64(0)
	for {
		if err := ctx.Err(); err != nil {
			return rows, tombstoned, err
		}
		done := false
		err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			ids, err := nextIDs(tx, spec.table, cursor, batchSize)
			if err != nil || len(ids) == 0 {
				done = err == nil
				return err
			}
			now := time.Now().UTC()
			// The new tables first hold what the rows hold, as a backfill
			// would make them; only then do the rows lose it.
			if _, err := syncRows(tx, spec, ids, SourceBackfill, true, now); err != nil {
				return err
			}
			changed, err := tombstoneRows(tx, spec, ids, true, now)
			if err != nil {
				return err
			}
			rows += int64(len(ids))
			tombstoned += changed
			cursor = ids[len(ids)-1]
			return nil
		})
		if err != nil {
			return rows, tombstoned, err
		}
		if done {
			return rows, tombstoned, nil
		}
		if afterBatch != nil {
			if err := afterBatch(spec.table); err != nil {
				return rows, tombstoned, err
			}
		}
	}
}

func nextIDs(tx *gorm.DB, table string, cursor uint64, limit int) ([]uint64, error) {
	var ids []uint64
	err := tx.Table(table).Where("id > ?", cursor).Order("id").Limit(limit).Pluck("id", &ids).Error
	return ids, err
}

// UnsplitOptions selects an unsplit.
type UnsplitOptions struct {
	// Tables are the split tables to unsplit; empty is all of them.
	Tables []string
	// Confirm must be set.
	Confirm bool
	// AllowAdopted unsplits a table a package's storage lease adopted: the
	// package would read the secrets written back. Set it only once the
	// package no longer holds the table (uninstalled, or a release without
	// the grant was leased since).
	AllowAdopted bool
	// Actor names who asked, for the audit entries.
	Actor string
	// BatchSize is the number of legacy rows per transaction.
	BatchSize int
	// DropViews drops the kernel API views that may exist only while table
	// is finalized (packagestore.DropFinalizedViews), in the transaction
	// that leaves the phase, before any secret is written back.
	DropViews func(tx *gorm.DB, table string) error
	// Now stamps the audit entries; zero is the current time.
	Now time.Time

	afterBatch func(table string) error
}

// TableUnsplit is the outcome of one table's unsplit. Counts only.
type TableUnsplit struct {
	Table   string `json:"table"`
	From    string `json:"from"`
	Refused string `json:"refused,omitempty"`
	// Rows counts the legacy rows read; Restored those written back.
	Rows     int64 `json:"rows"`
	Restored int64 `json:"restored"`
	// Unresolved counts the tombstones and placeholders the new tables
	// held no value for: they stay, and never authenticate.
	Unresolved int64 `json:"unresolved"`
	// Verify is the verification run after the secrets were written back.
	Verify *TableVerify `json:"verify,omitempty"`
}

// Unsplit is the way back from P3 (section 4.3): it writes the secrets of
// the new tables back into the legacy columns and returns the table to
// phase dual_read, where an older binary reads the legacy columns again.
//
//   - It needs Confirm, and each table finalized or in dual_read (an
//     interrupted unsplit is resumed by running it again). A table a
//     package's storage lease adopted is refused unless AllowAdopted.
//     Either every table may be unsplit or none is: ErrUnsplitRefused.
//   - The phase moves to dual_read first, and DropViews removes the views
//     that exist only while the table is finalized, in that transaction;
//     the writers then write secrets, not tombstones, again.
//   - The legacy rows are restored in batches by id: a credential column
//     takes the new table's value and a key hash column its hash; a JSON
//     column takes back its original document, byte for byte, where the
//     column is still the redacted form of it, or else the redacted
//     document with the new table's values at its placeholders.
//   - Each table is verified afterwards; a mismatch is ErrMismatch, with
//     the results, and the table stays in dual_read.
func Unsplit(ctx context.Context, db *gorm.DB, options UnsplitOptions) ([]TableUnsplit, error) {
	tables, err := selectTables(options.Tables)
	if err != nil {
		return nil, err
	}
	if err := requireSplitSchema(db); err != nil {
		return nil, err
	}
	now := options.Now
	if now.IsZero() {
		now = time.Now()
	}
	now = now.UTC()
	actor := actorName(options.Actor)
	batchSize := options.BatchSize
	if batchSize <= 0 {
		batchSize = DefaultBatchSize
	}

	results := make([]TableUnsplit, 0, len(tables))
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		results = results[:0]
		refused := false
		adopters, err := adoptingPackages(tx)
		if err != nil {
			return err
		}
		for _, table := range tables {
			row, err := lockSplitRow(tx, table)
			if err != nil {
				return err
			}
			result := TableUnsplit{Table: table, From: row.Phase}
			switch {
			case !options.Confirm:
				result.Refused = "unsplit writes the secrets back into the legacy columns; confirm it (-confirm)"
			case row.Phase != PhaseFinalized && row.Phase != PhaseDualRead:
				result.Refused = fmt.Sprintf("unsplit needs phase finalized (or dual_read, to resume); the table is in phase %q", row.Phase)
			case len(adopters[table]) > 0 && !options.AllowAdopted:
				result.Refused = fmt.Sprintf("package storage of %s adopts the table and would read the secrets written back; "+
					"remove the grant first, then unsplit with -adopted-ok", strings.Join(adopters[table], ", "))
			}
			refused = refused || result.Refused != ""
			results = append(results, result)
		}
		if refused {
			return ErrUnsplitRefused
		}
		for _, result := range results {
			if options.DropViews != nil {
				if err := options.DropViews(tx, result.Table); err != nil {
					return err
				}
			}
			if result.From != PhaseFinalized {
				continue
			}
			if err := tx.Model(&model.NodeSecretSplit{}).Where("table_name = ?", result.Table).Updates(map[string]any{
				"phase": PhaseDualRead, "finalized_at": nil, "finalized_by": "", "updated_at": now,
			}).Error; err != nil {
				return err
			}
			if err := tx.Create(splitAudit(result.Table, "unsplit", map[string]any{
				"table": result.Table, "from": result.From, "to": PhaseDualRead,
			}, actor, now)).Error; err != nil {
				return err
			}
		}
		return nil
	})
	invalidatePhases(db)
	if errors.Is(err, ErrUnsplitRefused) {
		return results, err
	}
	if err != nil {
		return nil, err
	}

	mismatch := false
	for i := range results {
		result := &results[i]
		if err := restoreTable(ctx, db, specs[result.Table], result, batchSize, options.afterBatch); err != nil {
			return results, fmt.Errorf("unsplit %s: %w", result.Table, err)
		}
		verified, err := Verify(ctx, db, VerifyOptions{Tables: []string{result.Table}})
		if len(verified) == 1 {
			result.Verify = &verified[0]
		}
		if errors.Is(err, ErrMismatch) {
			mismatch = true
			continue
		}
		if err != nil {
			return results, err
		}
	}
	if mismatch {
		return results, ErrMismatch
	}
	return results, nil
}

// restoreTable writes the secrets back into every legacy row of a table.
func restoreTable(ctx context.Context, db *gorm.DB, spec tableSpec, result *TableUnsplit, batchSize int, afterBatch func(string) error) error {
	cursor := uint64(0)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		done := false
		err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			ids, err := nextIDs(tx, spec.table, cursor, batchSize)
			if err != nil || len(ids) == 0 {
				done = err == nil
				return err
			}
			restored, unresolved, err := restoreRows(tx, spec, ids)
			if err != nil {
				return err
			}
			result.Rows += int64(len(ids))
			result.Restored += restored
			result.Unresolved += unresolved
			cursor = ids[len(ids)-1]
			return nil
		})
		if err != nil {
			return err
		}
		if done {
			return nil
		}
		if afterBatch != nil {
			if err := afterBatch(spec.table); err != nil {
				return err
			}
		}
	}
}

// credentialColumns names the credential kind each tombstone column of a
// credential table holds, and the hash column that goes with it.
var credentialColumns = map[string]map[string]struct{ kind, hash string }{
	TableNode:          {"api_key": {KindNodeAPIKey, "api_key_hash"}, "secret": {KindNodeSharedSecret, ""}},
	TableAuthorizedKey: {"key": {KindRegistrationKey, "key_hash"}},
	TableForwardNode:   {"api_token": {KindForwardNodeToken, ""}},
	TableCleanAgent:    {"token": {KindCleanAgentToken, ""}},
}

// restoreRows writes the new tables' secrets back into the given legacy
// rows. It answers the rows changed and the positions left unresolved.
func restoreRows(tx *gorm.DB, spec tableSpec, ids []uint64) (int64, int64, error) {
	rows, err := loadLegacyRows(tx, spec, ids, true)
	if err != nil {
		return 0, 0, err
	}
	credentials := map[credentialKey]model.NodeCredential{}
	if spec.subjectKind != "" {
		var current []model.NodeCredential
		if err := tx.Session(&gorm.Session{NewDB: true}).
			Where("subject_kind = ? AND subject_id IN ? AND kind IN ? AND status <> ?", spec.subjectKind, ids, spec.kinds, StatusRetired).
			Order("version").Find(&current).Error; err != nil {
			return 0, 0, err
		}
		for _, row := range current {
			credentials[credentialKey{SubjectID: row.SubjectID, Kind: row.Kind}] = row
		}
	}
	var secrets map[secretOwner]map[string]string
	if spec.scope != "" {
		if secrets, err = secretRows(tx, spec.scope, ids); err != nil {
			return 0, 0, err
		}
	}
	originals, err := originalValues(tx, spec.table, ids)
	if err != nil {
		return 0, 0, err
	}

	var restored, unresolved int64
	for _, row := range rows {
		updates := map[string]any{}
		for _, column := range legacyColumns[spec.table] {
			value, present := row.values[column.name]
			if !present {
				continue
			}
			switch column.kind {
			case columnTombstone:
				if !IsTombstone(value) {
					continue
				}
				secret, hash, ok := "", "", false
				if credential, isCredential := credentialColumns[spec.table][column.name]; isCredential {
					current, found := credentials[credentialKey{SubjectID: row.id, Kind: credential.kind}]
					if found && current.Value != "" {
						secret, ok = current.Value, true
						if credential.hash != "" && row.values[credential.hash] == "" {
							// The row's own empty hash where it had none.
							hash = current.KeyHash
							if original, marked := originals[secretOwner{owner: row.id, column: credential.hash}]; marked {
								hash = original
							}
							updates[credential.hash] = hash
						}
					}
				} else {
					secret, ok = secrets[secretOwner{owner: row.id, column: column.name}][""]
					ok = ok && secret != ""
				}
				if !ok {
					unresolved++
					continue
				}
				updates[column.name] = secret
			case columnDocument:
				document, missing := restoreDocument(value, secrets[secretOwner{owner: row.id, column: column.name}],
					originals[secretOwner{owner: row.id, column: column.name}])
				unresolved += missing
				if document != value {
					updates[column.name] = document
				}
			}
		}
		if len(updates) == 0 {
			continue
		}
		if err := tx.Session(&gorm.Session{NewDB: true}).Table(spec.table).Where("id = ?", row.id).UpdateColumns(updates).Error; err != nil {
			return restored, unresolved, err
		}
		restored++
	}
	if err := dropOriginals(tx, spec.table, ids, ""); err != nil {
		return restored, unresolved, err
	}
	return restored, unresolved, nil
}

// restoreDocument answers the legacy form of a finalized JSON column: its
// original document where the column is still exactly the redacted form of
// it and the new table holds its secrets unchanged, or else the column
// with the new table's values at its placeholders. It answers the
// placeholders the new table held no value for.
func restoreDocument(document string, values map[string]string, original string) (string, int64) {
	positions := SecretPositions(document)
	moved := false
	for _, position := range positions {
		moved = moved || position.Moved
	}
	if !moved {
		return document, 0
	}
	if original != "" && Redact(original) == document && originalMatches(original, values) {
		return original, 0
	}
	replacements := map[string]string{}
	var missing int64
	for _, position := range positions {
		if !position.Moved {
			continue
		}
		value, found := values[position.Pointer]
		if !found {
			missing++
			continue
		}
		replacements[position.Pointer] = value
	}
	if len(replacements) == 0 {
		return document, missing
	}
	restored, err := replacePositions(document, replacements)
	if err != nil {
		return document, int64(len(positions))
	}
	return restored, missing
}

// originalMatches reports whether every secret of original is the value
// the new table holds at its position.
func originalMatches(original string, values map[string]string) bool {
	positions := SecretPositions(original)
	if len(positions) == 0 {
		return false
	}
	for _, position := range positions {
		value, found := values[position.Pointer]
		if position.Moved || !found || value != position.Value {
			return false
		}
	}
	return true
}

// adoptingPackages answers the packages whose recorded storage lease adopts
// each split table.
func adoptingPackages(tx *gorm.DB) (map[string][]string, error) {
	out := map[string][]string{}
	if !tx.Migrator().HasTable(&model.PackageStorage{}) {
		return out, nil
	}
	var rows []model.PackageStorage
	if err := tx.Session(&gorm.Session{NewDB: true}).Select("package_id", "grants_json").Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		var grants struct {
			AdoptTables []string `json:"adopt_tables"`
		}
		if json.Unmarshal([]byte(row.GrantsJSON), &grants) != nil {
			continue
		}
		for _, table := range grants.AdoptTables {
			out[table] = append(out[table], row.PackageID)
		}
	}
	for table := range out {
		sort.Strings(out[table])
	}
	return out, nil
}

// lockSplitRow loads a table's state row with its row lock, creating it in
// phase dual_write when it is missing.
func lockSplitRow(tx *gorm.DB, table string) (model.NodeSecretSplit, error) {
	if _, err := splitRow(tx, table); err != nil {
		return model.NodeSecretSplit{}, err
	}
	var row model.NodeSecretSplit
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("table_name = ?", table).First(&row).Error
	return row, err
}

func requireSplitSchema(db *gorm.DB) error {
	if !installed(db) || !db.Migrator().HasTable(&model.NodeSecretSplit{}) {
		return errors.New("nodesecrets: the split tables do not exist; start Control or run its migrate command first")
	}
	if !db.Migrator().HasTable(&model.OperationLog{}) {
		return errors.New("nodesecrets: the audit log (v2_operation_log) does not exist; start Control or run its migrate command first")
	}
	return nil
}

func actorName(actor string) string {
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return "cli"
	}
	return actor
}

// splitAudit is an audit entry of the split's state: who, which table,
// what. It holds no secret.
func splitAudit(table, action string, content map[string]any, actor string, now time.Time) *model.OperationLog {
	encoded, err := json.Marshal(content)
	if err != nil {
		encoded = []byte(table + " " + action)
	}
	return &model.OperationLog{
		Username:   truncate(actor, 100),
		Action:     action,
		Module:     "node_secrets",
		TargetType: "node_secret_split",
		Content:    string(encoded),
		UserAgent:  "anix-control node-secrets " + strings.SplitN(action, "_", 2)[0],
		Status:     1,
		CreatedAt:  now,
	}
}

// AdoptionAllowed reports whether a package may adopt table in place
// (kernel.storage.adopt:<table>, section 4.6): only a table whose split
// is finalized, finalized_at recorded, may be. A table outside the split is
// not this rule's: it answers nil. The phase is read uncached.
func AdoptionAllowed(db *gorm.DB, table string) error {
	if _, split := specs[table]; !split {
		return nil
	}
	if !db.Migrator().HasTable(&model.NodeSecretSplit{}) {
		return fmt.Errorf("%s holds node credentials and is not finalized", table)
	}
	var row model.NodeSecretSplit
	if err := newSession(db).Where("table_name = ?", table).Limit(1).Find(&row).Error; err != nil {
		return err
	}
	if row.Phase != PhaseFinalized || row.FinalizedAt == nil {
		phase := row.Phase
		if phase == "" {
			phase = PhaseDualWrite
		}
		if phase == PhaseFinalized {
			phase = "finalizing"
		}
		return fmt.Errorf("%s holds node credentials until it is finalized (phase %s)", table, phase)
	}
	return nil
}

// FinalizedTables answers the split tables whose finalize completed
// (finalized, with finalized_at recorded), read uncached.
func FinalizedTables(db *gorm.DB) ([]string, error) {
	if !db.Migrator().HasTable(&model.NodeSecretSplit{}) {
		return nil, nil
	}
	var tables []string
	err := newSession(db).Model(&model.NodeSecretSplit{}).
		Where("phase = ? AND finalized_at IS NOT NULL", PhaseFinalized).Order("table_name").Pluck("table_name", &tables).Error
	return tables, err
}
