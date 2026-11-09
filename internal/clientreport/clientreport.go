// Package clientreport takes the connection outcomes AnixOps Net clients
// report about the nodes of their subscription (anx
// docs/FUTURE-IMPROVEMENTS.md F-55 item 1, the W2 client thread): per node,
// how many connections worked, fell back, timed out, were refused or reset
// (before or after the handshake), lost their keepalive, and whether the
// client gave up on the node. Counts only, never a destination. Reports
// add up per subscriber, hour, node and kind of network; Summary answers
// them per node for the administrator, the client-side signal of blocking
// beside the nodes' own (F-38).
//
// The endpoint is POST /{subscribe_path}/{token}/report: the subscription's
// own token in the path, 204 on success, 404 for an unknown token, 400 for
// a body out of bounds, 429 when the token reported within RateWindow
// (the client drops the report).
package clientreport

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Limits of a report.
const (
	MaxNodes      = 200
	MaxCount      = 1_000_000
	MinWindowSecs = 60
	MaxWindowSecs = 86400
	// RateWindow is how often one token may report.
	RateWindow = 10 * time.Minute
	// MaxBodyBytes caps the request body.
	MaxBodyBytes = 64 << 10
	// MaxSummaryHours caps the summary's look back.
	MaxSummaryHours = 24 * 14
)

// ErrInvalid: the report is out of bounds; the error says why.
var ErrInvalid = errors.New("invalid client report")

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

// Networks a report may name; anything else is "other".
var networks = map[string]bool{"wifi": true, "cellular": true, "ethernet": true, "other": true}

// NodeReport is one node's outcomes in the window.
type NodeReport struct {
	// Node is the node as the subscription named it.
	Node           string `json:"node"`
	Server         string `json:"server"`
	Proto          string `json:"proto"`
	OK             int64  `json:"ok"`
	Fallback       int64  `json:"fallback"`
	Timeout        int64  `json:"timeout"`
	Refused        int64  `json:"refused"`
	Reset          int64  `json:"reset"`
	AfterHandshake int64  `json:"after_handshake"`
	KeepaliveLost  int64  `json:"keepalive_lost"`
	Other          int64  `json:"other"`
	Stopped        bool   `json:"stopped"`
	ClockSkewSecs  *int64 `json:"clock_skew_secs"`
}

// Report is the body a client posts. Unknown fields are ignored.
type Report struct {
	Client     string       `json:"client"`
	Platform   string       `json:"platform"`
	Network    string       `json:"network"`
	WindowSecs int64        `json:"window_secs"`
	Nodes      []NodeReport `json:"nodes"`
}

func clip(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) > limit {
		value = value[:limit]
	}
	return value
}

// Validate checks the report's bounds and normalizes its strings.
func (r *Report) Validate() error {
	if r.WindowSecs < MinWindowSecs || r.WindowSecs > MaxWindowSecs {
		return invalid("window_secs is %d to %d", MinWindowSecs, MaxWindowSecs)
	}
	if len(r.Nodes) == 0 || len(r.Nodes) > MaxNodes {
		return invalid("nodes: 1 to %d", MaxNodes)
	}
	r.Client, r.Platform = clip(r.Client, 64), strings.ToLower(clip(r.Platform, 16))
	r.Network = strings.ToLower(strings.TrimSpace(r.Network))
	if !networks[r.Network] {
		r.Network = "other"
	}
	for i := range r.Nodes {
		node := &r.Nodes[i]
		node.Node, node.Server, node.Proto = clip(node.Node, 128), clip(node.Server, 255), strings.ToLower(clip(node.Proto, 32))
		if node.Node == "" {
			return invalid("nodes[%d]: node is required", i)
		}
		for _, count := range []int64{node.OK, node.Fallback, node.Timeout, node.Refused, node.Reset, node.AfterHandshake, node.KeepaliveLost, node.Other} {
			if count < 0 || count > MaxCount {
				return invalid("nodes[%d]: counts are 0 to %d", i, MaxCount)
			}
		}
		if node.ClockSkewSecs != nil && (*node.ClockSkewSecs > MaxWindowSecs*365 || *node.ClockSkewSecs < -MaxWindowSecs*365) {
			node.ClockSkewSecs = nil
		}
	}
	return nil
}

