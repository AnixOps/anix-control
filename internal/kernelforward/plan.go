package kernelforward

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/planner"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// PlanOutcome is what one plan did.
type PlanOutcome struct {
	// Refused is true when a violation refused the plan: nothing changed.
	Refused    bool
	Violations []planner.RouteViolation
	Warnings   []string
	// Changed are the nodes whose desired state moved to a new generation.
	Changed []string
}

// storedRoute is a route row with its decoded route.
type storedRoute struct {
	row   model.KernelForwardRoute
	route *forwardv1.Route
}

func loadRoutes(tx *gorm.DB) ([]storedRoute, error) {
	var rows []model.KernelForwardRoute
	if err := tx.Order("id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("kernel forward: load routes: %w", err)
	}
	out := make([]storedRoute, 0, len(rows))
	for _, row := range rows {
		route, err := decodeRoute(row)
		if err != nil {
			return nil, err
		}
		out = append(out, storedRoute{row: row, route: route})
	}
	return out, nil
}

func decodeRoute(row model.KernelForwardRoute) (*forwardv1.Route, error) {
	route := &forwardv1.Route{}
	if err := jsonRead.Unmarshal([]byte(row.RouteJSON), route); err != nil {
		return nil, fmt.Errorf("kernel forward: route %s: %w", row.ID, err)
	}
	route.Id, route.Revision = row.ID, row.Revision
	return route, nil
}

// Replan plans every stored route against the current inventory, after an
// inventory change or on demand. A refused plan changes nothing; it is
// recorded in the status row and logged.
func (s *Service) Replan(ctx context.Context, reason string) (PlanOutcome, error) {
	db, err := s.db(ctx)
	if err != nil {
		return PlanOutcome{}, err
	}
	var outcome PlanOutcome
	err = db.Transaction(func(tx *gorm.DB) error {
		now := s.now()
		if _, err := lockPlan(tx, now); err != nil {
			return err
		}
		routes, err := loadRoutes(tx)
		if err != nil {
			return err
		}
		outcome, err = s.replanTx(tx, routes, now)
		if err != nil {
			return err
		}
		return recordPlan(tx, outcome, reason, now)
	})
	if err != nil {
		return PlanOutcome{}, err
	}
	if outcome.Refused {
		logRefusedPlan(reason, outcome.Violations)
	}
	notify(outcome.Changed)
	return outcome, nil
}

// recordPlan writes the plan's outcome to the status row: an accepted plan
// counts, a refused one keeps its violations.
func recordPlan(tx *gorm.DB, outcome PlanOutcome, reason string, now time.Time) error {
	violations, _ := json.Marshal(violationsJSON(outcome.Violations))
	warnings, _ := json.Marshal(outcome.Warnings)
	if len(reason) > 64 {
		reason = reason[:64]
	}
	updates := map[string]any{
		"refused": outcome.Refused, "reason": reason, "violations_json": string(violations),
		"warnings_json": string(warnings), "planned_at": now, "updated_at": now,
	}
	if !outcome.Refused {
		updates["revision"] = gorm.Expr("revision + 1")
	}
	return tx.Model(&model.KernelForwardPlan{}).Where("id = ?", planRowID).Updates(updates).Error
}

