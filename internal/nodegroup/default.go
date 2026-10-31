package nodegroup

import (
	"context"
	"errors"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

// The default node group (owner decision N22: the first node needs no
// input). The first time an administrator opens the join dialog of a panel
// without node groups, Control creates one. A node group only owns a join
// token and the defaults of the nodes it adds; the nodes join clean, with no
// protocol. Everything in it can be edited afterwards.
const (
	// DefaultGroupName is the default node group's name.
	DefaultGroupName = "默认"
)

// DefaultGroupInput is the default node group as a create request.
func DefaultGroupInput() GroupInput { return GroupInput{Name: DefaultGroupName} }

// EnsureDefault makes sure the panel has a node group to join machines to:
// when no live node group exists it creates the default one,
// otherwise it changes nothing and answers the first group by sort and id. created tells which.
// It is safe to call concurrently: of two racing creates one wins and the
// other answers the winner's group.
func (s *Service) EnsureDefault(ctx context.Context, actor Actor) (view GroupView, created bool, err error) {
	var existing int64
	if err := s.DB.WithContext(ctx).Model(&model.NodeGroup{}).Count(&existing).Error; err != nil {
		return GroupView{}, false, err
	}
	if existing > 0 {
		view, err = s.first(ctx)
		return view, false, err
	}
	// Fail on a missing key-encryption key before creating anything.
	if _, err := s.sealer(); err != nil {
		return GroupView{}, false, err
	}
	view, err = s.Create(ctx, DefaultGroupInput(), actor)
	if errors.Is(err, ErrNameTaken) {
		// Another request created it first.
		view, err = s.first(ctx)
		return view, false, err
	}
	if err != nil {
		return GroupView{}, false, err
	}
	return view, true, nil
}

// first returns the first live group by sort, then id.
func (s *Service) first(ctx context.Context) (GroupView, error) {
	var group model.NodeGroup
	if err := s.DB.WithContext(ctx).Order("sort").Order("id").Take(&group).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return GroupView{}, ErrNotFound
		}
		return GroupView{}, err
	}
	return s.Get(ctx, group.ID)
}
