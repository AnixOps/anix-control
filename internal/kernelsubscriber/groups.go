package kernelsubscriber

import (
	"context"

	kernelsubscriberv1 "github.com/AnixOps/anix-control/sdk/api/kernelsubscriber/v1"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/AnixOps/anix-control/v4/internal/subscriber"
	"gorm.io/gorm"
)

// GrantSubscriptionGroup gives a subscriber one subscription group
// (kernel.subscriber.groups.v1), once per request id.
func (h *hostServer) GrantSubscriptionGroup(ctx context.Context, request *kernelsubscriberv1.GrantSubscriptionGroupRequest) (*kernelsubscriberv1.GrantSubscriptionGroupResponse, error) {
	db, err := h.begin(ctx, service.CapabilitySubscriberGroups)
	if err != nil {
		return nil, err
	}
	id, err := requestID(request.GetRequestId())
	if err != nil {
		return nil, err
	}
	user, err := userID(request.GetUserId())
	if err != nil {
		return nil, err
	}
	group, err := groupID(request.GetGroupId())
	if err != nil {
		return nil, err
	}
	grant := subscriber.GroupGrant{
		RequestID: id, UserID: user, GroupID: group,
		ExpiresAt: request.ExpiresAtUnix, TransferBytes: request.TransferBytes, NextRenewPrice: request.NextRenewPriceCents,
	}
	var result subscriber.GrantResult
	err = db.Transaction(func(tx *gorm.DB) error {
		var err error
		result, err = subscriber.GrantSubscriptionGroupTx(tx, grant, h.now())
		return err
	})
	if err != nil {
		return nil, failure("grant subscription group", err)
	}
	return &kernelsubscriberv1.GrantSubscriptionGroupResponse{Applied: result.Applied, Created: result.Created}, nil
}

// RevokeSubscriptionGroup takes one subscription group from a subscriber
// (kernel.subscriber.groups.v1), once per request id.
func (h *hostServer) RevokeSubscriptionGroup(ctx context.Context, request *kernelsubscriberv1.RevokeSubscriptionGroupRequest) (*kernelsubscriberv1.RevokeSubscriptionGroupResponse, error) {
	db, err := h.begin(ctx, service.CapabilitySubscriberGroups)
	if err != nil {
		return nil, err
	}
	id, err := requestID(request.GetRequestId())
	if err != nil {
		return nil, err
	}
	user, err := userID(request.GetUserId())
	if err != nil {
		return nil, err
	}
	group, err := groupID(request.GetGroupId())
	if err != nil {
		return nil, err
	}
	var result subscriber.MembershipResult
	err = db.Transaction(func(tx *gorm.DB) error {
		var err error
		result, err = subscriber.RevokeSubscriptionGroupTx(tx, id, user, group, h.now())
		return err
	})
	if err != nil {
		return nil, failure("revoke subscription group", err)
	}
	return &kernelsubscriberv1.RevokeSubscriptionGroupResponse{Applied: result.Applied}, nil
}

// RemoveSubscriptionGroupMembers takes a subscription group from every
// subscriber who holds it (kernel.subscriber.groups.v1), once per request
// id.
func (h *hostServer) RemoveSubscriptionGroupMembers(ctx context.Context, request *kernelsubscriberv1.RemoveSubscriptionGroupMembersRequest) (*kernelsubscriberv1.RemoveSubscriptionGroupMembersResponse, error) {
	db, err := h.begin(ctx, service.CapabilitySubscriberGroups)
	if err != nil {
		return nil, err
	}
	id, err := requestID(request.GetRequestId())
	if err != nil {
		return nil, err
	}
	group, err := groupID(request.GetGroupId())
	if err != nil {
		return nil, err
	}
	var result subscriber.MembershipResult
	err = db.Transaction(func(tx *gorm.DB) error {
		var err error
		result, err = subscriber.RemoveSubscriptionGroupMembersTx(tx, id, group, h.now())
		return err
	})
	if err != nil {
		return nil, failure("remove subscription group members", err)
	}
	return &kernelsubscriberv1.RemoveSubscriptionGroupMembersResponse{Applied: result.Applied, Removed: uint64(max(result.Removed, 0))}, nil
}
