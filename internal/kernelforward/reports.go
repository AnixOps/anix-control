package kernelforward

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Reports and the traffic ledger (forward-sdk.md sections 8.2 and 11). A
// node's NodeForwardReport arrives as a PackageReport: latest value wins,
// no acknowledgement, no spool. The kernel keeps the latest report per node
// and meters its counters:
//
//   - Counters are cumulative within a counter_epoch and only grow; a new
//     epoch starts from zero (hop objects re-created, Agent or node
//     restarted).
//   - The ledger keeps, per route, hop, node and epoch, the largest values
//     reported (KernelForwardCounter), and adds each report's growth over
//     them to the hour's bucket (KernelForwardTraffic). A field that went
//     down within an epoch adds nothing and keeps the stored value; a new
//     epoch adds its full values. So a lost, repeated or reordered report
//     never counts a byte twice, and a reset never subtracts.
//   - A route's metered traffic is the sum of its epochs' rows. No
//     multiplier is applied; billing does that later.
//   - A report observed before the stored one is dropped whole, counters
//     too: the stored one carried values at least as large.

// ErrInvalidReport: a report the kernel cannot store (a counter beyond the
// signed 64-bit range of its columns).
var ErrInvalidReport = errors.New("invalid forward report")

// ReportResult is what RecordReport did.
type ReportResult struct {
	// Stored is false when a report observed later was stored before.
	Stored bool
	// Metered counts the counters that grew the ledger.
	Metered int
	// Replanned is true when the report's traffic used up a route's quota
	// and a plan paused it.
	Replanned bool
}

// counterKey names one epoch row.
type counterKey struct {
	route string
	hop   uint32
	epoch string
}

// RecordReport stores node's report (already checked with
// wire.DecodeReport for the node) as its latest and meters its counters.
// observedAt is the report's observation time (the receive time when the
// Agent did not say).
func (s *Service) RecordReport(ctx context.Context, node agentcontrol.AgentNode, report *forwardv1.NodeForwardReport, observedAt, receivedAt time.Time) (ReportResult, error) {
	db, err := s.db(ctx)
	if err != nil {
		return ReportResult{}, err
	}
	for _, counter := range report.GetCounters() {
		for _, value := range []uint64{counter.GetUpBytes(), counter.GetDownBytes(), counter.GetUpPackets(), counter.GetDownPackets(), counter.GetTotalConns()} {
			if value > math.MaxInt64 {
				return ReportResult{}, fmt.Errorf("%w: a counter exceeds %d", ErrInvalidReport, int64(math.MaxInt64))
			}
		}
	}
	ref := node.String()
	observedAt, receivedAt = observedAt.UTC(), receivedAt.UTC()
	summary := proto.Clone(report).(*forwardv1.NodeForwardReport)
	summary.Counters = nil
	summary.ObservedAtUnixMs = observedAt.UnixMilli()
	encoded, err := jsonWrite.Marshal(summary)
	if err != nil {
		return ReportResult{}, fmt.Errorf("kernel forward: encode report: %w", err)
	}
	var result ReportResult
	var quotaRoutes []string
	err = db.Transaction(func(tx *gorm.DB) error {
		var rows []model.KernelForwardNodeReport
		if err := tx.Where("node_ref = ?", ref).Limit(1).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) > 0 && rows[0].ObservedAt.After(observedAt) {
			return nil
		}
		row := model.KernelForwardNodeReport{
			NodeRef: ref, Generation: report.GetGeneration(), StateHash: report.GetStateHash(), Applied: report.GetApplied(),
			HopErrors: len(report.GetErrors()), ReportJSON: string(encoded), ObservedAt: observedAt, ReceivedAt: receivedAt,
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "node_ref"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"generation", "state_hash", "applied", "hop_errors", "report_json", "observed_at", "received_at",
			}),
		}).Create(&row).Error; err != nil {
			return fmt.Errorf("kernel forward: store report: %w", err)
		}
		result.Stored = true
		entryGrew := map[string]bool{}
		for _, counter := range report.GetCounters() {
			grew, err := meter(tx, ref, counter, observedAt)
			if err != nil {
				return err
			}
			if grew {
				result.Metered++
				if counter.GetHopIndex() == 0 {
					entryGrew[counter.GetRouteId()] = true
				}
			}
		}
		var err error
		quotaRoutes, err = quotaCandidates(tx, entryGrew)
		return err
	})
	if err != nil {
		return ReportResult{}, err
	}
	if len(quotaRoutes) > 0 {
		outcome, err := s.Replan(ctx, "quota")
		if err != nil {
			return result, err
		}
		result.Replanned = !outcome.Refused
	}
	return result, nil
}

