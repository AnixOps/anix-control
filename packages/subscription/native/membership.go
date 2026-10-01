package native

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strconv"
	"strings"

	kernelsubscriberv1 "github.com/AnixOps/anix-control/sdk/api/kernelsubscriber/v1"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/packages/subscription/native/model"
	"github.com/gin-gonic/gin/binding"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// Subscriber is the part of KernelSubscriber the native routes call
// (kernel.subscriber.groups.v1): v2_user_subscription_group is subscriber
// state, and the kernel is its only writer.
type Subscriber interface {
	GrantSubscriptionGroup(ctx context.Context, in *kernelsubscriberv1.GrantSubscriptionGroupRequest, opts ...grpc.CallOption) (*kernelsubscriberv1.GrantSubscriptionGroupResponse, error)
	RevokeSubscriptionGroup(ctx context.Context, in *kernelsubscriberv1.RevokeSubscriptionGroupRequest, opts ...grpc.CallOption) (*kernelsubscriberv1.RevokeSubscriptionGroupResponse, error)
	RemoveSubscriptionGroupMembers(ctx context.Context, in *kernelsubscriberv1.RemoveSubscriptionGroupMembersRequest, opts ...grpc.CallOption) (*kernelsubscriberv1.RemoveSubscriptionGroupMembersResponse, error)
}

// The routes that change subscription group membership; they have a native
// handler only with a Subscriber.
const (
	DeleteGroupRouteID     = "subscription.admin.subscription.groups.id.delete"
	GrantUserGroupRouteID  = "subscription.admin.subscription.users.user_id.groups.post"
	RevokeUserGroupRouteID = "subscription.admin.subscription.users.user_id.groups.group_id.delete"
)

// GrantRequestID names an administrator's grant of a subscription group to
// a user in Control's subscriber request ledger, so a retried request is
// applied once. token identifies the HTTP request: its Idempotency-Key,
// else its X-Request-ID, else a fresh random value. The digest also covers
// the granted fields, so a key reused for other values is another grant.
// The kernel's legacy handler derives the same id
// (service.SubscriptionGroupGrantRequestID), so a retry is recognized
// whichever side serves it.
func GrantRequestID(userID, groupID uint, expireAt, transferEnable, nextRenewPrice *int64, token string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		token, digestField(expireAt), digestField(transferEnable), digestField(nextRenewPrice),
	}, "\x00")))
	return fmt.Sprintf("subscription.grant:%d:%d:%x", userID, groupID, sum[:12])
}

// RevokeRequestID names an administrator's removal of a user's group, as
// GrantRequestID names a grant (service.SubscriptionGroupRevokeRequestID).
func RevokeRequestID(userID, groupID uint, token string) string {
	sum := sha256.Sum256([]byte(token))
	return fmt.Sprintf("subscription.revoke:%d:%d:%x", userID, groupID, sum[:12])
}

// DeleteGroupRequestID names the removal of a deleted group's members, as
// GrantRequestID names a grant (service.SubscriptionGroupDeleteRequestID).
func DeleteGroupRequestID(groupID uint, token string) string {
	sum := sha256.Sum256([]byte(token))
	return fmt.Sprintf("subscription.delete_group:%d:%x", groupID, sum[:12])
}

func digestField(value *int64) string {
	if value == nil {
		return "-"
	}
	return strconv.FormatInt(*value, 10)
}

// requestToken is what identifies the request for the request ids.
func (s *Service) requestToken(request pluginhostsdk.NativeRequest) string {
	for _, name := range []string{"Idempotency-Key", "X-Request-Id"} {
		if value := header(request, name); value != "" {
			return value
		}
	}
	if s.NewToken != nil {
		return s.NewToken()
	}
	return uuid.NewString()
}

// header is the request's first value of a forwarded header, trimmed.
func header(request pluginhostsdk.NativeRequest, name string) string {
	for key, values := range request.Metadata.Headers {
		if strings.EqualFold(key, name) && len(values) > 0 {
			return strings.TrimSpace(values[0])
		}
	}
	return ""
}

// membershipFailed answers a failed KernelSubscriber call as the kernel's
// handler answers its service's error. A NotFound means the subscriber, the
// group or the membership is missing: the checks the kernel's handler makes
// first say which, and missing is the answer when both pass.
func (s *Service) membershipFailed(db *gorm.DB, fallback string, userID, groupID uint, missing, err error) (pluginhostsdk.NativeResponse, error) {
	if status.Code(err) != codes.NotFound {
		return s.panelError(fallback + ": " + status.Convert(err).Message())
	}
	if err := userExists(db, userID); err != nil {
		return s.bindingError(fallback, err)
	}
	if err := groupExists(db, groupID); err != nil {
		return s.bindingError(fallback, err)
	}
	return s.bindingError(fallback, missing)
}

