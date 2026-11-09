// Package entrypool deals node entries (internal/nodeentry) out to
// subscribers in pools, so a leaked subscription exposes only its pool's
// addresses and can be traced back (anx docs/FUTURE-IMPROVEMENTS.md F-55):
//
//   - An entry with a pool is offered only to the subscribers of that pool;
//     an entry without one to every subscriber, but only on nodes that have
//     no entry of the subscriber's own pool. A node whose entries are all in
//     other pools is not offered at all (never its bare host).
//   - A subscriber's pool is the administrator's assignment, else the
//     quarantine pool while the account is younger than QuarantineDays or
//     has no plan (new and trial accounts), else shard-(id % Shards) when
//     sharding is on, else none.
//   - A canary address is an entry in a pool of one subscriber (assign the
//     user to a pool like canary-42 and give a node an entry in it): when
//     that address is probed or blocked, Leak names the subscriber. Split
//     halves a pool's subscribers into two assigned pools to bisect a leak.
package entrypool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

var (
	// ErrInvalid: a request is invalid; the error says why.
	ErrInvalid = errors.New("invalid entry pool request")
	// ErrNotFound: no such entry or user.
	ErrNotFound = errors.New("not found")
)

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

// Limits and names.
const (
	MaxShards         = 64
	MaxQuarantineDays = 365
	DefaultQuarantine = "quarantine"
	ShardPrefix       = "shard-"
	// MaxListedUsers caps the user ids an answer lists.
	MaxListedUsers = 500
	policyRowID    = 1
	auditModule    = "entry_pool"
	auditTarget    = "entry_pool"

	AuditActionPolicy   = "entry_pool_policy"
	AuditActionAssign   = "entry_pool_assign"
	AuditActionUnassign = "entry_pool_unassign"
	AuditActionSplit    = "entry_pool_split"
)

var poolPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

// CheckPool validates a pool name; empty is "no pool".
func CheckPool(pool string) error {
	if pool != "" && !poolPattern.MatchString(pool) {
		return invalid("pool %q: 1 to 32 of a-z, 0-9, _ and -, starting with a letter or digit", pool)
	}
	return nil
}

// Actor is who made an administrator's change, for the audit entry.
type Actor struct {
	UserID uint
	Name   string
	IP     string
}

// Service manages the pools on one database.
type Service struct {
	DB *gorm.DB
	// Now defaults to time.Now.
	Now func() time.Time
}

// New returns the service on db.
func New(db *gorm.DB) *Service { return &Service{DB: db} }

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

// Policy is the policy as the API takes and answers it.
type Policy struct {
	Shards         int    `json:"shards"`
	QuarantineDays int    `json:"quarantine_days"`
	QuarantinePool string `json:"quarantine_pool"`
}

func loadPolicy(db *gorm.DB) (Policy, error) {
	policy := Policy{QuarantinePool: DefaultQuarantine}
	if !db.Migrator().HasTable(&model.EntryPoolPolicy{}) {
		return policy, nil
	}
	var rows []model.EntryPoolPolicy
	if err := db.Where("id = ?", policyRowID).Limit(1).Find(&rows).Error; err != nil {
		return policy, err
	}
	if len(rows) == 1 {
		policy = Policy{Shards: rows[0].Shards, QuarantineDays: rows[0].QuarantineDays, QuarantinePool: rows[0].QuarantinePool}
		if policy.QuarantinePool == "" {
			policy.QuarantinePool = DefaultQuarantine
		}
	}
	return policy, nil
}

// GetPolicy answers the policy (all off before the first write).
func (s *Service) GetPolicy(ctx context.Context) (Policy, error) {
	return loadPolicy(s.DB.WithContext(ctx))
}

// SetPolicy replaces the policy. Audited.
func (s *Service) SetPolicy(ctx context.Context, policy Policy, actor Actor) (Policy, error) {
	policy.QuarantinePool = strings.TrimSpace(policy.QuarantinePool)
	if policy.QuarantinePool == "" {
		policy.QuarantinePool = DefaultQuarantine
	}
	if policy.Shards < 0 || policy.Shards > MaxShards || policy.Shards == 1 {
		return Policy{}, invalid("shards is 0 (off) or 2 to %d", MaxShards)
	}
	if policy.QuarantineDays < 0 || policy.QuarantineDays > MaxQuarantineDays {
		return Policy{}, invalid("quarantine_days is 0 (off) to %d", MaxQuarantineDays)
	}
	if err := CheckPool(policy.QuarantinePool); err != nil {
		return Policy{}, err
	}
	if strings.HasPrefix(policy.QuarantinePool, ShardPrefix) {
		return Policy{}, invalid("quarantine_pool must not start with %q", ShardPrefix)
	}
	row := model.EntryPoolPolicy{ID: policyRowID, Shards: policy.Shards, QuarantineDays: policy.QuarantineDays,
		QuarantinePool: policy.QuarantinePool, UpdatedAt: s.now()}
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		return writeAudit(tx, actor, AuditActionPolicy, map[string]any{"shards": row.Shards, "quarantine_days": row.QuarantineDays, "quarantine_pool": row.QuarantinePool})
	})
	if err != nil {
		return Policy{}, err
	}
	return policy, nil
}