// meter adds one counter's growth to the ledger and answers whether it
// grew.
func meter(tx *gorm.DB, nodeRef string, counter *forwardv1.Counters, observedAt time.Time) (bool, error) {
	key := counterKey{route: counter.GetRouteId(), hop: counter.GetHopIndex(), epoch: counter.GetCounterEpoch()}
	var rows []model.KernelForwardCounter
	if err := tx.Where("route_id = ? AND hop_index = ? AND node_ref = ? AND counter_epoch = ?", key.route, key.hop, nodeRef, key.epoch).
		Limit(1).Find(&rows).Error; err != nil {
		return false, fmt.Errorf("kernel forward: load counter: %w", err)
	}
	reported := [5]uint64{counter.GetUpBytes(), counter.GetDownBytes(), counter.GetUpPackets(), counter.GetDownPackets(), counter.GetTotalConns()}
	var delta [5]uint64
	if len(rows) == 0 {
		delta = reported
		if err := tx.Create(&model.KernelForwardCounter{
			RouteID: key.route, HopIndex: key.hop, NodeRef: nodeRef, CounterEpoch: key.epoch,
			UpBytes: reported[0], DownBytes: reported[1], UpPackets: reported[2], DownPackets: reported[3], TotalConns: reported[4],
			ActiveConns: counter.GetActiveConns(), ObservedAt: observedAt, UpdatedAt: observedAt,
		}).Error; err != nil {
			return false, fmt.Errorf("kernel forward: store counter: %w", err)
		}
	} else {
		row := rows[0]
		stored := [5]uint64{row.UpBytes, row.DownBytes, row.UpPackets, row.DownPackets, row.TotalConns}
		kept := stored
		for i := range reported {
			if reported[i] > stored[i] {
				delta[i] = reported[i] - stored[i]
				kept[i] = reported[i]
			}
		}
		updates := map[string]any{}
		if kept != stored {
			updates["up_bytes"], updates["down_bytes"], updates["up_packets"] = kept[0], kept[1], kept[2]
			updates["down_packets"], updates["total_conns"] = kept[3], kept[4]
		}
		if observedAt.After(row.ObservedAt) {
			updates["active_conns"], updates["observed_at"] = counter.GetActiveConns(), observedAt
		}
		if len(updates) > 0 {
			updates["updated_at"] = observedAt
			if err := tx.Model(&model.KernelForwardCounter{}).
				Where("route_id = ? AND hop_index = ? AND node_ref = ? AND counter_epoch = ?", key.route, key.hop, nodeRef, key.epoch).
				Updates(updates).Error; err != nil {
				return false, fmt.Errorf("kernel forward: store counter: %w", err)
			}
		}
	}
	if delta == ([5]uint64{}) {
		return false, nil
	}
	table := model.KernelForwardTraffic{}.TableName()
	bucket := model.KernelForwardTraffic{
		RouteID: key.route, HopIndex: key.hop, NodeRef: nodeRef, HourStart: observedAt.Truncate(time.Hour),
		UpBytes: delta[0], DownBytes: delta[1], UpPackets: delta[2], DownPackets: delta[3], Conns: delta[4], UpdatedAt: observedAt,
	}
	if err := tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "route_id"}, {Name: "hop_index"}, {Name: "node_ref"}, {Name: "hour_start"}},
		DoUpdates: clause.Assignments(map[string]any{
			"up_bytes":     gorm.Expr(table+".up_bytes + ?", delta[0]),
			"down_bytes":   gorm.Expr(table+".down_bytes + ?", delta[1]),
			"up_packets":   gorm.Expr(table+".up_packets + ?", delta[2]),
			"down_packets": gorm.Expr(table+".down_packets + ?", delta[3]),
			"conns":        gorm.Expr(table+".conns + ?", delta[4]),
			"updated_at":   observedAt,
		}),
	}).Create(&bucket).Error; err != nil {
		return false, fmt.Errorf("kernel forward: meter traffic: %w", err)
	}
	return true, nil
}

