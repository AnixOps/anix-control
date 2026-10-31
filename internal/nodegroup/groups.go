package nodegroup

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Group field limits.
const (
	maxGroupName = 100
	// maxNodeNameRunes caps the name of a node a join creates.
	maxNodeNameRunes = 64
	maxTags          = 16
	maxTagRunes      = 32
	maxRate          = 1000
	// maxTagsJSON is the width of v2_node.tags, which the group's tags are
	// copied to.
	maxTagsJSON = 255
)

// GroupInput is an administrator's node group: the body of a create, and of
// an update, which replaces every field.
type GroupInput struct {
	Name string `json:"name"`
	// MaxNodes caps the group's live nodes; 0 is no cap.
	MaxNodes        int  `json:"max_nodes"`
	RequireApproval bool `json:"require_approval"`
	// Rate is the billing rate of the group's proxy nodes; 1 when absent.
	Rate              *float64 `json:"rate"`
	Tags              []string `json:"tags"`
	MonthlyLimitBytes int64    `json:"monthly_limit_bytes"`
	// ResetDay is the day of the month (1-28) the nodes' monthly traffic
	// resets; 1 when absent.
	ResetDay int `json:"reset_day"`
	Sort     int `json:"sort"`
	// TokenExpiresAt, when set, ends the token's use; it must be in the
	// future.
	TokenExpiresAt *time.Time `json:"token_expires_at"`
}

