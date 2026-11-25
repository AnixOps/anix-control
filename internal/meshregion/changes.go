package meshregion

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

// MaxTextLength bounds a change's reason and note, in characters.
const MaxTextLength = 500

// ChangeView is a region change with the member and region names.
type ChangeView struct {
	model.MeshRegionChange
	Hostname string `json:"hostname"`
	FromCode string `json:"from_code"`
	ToCode   string `json:"to_code"`
}

func checkText(field, text string) (string, error) {
	text = strings.TrimSpace(text)
	if utf8.RuneCountInString(text) > MaxTextLength {
		return "", invalid("%s: at most %d characters", field, MaxTextLength)
	}
	return text, nil
}

// RequestChange records a member's request to move to another region
// (R24). A member has at most one pending request. requestedBy is the user
// who asked on the member's behalf; nil for the member itself.
func (s *Service) RequestChange(memberID, toRegionID uint, reason string, requestedBy *uint) (model.MeshRegionChange, error) {
	reason, err := checkText("reason", reason)
	if err != nil {
		return model.MeshRegionChange{}, err
	}
	var change model.MeshRegionChange
	err = s.transaction(func(tx *gorm.DB) error {
		member, err := memberByID(tx, memberID)
		if err != nil {
			return err
		}
		if member.RegionID == nil {
			return conflict("the member has not entered a region yet; its first region is picked automatically")
		}
		if *member.RegionID == toRegionID {
			return invalid("to_region_id: the member is already in this region")
		}
		if _, err := regionByID(tx, toRegionID); err != nil {
			return err
		}
		var pending int64
		if err := tx.Model(&model.MeshRegionChange{}).Where("member_id = ? AND status = ?", memberID, model.MeshChangePending).Count(&pending).Error; err != nil {
			return err
		}
		if pending > 0 {
			return conflict("the member has a pending region change")
		}
		change = model.MeshRegionChange{
			MemberID: memberID, FromRegionID: *member.RegionID, ToRegionID: toRegionID, Source: model.MeshChangeByMember,
			Status: model.MeshChangePending, Reason: reason, RequestedBy: requestedBy, CreatedAt: s.now(),
		}
		return tx.Create(&change).Error
	})
	return change, err
}

// Changes answers the region changes, newest first; status "" for all.
func (s *Service) Changes(status string) ([]ChangeView, error) {
	switch status {
	case "", model.MeshChangePending, model.MeshChangeApproved, model.MeshChangeRejected:
	default:
		return nil, invalid("status: pending, approved or rejected")
	}
	var views []ChangeView
	err := s.transaction(func(tx *gorm.DB) error {
		query := tx.Order("created_at DESC, id DESC").Limit(500)
		if status != "" {
			query = query.Where("status = ?", status)
		}
		var changes []model.MeshRegionChange
		if err := query.Find(&changes).Error; err != nil {
			return err
		}
		var regions []model.MeshRegion
		if err := tx.Find(&regions).Error; err != nil {
			return err
		}
		codes := map[uint]string{}
		for _, region := range regions {
			codes[region.ID] = region.Code
		}
		var members []model.MeshMember
		if err := tx.Select("id", "hostname").Find(&members).Error; err != nil {
			return err
		}
		names := map[uint]string{}
		for _, member := range members {
			names[member.ID] = member.Hostname
		}
		views = make([]ChangeView, 0, len(changes))
		for _, change := range changes {
			views = append(views, ChangeView{
				MeshRegionChange: change, Hostname: names[change.MemberID],
				FromCode: codes[change.FromRegionID], ToCode: codes[change.ToRegionID],
			})
		}
		return nil
	})
	return views, err
}

// moveMember moves member to region to with a new address (requested for a
// fixed member, or picked) and answers the old and new addresses. A host
// group's gateway moves only together with the group's other gateway.
func moveMember(tx *gorm.DB, member *model.MeshMember, to model.MeshRegion, requested string, now time.Time) (oldAddress, newAddress string, err error) {
	if member.RegionID == nil {
		return "", "", conflict("the member has not entered a region yet")
	}
	if *member.RegionID == to.ID {
		return "", "", invalid("to_region_id: the member is already in this region")
	}
	if err := checkNameFree(tx, to.ID, member.Hostname, member.ID, 0); err != nil {
		return "", "", err
	}
	// A gateway's published names move with it.
	var published []model.MeshDeviceName
	if err := tx.Where("member_id = ?", member.ID).Find(&published).Error; err != nil {
		return "", "", err
	}
	for _, name := range published {
		if err := checkNameFree(tx, to.ID, name.Name, member.ID, name.ID); err != nil {
			return "", "", err
		}
	}
	var groups []model.MeshHostGroup
	if err := tx.Where("gateway_member_id = ? OR standby_member_id = ?", member.ID, member.ID).Find(&groups).Error; err != nil {
		return "", "", err
	}
	for _, group := range groups {
		other := group.GatewayMemberID
		if other == member.ID {
			other = 0
			if group.StandbyMemberID != nil {
				other = *group.StandbyMemberID
			}
		}
		if other == 0 {
			continue
		}
		peer, err := memberByID(tx, other)
		if err != nil {
			return "", "", err
		}
		if peer.RegionID == nil || *peer.RegionID != to.ID {
			return "", "", conflict("host group %s: its other gateway %s stays in the old region; move it first or remove it from the group", group.Name, peer.Hostname)
		}
	}
	if member.Address != nil {
		oldAddress = *member.Address
	}
	member.RegionID = &to.ID
	member.Address = nil
	if err := placeAddress(tx, to, member, requested, now); err != nil {
		return "", "", err
	}
	member.UpdatedAt = now
	if err := tx.Save(member).Error; err != nil {
		return "", "", err
	}
	return oldAddress, *member.Address, nil
}