// userFacts is what the pool of a user depends on.
type userFacts struct {
	ID        uint
	PlanID    *uint
	CreatedAt time.Time
}

// poolOf answers a user's pool: the assignment, quarantine, shard, or "".
func poolOf(policy Policy, user userFacts, assigned string, hasAssignment bool, now time.Time) (pool, reason string) {
	if hasAssignment {
		return assigned, "assigned"
	}
	if policy.QuarantineDays > 0 && (user.PlanID == nil || user.CreatedAt.After(now.AddDate(0, 0, -policy.QuarantineDays))) {
		return policy.QuarantinePool, "quarantine"
	}
	if policy.Shards > 1 {
		return fmt.Sprintf("%s%d", ShardPrefix, user.ID%uint(policy.Shards)), "shard"
	}
	return "", "none"
}

// UserPool answers the pool of user (and why: assigned, quarantine, shard
// or none).
func UserPool(db *gorm.DB, userID uint, now time.Time) (string, string, error) {
	policy, err := loadPolicy(db)
	if err != nil {
		return "", "", err
	}
	var users []userFacts
	if err := db.Model(&model.User{}).Select("id", "plan_id", "created_at").Where("id = ?", userID).Limit(1).Find(&users).Error; err != nil {
		return "", "", err
	}
	if len(users) == 0 {
		return "", "", ErrNotFound
	}
	assigned, ok, err := assignment(db, userID)
	if err != nil {
		return "", "", err
	}
	pool, reason := poolOf(policy, users[0], assigned, ok, now)
	return pool, reason, nil
}

func assignment(db *gorm.DB, userID uint) (string, bool, error) {
	if !db.Migrator().HasTable(&model.UserEntryPool{}) {
		return "", false, nil
	}
	var rows []model.UserEntryPool
	if err := db.Where("user_id = ?", userID).Limit(1).Find(&rows).Error; err != nil {
		return "", false, err
	}
	if len(rows) == 0 {
		return "", false, nil
	}
	return rows[0].Pool, true, nil
}

// Filter answers the entries of each node a subscriber of pool sees, and
// the nodes the subscriber must not be offered at all (they have entries,
// none of them visible). Entries of the pool win over the shared ones
// (no pool) on a node; entries of other pools are never seen.
func Filter(entries map[uint][]model.NodeEntry, pool string) (map[uint][]model.NodeEntry, map[uint]bool) {
	out := make(map[uint][]model.NodeEntry, len(entries))
	hidden := map[uint]bool{}
	for nodeID, list := range entries {
		var own, shared []model.NodeEntry
		for _, entry := range list {
			switch {
			case pool != "" && entry.Pool == pool:
				own = append(own, entry)
			case entry.Pool == "":
				shared = append(shared, entry)
			}
		}
		switch {
		case len(own) > 0:
			out[nodeID] = own
		case len(shared) > 0:
			out[nodeID] = shared
		case len(list) > 0:
			hidden[nodeID] = true
		}
	}
	return out, hidden
}

// AssignInput assigns users to a pool; an empty pool removes their
// assignment (back to the policy).
type AssignInput struct {
	UserIDs []uint `json:"user_ids"`
	Pool    string `json:"pool"`
	Reason  string `json:"reason"`
}

// Assign sets or removes the users' assignment. Audited.
func (s *Service) Assign(ctx context.Context, input AssignInput, actor Actor) (int, error) {
	pool := strings.TrimSpace(input.Pool)
	if err := CheckPool(pool); err != nil {
		return 0, err
	}
	if len(input.UserIDs) == 0 || len(input.UserIDs) > 10000 {
		return 0, invalid("user_ids: 1 to 10000 users")
	}
	reason := truncate(strings.TrimSpace(input.Reason), 128)
	now := s.now()
	ids := dedupe(input.UserIDs)
	if len(ids) == 0 {
		return 0, invalid("user_ids: 1 to 10000 users")
	}
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&model.User{}).Where("id IN ?", ids).Count(&count).Error; err != nil {
			return err
		}
		if int(count) != len(ids) {
			return fmt.Errorf("%w: %d of the users do not exist", ErrNotFound, len(ids)-int(count))
		}
		if err := tx.Where("user_id IN ?", ids).Delete(&model.UserEntryPool{}).Error; err != nil {
			return err
		}
		action := AuditActionUnassign
		if pool != "" {
			action = AuditActionAssign
			rows := make([]model.UserEntryPool, 0, len(ids))
			for _, id := range ids {
				rows = append(rows, model.UserEntryPool{UserID: id, Pool: pool, Reason: reason, UpdatedAt: now})
			}
			if err := tx.CreateInBatches(rows, 500).Error; err != nil {
				return err
			}
		}
		return writeAudit(tx, actor, action, map[string]any{"pool": pool, "users": len(ids), "reason": reason})
	})
	if err != nil {
		return 0, err
	}
	return len(ids), nil
}

