// Package iwanline is the Panabit iWAN lines subscribers log into: a
// third-party iWAN ingress (a real Panabit device, say) that carries their
// traffic over the line, an IPLC for example, to its exit. Control runs
// nothing on a line; it only hands AnixOps Net the line's address and its
// one shared account. A proxy node is offered through a line by an iWAN
// node entry (internal/nodeentry): the client logs into the line and
// reaches the node through it, so the node's own protocol encrypts the
// whole path (iWAN itself does not).
package iwanline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/nodeentry"
	"gorm.io/gorm"
)

var (
	// ErrNotFound: no line with that id.
	ErrNotFound = errors.New("iwan line not found")
	// ErrInvalid: the line is invalid; the error says why.
	ErrInvalid = errors.New("invalid iwan line")
	// ErrInUse: node entries still reach their nodes through the line.
	ErrInUse = errors.New("iwan line in use")
)

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

// Limits of a line.
const (
	MaxLines          = 64
	maxNameLength     = 64
	maxLoginLength    = 128
	MinMTU            = 576
	MaxMTU            = 1600
	auditModule       = "iwan_line"
	auditTarget       = "iwan_line"
	AuditActionCreate = "iwan_line_create"
	AuditActionUpdate = "iwan_line_update"
	AuditActionDelete = "iwan_line_delete"
)

// Input is a line as an administrator writes it. An empty password on an
// update keeps the stored one.
type Input struct {
	Name     string `json:"name"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	MTU      int    `json:"mtu"`
	Encrypt  bool   `json:"encrypt"`
}

// View is a line as the API answers it: never the password.
type View struct {
	ID       uint   `json:"id"`
	Name     string `json:"name"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	MTU      int    `json:"mtu"`
	Encrypt  bool   `json:"encrypt"`
	// Entries is how many node entries reach their nodes through the line.
	Entries int `json:"entries"`
}

// Actor is who made an administrator's change, for the audit entry.
type Actor struct {
	UserID uint
	Name   string
	IP     string
}

// Service manages the lines on one database.
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

// List answers every line by name.
func (s *Service) List(ctx context.Context) ([]View, error) {
	var rows []model.IWANLine
	if err := s.DB.WithContext(ctx).Order("name ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	var counts []struct {
		IWANLineID uint
		Count      int
	}
	if err := s.DB.WithContext(ctx).Model(&model.NodeEntry{}).
		Select("iwan_line_id, count(*) AS count").
		Where("iwan_line_id IS NOT NULL").
		Group("iwan_line_id").Scan(&counts).Error; err != nil {
		return nil, err
	}
	used := map[uint]int{}
	for _, c := range counts {
		used[c.IWANLineID] = c.Count
	}
	views := make([]View, 0, len(rows))
	for _, row := range rows {
		views = append(views, viewOf(row, used[row.ID]))
	}
	return views, nil
}

// Create adds a line, audited.
func (s *Service) Create(ctx context.Context, input Input, actor Actor) (View, error) {
	row, err := validate(input, true)
	if err != nil {
		return View{}, err
	}
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&model.IWANLine{}).Count(&count).Error; err != nil {
			return err
		}
		if count >= MaxLines {
			return invalid("at most %d lines", MaxLines)
		}
		if err := requireUniqueName(tx, row.Name, 0); err != nil {
			return err
		}
		row.CreatedAt = s.now()
		row.UpdatedAt = row.CreatedAt
		// A false Encrypt gets the column's default, which is false too.
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		return writeAudit(tx, actor, AuditActionCreate, row)
	})
	if err != nil {
		return View{}, err
	}
	return s.view(ctx, row.ID)
}

// Update changes a line, audited. An empty password keeps the stored one.
func (s *Service) Update(ctx context.Context, id uint, input Input, actor Actor) (View, error) {
	row, err := validate(input, false)
	if err != nil {
		return View{}, err
	}
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current model.IWANLine
		if err := tx.First(&current, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		if err := requireUniqueName(tx, row.Name, id); err != nil {
			return err
		}
		if row.Password == "" {
			row.Password = current.Password
		}
		row.ID = id
		row.CreatedAt = current.CreatedAt
		row.UpdatedAt = s.now()
		if err := tx.Model(&current).Select("*").Updates(&row).Error; err != nil {
			return err
		}
		return writeAudit(tx, actor, AuditActionUpdate, row)
	})
	if err != nil {
		return View{}, err
	}
	return s.view(ctx, id)
}

