package subscriber

import (
	"fmt"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// A group's removal takes every member, however many: the members are
// selected by a subquery and the changes inserted in batches, so no
// statement outgrows PostgreSQL's parameter limit.
func TestRemoveMembersOfALargeGroup(t *testing.T) {
	db := openDB(t)
	const members = 2500
	users := make([]model.User, 0, members)
	memberships := make([]model.UserSubscriptionGroup, 0, members)
	for id := uint(1); id <= members; id++ {
		users = append(users, model.User{ID: id, Email: fmt.Sprintf("u%d@example.test", id), Token: fmt.Sprintf("t%d", id), UUID: fmt.Sprintf("u%d", id), TransferEnable: 10})
		memberships = append(memberships, model.UserSubscriptionGroup{UserID: id, GroupID: 3})
	}
	require.NoError(t, db.CreateInBatches(&users, 500).Error)
	require.NoError(t, db.CreateInBatches(&memberships, 500).Error)
	require.NoError(t, db.Model(&model.User{}).Where("id % 2 = 0").Update("banned", 1).Error)

	var result MembershipResult
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		var err error
		result, err = RemoveSubscriptionGroupMembersTx(tx, "remove:3", 3, time.Now())
		return err
	}))
	require.EqualValues(t, members, result.Removed)
	var changed []uint
	require.NoError(t, db.Model(&model.SubscriberChange{}).Order("id").Pluck("user_id", &changed).Error)
	require.Len(t, changed, members/2, "the active (odd) members")
	require.EqualValues(t, 1, changed[0])
	require.EqualValues(t, members-1, changed[len(changed)-1])
}