// Decision is an administrator's decision on a change.
type Decision struct {
	// Address is the new fixed address; empty picks one.
	Address string
	Note    string
	// DecidedBy is the administrator; nil for an API token without a user.
	DecidedBy *uint
}

func pendingChange(tx *gorm.DB, id uint) (model.MeshRegionChange, error) {
	var change model.MeshRegionChange
	if err := tx.First(&change, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return change, notFound("region change")
		}
		return change, err
	}
	if change.Status != model.MeshChangePending {
		return change, conflict("the region change was already %s", change.Status)
	}
	return change, nil
}

// Approve moves the member of a pending change to the region it asked for.
func (s *Service) Approve(id uint, decision Decision) (model.MeshRegionChange, error) {
	note, err := checkText("note", decision.Note)
	if err != nil {
		return model.MeshRegionChange{}, err
	}
	var change model.MeshRegionChange
	err = s.transaction(func(tx *gorm.DB) error {
		if change, err = pendingChange(tx, id); err != nil {
			return err
		}
		member, err := memberByID(tx, change.MemberID)
		if err != nil {
			return err
		}
		if member.RegionID == nil || *member.RegionID != change.FromRegionID {
			return conflict("the member left the region it asked from; reject the change")
		}
		to, err := regionByID(tx, change.ToRegionID)
		if err != nil {
			return err
		}
		now := s.now()
		change.OldAddress, change.NewAddress, err = moveMember(tx, &member, to, decision.Address, now)
		if err != nil {
			return err
		}
		change.Status, change.Note, change.DecidedBy, change.DecidedAt = model.MeshChangeApproved, note, decision.DecidedBy, &now
		return tx.Save(&change).Error
	})
	return change, err
}

// Reject closes a pending change; the member stays where it is.
func (s *Service) Reject(id uint, decision Decision) (model.MeshRegionChange, error) {
	note, err := checkText("note", decision.Note)
	if err != nil {
		return model.MeshRegionChange{}, err
	}
	var change model.MeshRegionChange
	err = s.transaction(func(tx *gorm.DB) error {
		if change, err = pendingChange(tx, id); err != nil {
			return err
		}
		now := s.now()
		change.Status, change.Note, change.DecidedBy, change.DecidedAt = model.MeshChangeRejected, note, decision.DecidedBy, &now
		return tx.Save(&change).Error
	})
	return change, err
}

// Move is an administrator's own move of a member to another region,
// recorded as an approved change. A pending request of the member for the
// same region is approved by it (and answered); any other pending request
// is rejected.
func (s *Service) Move(memberID, toRegionID uint, decision Decision) (model.MeshRegionChange, error) {
	note, err := checkText("note", decision.Note)
	if err != nil {
		return model.MeshRegionChange{}, err
	}
	var change model.MeshRegionChange
	err = s.transaction(func(tx *gorm.DB) error {
		member, err := memberByID(tx, memberID)
		if err != nil {
			return err
		}
		to, err := regionByID(tx, toRegionID)
		if err != nil {
			return err
		}
		var from uint
		if member.RegionID != nil {
			from = *member.RegionID
		}
		now := s.now()
		oldAddress, newAddress, err := moveMember(tx, &member, to, decision.Address, now)
		if err != nil {
			return err
		}
		var pending []model.MeshRegionChange
		if err := tx.Where("member_id = ? AND status = ?", memberID, model.MeshChangePending).Find(&pending).Error; err != nil {
			return err
		}
		answered := false
		for _, open := range pending {
			open.DecidedBy, open.DecidedAt = decision.DecidedBy, &now
			if open.ToRegionID == toRegionID && !answered {
				// The move grants the member's own request.
				answered = true
				open.Status, open.Note, open.OldAddress, open.NewAddress = model.MeshChangeApproved, note, oldAddress, newAddress
				change = open
			} else {
				open.Status = model.MeshChangeRejected
			}
			if err := tx.Save(&open).Error; err != nil {
				return err
			}
		}
		if answered {
			return nil
		}
		change = model.MeshRegionChange{
			MemberID: memberID, FromRegionID: from, ToRegionID: toRegionID, Source: model.MeshChangeByAdmin,
			Status: model.MeshChangeApproved, Note: note, OldAddress: oldAddress, NewAddress: newAddress,
			RequestedBy: decision.DecidedBy, DecidedBy: decision.DecidedBy, DecidedAt: &now, CreatedAt: now,
		}
		return tx.Create(&change).Error
	})
	return change, err
}
