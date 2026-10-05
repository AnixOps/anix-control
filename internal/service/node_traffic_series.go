package service

import (
	"errors"
	"fmt"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
)

// Granularities of a node's traffic series.
const (
	// NodeTrafficHour: hourly buckets from the raw traffic log
	// (v2_server_log), aligned to UTC hours.
	NodeTrafficHour = "hour"
	// NodeTrafficDay: daily buckets from the daily server statistics
	// (v2_stat_server, record_type d), aligned to the local midnight the
	// statistics were recorded at.
	NodeTrafficDay = "day"

	// MaxNodeTrafficHours bounds an hourly series: the same 30 days the
	// admin hourly traffic page (GetHourlyTraffic) clamps to.
	MaxNodeTrafficHours = maxHourlyWindow
	// MaxNodeTrafficDays bounds a daily series.
	MaxNodeTrafficDays = 366
)

// ErrNodeTrafficWindow is a bad granularity or window of a node traffic
// request; its text says what is wrong.
var ErrNodeTrafficWindow = errors.New("invalid node traffic window")

// ErrNodeTrafficNode is a node traffic request for a node that does not
// exist.
var ErrNodeTrafficNode = errors.New("node not found")

// NodeTrafficPoint is one bucket of a node's traffic series: the bytes the
// node's users moved through it, with the node's traffic rate applied (the
// bytes the panel meters, as /admin/traffic/hourly and the statistics
// tables count them).
type NodeTrafficPoint struct {
	Start time.Time
	Up    int64
	Down  int64
}

// NodeTrafficSeries is a node's traffic over [Since, Until) in buckets of
// Granularity, ascending, with the buckets without traffic as zeros.
type NodeTrafficSeries struct {
	NodeID      uint
	Granularity string
	Since       time.Time
	Until       time.Time
	Points      []NodeTrafficPoint
}

// NodeTrafficBucketBounds aligns a requested window to the buckets of
// granularity: since rounds down to a bucket start, until up to a bucket
// boundary; a zero until is the end of the current bucket and a zero since
// the 24 hours (hour) or 30 days (day) before until. It returns
// ErrNodeTrafficWindow for an unknown granularity, an empty or reversed
// window, or one of more than MaxNodeTrafficHours hours or MaxNodeTrafficDays
// days.
func NodeTrafficBucketBounds(granularity string, since, until, now time.Time) (time.Time, time.Time, error) {
	var (
		floor   func(time.Time) time.Time
		next    func(time.Time) time.Time
		buckets int
		span    int
	)
	switch granularity {
	case NodeTrafficHour:
		floor = func(t time.Time) time.Time { return t.UTC().Truncate(time.Hour) }
		next = func(t time.Time) time.Time { return t.Add(time.Hour) }
		buckets, span = MaxNodeTrafficHours, 24
	case NodeTrafficDay:
		floor = func(t time.Time) time.Time {
			t = t.In(time.Local)
			return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
		}
		next = func(t time.Time) time.Time { return t.AddDate(0, 0, 1) }
		buckets, span = MaxNodeTrafficDays, 30
	default:
		return time.Time{}, time.Time{}, fmt.Errorf("%w: granularity must be %s or %s", ErrNodeTrafficWindow, NodeTrafficHour, NodeTrafficDay)
	}
	if until.IsZero() {
		until = next(floor(now))
	} else if rounded := floor(until); rounded.Before(until) {
		until = next(rounded)
	} else {
		until = rounded
	}
	if since.IsZero() {
		since = until
		for i := 0; i < span; i++ {
			since = floor(since.Add(-time.Nanosecond))
		}
	}
	since = floor(since)
	if !since.Before(until) {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: since must be before until", ErrNodeTrafficWindow)
	}
	count := 0
	for t := since; t.Before(until); t = next(t) {
		if count++; count > buckets {
			return time.Time{}, time.Time{}, fmt.Errorf("%w: at most %d %s buckets", ErrNodeTrafficWindow, buckets, granularity)
		}
	}
	return since, until, nil
}