// Retention is how long reports are kept.
const Retention = 30 * 24 * time.Hour

var pruned struct {
	sync.Mutex
	at time.Time
}

// prune drops reports older than Retention, at most once an hour.
func prune(ctx context.Context, db *gorm.DB, now time.Time) error {
	pruned.Lock()
	defer pruned.Unlock()
	if now.Sub(pruned.at) < time.Hour {
		return nil
	}
	pruned.at = now
	return db.WithContext(ctx).Where("hour < ?", now.Add(-Retention)).Delete(&model.ClientFailureReport{}).Error
}

// Record adds the report to the subscriber's rows of the hour of now, and
// drops reports older than Retention.
func Record(ctx context.Context, db *gorm.DB, userID uint, report Report, now time.Time) error {
	if err := prune(ctx, db, now); err != nil {
		return err
	}
	hour := now.UTC().Truncate(time.Hour)
	// The same node twice in one report adds up too.
	merged := map[string]*model.ClientFailureReport{}
	var order []string
	for _, node := range report.Nodes {
		row := merged[node.Node]
		if row == nil {
			row = &model.ClientFailureReport{UserID: userID, Hour: hour, Node: node.Node, Network: report.Network,
				Platform: report.Platform, Client: report.Client, UpdatedAt: now.UTC()}
			merged[node.Node] = row
			order = append(order, node.Node)
		}
		row.Server, row.Proto = node.Server, node.Proto
		row.OK += node.OK
		row.Fallback += node.Fallback
		row.Timeout += node.Timeout
		row.Refused += node.Refused
		row.Reset += node.Reset
		row.AfterHandshake += node.AfterHandshake
		row.KeepaliveLost += node.KeepaliveLost
		row.Other += node.Other
		row.Stopped = row.Stopped || node.Stopped
		if node.ClockSkewSecs != nil {
			row.ClockSkewSecs = node.ClockSkewSecs
		}
	}
	rows := make([]model.ClientFailureReport, 0, len(order))
	for _, key := range order {
		rows = append(rows, *merged[key])
	}
	add := func(column string) clause.Expr {
		return gorm.Expr(fmt.Sprintf("%s.%s + excluded.%s", model.ClientFailureReport{}.TableName(), column, column))
	}
	return db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}, {Name: "hour"}, {Name: "node"}, {Name: "network"}},
		DoUpdates: clause.Set{
			{Column: clause.Column{Name: "ok"}, Value: add("ok")},
			{Column: clause.Column{Name: "fallback"}, Value: add("fallback")},
			{Column: clause.Column{Name: "timeout"}, Value: add("timeout")},
			{Column: clause.Column{Name: "refused"}, Value: add("refused")},
			{Column: clause.Column{Name: "reset"}, Value: add("reset")},
			{Column: clause.Column{Name: "after_handshake"}, Value: add("after_handshake")},
			{Column: clause.Column{Name: "keepalive_lost"}, Value: add("keepalive_lost")},
			{Column: clause.Column{Name: "other"}, Value: add("other")},
			{Column: clause.Column{Name: "stopped"}, Value: gorm.Expr("excluded.stopped")},
			{Column: clause.Column{Name: "clock_skew_secs"}, Value: gorm.Expr(fmt.Sprintf("COALESCE(excluded.clock_skew_secs, %s.clock_skew_secs)", model.ClientFailureReport{}.TableName()))},
			{Column: clause.Column{Name: "server"}, Value: gorm.Expr("excluded.server")},
			{Column: clause.Column{Name: "proto"}, Value: gorm.Expr("excluded.proto")},
			{Column: clause.Column{Name: "platform"}, Value: gorm.Expr("excluded.platform")},
			{Column: clause.Column{Name: "client"}, Value: gorm.Expr("excluded.client")},
			{Column: clause.Column{Name: "updated_at"}, Value: gorm.Expr("excluded.updated_at")},
		},
	}).Create(&rows).Error
}

