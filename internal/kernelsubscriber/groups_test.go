package kernelsubscriber

import (
	"context"
	"testing"
	"time"

	kernelsubscriberv1 "github.com/AnixOps/anix-control/sdk/api/kernelsubscriber/v1"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// withGroups is every family, the membership one included.
var withGroups = grants{
	service.CapabilitySubscriberGroups: true, service.CapabilitySubscriberDirectory: true,
}

// groupFixture adds subscription groups 7 and 8 to the fixture: user 1 and
// 2 are active, user 3 is banned.
func groupFixture(t *testing.T, authorizer Authorizer) (*gorm.DB, kernelsubscriberv1.KernelSubscriberServer) {
	t.Helper()
	db, server := fixture(t, authorizer)
	require.NoError(t, db.AutoMigrate(&model.SubscriptionGroup{}))
	require.NoError(t, db.Create(&[]model.SubscriptionGroup{{ID: 7, Name: "seven", Enable: 1}, {ID: 8, Name: "eight", Enable: 1}}).Error)
	return db, server
}

// changes are the change log's subscribers, in order.
func changes(t *testing.T, db *gorm.DB) []uint {
	t.Helper()
	var users []uint
	require.NoError(t, db.Model(&model.SubscriberChange{}).Order("id").Pluck("user_id", &users).Error)
	return users
}

func recorded(t *testing.T, db *gorm.DB, requestID string) []model.SubscriberRequest {
	t.Helper()
	var rows []model.SubscriberRequest
	require.NoError(t, db.Where("request_id = ?", requestID).Find(&rows).Error)
	return rows
}

type fenced struct{}

func (fenced) AuthorizeCapability(context.Context, packagebridge.HostIdentity, string) error {
	return packagebridge.ErrHostFenced
}

func TestMembershipCallsNeedTheGroupsCapability(t *testing.T) {
	ctx := context.Background()
	for name, authorizer := range map[string]Authorizer{"other families": allFamilies, "fenced generation": fenced{}} {
		t.Run(name, func(t *testing.T) {
			db, server := groupFixture(t, authorizer)
			_, err := server.GrantSubscriptionGroup(ctx, &kernelsubscriberv1.GrantSubscriptionGroupRequest{RequestId: "g", UserId: 1, GroupId: 7})
			require.Equal(t, codes.PermissionDenied, status.Code(err))
			_, err = server.RevokeSubscriptionGroup(ctx, &kernelsubscriberv1.RevokeSubscriptionGroupRequest{RequestId: "r", UserId: 1, GroupId: 7})
			require.Equal(t, codes.PermissionDenied, status.Code(err))
			_, err = server.RemoveSubscriptionGroupMembers(ctx, &kernelsubscriberv1.RemoveSubscriptionGroupMembersRequest{RequestId: "m", GroupId: 7})
			require.Equal(t, codes.PermissionDenied, status.Code(err))
			var memberships, requests int64
			require.NoError(t, db.Model(&model.UserSubscriptionGroup{}).Count(&memberships).Error)
			require.NoError(t, db.Model(&model.SubscriberRequest{}).Count(&requests).Error)
			require.Zero(t, memberships+requests)
		})
	}
	_, onlyGroups := groupFixture(t, grants{service.CapabilitySubscriberGroups: true})
	_, err := onlyGroups.ApplyEntitlement(ctx, &kernelsubscriberv1.ApplyEntitlementRequest{RequestId: "r", UserId: 1, Plan: &kernelsubscriberv1.PlanSnapshot{PlanId: 1}})
	require.Equal(t, codes.PermissionDenied, status.Code(err), "the membership family grants nothing else")
}

func TestGrantIsIdempotentAndRecordsChanges(t *testing.T) {
	db, server := groupFixture(t, withGroups)
	ctx := context.Background()
	expires := time.Now().Add(24 * time.Hour).Unix()
	grant := &kernelsubscriberv1.GrantSubscriptionGroupRequest{
		RequestId: "subscription.grant:1", UserId: 1, GroupId: 7, ExpiresAtUnix: &expires, NextRenewPriceCents: ptr(int64(990)),
	}
	first, err := server.GrantSubscriptionGroup(ctx, grant)
	require.NoError(t, err)
	require.True(t, first.GetApplied())
	require.True(t, first.GetCreated())
	again, err := server.GrantSubscriptionGroup(ctx, grant)
	require.NoError(t, err)
	require.False(t, again.GetApplied(), "a repeat is not applied again")
	require.True(t, again.GetCreated(), "a repeat answers the first result")
	var memberships []model.UserSubscriptionGroup
	require.NoError(t, db.Find(&memberships).Error)
	require.Len(t, memberships, 1)
	require.Equal(t, expires, *memberships[0].ExpireAt)
	require.Nil(t, memberships[0].TransferEnable)
	require.Equal(t, int64(990), *memberships[0].NextRenewPrice)
	ledger := recorded(t, db, "subscription.grant:1")
	require.Len(t, ledger, 1)
	require.Equal(t, "grant_subscription_group", ledger[0].Method)
	require.EqualValues(t, 1, ledger[0].UserID)
	require.JSONEq(t, `{"created":true}`, ledger[0].Result)
	require.Equal(t, []uint{1}, changes(t, db), "a new membership changes an active subscriber's groups, once")

	// The watched subscriber carries the group.
	listed, err := server.GetSubscribers(ctx, &kernelsubscriberv1.GetSubscribersRequest{UserIds: []uint64{1}})
	require.NoError(t, err)
	require.Equal(t, []uint64{7}, listed.GetSubscribers()[0].GetSubscriptionGroupIds())

	// Another grant sets only the given fields; a traffic or price change
	// alters neither the groups nor until when, and records no change.
	updated, err := server.GrantSubscriptionGroup(ctx, &kernelsubscriberv1.GrantSubscriptionGroupRequest{
		RequestId: "subscription.grant:2", UserId: 1, GroupId: 7, TransferBytes: ptr(int64(1 << 30)), ExpiresAtUnix: &expires,
	})
	require.NoError(t, err)
	require.True(t, updated.GetApplied())
	require.False(t, updated.GetCreated())
	require.NoError(t, db.Find(&memberships).Error)
	require.Equal(t, int64(1<<30), *memberships[0].TransferEnable)
	require.Equal(t, int64(990), *memberships[0].NextRenewPrice, "an absent field is kept")
	require.Equal(t, []uint{1}, changes(t, db))
	later := expires + 3600
	_, err = server.GrantSubscriptionGroup(ctx, &kernelsubscriberv1.GrantSubscriptionGroupRequest{RequestId: "subscription.grant:3", UserId: 1, GroupId: 7, ExpiresAtUnix: &later})
	require.NoError(t, err)
	require.Equal(t, []uint{1, 1}, changes(t, db), "a new expiry is a change")

	// A banned subscriber is on no watcher's list: no change.
	_, err = server.GrantSubscriptionGroup(ctx, &kernelsubscriberv1.GrantSubscriptionGroupRequest{RequestId: "subscription.grant:4", UserId: 3, GroupId: 7})
	require.NoError(t, err)
	require.Equal(t, []uint{1, 1}, changes(t, db))
}

func TestMembershipNotFoundRecordsNothing(t *testing.T) {
	db, server := groupFixture(t, withGroups)
	ctx := context.Background()
	_, err := server.GrantSubscriptionGroup(ctx, &kernelsubscriberv1.GrantSubscriptionGroupRequest{RequestId: "g:user", UserId: 99, GroupId: 7})
	require.Equal(t, codes.NotFound, status.Code(err))
	require.Equal(t, "subscriber not found", status.Convert(err).Message())
	_, err = server.GrantSubscriptionGroup(ctx, &kernelsubscriberv1.GrantSubscriptionGroupRequest{RequestId: "g:group", UserId: 1, GroupId: 99})
	require.Equal(t, codes.NotFound, status.Code(err))
	require.Equal(t, "subscription group not found", status.Convert(err).Message())
	_, err = server.RevokeSubscriptionGroup(ctx, &kernelsubscriberv1.RevokeSubscriptionGroupRequest{RequestId: "r:user", UserId: 99, GroupId: 7})
	require.Equal(t, "subscriber not found", status.Convert(err).Message())
	_, err = server.RevokeSubscriptionGroup(ctx, &kernelsubscriberv1.RevokeSubscriptionGroupRequest{RequestId: "r:group", UserId: 1, GroupId: 99})
	require.Equal(t, "subscription group not found", status.Convert(err).Message())
	_, err = server.RevokeSubscriptionGroup(ctx, &kernelsubscriberv1.RevokeSubscriptionGroupRequest{RequestId: "r:membership", UserId: 1, GroupId: 7})
	require.Equal(t, codes.NotFound, status.Code(err))
	require.Equal(t, "subscription group membership not found", status.Convert(err).Message())
	var requests int64
	require.NoError(t, db.Model(&model.SubscriberRequest{}).Count(&requests).Error)
	require.Zero(t, requests)
	require.Empty(t, changes(t, db))

	for _, request := range []*kernelsubscriberv1.GrantSubscriptionGroupRequest{
		{UserId: 1, GroupId: 7}, {RequestId: "g", GroupId: 7}, {RequestId: "g", UserId: 1}, {RequestId: "g", UserId: 1, GroupId: 1 << 32},
	} {
		_, err = server.GrantSubscriptionGroup(ctx, request)
		require.Equal(t, codes.InvalidArgument, status.Code(err), "%v", request)
	}
	_, err = server.RemoveSubscriptionGroupMembers(ctx, &kernelsubscriberv1.RemoveSubscriptionGroupMembersRequest{RequestId: "m"})
	require.Equal(t, codes.InvalidArgument, status.Code(err))

	// The same id applies once the membership exists.
	require.NoError(t, db.Create(&model.UserSubscriptionGroup{UserID: 1, GroupID: 7}).Error)
	revoked, err := server.RevokeSubscriptionGroup(ctx, &kernelsubscriberv1.RevokeSubscriptionGroupRequest{RequestId: "r:membership", UserId: 1, GroupId: 7})
	require.NoError(t, err)
	require.True(t, revoked.GetApplied())
	again, err := server.RevokeSubscriptionGroup(ctx, &kernelsubscriberv1.RevokeSubscriptionGroupRequest{RequestId: "r:membership", UserId: 1, GroupId: 7})
	require.NoError(t, err)
	require.False(t, again.GetApplied(), "a repeat of a revocation succeeds without the membership")
	require.Equal(t, []uint{1}, changes(t, db))
	require.Equal(t, "revoke_subscription_group", recorded(t, db, "r:membership")[0].Method)
}

func TestRemoveMembersRecordsAChangePerActiveMember(t *testing.T) {
	db, server := groupFixture(t, withGroups)
	ctx := context.Background()
	past := time.Now().Add(-time.Hour).Unix()
	require.NoError(t, db.Create(&[]model.UserSubscriptionGroup{
		{UserID: 2, GroupID: 7}, {UserID: 1, GroupID: 7, ExpireAt: &past}, {UserID: 3, GroupID: 7}, {UserID: 1, GroupID: 8},
	}).Error)
	removed, err := server.RemoveSubscriptionGroupMembers(ctx, &kernelsubscriberv1.RemoveSubscriptionGroupMembersRequest{RequestId: "subscription.delete_group:7", GroupId: 7})
	require.NoError(t, err)
	require.True(t, removed.GetApplied())
	require.EqualValues(t, 3, removed.GetRemoved())
	require.Equal(t, []uint{1, 2}, changes(t, db), "active members in id order, an expired membership included; not the banned one")
	var left []model.UserSubscriptionGroup
	require.NoError(t, db.Find(&left).Error)
	require.Len(t, left, 1)
	require.EqualValues(t, 8, left[0].GroupID)
	ledger := recorded(t, db, "subscription.delete_group:7")
	require.Len(t, ledger, 1)
	require.Equal(t, "remove_group_members", ledger[0].Method)
	require.Zero(t, ledger[0].UserID)

	again, err := server.RemoveSubscriptionGroupMembers(ctx, &kernelsubscriberv1.RemoveSubscriptionGroupMembersRequest{RequestId: "subscription.delete_group:7", GroupId: 7})
	require.NoError(t, err)
	require.False(t, again.GetApplied())
	require.EqualValues(t, 3, again.GetRemoved())
	// A group that no longer exists still loses its members: a retry with
	// a new id after the group is gone completes.
	require.NoError(t, db.Delete(&model.SubscriptionGroup{}, 8).Error)
	gone, err := server.RemoveSubscriptionGroupMembers(ctx, &kernelsubscriberv1.RemoveSubscriptionGroupMembersRequest{RequestId: "subscription.delete_group:8", GroupId: 8})
	require.NoError(t, err)
	require.EqualValues(t, 1, gone.GetRemoved())
	none, err := server.RemoveSubscriptionGroupMembers(ctx, &kernelsubscriberv1.RemoveSubscriptionGroupMembersRequest{RequestId: "subscription.delete_group:8:retry", GroupId: 8})
	require.NoError(t, err)
	require.True(t, none.GetApplied())
	require.Zero(t, none.GetRemoved())
	require.Equal(t, []uint{1, 2, 1}, changes(t, db))
}
