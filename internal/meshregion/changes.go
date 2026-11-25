package meshregion

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ChangeInput asks to move a node (Node, proxy-<id> or forward-<id>) or a
// device (DeviceID) to the region ToRegionID (R20, R24).
type ChangeInput struct {
	Node       string `json:"node"`
	DeviceID   uint   `json:"device_id"`
	ToRegionID uint   `json:"to_region_id"`
	Reason     string `json:"reason"`
}

// ChangeView is a region change with its regions' codes.
type ChangeView struct {
	model.MeshRegionChange
	FromRegionCode string `json:"from_region_code"`
	ToRegionCode   string `json:"to_region_code"`
}

// maxChanges is how many changes a listing answers.
const maxChanges = 500

func checkText(field, raw string) (string, error) {
	text := strings.TrimSpace(raw)
	if utf8.RuneCountInString(text) > 500 {
		return "", invalid("%s: at most 500 characters", field)
	}
	return text, nil
}

// subject answers the current region and address of the change's node or
// device.
func subject(tx *gorm.DB, nodeRef *string, deviceID *uint) (uint, string, error) {
	if nodeRef != nil {
		home, err := homeOf(tx, *nodeRef)
		if err != nil {
			return 0, "", err
		}
		return home.RegionID, home.Address, nil
	}
	var device model.MeshDevice
	if err := findDevice(tx, *deviceID, &device); err != nil {
		return 0, "", err
	}
	address := ""
	if device.Address != nil {
		address = *device.Address
	}
	return device.RegionID, address, nil
}

// RequestChange records a pending region change; a node or device has at
// most one. source is model.MeshChangeMember or model.MeshChangeAdmin.
func RequestChange(ctx context.Context, db *gorm.DB, input ChangeInput, source string, requestedBy *uint, now time.Time) (model.MeshRegionChange, error) {
	reason, err := checkText("reason", input.Reason)
	if err != nil {
		return model.MeshRegionChange{}, err
	}
	row := model.MeshRegionChange{ToRegionID: input.ToRegionID, Source: source, Status: model.MeshChangePending, Reason: reason, RequestedBy: requestedBy, CreatedAt: now}
	node := strings.TrimSpace(input.Node)
	switch {
	case node != "" && input.DeviceID == 0:
		row.NodeRef = &node
	case node == "" && input.DeviceID != 0:
		id := input.DeviceID
		row.DeviceID = &id
	default:
		return model.MeshRegionChange{}, invalid("node or device_id: one of them")
	}
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockSubject(tx, row.NodeRef, row.DeviceID); err != nil {
			return err
		}
		from, address, err := subject(tx, row.NodeRef, row.DeviceID)
		if err != nil {
			return err
		}
		var region model.MeshRegion
		if err := findRegion(tx, input.ToRegionID, &region); err != nil {
			return err
		}
		if from == region.ID {
			return invalid("to_region_id: already the region")
		}
		if !region.Enabled {
			return invalid("to_region_id: the region is disabled")
		}
		var pending int64
		if err := pendingOf(tx, row.NodeRef, row.DeviceID).Count(&pending).Error; err != nil {
			return err
		}
		if pending > 0 {
			return conflict("a region change is already pending; decide it first")
		}
		row.FromRegionID, row.OldAddress = from, address
		return tx.Create(&row).Error
	})
	return row, err
}

// lockSubject locks the node's home membership or the device while a
// request is recorded, so that of two requests at once only one finds no
// pending change (PostgreSQL; SQLite writes one transaction at a time).
func lockSubject(tx *gorm.DB, nodeRef *string, deviceID *uint) error {
	if tx.Name() != "postgres" {
		return nil
	}
	locked := tx.Clauses(clause.Locking{Strength: "UPDATE"})
	if nodeRef != nil {
		return locked.Where("node_ref = ? AND home = ?", *nodeRef, true).Find(&[]model.MeshMember{}).Error
	}
	return locked.Where("id = ?", *deviceID).Find(&[]model.MeshDevice{}).Error
}

func pendingOf(tx *gorm.DB, nodeRef *string, deviceID *uint) *gorm.DB {
	query := tx.Model(&model.MeshRegionChange{}).Where("status = ?", model.MeshChangePending)
	if nodeRef != nil {
		return query.Where("node_ref = ?", *nodeRef)
	}
	return query.Where("device_id = ?", *deviceID)
}