// quotaCandidates answers the routes among grew whose entry traffic now
// reaches their quota while the kernel does not pause them yet.
func quotaCandidates(tx *gorm.DB, grew map[string]bool) ([]string, error) {
	if len(grew) == 0 {
		return nil, nil
	}
	ids := make([]string, 0, len(grew))
	for id := range grew {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var rows []model.KernelForwardRoute
	if err := tx.Where("id IN ? AND enforced = ?", ids, "").Find(&rows).Error; err != nil {
		return nil, err
	}
	var candidates []string
	var used map[string]uint64
	for _, row := range rows {
		route, err := decodeRoute(row)
		if err != nil {
			return nil, err
		}
		if route.GetLimits().GetQuotaBytes() == 0 {
			continue
		}
		if used == nil {
			if used, err = entryUsage(tx); err != nil {
				return nil, err
			}
		}
		if used[row.ID] >= route.GetLimits().GetQuotaBytes() {
			candidates = append(candidates, row.ID)
		}
	}
	return candidates, nil
}

// RouteStats answers a route's metered counters per hop and node, summed
// over counter epochs (ForwardControl.GetRouteStats): active_conns and
// observed_at are the latest epoch's, counter_epoch is empty. ErrNotFound
// for an unknown route.
func (s *Service) RouteStats(ctx context.Context, routeID string) ([]*forwardv1.Counters, error) {
	if _, err := s.GetRoute(ctx, routeID); err != nil {
		return nil, err
	}
	db, err := s.db(ctx)
	if err != nil {
		return nil, err
	}
	var rows []model.KernelForwardCounter
	if err := db.Where("route_id = ?", routeID).Order("hop_index, node_ref, observed_at").Find(&rows).Error; err != nil {
		return nil, err
	}
	var out []*forwardv1.Counters
	index := map[string]*forwardv1.Counters{}
	for _, row := range rows {
		key := fmt.Sprintf("%d/%s", row.HopIndex, row.NodeRef)
		total, ok := index[key]
		if !ok {
			total = &forwardv1.Counters{RouteId: routeID, HopIndex: row.HopIndex, NodeRef: row.NodeRef}
			index[key] = total
			out = append(out, total)
		}
		total.UpBytes += row.UpBytes
		total.DownBytes += row.DownBytes
		total.UpPackets += row.UpPackets
		total.DownPackets += row.DownPackets
		total.TotalConns += row.TotalConns
		if observed := row.ObservedAt.UnixMilli(); observed >= total.ObservedAtUnixMs {
			total.ObservedAtUnixMs, total.ActiveConns = observed, row.ActiveConns
		}
	}
	return out, nil
}

// RouteTraffic is one hour of a route hop's metered traffic on a node.
type RouteTraffic = model.KernelForwardTraffic

// Traffic answers the ledger's hourly buckets of a route from since
// (inclusive) to until (exclusive), by hour, hop and node.
func (s *Service) Traffic(ctx context.Context, routeID string, since, until time.Time) ([]RouteTraffic, error) {
	db, err := s.db(ctx)
	if err != nil {
		return nil, err
	}
	var rows []model.KernelForwardTraffic
	err = db.Where("route_id = ? AND hour_start >= ? AND hour_start < ?", routeID, since.UTC(), until.UTC()).
		Order("hour_start, hop_index, node_ref").Find(&rows).Error
	return rows, err
}

// RouteHealth answers the health of every upstream of a route's hops from
// its nodes' latest reports (ForwardControl.GetRouteHealth). ErrNotFound
// for an unknown route.
func (s *Service) RouteHealth(ctx context.Context, routeID string) ([]*forwardv1.UpstreamHealth, error) {
	if _, err := s.GetRoute(ctx, routeID); err != nil {
		return nil, err
	}
	db, err := s.db(ctx)
	if err != nil {
		return nil, err
	}
	var refs []string
	if err := db.Model(&model.KernelForwardAllocation{}).Distinct("node_ref").
		Where("route_id = ? AND released_at IS NULL", routeID).Order("node_ref").Pluck("node_ref", &refs).Error; err != nil {
		return nil, err
	}
	if len(refs) == 0 {
		return nil, nil
	}
	var rows []model.KernelForwardNodeReport
	if err := db.Where("node_ref IN ?", refs).Order("node_ref").Find(&rows).Error; err != nil {
		return nil, err
	}
	var out []*forwardv1.UpstreamHealth
	for _, row := range rows {
		report := &forwardv1.NodeForwardReport{}
		if err := jsonRead.Unmarshal([]byte(row.ReportJSON), report); err != nil {
			return nil, fmt.Errorf("kernel forward: report of %s: %w", row.NodeRef, err)
		}
		for _, health := range report.GetHealth() {
			if health.GetRouteId() == routeID {
				out = append(out, health)
			}
		}
	}
	return out, nil
}

// Convergence is one node's desired and reported generation.
type Convergence struct {
	NodeRef string
	// Desired is the generation of the node's stored state.
	Desired uint64
	// Reported is false before the node's first report; Applied,
	// Generation and HopErrors are its latest report's.
	Reported   bool
	Generation uint64
	Applied    bool
	HopErrors  int
	ObservedAt time.Time
}

// Lag is how many generations the node runs behind its desired one: the
// desired generation before its first report.
func (c Convergence) Lag() uint64 {
	switch {
	case !c.Reported:
		return c.Desired
	case c.Generation >= c.Desired:
		return 0
	default:
		return c.Desired - c.Generation
	}
}

// Converged is true when the node reported that it runs its desired
// generation.
func (c Convergence) Converged() bool { return c.Reported && c.Generation >= c.Desired }

// NodeConvergence answers every node with a desired state and its latest
// report, by node.
func NodeConvergence(ctx context.Context, db *gorm.DB) ([]Convergence, error) {
	db = db.WithContext(ctx)
	var states []model.KernelForwardNodeState
	if err := db.Select("node_ref", "generation").Order("node_ref").Find(&states).Error; err != nil {
		return nil, err
	}
	var reports []model.KernelForwardNodeReport
	if err := db.Select("node_ref", "generation", "applied", "hop_errors", "observed_at").Find(&reports).Error; err != nil {
		return nil, err
	}
	byNode := make(map[string]model.KernelForwardNodeReport, len(reports))
	for _, report := range reports {
		byNode[report.NodeRef] = report
	}
	out := make([]Convergence, 0, len(states))
	for _, state := range states {
		c := Convergence{NodeRef: state.NodeRef, Desired: state.Generation}
		if report, ok := byNode[state.NodeRef]; ok {
			c.Reported, c.Generation, c.Applied, c.HopErrors, c.ObservedAt = true, report.Generation, report.Applied, report.HopErrors, report.ObservedAt
		}
		out = append(out, c)
	}
	return out, nil
}

// LatestReport answers a node's latest report without its counters; found
// is false before its first.
func (s *Service) LatestReport(ctx context.Context, nodeRef string) (*forwardv1.NodeForwardReport, bool, error) {
	db, err := s.db(ctx)
	if err != nil {
		return nil, false, err
	}
	var rows []model.KernelForwardNodeReport
	if err := db.Where("node_ref = ?", nodeRef).Limit(1).Find(&rows).Error; err != nil || len(rows) == 0 {
		return nil, false, err
	}
	report := &forwardv1.NodeForwardReport{}
	if err := jsonRead.Unmarshal([]byte(rows[0].ReportJSON), report); err != nil {
		return nil, false, err
	}
	return report, true, nil
}