// GetNodeTraffic returns one node's traffic series between since and until
// (see NodeTrafficBucketBounds for how they are aligned and bounded).
//
// Hourly buckets sum the node's rows of v2_server_log, (u * rate) up and
// (d * rate) down, over every server type the node reported under (agent
// and legacy reports write "node", UniProxy the protocol name; the node id
// is the same). It reads the rows of one node in the window through
// idx_v2_server_log_server_id and idx_v2_server_log_log_at: cheaper than the
// global GetHourlyTraffic, which scans the window of every node, and bounded
// by the retention of the raw log (docs/reference/traffic-stats-operations.md).
// Daily buckets are the node's v2_stat_server day rows, which are kept and
// small, and are recorded with the rate applied already.
//
// ErrNodeTrafficNode when the node does not exist.
func (s *StatsService) GetNodeTraffic(nodeID uint, granularity string, since, until time.Time) (*NodeTrafficSeries, error) {
	since, until, err := NodeTrafficBucketBounds(granularity, since, until, time.Now())
	if err != nil {
		return nil, err
	}
	var count int64
	if err := s.db.Model(&model.Node{}).Where("id = ?", nodeID).Count(&count).Error; err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, ErrNodeTrafficNode
	}

	series := &NodeTrafficSeries{NodeID: nodeID, Granularity: granularity, Since: since, Until: until}
	var rows map[int64]*NodeTrafficPoint
	switch granularity {
	case NodeTrafficHour:
		rows, err = s.nodeHourlyTraffic(nodeID, since, until)
	default:
		rows, err = s.nodeDailyTraffic(nodeID, since, until)
	}
	if err != nil {
		return nil, err
	}
	for start := since; start.Before(until); {
		point := NodeTrafficPoint{Start: start}
		if row, ok := rows[start.Unix()]; ok {
			point.Up, point.Down = row.Up, row.Down
		}
		series.Points = append(series.Points, point)
		if granularity == NodeTrafficHour {
			start = start.Add(time.Hour)
		} else {
			start = start.AddDate(0, 0, 1)
		}
	}
	return series, nil
}

func (s *StatsService) nodeHourlyTraffic(nodeID uint, since, until time.Time) (map[int64]*NodeTrafficPoint, error) {
	type bucketRow struct {
		Hour int64
		Up   int64
		Down int64
	}
	var rows []bucketRow
	if err := s.db.Model(&model.TrafficLog{}).
		Where("server_id = ? AND log_at >= ? AND log_at < ?", nodeID, since.Unix(), until.Unix()).
		Select("(log_at / ?) * ? AS hour, "+s.trafficLogDirectionExpr("u")+" AS up, "+s.trafficLogDirectionExpr("d")+" AS down", hourSeconds, hourSeconds).
		Group("hour").
		Order("hour ASC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[int64]*NodeTrafficPoint, len(rows))
	for _, row := range rows {
		out[row.Hour] = &NodeTrafficPoint{Up: row.Up, Down: row.Down}
	}
	return out, nil
}

func (s *StatsService) nodeDailyTraffic(nodeID uint, since, until time.Time) (map[int64]*NodeTrafficPoint, error) {
	var rows []model.StatServer
	if err := s.db.Model(&model.StatServer{}).
		Select("record_at, u, d").
		Where("server_id = ? AND record_type = ? AND record_at >= ? AND record_at < ?", nodeID, "d", since.Unix(), until.Unix()).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	// A row belongs to the local day that contains its record time: the
	// statistics of a panel imported from v2board were recorded at another
	// zone's midnight, and a node's several server types have one row each.
	out := make(map[int64]*NodeTrafficPoint, len(rows))
	for _, row := range rows {
		day := time.Unix(row.RecordAt, 0).In(time.Local)
		key := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.Local).Unix()
		point := out[key]
		if point == nil {
			point = &NodeTrafficPoint{}
			out[key] = point
		}
		point.Up += max(row.U, 0)
		point.Down += max(row.D, 0)
	}
	return out, nil
}

// trafficLogDirectionExpr is trafficLogAggregateExpr for one direction
// column of v2_server_log (u or d): the sum of the column with the row's
// rate applied, clamped to int64, negative values counting as zero, in the
// SQL of the database in use.
func (s *StatsService) trafficLogDirectionExpr(column string) string {
	if s.db != nil && s.db.Name() == "postgres" {
		return "FLOOR(LEAST(COALESCE(SUM(" +
			"GREATEST(COALESCE(" + column + ", 0), 0)::numeric * (CASE WHEN rate > 0 THEN rate::numeric ELSE 1::numeric END)" +
			"), 0), " + maxInt64SQL + "::numeric))::bigint"
	}
	return "CAST(MIN(COALESCE(SUM(" +
		"CAST(CASE WHEN " + column + " > 0 THEN " + column + " ELSE 0 END AS REAL) * (CASE WHEN rate > 0 THEN rate ELSE 1 END)" +
		"), 0), " + maxInt64SQL + ") AS INTEGER)"
}