// Changes answers the latest region changes, pending first; status filters
// them when not empty.
func Changes(ctx context.Context, db *gorm.DB, status string) ([]ChangeView, error) {
	db = db.WithContext(ctx)
	query := db.Order("CASE WHEN status = 'pending' THEN 0 ELSE 1 END, id DESC").Limit(maxChanges)
	switch status {
	case "":
	case model.MeshChangePending, model.MeshChangeApproved, model.MeshChangeRejected:
		query = query.Where("status = ?", status)
	default:
		return nil, invalid("status: pending, approved or rejected")
	}
	var rows []model.MeshRegionChange
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	codes, err := regionCodes(db)
	if err != nil {
		return nil, err
	}
	views := make([]ChangeView, 0, len(rows))
	for _, row := range rows {
		views = append(views, ChangeView{MeshRegionChange: row, FromRegionCode: codes[row.FromRegionID], ToRegionCode: codes[row.ToRegionID]})
	}
	return views, nil
}

// ApproveChange moves the change's node or device to the requested region,
// where it gets a new address, and records the decision. A change whose
// node or device has moved since is refused: reject it.
func ApproveChange(ctx context.Context, db *gorm.DB, id uint, note string, decidedBy *uint, now time.Time) (model.MeshRegionChange, error) {
	note, err := checkText("note", note)
	if err != nil {
		return model.MeshRegionChange{}, err
	}
	var row model.MeshRegionChange
	err = change(ctx, db, func(tx *gorm.DB) error {
		if err := findPending(tx, id, &row); err != nil {
			return err
		}
		from, _, err := subject(tx, row.NodeRef, row.DeviceID)
		if err != nil {
			return err
		}
		if from != row.FromRegionID {
			return conflict("the member is no longer in the region it asked to leave; reject the change")
		}
		var target model.MeshRegion
		if err := findRegion(tx, row.ToRegionID, &target); err != nil {
			return err
		}
		if !target.Enabled {
			return conflict("the region %q is disabled; enable it or reject the change", target.Code)
		}
		address, err := move(tx, row, now)
		if err != nil {
			return err
		}
		row.Status, row.Note, row.NewAddress, row.DecidedBy, row.DecidedAt = model.MeshChangeApproved, note, address, decidedBy, &now
		return decide(tx, row)
	})
	return row, err
}

// decide writes row's decision while the change is still pending: of two
// decisions at once, the second is refused.
func decide(tx *gorm.DB, row model.MeshRegionChange) error {
	result := tx.Model(&model.MeshRegionChange{}).Where("id = ? AND status = ?", row.ID, model.MeshChangePending).Updates(map[string]any{
		"status": row.Status, "note": row.Note, "new_address": row.NewAddress, "decided_by": row.DecidedBy, "decided_at": row.DecidedAt,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return conflict("the change was decided meanwhile")
	}
	return nil
}

// move carries out row and answers the new address.
func move(tx *gorm.DB, row model.MeshRegionChange, now time.Time) (string, error) {
	if row.NodeRef != nil {
		home, err := homeOf(tx, *row.NodeRef)
		if err != nil {
			return "", err
		}
		moved, err := moveNode(tx, home, row.ToRegionID, home.Boundary, now)
		if err != nil {
			return "", err
		}
		if home.Boundary {
			if err := joinOthers(tx, home.NodeRef, row.ToRegionID, now); err != nil {
				return "", err
			}
		}
		return moved.Address, nil
	}
	var device model.MeshDevice
	if err := findDevice(tx, *row.DeviceID, &device); err != nil {
		return "", err
	}
	var region model.MeshRegion
	if err := findRegion(tx, row.ToRegionID, &region); err != nil {
		return "", err
	}
	address, err := deviceAddress(tx, region, device.ID, now)
	if err != nil {
		return "", err
	}
	device.RegionID, device.Address, device.AddressUsedAt, device.UpdatedAt = region.ID, &address, &now, now
	return address, tx.Save(&device).Error
}

// RejectChange records a rejection; nothing moves.
func RejectChange(ctx context.Context, db *gorm.DB, id uint, note string, decidedBy *uint, now time.Time) (model.MeshRegionChange, error) {
	note, err := checkText("note", note)
	if err != nil {
		return model.MeshRegionChange{}, err
	}
	var row model.MeshRegionChange
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := findPending(tx, id, &row); err != nil {
			return err
		}
		row.Status, row.Note, row.DecidedBy, row.DecidedAt = model.MeshChangeRejected, note, decidedBy, &now
		return decide(tx, row)
	})
	return row, err
}