type violationJSON struct {
	RouteID string `json:"route_id"`
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func violationsJSON(violations []planner.RouteViolation) []violationJSON {
	out := make([]violationJSON, 0, len(violations))
	for _, v := range violations {
		out = append(out, violationJSON{RouteID: v.RouteID, Field: v.Field, Code: string(v.Code), Message: v.Message})
	}
	return out
}

// PlanStatus is the outcome of the last plan.
type PlanStatus struct {
	// Revision counts accepted plans.
	Revision   uint64
	Refused    bool
	Reason     string
	Violations []*forwardv1.Violation
	Warnings   []string
	PlannedAt  *time.Time
}

// PlanStatus answers the outcome of the last plan; the zero status before
// the first.
func (s *Service) PlanStatus(ctx context.Context) (PlanStatus, error) {
	db, err := s.db(ctx)
	if err != nil {
		return PlanStatus{}, err
	}
	var rows []model.KernelForwardPlan
	if err := db.Where("id = ?", planRowID).Limit(1).Find(&rows).Error; err != nil || len(rows) == 0 {
		return PlanStatus{}, err
	}
	row := rows[0]
	status := PlanStatus{Revision: row.Revision, Refused: row.Refused, Reason: row.Reason, PlannedAt: row.PlannedAt}
	var violations []violationJSON
	_ = json.Unmarshal([]byte(row.ViolationsJSON), &violations)
	for _, v := range violations {
		status.Violations = append(status.Violations, &forwardv1.Violation{RouteId: v.RouteID, Field: v.Field, Code: v.Code, Message: v.Message})
	}
	_ = json.Unmarshal([]byte(row.WarningsJSON), &status.Warnings)
	return status, nil
}

// replanTx plans routes in tx, which holds the plan lock. A refused plan
// writes nothing. An accepted one writes the allocations, the generations
// and states that moved, and the routes' enforcement.
func (s *Service) replanTx(tx *gorm.DB, routes []storedRoute, now time.Time) (PlanOutcome, error) {
	inv, err := loadInventory(tx)
	if err != nil {
		return PlanOutcome{}, err
	}
	previous, taken, err := loadAllocations(tx, now)
	if err != nil {
		return PlanOutcome{}, err
	}
	used, err := entryUsage(tx)
	if err != nil {
		return PlanOutcome{}, err
	}
	effective := make([]*forwardv1.Route, 0, len(routes))
	enforced := make(map[string]string, len(routes))
	for _, stored := range routes {
		reason := enforcement(stored.route, used[stored.route.GetId()], now)
		enforced[stored.route.GetId()] = reason
		route := stored.route
		if reason != "" && !route.GetPaused() {
			route = proto.Clone(route).(*forwardv1.Route)
			route.Paused = true
		}
		effective = append(effective, route)
	}
	result, err := planner.Plan(effective, inv.nodes, previous, planner.Options{
		Cluster: s.cluster(), ReservedPorts: inv.reserved, Taken: taken, Now: now,
	})
	if err != nil {
		return PlanOutcome{}, fmt.Errorf("kernel forward: plan: %w", err)
	}
	outcome := PlanOutcome{Warnings: result.Warnings}
	if len(result.Violations) > 0 {
		outcome.Refused, outcome.Violations = true, result.Violations
		return outcome, nil
	}
	changed, err := writeStates(tx, result.States, now)
	if err != nil {
		return PlanOutcome{}, err
	}
	outcome.Changed = changed
	if err := writeAllocations(tx, result.Allocations, now); err != nil {
		return PlanOutcome{}, err
	}
	for _, stored := range routes {
		if reason := enforced[stored.route.GetId()]; reason != stored.row.Enforced {
			if err := tx.Model(&model.KernelForwardRoute{}).Where("id = ?", stored.row.ID).Update("enforced", reason).Error; err != nil {
				return PlanOutcome{}, fmt.Errorf("kernel forward: route %s enforcement: %w", stored.row.ID, err)
			}
		}
	}
	return outcome, nil
}

// enforcement is the kernel's pause of a route: expired once its expiry has
// passed, quota once its entry hop's metered bytes reach its quota. Control
// is authoritative for both (forward-sdk.md section 5.3); the nodes enforce
// them locally too.
func enforcement(route *forwardv1.Route, used uint64, now time.Time) string {
	limits := route.GetLimits()
	if expires := limits.GetExpiresAtUnixMs(); expires > 0 && now.UnixMilli() >= expires {
		return EnforcedExpired
	}
	if quota := limits.GetQuotaBytes(); quota > 0 && used >= quota {
		return EnforcedQuota
	}
	return ""
}

// entryUsage answers each route's metered bytes on its entry hop, both
// directions, summed over nodes and counter epochs.
func entryUsage(tx *gorm.DB) (map[string]uint64, error) {
	var rows []struct {
		RouteID string
		Used    int64
	}
	if err := tx.Model(&model.KernelForwardCounter{}).Select("route_id, SUM(up_bytes + down_bytes) AS used").
		Where("hop_index = ?", 0).Group("route_id").Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("kernel forward: entry usage: %w", err)
	}
	out := make(map[string]uint64, len(rows))
	for _, row := range rows {
		if row.Used > 0 {
			out[row.RouteID] = uint64(row.Used)
		}
	}
	return out, nil
}

// loadAllocations answers the active allocations (the planner's previous
// ones) and those of deleted routes still in their grace period (taken).
// Allocations whose grace period ended are deleted.
func loadAllocations(tx *gorm.DB, now time.Time) (previous, taken planner.Allocations, err error) {
	if err := tx.Where("released_at IS NOT NULL AND released_at < ?", now.Add(-ReleaseGrace)).Delete(&model.KernelForwardAllocation{}).Error; err != nil {
		return nil, nil, fmt.Errorf("kernel forward: release allocations: %w", err)
	}
	var rows []model.KernelForwardAllocation
	if err := tx.Order("route_id, hop_index, node_ref").Find(&rows).Error; err != nil {
		return nil, nil, fmt.Errorf("kernel forward: load allocations: %w", err)
	}
	previous, taken = planner.Allocations{}, planner.Allocations{}
	for _, row := range rows {
		key := planner.Key{RouteID: row.RouteID, HopIndex: row.HopIndex, NodeRef: row.NodeRef}
		slot := planner.Slot{Port: row.Port, Mark: row.Mark}
		if row.ReleasedAt != nil {
			taken[key] = slot
		} else {
			previous[key] = slot
		}
	}
	return previous, taken, nil
}

