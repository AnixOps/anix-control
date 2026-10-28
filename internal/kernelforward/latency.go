package kernelforward

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"time"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

// The latency history (forward-sdk.md section 7.5, L3). Every report
// carries each node's latest health check of each upstream of its hops
// (UpstreamHealth: the connect round trip, 0 when it failed, and when it
// ran). Control folds the checks into five-minute buckets per route, hop,
// node and upstream (KernelForwardLatency):
//
//   - A check is counted once: a bucket keeps the newest check it holds
//     (LastCheckedAt), and a report that carries the same check again, or
//     an older one, adds nothing.
//   - A check with a round trip is a sample; one without is a failure.
//   - Checks dated in the future (beyond LatencyClockSkew) or older than
//     LatencyRetention are ignored; the maintenance prunes buckets older
//     than LatencyRetention.

const (
	// LatencyBucket is the width of a latency bucket.
	LatencyBucket = 5 * time.Minute
	// LatencyRetention is how long latency buckets are kept, and the
	// widest window GetLatency answers.
	LatencyRetention = 7 * 24 * time.Hour
	// LatencyClockSkew is how far in the future a check may be dated.
	LatencyClockSkew = time.Minute
	// MaxLatencyBuckets bounds one GetLatency answer.
	MaxLatencyBuckets = 20000
	// maxUpstreamKey bounds an upstream's host:port (the column's size).
	maxUpstreamKey = 300
)

// foldLatency adds the health checks of one node's report to the latency
// history.
func foldLatency(tx *gorm.DB, nodeRef string, health []*forwardv1.UpstreamHealth, now time.Time) error {
	for _, check := range health {
		if check.GetCheckedAtUnixMs() <= 0 || check.GetRouteId() == "" || check.GetAddress() == "" {
			continue
		}
		checkedAt := time.UnixMilli(check.GetCheckedAtUnixMs()).UTC()
		if checkedAt.After(now.Add(LatencyClockSkew)) || checkedAt.Before(now.Add(-LatencyRetention)) {
			continue
		}
		upstream := net.JoinHostPort(check.GetAddress(), strconv.FormatUint(uint64(check.GetPort()), 10))
		if len(upstream) > maxUpstreamKey {
			continue
		}
		bucketStart := checkedAt.Truncate(LatencyBucket)
		var rows []model.KernelForwardLatency
		if err := tx.Where("route_id = ? AND hop_index = ? AND node_ref = ? AND upstream = ? AND bucket_start = ?",
			check.GetRouteId(), check.GetHopIndex(), nodeRef, upstream, bucketStart).Limit(1).Find(&rows).Error; err != nil {
			return fmt.Errorf("kernel forward: load latency: %w", err)
		}
		row := model.KernelForwardLatency{
			RouteID: check.GetRouteId(), HopIndex: check.GetHopIndex(), NodeRef: nodeRef, Upstream: upstream, BucketStart: bucketStart,
		}
		if len(rows) > 0 {
			if !checkedAt.After(rows[0].LastCheckedAt) {
				continue
			}
			row = rows[0]
		}
		row.Samples++
		if rtt := check.GetRttUs(); rtt > 0 {
			row.RTTSumUs += uint64(rtt)
			if row.RTTMinUs == 0 || rtt < row.RTTMinUs {
				row.RTTMinUs = rtt
			}
			if rtt > row.RTTMaxUs {
				row.RTTMaxUs = rtt
			}
		} else {
			row.Failures++
		}
		row.LastCheckedAt = checkedAt
		var err error
		if len(rows) > 0 {
			err = tx.Model(&model.KernelForwardLatency{}).
				Where("route_id = ? AND hop_index = ? AND node_ref = ? AND upstream = ? AND bucket_start = ?",
					row.RouteID, row.HopIndex, row.NodeRef, row.Upstream, row.BucketStart).
				Updates(map[string]any{
					"samples": row.Samples, "failures": row.Failures, "rtt_sum_us": row.RTTSumUs,
					"rtt_min_us": row.RTTMinUs, "rtt_max_us": row.RTTMaxUs, "last_checked_at": row.LastCheckedAt,
				}).Error
		} else {
			err = tx.Create(&row).Error
		}
		if err != nil {
			return fmt.Errorf("kernel forward: store latency: %w", err)
		}
	}
	return nil
}

// Latency answers a route's latency buckets from since (inclusive) to until
// (exclusive), by bucket, hop, node and upstream (ForwardControl.GetLatency).
// Zero times default to the 24 hours up to the end of the current bucket.
// ErrNotFound for an unknown route.
func (s *Service) Latency(ctx context.Context, routeID string, since, until time.Time) ([]*forwardv1.LatencyBucket, bool, error) {
	if _, err := s.GetRoute(ctx, routeID); err != nil {
		return nil, false, err
	}
	db, err := s.db(ctx)
	if err != nil {
		return nil, false, err
	}
	if until.IsZero() {
		until = s.now().Truncate(LatencyBucket).Add(LatencyBucket)
	}
	if since.IsZero() {
		since = until.Add(-24 * time.Hour)
	}
	if !since.Before(until) || until.Sub(since) > LatencyRetention {
		return nil, false, fmt.Errorf("%w: since must be before until, at most 7 days apart", ErrInvalidRequest)
	}
	var rows []model.KernelForwardLatency
	if err := db.Where("route_id = ? AND bucket_start >= ? AND bucket_start < ?", routeID, since.UTC(), until.UTC()).
		Order("bucket_start, hop_index, node_ref, upstream").Limit(MaxLatencyBuckets + 1).Find(&rows).Error; err != nil {
		return nil, false, err
	}
	truncated := len(rows) > MaxLatencyBuckets
	if truncated {
		rows = rows[:MaxLatencyBuckets]
	}
	out := make([]*forwardv1.LatencyBucket, 0, len(rows))
	for _, row := range rows {
		bucket := &forwardv1.LatencyBucket{
			RouteId: row.RouteID, HopIndex: row.HopIndex, NodeRef: row.NodeRef, BucketStartUnixMs: row.BucketStart.UnixMilli(),
			Samples: row.Samples, Failures: row.Failures, RttMinUs: row.RTTMinUs, RttMaxUs: row.RTTMaxUs,
		}
		if host, port, err := net.SplitHostPort(row.Upstream); err == nil {
			bucket.Address = host
			if value, err := strconv.ParseUint(port, 10, 32); err == nil {
				bucket.Port = uint32(value)
			}
		}
		if ok := row.Samples - row.Failures; ok > 0 {
			bucket.RttAvgUs = uint32(row.RTTSumUs / uint64(ok))
		}
		out = append(out, bucket)
	}
	return out, truncated, nil
}

// pruneLatency deletes the latency buckets older than LatencyRetention.
func pruneLatency(db *gorm.DB, now time.Time) error {
	if err := db.Where("bucket_start < ?", now.Add(-LatencyRetention).UTC()).Delete(&model.KernelForwardLatency{}).Error; err != nil {
		return fmt.Errorf("kernel forward: prune latency: %w", err)
	}
	return nil
}
