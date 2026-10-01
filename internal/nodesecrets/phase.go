package nodesecrets

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// VerifyMaxAge is how recent a table's last verification must be for its
// readers to move to dual_read: the verification that matched must be the
// latest one, with no mismatch, and have run within this hour. Dual-write
// keeps both forms equal after it, so the gate only has to rule out a
// verification that is stale or was followed by a failing one.
const VerifyMaxAge = time.Hour

// ErrPhaseRefused is returned by SetPhase when a table may not move to the
// requested phase. Nothing is changed.
var ErrPhaseRefused = errors.New("nodesecrets: phase change refused")

// PhaseChange is the outcome of SetPhase for one table.
type PhaseChange struct {
	Table   string `json:"table"`
	From    string `json:"from"`
	To      string `json:"to"`
	Changed bool   `json:"changed"`
	// Refused says why the table may not move; empty when it may.
	Refused string `json:"refused,omitempty"`
	// VerifiedAt and Digest are the verification the move to dual_read
	// relied on.
	VerifiedAt *time.Time `json:"verified_at,omitempty"`
	Digest     string     `json:"digest,omitempty"`
}

// PhaseOptions selects a phase change.
type PhaseOptions struct {
	// Tables are the split tables to move; empty is all of them.
	Tables []string
	// Phase is dual_read (the readers read the new tables) or dual_write
	// (the way back: the readers read the legacy columns again).
	Phase string
	// Actor names who asked, for the audit entry.
	Actor string
	// Now is the time the verification's age is measured at; zero is the
	// current time.
	Now time.Time
}

// SetPhase moves the readers of the given tables to dual_read, or back to
// dual_write (section 4.3, P2). Writers dual-write in both phases, so the
// legacy columns keep every value and an older binary keeps working.
//
//   - dual_read requires each table's latest verification to have matched,
//     with no mismatch, within VerifyMaxAge (`node-secrets verify`).
//   - dual_write, the rollback, is always allowed from dual_read.
//   - A finalized table, and any other phase, is refused: finalize and
//     unsplit come with P3.
//
// Either every table moves or none does: one refusal refuses the whole
// change, with ErrPhaseRefused and the reasons. Each table that changes
// gets an audit entry (v2_operation_log, module node_secrets) in the same
// transaction. The phases this process reads are refreshed at once; other
// Control processes follow within PhaseCacheTTL.
func SetPhase(ctx context.Context, db *gorm.DB, options PhaseOptions) ([]PhaseChange, error) {
	if options.Phase != PhaseDualRead && options.Phase != PhaseDualWrite {
		return nil, fmt.Errorf("nodesecrets: phase %q is not dual_read or dual_write", options.Phase)
	}
	tables, err := selectTables(options.Tables)
	if err != nil {
		return nil, err
	}
	if !db.Migrator().HasTable(&model.NodeSecretSplit{}) {
		return nil, errors.New("nodesecrets: the split tables do not exist; start Control or run its migrate command first")
	}
	if !db.Migrator().HasTable(&model.OperationLog{}) {
		return nil, errors.New("nodesecrets: the audit log (v2_operation_log) does not exist; start Control or run its migrate command first")
	}
	now := options.Now
	if now.IsZero() {
		now = time.Now()
	}
	now = now.UTC()
	actor := strings.TrimSpace(options.Actor)
	if actor == "" {
		actor = "cli"
	}

	var changes []PhaseChange
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		changes = changes[:0]
		refused := false
		rows := make([]model.NodeSecretSplit, 0, len(tables))
		for _, table := range tables {
			if _, err := splitRow(tx, table); err != nil {
				return err
			}
			var row model.NodeSecretSplit
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("table_name = ?", table).First(&row).Error; err != nil {
				return err
			}
			change := PhaseChange{Table: table, From: row.Phase, To: options.Phase}
			change.Refused = phaseRefusal(row, options.Phase, now)
			if change.Refused == "" && options.Phase == PhaseDualRead {
				change.VerifiedAt, change.Digest = row.VerifiedAt, row.Digest
			}
			refused = refused || change.Refused != ""
			changes = append(changes, change)
			rows = append(rows, row)
		}
		if refused {
			return ErrPhaseRefused
		}
		for i := range changes {
			change := &changes[i]
			if change.From == change.To {
				continue
			}
			if err := tx.Model(&model.NodeSecretSplit{}).Where("table_name = ?", change.Table).
				Updates(map[string]any{"phase": change.To, "updated_at": now}).Error; err != nil {
				return err
			}
			if err := tx.Create(phaseAudit(*change, rows[i], actor, now)).Error; err != nil {
				return err
			}
			change.Changed = true
		}
		return nil
	})
	invalidatePhases(db)
	if errors.Is(err, ErrPhaseRefused) {
		return changes, err
	}
	if err != nil {
		return nil, err
	}
	return changes, nil
}

// phaseRefusal says why row's table may not move to phase at now; empty
// when it may.
func phaseRefusal(row model.NodeSecretSplit, phase string, now time.Time) string {
	if row.Phase == phase {
		return ""
	}
	switch row.Phase {
	case PhaseDualWrite, PhaseDualRead:
	default:
		return fmt.Sprintf("the table is in phase %q, which this command does not leave", row.Phase)
	}
	if phase == PhaseDualWrite {
		return ""
	}
	switch {
	case row.CheckedAt == nil || row.VerifiedAt == nil:
		return "no verification has matched; run node-secrets verify"
	case row.Mismatches != 0 || row.CheckedAt.After(*row.VerifiedAt) || row.Digest == "":
		return "the latest verification found mismatches; run node-secrets backfill, then verify"
	case now.Sub(*row.VerifiedAt) > VerifyMaxAge:
		return fmt.Sprintf("the latest verification is older than %s; run node-secrets verify again", VerifyMaxAge)
	case row.VerifiedAt.Sub(now) > time.Minute:
		return "the latest verification is dated in the future; check the clocks and run node-secrets verify again"
	}
	return ""
}

// phaseAudit is the audit entry of one table's phase change: who, which
// table, from and to, and the verification it relied on. It holds no
// secret.
func phaseAudit(change PhaseChange, row model.NodeSecretSplit, actor string, now time.Time) *model.OperationLog {
	content := map[string]any{
		"table": change.Table, "from": change.From, "to": change.To,
	}
	if change.To == PhaseDualRead {
		content["verified_at"] = row.VerifiedAt
		content["digest"] = row.Digest
	}
	encoded, err := json.Marshal(content)
	if err != nil {
		encoded = []byte(change.Table + " " + change.From + " -> " + change.To)
	}
	return &model.OperationLog{
		Username:   truncate(actor, 100),
		Action:     "phase_" + change.To,
		Module:     "node_secrets",
		TargetType: "node_secret_split",
		Content:    string(encoded),
		UserAgent:  "anix-control node-secrets phase",
		Status:     1,
		CreatedAt:  now,
	}
}

// truncate keeps at most limit bytes of value, on a character boundary.
func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	cut := 0
	for index := range value {
		if index > limit {
			break
		}
		cut = index
	}
	return value[:cut]
}