// writeAllocations makes the active allocations exactly allocations.
func writeAllocations(tx *gorm.DB, allocations planner.Allocations, now time.Time) error {
	var rows []model.KernelForwardAllocation
	if err := tx.Where("released_at IS NULL").Find(&rows).Error; err != nil {
		return fmt.Errorf("kernel forward: load allocations: %w", err)
	}
	for _, row := range rows {
		key := planner.Key{RouteID: row.RouteID, HopIndex: row.HopIndex, NodeRef: row.NodeRef}
		slot, kept := allocations[key]
		if kept && slot.Port == row.Port && slot.Mark == row.Mark {
			delete(allocations, key)
			continue
		}
		if !kept {
			if err := tx.Where("route_id = ? AND hop_index = ? AND node_ref = ?", row.RouteID, row.HopIndex, row.NodeRef).
				Delete(&model.KernelForwardAllocation{}).Error; err != nil {
				return fmt.Errorf("kernel forward: drop allocation: %w", err)
			}
		}
	}
	for _, allocation := range allocations.Sorted() {
		row := model.KernelForwardAllocation{
			RouteID: allocation.RouteID, HopIndex: allocation.HopIndex, NodeRef: allocation.NodeRef,
			Port: allocation.Port, Mark: allocation.Mark, UpdatedAt: now,
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "route_id"}, {Name: "hop_index"}, {Name: "node_ref"}},
			DoUpdates: clause.Assignments(map[string]any{"port": row.Port, "mark": row.Mark, "released_at": nil, "updated_at": now}),
		}).Create(&row).Error; err != nil {
			return fmt.Errorf("kernel forward: store allocation: %w", err)
		}
	}
	return nil
}

// writeStates stamps states from the stored generations (or the reported
// ones where a node's report is ahead) and stores the ones that moved. A node with a stored state that the plan does not cover
// (it left the inventory) gets an empty state, so it never keeps a deleted
// route's hops. It answers the nodes whose generation moved.
func writeStates(tx *gorm.DB, states map[string]*forwardv1.NodeForwardState, now time.Time) ([]string, error) {
	var rows []model.KernelForwardNodeState
	if err := tx.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("kernel forward: load node states: %w", err)
	}
	reported, err := loadReported(tx)
	if err != nil {
		return nil, err
	}
	stored := make(map[string]planner.Generation, len(rows))
	for _, row := range rows {
		stored[row.NodeRef] = planner.Generation{Generation: row.Generation, StateHash: row.StateHash}
		if _, ok := states[row.NodeRef]; !ok {
			states[row.NodeRef] = &forwardv1.NodeForwardState{NodeRef: row.NodeRef}
		}
	}
	// A node whose report is ahead of its stored generation (Control's
	// database was reset or restored) is stamped from the reported one, so
	// its Agent never ignores the state (generation.go).
	previous := make(map[string]planner.Generation, len(stored))
	for ref, g := range stored {
		previous[ref] = g
	}
	var recoveredRefs []string
	for ref := range states {
		if report, ok := reported[ref]; ok && reportAhead(stored[ref], report) {
			previous[ref] = report
			recoveredRefs = append(recoveredRefs, ref)
		}
	}
	next := planner.Stamp(states, previous)
	sort.Strings(recoveredRefs)
	for _, ref := range recoveredRefs {
		countRecovery(RecoveryPlan)
		logRecovery(RecoveryPlan, ref, stored[ref].Generation, reported[ref].Generation, next[ref].Generation)
	}
	refs := make([]string, 0, len(states))
	for ref := range states {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	var changed []string
	for _, ref := range refs {
		old, had := stored[ref]
		if had && old == next[ref] {
			continue
		}
		encoded, err := jsonWrite.Marshal(states[ref])
		if err != nil {
			return nil, fmt.Errorf("kernel forward: encode state of %s: %w", ref, err)
		}
		row := model.KernelForwardNodeState{
			NodeRef: ref, Generation: next[ref].Generation, StateHash: next[ref].StateHash, StateJSON: string(encoded), PlannedAt: now,
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "node_ref"}},
			DoUpdates: clause.Assignments(map[string]any{
				"generation": row.Generation, "state_hash": row.StateHash, "state_json": row.StateJSON, "planned_at": now,
			}),
		}).Create(&row).Error; err != nil {
			return nil, fmt.Errorf("kernel forward: store state of %s: %w", ref, err)
		}
		changed = append(changed, ref)
	}
	return changed, nil
}

// State answers a node's desired state as the last accepted plan stamped
// it; found is false before the node's first plan.
func (s *Service) State(ctx context.Context, nodeRef string) (*forwardv1.NodeForwardState, bool, error) {
	db, err := s.db(ctx)
	if err != nil {
		return nil, false, err
	}
	return loadState(db, nodeRef)
}

func loadState(db *gorm.DB, nodeRef string) (*forwardv1.NodeForwardState, bool, error) {
	var rows []model.KernelForwardNodeState
	if err := db.Where("node_ref = ?", nodeRef).Limit(1).Find(&rows).Error; err != nil {
		return nil, false, err
	}
	if len(rows) == 0 {
		return nil, false, nil
	}
	state := &forwardv1.NodeForwardState{}
	if err := jsonRead.Unmarshal([]byte(rows[0].StateJSON), state); err != nil {
		return nil, false, fmt.Errorf("kernel forward: state of %s: %w", nodeRef, err)
	}
	return state, true, nil
}
