package kernelsubscriber

import (
	"context"
	"net"
	"testing"
	"time"

	kernelsubscriberv1 "github.com/AnixOps/anix-control/sdk/api/kernelsubscriber/v1"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/AnixOps/anix-control/v4/internal/subscriber"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// grants authorizes the capabilities it holds.
type grants map[string]bool

func (g grants) AuthorizeCapability(_ context.Context, _ packagebridge.HostIdentity, capability string) error {
	if g[capability] {
		return nil
	}
	return service.ErrCapabilityNotAuthorized
}

var allFamilies = grants{
	service.CapabilitySubscriberEntitlements: true, service.CapabilitySubscriberTraffic: true,
	service.CapabilitySubscriberCredentials: true, service.CapabilitySubscriberBalance: true,
	service.CapabilitySubscriberDirectory: true,
}

func fixture(t *testing.T, authorizer Authorizer) (*gorm.DB, kernelsubscriberv1.KernelSubscriberServer) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSubscriptionGroup{}, &model.SubscriberRequest{},
		&model.SubscriberChange{}, &model.IdentityRevocation{}, &model.IdentitySessionRevocation{}))
	future := time.Now().Add(time.Hour).Unix()
	require.NoError(t, db.Create(&[]model.User{
		{ID: 1, Email: "a@x", Token: "token-a", UUID: "uuid-a", TransferEnable: 100, ExpiredAt: &future, GroupID: ptr(uint(4)), CommissionBalance: 50},
		{ID: 2, Email: "b@x", Token: "token-b", UUID: "uuid-b", TransferEnable: 100, GroupID: ptr(uint(5))},
		{ID: 3, Email: "c@x", Token: "token-c", UUID: "uuid-c", TransferEnable: 100, GroupID: ptr(uint(4)), Banned: 1},
	}).Error)
	server := &Server{DB: db, Authorizer: authorizer}
	return db, server.For(packagebridge.HostIdentity{PackageID: "order", Version: "4.1.0", Generation: 1})
}

func ptr[T any](value T) *T { return &value }

func TestEachMethodFamilyNeedsItsCapability(t *testing.T) {
	_, onlyTraffic := fixture(t, grants{service.CapabilitySubscriberTraffic: true})
	ctx := context.Background()
	_, err := onlyTraffic.ApplyEntitlement(ctx, &kernelsubscriberv1.ApplyEntitlementRequest{RequestId: "r", UserId: 1, Plan: &kernelsubscriberv1.PlanSnapshot{PlanId: 1}})
	require.Equal(t, codes.PermissionDenied, status.Code(err))
	_, err = onlyTraffic.AdjustBalance(ctx, &kernelsubscriberv1.AdjustBalanceRequest{RequestId: "r", UserId: 1, Kind: kernelsubscriberv1.BalanceKind_BALANCE_KIND_ACCOUNT})
	require.Equal(t, codes.PermissionDenied, status.Code(err))
	_, err = onlyTraffic.GetSubscribers(ctx, &kernelsubscriberv1.GetSubscribersRequest{UserIds: []uint64{1}})
	require.Equal(t, codes.PermissionDenied, status.Code(err))
	_, err = onlyTraffic.RecordTraffic(ctx, &kernelsubscriberv1.RecordTrafficRequest{BatchId: "b", Entries: []*kernelsubscriberv1.TrafficEntry{{UserId: 1, UploadBytes: 1}}})
	require.NoError(t, err)
}