// MoveDevice moves a device to another region at once, recorded as an
// administrator's approved change.
func MoveDevice(ctx context.Context, db *gorm.DB, deviceID, toRegionID uint, decidedBy *uint, now time.Time) (model.MeshRegionChange, error) {
	var row model.MeshRegionChange
	err := change(ctx, db, func(tx *gorm.DB) error {
		var device model.MeshDevice
		if err := findDevice(tx, deviceID, &device); err != nil {
			return err
		}
		var region model.MeshRegion
		if err := findRegion(tx, toRegionID, &region); err != nil {
			return err
		}
		if device.RegionID == region.ID {
			return invalid("region_id: already the device's region")
		}
		id := device.ID
		row = model.MeshRegionChange{DeviceID: &id, FromRegionID: device.RegionID, ToRegionID: region.ID, Source: model.MeshChangeAdmin, RequestedBy: decidedBy, CreatedAt: now}
		if device.Address != nil {
			row.OldAddress = *device.Address
		}
		address, err := move(tx, row, now)
		if err != nil {
			return err
		}
		if err := settlePending(tx, nil, &id, region.ID, decidedBy, now); err != nil {
			return err
		}
		row.Status, row.NewAddress, row.DecidedBy, row.DecidedAt = model.MeshChangeApproved, address, decidedBy, &now
		return tx.Create(&row).Error
	})
	return row, err
}

// recordMove records a move an administrator made directly and settles the
// member's pending change: approved when it asked for that region,
// rejected otherwise. actor is the administrator.
func recordMove(tx *gorm.DB, nodeRef *string, deviceID *uint, from, to uint, oldAddress, newAddress string, actor *uint, now time.Time) error {
	if err := settlePending(tx, nodeRef, deviceID, to, actor, now); err != nil {
		return err
	}
	return tx.Create(&model.MeshRegionChange{
		NodeRef: nodeRef, DeviceID: deviceID, FromRegionID: from, ToRegionID: to, Source: model.MeshChangeAdmin, Status: model.MeshChangeApproved,
		OldAddress: oldAddress, NewAddress: newAddress, RequestedBy: actor, DecidedBy: actor, DecidedAt: &now, CreatedAt: now,
	}).Error
}

func settlePending(tx *gorm.DB, nodeRef *string, deviceID *uint, to uint, actor *uint, now time.Time) error {
	var pending []model.MeshRegionChange
	if err := pendingOf(tx, nodeRef, deviceID).Find(&pending).Error; err != nil {
		return err
	}
	for _, row := range pending {
		status, note := model.MeshChangeRejected, "an administrator moved the member to another region"
		if row.ToRegionID == to {
			status, note = model.MeshChangeApproved, "an administrator moved the member"
		}
		if err := tx.Model(&model.MeshRegionChange{}).Where("id = ? AND status = ?", row.ID, model.MeshChangePending).
			Updates(map[string]any{"status": status, "note": note, "decided_by": actor, "decided_at": now}).Error; err != nil {
			return err
		}
	}
	return nil
}

// rejectPendingOf rejects the pending change of a node leaving the mesh.
func rejectPendingOf(tx *gorm.DB, nodeRef, note string, now time.Time) error {
	return pendingOf(tx, &nodeRef, nil).Updates(map[string]any{"status": model.MeshChangeRejected, "note": note, "decided_at": now}).Error
}

func regionHasPendingChanges(tx *gorm.DB, id uint) error {
	var count int64
	if err := tx.Model(&model.MeshRegionChange{}).Where("status = ? AND to_region_id = ?", model.MeshChangePending, id).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return conflict("%d pending region changes ask for the region; decide them first", count)
	}
	return nil
}

func findPending(tx *gorm.DB, id uint, row *model.MeshRegionChange) error {
	var rows []model.MeshRegionChange
	if err := tx.Where("id = ?", id).Limit(1).Find(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return fmt.Errorf("%w: region change %d", ErrNotFound, id)
	}
	if rows[0].Status != model.MeshChangePending {
		return conflict("the region change is already %s", rows[0].Status)
	}
	*row = rows[0]
	return nil
}

// The node's own region change request: the package report
// "mesh.region_request" (agent-contract.md section 5), which the node's
// kernel sends when its owner asks for another region.
const (
	RegionRequestKind    = "mesh.region_request"
	RegionRequestVersion = "v1"
)

// RequestNodeChange records node's request, a "mesh.region_request"
// payload {"region": "<code>", "reason": "..."}, as a pending change of the
// source model.MeshChangeMember. A node asks only for itself.
func RequestNodeChange(ctx context.Context, db *gorm.DB, node agentcontrol.AgentNode, payload []byte, now time.Time) (model.MeshRegionChange, error) {
	var request struct {
		Region string `json:"region"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(payload, &request); err != nil || request.Region == "" {
		return model.MeshRegionChange{}, invalid("payload: a JSON object with a region code")
	}
	var regions []model.MeshRegion
	if err := db.WithContext(ctx).Where("code = ?", request.Region).Limit(1).Find(&regions).Error; err != nil {
		return model.MeshRegionChange{}, err
	}
	if len(regions) == 0 {
		return model.MeshRegionChange{}, fmt.Errorf("%w: region %q", ErrNotFound, request.Region)
	}
	return RequestChange(ctx, db, ChangeInput{Node: node.String(), ToRegionID: regions[0].ID, Reason: request.Reason}, model.MeshChangeMember, nil, now)
}