// PoolView is one pool: its entries and its subscribers.
type PoolView struct {
	Pool    string         `json:"pool"`
	Entries []EntryRef     `json:"entries"`
	Users   int            `json:"users"`
	UserIDs []uint         `json:"user_ids"`
	Reasons map[string]int `json:"reasons"`
}

// EntryRef names a node entry.
type EntryRef struct {
	ID     uint   `json:"id"`
	NodeID uint   `json:"node_id"`
	Label  string `json:"label"`
	Host   string `json:"host"`
}

// Overview is the policy and every pool that has entries or subscribers.
type Overview struct {
	Policy Policy     `json:"policy"`
	Pools  []PoolView `json:"pools"`
}

// members answers the users of each pool (ids capped by MaxListedUsers)
// and why they are in it.
func (s *Service) members(ctx context.Context) (map[string]*PoolView, error) {
	db := s.DB.WithContext(ctx)
	policy, err := loadPolicy(db)
	if err != nil {
		return nil, err
	}
	assigned := map[uint]string{}
	if db.Migrator().HasTable(&model.UserEntryPool{}) {
		var rows []model.UserEntryPool
		if err := db.Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			assigned[row.UserID] = row.Pool
		}
	}
	pools := map[string]*PoolView{}
	now := s.now()
	var batch []userFacts
	err = db.Model(&model.User{}).Select("id", "plan_id", "created_at").Order("id ASC").
		FindInBatches(&batch, 2000, func(*gorm.DB, int) error {
			for _, user := range batch {
				name, ok := assigned[user.ID]
				pool, reason := poolOf(policy, user, name, ok, now)
				view := pools[pool]
				if view == nil {
					view = &PoolView{Pool: pool, Entries: []EntryRef{}, UserIDs: []uint{}, Reasons: map[string]int{}}
					pools[pool] = view
				}
				view.Users++
				view.Reasons[reason]++
				if len(view.UserIDs) < MaxListedUsers {
					view.UserIDs = append(view.UserIDs, user.ID)
				}
			}
			return nil
		}).Error
	if err != nil {
		return nil, err
	}
	return pools, nil
}

// Overview answers the policy and the pools.
func (s *Service) Overview(ctx context.Context) (*Overview, error) {
	policy, err := s.GetPolicy(ctx)
	if err != nil {
		return nil, err
	}
	pools, err := s.members(ctx)
	if err != nil {
		return nil, err
	}
	var entries []model.NodeEntry
	if err := s.DB.WithContext(ctx).Order("node_id ASC, sort ASC, id ASC").Find(&entries).Error; err != nil {
		return nil, err
	}
	for _, entry := range entries {
		view := pools[entry.Pool]
		if view == nil {
			view = &PoolView{Pool: entry.Pool, Entries: []EntryRef{}, UserIDs: []uint{}, Reasons: map[string]int{}}
			pools[entry.Pool] = view
		}
		view.Entries = append(view.Entries, EntryRef{ID: entry.ID, NodeID: entry.NodeID, Label: entry.Label, Host: entry.Host})
	}
	out := &Overview{Policy: policy, Pools: []PoolView{}}
	for _, view := range pools {
		out.Pools = append(out.Pools, *view)
	}
	sort.Slice(out.Pools, func(i, j int) bool { return out.Pools[i].Pool < out.Pools[j].Pool })
	return out, nil
}

// LeakInput names a leaked address: an entry id, or a host (and port,
// unused) seen in a leaked subscription or probed.
type LeakInput struct {
	EntryID uint   `json:"entry_id"`
	Host    string `json:"host"`
}

// LeakMatch is an entry that matches, with the subscribers who were given it.
type LeakMatch struct {
	Entry EntryRef `json:"entry"`
	Pool  string   `json:"pool"`
	// Shared: the entry has no pool; every subscriber without an entry of
	// their own pool on that node got it.
	Shared  bool   `json:"shared"`
	Users   int    `json:"users"`
	UserIDs []uint `json:"user_ids"`
}