// Limiter lets one report per token through per RateWindow. It lives in
// the Control process: a restart forgets it, which only lets one more
// report through.
type Limiter struct {
	mu   sync.Mutex
	last map[string]time.Time
}

// NewLimiter returns an empty limiter.
func NewLimiter() *Limiter { return &Limiter{last: map[string]time.Time{}} }

// Allow reports whether key may report at now, and records it if so.
func (l *Limiter) Allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if last, ok := l.last[key]; ok && now.Sub(last) < RateWindow {
		return false
	}
	if len(l.last) > 100_000 {
		for k, t := range l.last {
			if now.Sub(t) >= RateWindow {
				delete(l.last, k)
			}
		}
	}
	l.last[key] = now
	return true
}

// NodeSummary is a node's outcomes over the summary's hours, all
// subscribers and networks together.
type NodeSummary struct {
	Node           string           `json:"node"`
	Server         string           `json:"server"`
	Proto          string           `json:"proto"`
	Users          int              `json:"users"`
	StoppedUsers   int              `json:"stopped_users"`
	OK             int64            `json:"ok"`
	Fallback       int64            `json:"fallback"`
	Timeout        int64            `json:"timeout"`
	Refused        int64            `json:"refused"`
	Reset          int64            `json:"reset"`
	AfterHandshake int64            `json:"after_handshake"`
	KeepaliveLost  int64            `json:"keepalive_lost"`
	Other          int64            `json:"other"`
	Networks       map[string]int64 `json:"networks"`
	// FailureRatio is the failures over every outcome (0 without any).
	FailureRatio float64   `json:"failure_ratio"`
	LastAt       time.Time `json:"last_at"`
}

// Summary answers every reported node's outcomes over the last hours,
// most failing first.
func Summary(ctx context.Context, db *gorm.DB, hours int, now time.Time) ([]NodeSummary, error) {
	if hours < 1 || hours > MaxSummaryHours {
		return nil, invalid("hours is 1 to %d", MaxSummaryHours)
	}
	out := []NodeSummary{}
	if !db.Migrator().HasTable(&model.ClientFailureReport{}) {
		return out, nil
	}
	since := now.UTC().Truncate(time.Hour).Add(-time.Duration(hours-1) * time.Hour)
	var rows []model.ClientFailureReport
	if err := db.WithContext(ctx).Where("hour >= ?", since).Order("updated_at ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	byNode := map[string]*NodeSummary{}
	users := map[string]map[uint]bool{}
	stopped := map[string]map[uint]bool{}
	for _, row := range rows {
		s := byNode[row.Node]
		if s == nil {
			s = &NodeSummary{Node: row.Node, Networks: map[string]int64{}}
			byNode[row.Node] = s
			users[row.Node], stopped[row.Node] = map[uint]bool{}, map[uint]bool{}
		}
		s.Server, s.Proto = row.Server, row.Proto
		s.OK += row.OK
		s.Fallback += row.Fallback
		s.Timeout += row.Timeout
		s.Refused += row.Refused
		s.Reset += row.Reset
		s.AfterHandshake += row.AfterHandshake
		s.KeepaliveLost += row.KeepaliveLost
		s.Other += row.Other
		failures := row.Fallback + row.Timeout + row.Refused + row.Reset + row.AfterHandshake + row.KeepaliveLost + row.Other
		s.Networks[row.Network] += row.OK + failures
		users[row.Node][row.UserID] = true
		if row.Stopped {
			stopped[row.Node][row.UserID] = true
		}
		if row.UpdatedAt.After(s.LastAt) {
			s.LastAt = row.UpdatedAt
		}
	}
	for node, s := range byNode {
		s.Users, s.StoppedUsers = len(users[node]), len(stopped[node])
		failures := s.Fallback + s.Timeout + s.Refused + s.Reset + s.AfterHandshake + s.KeepaliveLost + s.Other
		if total := s.OK + failures; total > 0 {
			s.FailureRatio = float64(failures) / float64(total)
		}
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].FailureRatio != out[j].FailureRatio {
			return out[i].FailureRatio > out[j].FailureRatio
		}
		return out[i].Node < out[j].Node
	})
	return out, nil
}