func TestWritesAreIdempotentAndValidated(t *testing.T) {
	db, server := fixture(t, allFamilies)
	ctx := context.Background()

	apply := &kernelsubscriberv1.ApplyEntitlementRequest{
		RequestId: "order:9", UserId: 2, RenewSamePlan: true, ResetTraffic: true,
		Plan:   &kernelsubscriberv1.PlanSnapshot{PlanId: 7, GroupId: 5, SubscriptionGroupIds: []uint64{5}, TransferBytes: 1 << 30},
		Expiry: &kernelsubscriberv1.ApplyEntitlementRequest_Period{Period: &kernelsubscriberv1.Period{Months: 1}},
	}
	first, err := server.ApplyEntitlement(ctx, apply)
	require.NoError(t, err)
	require.True(t, first.GetApplied())
	again, err := server.ApplyEntitlement(ctx, apply)
	require.NoError(t, err)
	require.False(t, again.GetApplied())
	require.Equal(t, first.GetExpiresAtUnix(), again.GetExpiresAtUnix())
	_, err = server.ApplyEntitlement(ctx, &kernelsubscriberv1.ApplyEntitlementRequest{UserId: 2, Plan: apply.Plan})
	require.Equal(t, codes.InvalidArgument, status.Code(err), "a request id is required")

	adjusted, err := server.AdjustEntitlement(ctx, &kernelsubscriberv1.AdjustEntitlementRequest{RequestId: "admin:1", UserId: 2, ClearExpiry: true, TransferBytes: ptr(int64(5))})
	require.NoError(t, err)
	require.True(t, adjusted.GetApplied())
	var user model.User
	require.NoError(t, db.Take(&user, 2).Error)
	require.Nil(t, user.ExpiredAt)
	require.Equal(t, int64(5), user.TransferEnable)

	traffic, err := server.RecordTraffic(ctx, &kernelsubscriberv1.RecordTrafficRequest{
		BatchId: "node:1:1", Rate: 2, Entries: []*kernelsubscriberv1.TrafficEntry{{UserId: 1, UploadBytes: 30, DownloadBytes: 20}},
	})
	require.NoError(t, err)
	require.Equal(t, []uint64{1}, traffic.GetExhaustedUserIds(), "(30 + 20) × 2 reaches the 100-byte limit")
	_, err = server.RecordTraffic(ctx, &kernelsubscriberv1.RecordTrafficRequest{Entries: []*kernelsubscriberv1.TrafficEntry{{UserId: 1}}})
	require.Equal(t, codes.InvalidArgument, status.Code(err), "a batch id is required")

	credentials, err := server.ResetCredentials(ctx, &kernelsubscriberv1.ResetCredentialsRequest{RequestId: "reset:1", UserId: 1, SubscriptionToken: true})
	require.NoError(t, err)
	require.True(t, credentials.GetApplied())
	var member model.User
	require.NoError(t, db.Take(&member, 1).Error)
	require.NotEqual(t, "token-a", member.Token)
	require.Equal(t, "uuid-a", member.UUID)
	token := member.Token
	credentials, err = server.ResetCredentials(ctx, &kernelsubscriberv1.ResetCredentialsRequest{RequestId: "reset:1", UserId: 1, SubscriptionToken: true})
	require.NoError(t, err)
	require.False(t, credentials.GetApplied())
	var unchanged model.User
	require.NoError(t, db.Take(&unchanged, 1).Error)
	require.Equal(t, token, unchanged.Token)

	_, err = server.AdjustBalance(ctx, &kernelsubscriberv1.AdjustBalanceRequest{RequestId: "withdraw:1", UserId: 1, Kind: kernelsubscriberv1.BalanceKind_BALANCE_KIND_COMMISSION, AmountCents: -80})
	require.Equal(t, codes.FailedPrecondition, status.Code(err))
	balance, err := server.AdjustBalance(ctx, &kernelsubscriberv1.AdjustBalanceRequest{RequestId: "withdraw:2", UserId: 1, Kind: kernelsubscriberv1.BalanceKind_BALANCE_KIND_COMMISSION, AmountCents: -30})
	require.NoError(t, err)
	require.Equal(t, int64(20), balance.GetBalanceCents())
	_, err = server.AdjustBalance(ctx, &kernelsubscriberv1.AdjustBalanceRequest{RequestId: "x", UserId: 99, Kind: kernelsubscriberv1.BalanceKind_BALANCE_KIND_ACCOUNT, AmountCents: 1})
	require.Equal(t, codes.NotFound, status.Code(err))
}

