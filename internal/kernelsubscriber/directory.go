package kernelsubscriber

import (
	"context"
	"errors"
	"time"

	kernelsubscriberv1 "github.com/AnixOps/anix-control/sdk/api/kernelsubscriber/v1"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/AnixOps/anix-control/v4/internal/subscriber"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// subscriberColumns are the v2_user columns a Subscriber carries: no
// password, token or identity fields beyond the ban flag.
var subscriberColumns = []string{
	"id", "uuid", "plan_id", "group_id", "expired_at", "transfer_enable", "u", "d", "speed_limit", "device_limit",
	"banned", "flow_reset_time",
}

// toSubscribers converts users, with their subscription groups.
func toSubscribers(db *gorm.DB, users []model.User) ([]*kernelsubscriberv1.Subscriber, error) {
	if len(users) == 0 {
		return nil, nil
	}
	ids := make([]uint, 0, len(users))
	for _, user := range users {
		ids = append(ids, user.ID)
	}
	var memberships []model.UserSubscriptionGroup
	if err := db.Where("user_id IN ?", ids).Order("group_id").Find(&memberships).Error; err != nil {
		return nil, err
	}
	groups := map[uint][]uint64{}
	for _, membership := range memberships {
		groups[membership.UserID] = append(groups[membership.UserID], uint64(membership.GroupID))
	}
	out := make([]*kernelsubscriberv1.Subscriber, 0, len(users))
	for _, user := range users {
		out = append(out, toSubscriber(user, groups[user.ID]))
	}
	return out, nil
}

func toSubscriber(user model.User, groups []uint64) *kernelsubscriberv1.Subscriber {
	s := &kernelsubscriberv1.Subscriber{
		UserId: uint64(user.ID), Uuid: user.UUID, SubscriptionGroupIds: groups,
		TransferBytes: nonNegative(user.TransferEnable), UploadBytes: nonNegative(user.U), DownloadBytes: nonNegative(user.D),
		Banned: user.Banned == 1, FlowResetDay: user.FlowResetTime,
	}
	if user.PlanID != nil {
		s.PlanId = uint64(*user.PlanID)
	}
	if user.GroupID != nil {
		s.GroupId = uint64(*user.GroupID)
	}
	if user.ExpiredAt != nil {
		s.ExpiresAtUnix = *user.ExpiredAt
	}
	if user.SpeedLimit != nil {
		speed := *user.SpeedLimit
		s.SpeedLimitMbps = &speed
	}
	if user.DeviceLimit != nil {
		devices := int32(min(*user.DeviceLimit, 1<<30)) // #nosec G115 -- clamped.
		s.DeviceLimit = &devices
	}
	return s
}

func nonNegative(value int64) uint64 {
	if value < 0 {
		return 0
	}
	return uint64(value)
}

func (h *hostServer) GetSubscribers(ctx context.Context, request *kernelsubscriberv1.GetSubscribersRequest) (*kernelsubscriberv1.GetSubscribersResponse, error) {
	db, err := h.begin(ctx, service.CapabilitySubscriberDirectory)
	if err != nil {
		return nil, err
	}
	if len(request.GetUserIds()) > maxLookupIDs {
		return nil, status.Errorf(codes.InvalidArgument, "at most %d user ids", maxLookupIDs)
	}
	ids := make([]uint, 0, len(request.GetUserIds()))
	for _, raw := range request.GetUserIds() {
		id, err := userID(raw)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	var users []model.User
	if len(ids) > 0 {
		if err := db.Select(subscriberColumns).Where("id IN ?", ids).Order("id").Find(&users).Error; err != nil {
			return nil, failure("get subscribers", err)
		}
	}
	subscribers, err := toSubscribers(db, users)
	if err != nil {
		return nil, failure("get subscribers", err)
	}
	return &kernelsubscriberv1.GetSubscribersResponse{Subscribers: subscribers}, nil
}

func (h *hostServer) LookupBySubscriptionToken(ctx context.Context, request *kernelsubscriberv1.LookupBySubscriptionTokenRequest) (*kernelsubscriberv1.LookupBySubscriptionTokenResponse, error) {
	db, err := h.begin(ctx, service.CapabilitySubscriberDirectory)
	if err != nil {
		return nil, err
	}
	token := request.GetToken()
	if token == "" || len(token) > 64 {
		return &kernelsubscriberv1.LookupBySubscriptionTokenResponse{}, nil
	}
	var user model.User
	err = db.Select(subscriberColumns).Where("token = ?", token).Take(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &kernelsubscriberv1.LookupBySubscriptionTokenResponse{}, nil
	}
	if err != nil {
		return nil, failure("lookup subscription token", err)
	}
	subscribers, err := toSubscribers(db, []model.User{user})
	if err != nil {
		return nil, failure("lookup subscription token", err)
	}
	return &kernelsubscriberv1.LookupBySubscriptionTokenResponse{Found: true, Subscriber: subscribers[0]}, nil
}

func (h *hostServer) ListActiveSubscribers(ctx context.Context, request *kernelsubscriberv1.ListActiveSubscribersRequest) (*kernelsubscriberv1.ListActiveSubscribersResponse, error) {
	db, err := h.begin(ctx, service.CapabilitySubscriberDirectory)
	if err != nil {
		return nil, err
	}
	limit := int(request.GetLimit())
	if limit <= 0 {
		limit = defaultListLimit
	}
	limit = min(limit, maxListLimit)
	// The cursor is read first: a change during the listing is then seen
	// again by a watch from it, never missed.
	cursor, err := subscriber.Cursor(db)
	if err != nil {
		return nil, failure("list active subscribers", err)
	}
	query := subscriber.Active(db.Model(&model.User{}), h.now()).Select(subscriberColumns).
		Where("id > ?", request.GetAfterUserId())
	if groups := request.GetGroupIds(); len(groups) > 0 {
		query = query.Where("group_id IN ?", groups)
	}
	var users []model.User
	if err := query.Order("id").Limit(limit).Find(&users).Error; err != nil {
		return nil, failure("list active subscribers", err)
	}
	subscribers, err := toSubscribers(db, users)
	if err != nil {
		return nil, failure("list active subscribers", err)
	}
	return &kernelsubscriberv1.ListActiveSubscribersResponse{Subscribers: subscribers, Cursor: cursor, Done: len(users) < limit}, nil
}

// WatchSubscriberChanges streams the change log after a cursor. Each change
// carries the subscriber's current state: UPSERT when it is active, REMOVE
// otherwise. A cursor older than the log's retention gets RESYNC.
func (h *hostServer) WatchSubscriberChanges(request *kernelsubscriberv1.WatchSubscriberChangesRequest, stream grpc.ServerStreamingServer[kernelsubscriberv1.SubscriberChange]) error {
	ctx := stream.Context()
	db, err := h.begin(ctx, service.CapabilitySubscriberDirectory)
	if err != nil {
		return err
	}
	cursor := request.GetAfterCursor()
	authorized := time.Now()
	for {
		if time.Since(authorized) > watchReauthorize {
			if db, err = h.begin(ctx, service.CapabilitySubscriberDirectory); err != nil {
				return err
			}
			authorized = time.Now()
		}
		changes, resync, err := subscriber.ChangesAfter(db, cursor, watchBatch)
		if err != nil {
			return failure("watch subscriber changes", err)
		}
		if resync {
			return stream.Send(&kernelsubscriberv1.SubscriberChange{Cursor: cursor, Kind: kernelsubscriberv1.ChangeKind_CHANGE_KIND_RESYNC})
		}
		for _, change := range changes {
			message, err := h.resolve(db, change)
			if err != nil {
				return failure("watch subscriber changes", err)
			}
			if err := stream.Send(message); err != nil {
				return err
			}
			cursor = change.ID
		}
		if len(changes) == watchBatch {
			continue
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(watchPoll):
		}
	}
}

func (h *hostServer) resolve(db *gorm.DB, change model.SubscriberChange) (*kernelsubscriberv1.SubscriberChange, error) {
	message := &kernelsubscriberv1.SubscriberChange{Cursor: change.ID, UserId: uint64(change.UserID), Kind: kernelsubscriberv1.ChangeKind_CHANGE_KIND_REMOVE}
	if change.Deleted {
		return message, nil
	}
	var users []model.User
	if err := subscriber.Active(db.Model(&model.User{}), h.now()).Select(subscriberColumns).
		Where("id = ?", change.UserID).Limit(1).Find(&users).Error; err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return message, nil
	}
	subscribers, err := toSubscribers(db, users)
	if err != nil {
		return nil, err
	}
	message.Kind, message.Subscriber = kernelsubscriberv1.ChangeKind_CHANGE_KIND_UPSERT, subscribers[0]
	return message, nil
}