// Delete removes a line no node entry uses, audited.
func (s *Service) Delete(ctx context.Context, id uint, actor Actor) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current model.IWANLine
		if err := tx.First(&current, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		var used int64
		if err := tx.Model(&model.NodeEntry{}).Where("iwan_line_id = ?", id).Count(&used).Error; err != nil {
			return err
		}
		if used > 0 {
			return fmt.Errorf("%w: %d node entries reach their nodes through it; remove them first", ErrInUse, used)
		}
		if err := tx.Delete(&model.IWANLine{}, id).Error; err != nil {
			return err
		}
		return writeAudit(tx, actor, AuditActionDelete, current)
	})
}

func (s *Service) view(ctx context.Context, id uint) (View, error) {
	views, err := s.List(ctx)
	if err != nil {
		return View{}, err
	}
	for _, view := range views {
		if view.ID == id {
			return view, nil
		}
	}
	return View{}, ErrNotFound
}

// ByID answers the lines with the given ids, by id, passwords included, for
// subscriptions.
func ByID(db *gorm.DB, ids []uint) (map[uint]model.IWANLine, error) {
	lines := map[uint]model.IWANLine{}
	if len(ids) == 0 {
		return lines, nil
	}
	var rows []model.IWANLine
	if err := db.Where("id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		lines[row.ID] = row
	}
	return lines, nil
}

func requireUniqueName(tx *gorm.DB, name string, except uint) error {
	var count int64
	if err := tx.Model(&model.IWANLine{}).Where("name = ? AND id <> ?", name, except).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return invalid("the name %q is used by another line", name)
	}
	return nil
}

// The name is a subscription node name and what nodes behind the line name
// it by, so it has no | and no control characters.
var namePattern = regexp.MustCompile(`^[^\x00-\x1f\x7f|]+$`)

func validate(input Input, create bool) (model.IWANLine, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" || len(name) > maxNameLength || !namePattern.MatchString(name) {
		return model.IWANLine{}, invalid("the name must be 1 to %d characters, without | or control characters", maxNameLength)
	}
	host, err := nodeentry.NormalizeHost(input.Host)
	if err != nil {
		return model.IWANLine{}, invalid("%v", err)
	}
	if input.Port < 1 || input.Port > 65535 {
		return model.IWANLine{}, invalid("the port must be 1 to 65535")
	}
	username := strings.TrimSpace(input.Username)
	if username == "" || len(username) > maxLoginLength || strings.ContainsAny(username, "\x00\r\n") {
		return model.IWANLine{}, invalid("the username must be 1 to %d characters", maxLoginLength)
	}
	if len(input.Password) > maxLoginLength || strings.ContainsAny(input.Password, "\x00\r\n") {
		return model.IWANLine{}, invalid("the password must be at most %d characters", maxLoginLength)
	}
	if create && input.Password == "" {
		return model.IWANLine{}, invalid("the password is required")
	}
	if input.MTU != 0 && (input.MTU < MinMTU || input.MTU > MaxMTU) {
		return model.IWANLine{}, invalid("the MTU must be 0 (the client's default) or %d to %d", MinMTU, MaxMTU)
	}
	return model.IWANLine{
		Name: name, Host: host, Port: input.Port, Username: username, Password: input.Password,
		MTU: input.MTU, Encrypt: input.Encrypt,
	}, nil
}

func viewOf(row model.IWANLine, entries int) View {
	return View{
		ID: row.ID, Name: row.Name, Host: row.Host, Port: row.Port, Username: row.Username,
		MTU: row.MTU, Encrypt: row.Encrypt, Entries: entries,
	}
}

// writeAudit records the change without the login.
func writeAudit(tx *gorm.DB, actor Actor, action string, row model.IWANLine) error {
	encoded, err := json.Marshal(map[string]any{"name": row.Name, "family": nodeentry.Family(row.Host), "port": row.Port, "mtu": row.MTU, "encrypt": row.Encrypt})
	if err != nil {
		return err
	}
	var userID *uint
	if actor.UserID != 0 {
		id := actor.UserID
		userID = &id
	}
	target := row.ID
	return tx.Create(&model.OperationLog{
		UserID: userID, Username: truncate(actor.Name, 100), Action: action, Module: auditModule,
		TargetType: auditTarget, TargetID: &target, Content: string(encoded), IP: truncate(actor.IP, 45), Status: 1,
	}).Error
}

func truncate(value string, limit int) string {
	if len(value) > limit {
		return value[:limit]
	}
	return value
}