// Leak answers who was given a leaked address.
func (s *Service) Leak(ctx context.Context, input LeakInput) ([]LeakMatch, error) {
	db := s.DB.WithContext(ctx)
	query := db.Order("id ASC")
	host := strings.ToLower(strings.Trim(strings.TrimSpace(input.Host), "[]"))
	switch {
	case input.EntryID != 0:
		query = query.Where("id = ?", input.EntryID)
	case host != "":
		query = query.Where("LOWER(host) = ?", host)
	default:
		return nil, invalid("give entry_id or host")
	}
	var entries []model.NodeEntry
	if err := query.Find(&entries).Error; err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, ErrNotFound
	}
	pools, err := s.members(ctx)
	if err != nil {
		return nil, err
	}
	out := []LeakMatch{}
	for _, entry := range entries {
		match := LeakMatch{Entry: EntryRef{ID: entry.ID, NodeID: entry.NodeID, Label: entry.Label, Host: entry.Host},
			Pool: entry.Pool, Shared: entry.Pool == "", UserIDs: []uint{}}
		if view := pools[entry.Pool]; view != nil {
			match.Users, match.UserIDs = view.Users, view.UserIDs
		}
		out = append(out, match)
	}
	return out, nil
}

// SplitInput halves the subscribers of Pool into Into[0] and Into[1].
type SplitInput struct {
	Pool string    `json:"pool"`
	Into [2]string `json:"into"`
}

// SplitResult is how many subscribers went to each half.
type SplitResult struct {
	Pool string         `json:"pool"`
	Into map[string]int `json:"into"`
}

// Split assigns the subscribers of a pool alternately to two new pools, to
// bisect a leak: give each half its own entry, see which one leaks, split
// it again. Audited.
func (s *Service) Split(ctx context.Context, input SplitInput, actor Actor) (*SplitResult, error) {
	pool := strings.TrimSpace(input.Pool)
	a, b := strings.TrimSpace(input.Into[0]), strings.TrimSpace(input.Into[1])
	if pool == "" || a == "" || b == "" || a == b || a == pool || b == pool {
		return nil, invalid("give a pool and two other, different pools to split it into")
	}
	for _, name := range []string{pool, a, b} {
		if err := CheckPool(name); err != nil {
			return nil, err
		}
	}
	db := s.DB.WithContext(ctx)
	policy, err := loadPolicy(db)
	if err != nil {
		return nil, err
	}
	assigned := map[uint]string{}
	var rows []model.UserEntryPool
	if err := db.Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		assigned[row.UserID] = row.Pool
	}
	now := s.now()
	var users []userFacts
	if err := db.Model(&model.User{}).Select("id", "plan_id", "created_at").Order("id ASC").Find(&users).Error; err != nil {
		return nil, err
	}
	var members []uint
	for _, user := range users {
		name, ok := assigned[user.ID]
		if got, _ := poolOf(policy, user, name, ok, now); got == pool {
			members = append(members, user.ID)
		}
	}
	if len(members) < 2 {
		return nil, invalid("pool %q has %d subscriber(s); nothing to split", pool, len(members))
	}
	result := &SplitResult{Pool: pool, Into: map[string]int{a: 0, b: 0}}
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id IN ?", members).Delete(&model.UserEntryPool{}).Error; err != nil {
			return err
		}
		out := make([]model.UserEntryPool, 0, len(members))
		for i, id := range members {
			half := a
			if i%2 == 1 {
				half = b
			}
			result.Into[half]++
			out = append(out, model.UserEntryPool{UserID: id, Pool: half, Reason: truncate("split of "+pool, 128), UpdatedAt: now})
		}
		if err := tx.CreateInBatches(out, 500).Error; err != nil {
			return err
		}
		return writeAudit(tx, actor, AuditActionSplit, map[string]any{"pool": pool, "into": result.Into})
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func dedupe(ids []uint) []uint {
	seen := map[uint]bool{}
	out := make([]uint, 0, len(ids))
	for _, id := range ids {
		if id != 0 && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func writeAudit(tx *gorm.DB, actor Actor, action string, content map[string]any) error {
	encoded, err := json.Marshal(content)
	if err != nil {
		return err
	}
	var userID *uint
	if actor.UserID != 0 {
		id := actor.UserID
		userID = &id
	}
	return tx.Create(&model.OperationLog{
		UserID: userID, Username: truncate(actor.Name, 100), Action: action, Module: auditModule,
		TargetType: auditTarget, Content: string(encoded), IP: truncate(actor.IP, 45), Status: 1,
	}).Error
}

func truncate(value string, limit int) string {
	if len(value) > limit {
		return value[:limit]
	}
	return value
}