// GrantUserGroup is POST /api/v2/admin/subscription/users/:user_id/groups:
// the user holds the group, with its own expiry, traffic and renewal price
// when given. Control creates the membership, or sets the given fields of
// an existing one (KernelSubscriber.GrantSubscriptionGroup).
func (s *Service) GrantUserGroup(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	userID, ok := pathUint(request, "user_id")
	if !ok {
		return s.panelError("用户 ID 无效")
	}
	var body struct {
		GroupID        uint   `json:"group_id" binding:"required"`
		ExpireAt       *int64 `json:"expire_at"`
		TransferEnable *int64 `json:"transfer_enable"`  // bytes
		NextRenewPrice *int64 `json:"next_renew_price"` // 单位分
	}
	if err := binding.JSON.BindBody(request.Body, &body); err != nil {
		return s.panelError("参数错误: " + err.Error())
	}
	db, err := s.Open(ctx)
	if err != nil {
		return s.bindingError("分配失败", err)
	}
	if err := userExists(db, userID); err != nil {
		return s.bindingError("分配失败", err)
	}
	if err := groupExists(db, body.GroupID); err != nil {
		return s.bindingError("分配失败", err)
	}
	_, err = s.Subscriber.GrantSubscriptionGroup(ctx, &kernelsubscriberv1.GrantSubscriptionGroupRequest{
		RequestId: GrantRequestID(userID, body.GroupID, body.ExpireAt, body.TransferEnable, body.NextRenewPrice, s.requestToken(request)),
		UserId:    uint64(userID), GroupId: uint64(body.GroupID),
		ExpiresAtUnix: body.ExpireAt, TransferBytes: body.TransferEnable, NextRenewPriceCents: body.NextRenewPrice,
		Reason: "administrator grant",
	})
	if err != nil {
		return s.membershipFailed(db, "分配失败", userID, body.GroupID, errUserNotFound, err)
	}
	return s.panel(map[string]any{"message": "分配成功"})
}

// RevokeUserGroup is DELETE
// /api/v2/admin/subscription/users/:user_id/groups/:group_id
// (KernelSubscriber.RevokeSubscriptionGroup).
func (s *Service) RevokeUserGroup(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	userID, ok := pathUint(request, "user_id")
	if !ok {
		return s.panelError("用户 ID 无效")
	}
	groupID, ok := pathUint(request, "group_id")
	if !ok {
		return s.panelError("分组 ID 无效")
	}
	db, err := s.Open(ctx)
	if err != nil {
		return s.bindingError("移除失败", err)
	}
	if err := userExists(db, userID); err != nil {
		return s.bindingError("移除失败", err)
	}
	if err := groupExists(db, groupID); err != nil {
		return s.bindingError("移除失败", err)
	}
	_, err = s.Subscriber.RevokeSubscriptionGroup(ctx, &kernelsubscriberv1.RevokeSubscriptionGroupRequest{
		RequestId: RevokeRequestID(userID, groupID, s.requestToken(request)),
		UserId:    uint64(userID), GroupId: uint64(groupID), Reason: "administrator revocation",
	})
	if err != nil {
		return s.membershipFailed(db, "移除失败", userID, groupID, errUserGroupNotFound, err)
	}
	return s.panel(map[string]any{"message": "移除成功"})
}

// DeleteGroup is DELETE /api/v2/admin/subscription/groups/:id. Control
// first takes the group from its members
// (KernelSubscriber.RemoveSubscriptionGroupMembers); then the group's
// templates, plan links and node protocol links go with the group, in one
// transaction. A retry after a failure in between finds no members and
// completes.
func (s *Service) DeleteGroup(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	id, ok := pathUint(request, "id")
	if !ok {
		return s.panelError(invalidID)
	}
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	var group model.SubscriptionGroup
	if err := db.First(&group, id).Error; err != nil {
		return s.groupDeleteFailed(err)
	}
	if _, err := s.Subscriber.RemoveSubscriptionGroupMembers(ctx, &kernelsubscriberv1.RemoveSubscriptionGroupMembersRequest{
		RequestId: DeleteGroupRequestID(id, s.requestToken(request)), GroupId: uint64(id), Reason: "subscription group deleted",
	}); err != nil {
		return s.panelError(status.Convert(err).Message())
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		var group model.SubscriptionGroup
		if err := tx.First(&group, id).Error; err != nil {
			return err
		}
		if err := tx.Where("group_id = ?", id).Delete(&model.SubscriptionTemplate{}).Error; err != nil {
			return err
		}
		if err := tx.Where("group_id = ?", id).Delete(&model.PlanSubscriptionGroup{}).Error; err != nil {
			return err
		}
		// The group's node protocol links reference it: PostgreSQL refuses
		// to delete a linked group, and SQLite would keep orphan links.
		if err := tx.Where("subscription_group_id = ?", id).Delete(&model.GroupProtocol{}).Error; err != nil {
			return err
		}
		return tx.Delete(&group).Error
	})
	if err != nil {
		return s.groupDeleteFailed(err)
	}
	return s.panel(map[string]any{"message": "删除成功"})
}

func (s *Service) groupDeleteFailed(err error) (pluginhostsdk.NativeResponse, error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return s.panelError(groupNotFound)
	}
	return s.panelError(err.Error())
}