func TestDirectory(t *testing.T) {
	_, server := fixture(t, allFamilies)
	ctx := context.Background()
	found, err := server.LookupBySubscriptionToken(ctx, &kernelsubscriberv1.LookupBySubscriptionTokenRequest{Token: "token-b"})
	require.NoError(t, err)
	require.True(t, found.GetFound())
	require.Equal(t, uint64(2), found.GetSubscriber().GetUserId())
	missing, err := server.LookupBySubscriptionToken(ctx, &kernelsubscriberv1.LookupBySubscriptionTokenRequest{Token: "nope"})
	require.NoError(t, err)
	require.False(t, missing.GetFound())

	list, err := server.ListActiveSubscribers(ctx, &kernelsubscriberv1.ListActiveSubscribersRequest{GroupIds: []uint64{4}})
	require.NoError(t, err)
	require.Len(t, list.GetSubscribers(), 1, "the banned member of group 4 is not active")
	require.Equal(t, "uuid-a", list.GetSubscribers()[0].GetUuid())
	require.True(t, list.GetDone())

	page, err := server.ListActiveSubscribers(ctx, &kernelsubscriberv1.ListActiveSubscribersRequest{Limit: 1})
	require.NoError(t, err)
	require.Len(t, page.GetSubscribers(), 1)
	require.False(t, page.GetDone())
	next, err := server.ListActiveSubscribers(ctx, &kernelsubscriberv1.ListActiveSubscribersRequest{Limit: 1, AfterUserId: page.GetSubscribers()[0].GetUserId()})
	require.NoError(t, err)
	require.Equal(t, uint64(2), next.GetSubscribers()[0].GetUserId())
}

func TestWatchStreamsCurrentStateOfChanges(t *testing.T) {
	db, server := fixture(t, allFamilies)
	listener := bufconn.Listen(1 << 20)
	grpcServer := grpc.NewServer()
	kernelsubscriberv1.RegisterKernelSubscriberServer(grpcServer, server)
	go func() { _ = grpcServer.Serve(listener) }()
	t.Cleanup(grpcServer.Stop)
	conn, err := grpc.NewClient("passthrough:///kernel", grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	client := kernelsubscriberv1.NewKernelSubscriberClient(conn)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return subscriber.RecordChangesTx(tx, []uint{1, 3}, false, time.Now())
	}))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return subscriber.RecordChangesTx(tx, []uint{2}, true, time.Now())
	}))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stream, err := client.WatchSubscriberChanges(ctx, &kernelsubscriberv1.WatchSubscriberChangesRequest{})
	require.NoError(t, err)
	kinds := map[uint64]kernelsubscriberv1.ChangeKind{}
	for range 3 {
		change, err := stream.Recv()
		require.NoError(t, err)
		kinds[change.GetUserId()] = change.GetKind()
	}
	require.Equal(t, map[uint64]kernelsubscriberv1.ChangeKind{
		1: kernelsubscriberv1.ChangeKind_CHANGE_KIND_UPSERT,
		3: kernelsubscriberv1.ChangeKind_CHANGE_KIND_REMOVE, // banned
		2: kernelsubscriberv1.ChangeKind_CHANGE_KIND_REMOVE, // deleted
	}, kinds)

	// Later changes arrive on the open stream.
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return subscriber.RecordChangesTx(tx, []uint{1}, false, time.Now())
	}))
	change, err := stream.Recv()
	require.NoError(t, err)
	require.Equal(t, uint64(1), change.GetUserId())
	require.Equal(t, kernelsubscriberv1.ChangeKind_CHANGE_KIND_UPSERT, change.GetKind())

	// A cursor older than the kept log must list again.
	require.NoError(t, db.Where("id <= ?", 2).Delete(&model.SubscriberChange{}).Error)
	resync, err := client.WatchSubscriberChanges(ctx, &kernelsubscriberv1.WatchSubscriberChangesRequest{AfterCursor: 1})
	require.NoError(t, err)
	first, err := resync.Recv()
	require.NoError(t, err)
	require.Equal(t, kernelsubscriberv1.ChangeKind_CHANGE_KIND_RESYNC, first.GetKind())
}