// GroupView is a node group as the administrator API shows it: never the
// token.
type GroupView struct {
	ID                uint       `json:"id"`
	Name              string     `json:"name"`
	MaxNodes          int        `json:"max_nodes"`
	RequireApproval   bool       `json:"require_approval"`
	Rate              float64    `json:"rate"`
	Tags              []string   `json:"tags"`
	MonthlyLimitBytes int64      `json:"monthly_limit_bytes"`
	ResetDay          int        `json:"reset_day"`
	Sort              int        `json:"sort"`
	TokenResetAt      time.Time  `json:"token_reset_at"`
	TokenExpiresAt    *time.Time `json:"token_expires_at"`
	// TokenExpired is true once TokenExpiresAt has passed.
	TokenExpired bool       `json:"token_expired"`
	Nodes        NodeCounts `json:"nodes"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// NodeCounts counts a group's live nodes (rejected ones are not live) and
// those of them pending approval.
type NodeCounts struct {
	Live    int64 `json:"live"`
	Pending int64 `json:"pending"`
}

// checked is a validated GroupInput in its stored form.
type checked struct {
	name, tags               string
	maxNodes, resetDay, sort int
	requireApproval          bool
	rate                     float64
	monthlyLimit             int64
	expiresAt                *time.Time
}

// check validates input.
func (s *Service) check(input GroupInput) (checked, error) {
	var out checked
	out.name = strings.TrimSpace(input.Name)
	if out.name == "" || utf8.RuneCountInString(out.name) > maxGroupName || hasControl(out.name) {
		return out, invalid("name is required, at most %d characters, without control characters", maxGroupName)
	}
	if input.MaxNodes < 0 || input.MaxNodes > 1<<20 {
		return out, invalid("max_nodes must be 0 (no cap) or a positive number")
	}
	out.maxNodes, out.requireApproval, out.sort = input.MaxNodes, input.RequireApproval, input.Sort
	out.rate = 1
	if input.Rate != nil {
		out.rate = *input.Rate
	}
	if math.IsNaN(out.rate) || out.rate <= 0 || out.rate > maxRate {
		return out, invalid("rate must be above 0 and at most %d", maxRate)
	}
	if input.MonthlyLimitBytes < 0 {
		return out, invalid("monthly_limit_bytes must be 0 (no limit) or positive")
	}
	out.monthlyLimit = input.MonthlyLimitBytes
	out.resetDay = input.ResetDay
	if out.resetDay == 0 {
		out.resetDay = 1
	}
	if out.resetDay < 1 || out.resetDay > 28 {
		return out, invalid("reset_day must be within 1-28")
	}
	tags, err := checkTags(input.Tags)
	if err != nil {
		return out, err
	}
	out.tags = tags
	if input.TokenExpiresAt != nil {
		expires := input.TokenExpiresAt.UTC()
		if !expires.After(s.now()) {
			return out, invalid("token_expires_at must be in the future")
		}
		out.expiresAt = &expires
	}
	return out, nil
}

func checkTags(tags []string) (string, error) {
	if len(tags) > maxTags {
		return "", invalid("at most %d tags", maxTags)
	}
	clean := make([]string, 0, len(tags))
	seen := map[string]bool{}
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" || utf8.RuneCountInString(tag) > maxTagRunes || hasControl(tag) {
			return "", invalid("a tag is 1-%d characters without control characters", maxTagRunes)
		}
		if !seen[tag] {
			seen[tag] = true
			clean = append(clean, tag)
		}
	}
	if len(clean) == 0 {
		return "", nil
	}
	encoded, err := json.Marshal(clean)
	if err != nil {
		return "", err
	}
	if len(encoded) > maxTagsJSON {
		return "", invalid("the tags are too long together (at most %d bytes as JSON)", maxTagsJSON)
	}
	return string(encoded), nil
}

// nameTaken tells whether a live group other than id has name.
func nameTaken(tx *gorm.DB, name string, id uint) (bool, error) {
	var count int64
	query := tx.Model(&model.NodeGroup{}).Where("name = ?", name)
	if id != 0 {
		query = query.Where("id <> ?", id)
	}
	if err := query.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// Create stores a new node group with a new join token. It needs the
// key-encryption key that seals the token (ErrNoKEK).
func (s *Service) Create(ctx context.Context, input GroupInput, actor Actor) (GroupView, error) {
	sealer, err := s.sealer()
	if err != nil {
		return GroupView{}, err
	}
	var group model.NodeGroup
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		fields, err := s.check(input)
		if err != nil {
			return err
		}
		if taken, err := nameTaken(tx, fields.name, 0); err != nil {
			return err
		} else if taken {
			return ErrNameTaken
		}
		_, hash, sealed, err := newSealedToken(sealer)
		if err != nil {
			return err
		}
		now := s.now()
		group = model.NodeGroup{
			Name: fields.name, JoinTokenHash: hash, JoinTokenSealed: sealed, TokenResetAt: now,
			CreatedAt: now, UpdatedAt: now,
		}
		fields.apply(&group)
		if err := tx.Create(&group).Error; err != nil {
			return err
		}
		return writeAudit(tx, actor, AuditActionCreate, auditTargetGroup, group.ID, map[string]any{
			"group": group.Name, "require_approval": group.RequireApproval, "max_nodes": group.MaxNodes,
		})
	})
	if err != nil {
		return GroupView{}, err
	}
	return s.Get(ctx, group.ID)
}

func (c checked) apply(group *model.NodeGroup) {
	group.Name, group.MaxNodes, group.RequireApproval, group.Rate = c.name, c.maxNodes, c.requireApproval, c.rate
	group.Tags, group.MonthlyLimitBytes, group.ResetDay, group.Sort = c.tags, c.monthlyLimit, c.resetDay, c.sort
	group.TokenExpiresAt = c.expiresAt
}

// Update replaces a group's fields but its token. Nodes that
// joined keep what they got: changed defaults apply to later joins.
func (s *Service) Update(ctx context.Context, id uint, input GroupInput, actor Actor) (GroupView, error) {
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		group, err := lockGroup(tx, id)
		if err != nil {
			return err
		}
		fields, err := s.check(input)
		if err != nil {
			return err
		}
		if taken, err := nameTaken(tx, fields.name, id); err != nil {
			return err
		} else if taken {
			return ErrNameTaken
		}
		fields.apply(&group)
		group.UpdatedAt = s.now()
		if err := tx.Model(&model.NodeGroup{}).Where("id = ?", id).Select(
			"name", "max_nodes", "require_approval", "rate", "tags", "monthly_limit_bytes", "reset_day", "sort",
			"token_expires_at", "updated_at",
		).Updates(&group).Error; err != nil {
			return err
		}
		return writeAudit(tx, actor, AuditActionUpdate, auditTargetGroup, id, map[string]any{
			"group": group.Name, "require_approval": group.RequireApproval, "max_nodes": group.MaxNodes,
		})
	})
	if err != nil {
		return GroupView{}, err
	}
	return s.Get(ctx, id)
}

// Delete soft-deletes a group without live nodes (ErrInUse otherwise). Its
// token joins nothing from then on.
func (s *Service) Delete(ctx context.Context, id uint, actor Actor) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		group, err := lockGroup(tx, id)
		if err != nil {
			return err
		}
		counts, err := countNodes(tx, group)
		if err != nil {
			return err
		}
		if counts.Live > 0 {
			return ErrInUse
		}
		if err := tx.Delete(&group).Error; err != nil {
			return err
		}
		return writeAudit(tx, actor, AuditActionDelete, auditTargetGroup, id, map[string]any{"group": group.Name})
	})
}

// ResetToken replaces a group's join token: every earlier install command
// stops working. Nodes that joined keep their certificates.
func (s *Service) ResetToken(ctx context.Context, id uint, actor Actor) (GroupView, error) {
	sealer, err := s.sealer()
	if err != nil {
		return GroupView{}, err
	}
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		group, err := lockGroup(tx, id)
		if err != nil {
			return err
		}
		_, hash, sealed, err := newSealedToken(sealer)
		if err != nil {
			return err
		}
		now := s.now()
		if err := tx.Model(&model.NodeGroup{}).Where("id = ?", id).Updates(map[string]any{
			"join_token_hash": hash, "join_token_sealed": sealed, "token_reset_at": now, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		return writeAudit(tx, actor, AuditActionTokenReset, auditTargetGroup, id, map[string]any{
			"group": group.Name, "token_reset_at": now.Format(time.RFC3339),
		})
	})
	if err != nil {
		return GroupView{}, err
	}
	return s.Get(ctx, id)
}

// RevealedToken is a group's join token, opened for an administrator.
type RevealedToken struct {
	Group GroupView
	Token string
}

// RevealToken opens a group's sealed join token for the install command,
// and records the read in the operation log (the token is not in it).
func (s *Service) RevealToken(ctx context.Context, id uint, actor Actor) (RevealedToken, error) {
	sealer, err := s.sealer()
	if err != nil {
		return RevealedToken{}, err
	}
	var token string
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var group model.NodeGroup
		if err := tx.Where("id = ?", id).Take(&group).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		token, err = sealer.open(group.JoinTokenHash, group.JoinTokenSealed)
		if err != nil {
			return err
		}
		return writeAudit(tx, actor, AuditActionTokenReveal, auditTargetGroup, id, map[string]any{
			"group": group.Name, "token_reset_at": group.TokenResetAt.UTC().Format(time.RFC3339),
		})
	})
	if err != nil {
		return RevealedToken{}, err
	}
	view, err := s.Get(ctx, id)
	if err != nil {
		return RevealedToken{}, err
	}
	return RevealedToken{Group: view, Token: token}, nil
}

// Get returns a live group.
func (s *Service) Get(ctx context.Context, id uint) (GroupView, error) {
	db := s.DB.WithContext(ctx)
	var group model.NodeGroup
	if err := db.Where("id = ?", id).Take(&group).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return GroupView{}, ErrNotFound
		}
		return GroupView{}, err
	}
	counts, err := countNodes(db, group)
	if err != nil {
		return GroupView{}, err
	}
	return s.view(group, counts), nil
}

// List returns the live groups by sort, then id.
func (s *Service) List(ctx context.Context) ([]GroupView, error) {
	db := s.DB.WithContext(ctx)
	var groups []model.NodeGroup
	if err := db.Order("sort").Order("id").Find(&groups).Error; err != nil {
		return nil, err
	}
	views := make([]GroupView, 0, len(groups))
	for _, group := range groups {
		counts, err := countNodes(db, group)
		if err != nil {
			return nil, err
		}
		views = append(views, s.view(group, counts))
	}
	return views, nil
}

func (s *Service) view(group model.NodeGroup, counts NodeCounts) GroupView {
	view := GroupView{
		ID: group.ID, Name: group.Name, MaxNodes: group.MaxNodes, RequireApproval: group.RequireApproval,
		Rate: group.Rate, Tags: []string{}, MonthlyLimitBytes: group.MonthlyLimitBytes, ResetDay: group.ResetDay, Sort: group.Sort,
		TokenResetAt: group.TokenResetAt, TokenExpiresAt: group.TokenExpiresAt, Nodes: counts,
		CreatedAt: group.CreatedAt, UpdatedAt: group.UpdatedAt,
	}
	if group.Tags != "" {
		_ = json.Unmarshal([]byte(group.Tags), &view.Tags)
	}
	view.TokenExpired = group.TokenExpiresAt != nil && !group.TokenExpiresAt.After(s.now())
	return view
}

// lockGroup loads a live group for update.
func lockGroup(tx *gorm.DB, id uint) (model.NodeGroup, error) {
	var group model.NodeGroup
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", id).Take(&group).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.NodeGroup{}, ErrNotFound
		}
		return model.NodeGroup{}, err
	}
	return group, nil
}

// liveNodes is the query of a group's live regular nodes: every node of the
// group but the rejected ones.
func liveNodes(tx *gorm.DB, group model.NodeGroup) *gorm.DB {
	return tx.Model(&model.Node{}).Where("node_group_id = ? AND approval <> ?", group.ID, model.NodeApprovalRejected)
}

func countNodes(tx *gorm.DB, group model.NodeGroup) (NodeCounts, error) {
	var counts NodeCounts
	if err := liveNodes(tx, group).Count(&counts.Live).Error; err != nil {
		return NodeCounts{}, err
	}
	if err := liveNodes(tx, group).Where("approval = ?", model.NodeApprovalPending).Count(&counts.Pending).Error; err != nil {
		return NodeCounts{}, err
	}
	return counts, nil
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func hasControl(value string) bool {
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}
