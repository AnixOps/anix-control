package grpc

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	pb "github.com/AnixOps/anix-control/v4/api/grpc/v2boardpb"
	"github.com/AnixOps/anix-control/v4/internal/cache"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/nodesecrets"
	"github.com/AnixOps/anix-control/v4/internal/subscriber"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/reflect/protoreflect"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// The cursor decision, the one rule that chooses between resuming and a full
// resync (PROTOCOL.md, "Users").
func TestDecideUsersStart(t *testing.T) {
	tests := []struct {
		name                   string
		cursor, oldest, latest uint64
		want                   usersStart
	}{
		{"no cursor, empty log", 0, 0, 0, usersResyncNoCursor},
		{"no cursor", 0, 1, 9, usersResyncNoCursor},
		{"cursor ahead of an empty log", 5, 0, 0, usersResyncUnknown},
		{"cursor ahead of the log", 10, 1, 9, usersResyncUnknown},
		{"cursor at the latest change", 9, 1, 9, usersResume},
		{"cursor at the first change", 1, 1, 9, usersResume},
		{"cursor just before the oldest remaining row", 3, 4, 9, usersResume},
		{"cursor before a pruned row", 2, 4, 9, usersResyncStale},
		{"cursor long pruned", 1, 100, 200, usersResyncStale},
		{"cursor in the middle", 5, 1, 9, usersResume},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, decideUsersStart(test.cursor, test.oldest, test.latest))
		})
	}
}

// Forward nodes have no user list, so they are not offered users.v1.
func TestAgentControlUsersServedToProxyNodesOnly(t *testing.T) {
	server := &AgentControlGRPCServer{}
	// With no data-plane capability from the agent only users.v1 is offered.
	agent := validAgentHello(1).GetHello().Capabilities
	proxy := server.serverCapabilities(agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: 1}, agent)
	require.Len(t, proxy, 1)
	assert.Equal(t, agentcontrol.CapabilityUsers, proxy[0].Name)
	assert.Equal(t, agentcontrol.CapabilityVersionV1, proxy[0].Version)
	assert.Empty(t, server.serverCapabilities(agentcontrol.AgentNode{Kind: agentcontrol.NodeKindForward, ID: 1}, agent))
}

// agentUsersEnvironment is an Agent Control server over a node with a plan
// group, on SQLite or PostgreSQL, with small delta bounds.
type agentUsersEnvironment struct {
	node    model.Node
	apiKey  string
	conn    *grpc.ClientConn
	db      *gorm.DB
	groupID uint
}

const usersPostgresDSN = "ANIX_TEST_POSTGRES_DSN"

// forEachUsersDatabase runs body on SQLite and, when ANIX_TEST_POSTGRES_DSN
// is set, on a throwaway PostgreSQL schema.
func forEachUsersDatabase(t *testing.T, body func(t *testing.T, environment *agentUsersEnvironment)) {
	t.Run("sqlite", func(t *testing.T) {
		body(t, newAgentUsersEnvironment(t, func(t *testing.T) { requireInMemoryDatabase(t) }))
	})
	t.Run("postgres", func(t *testing.T) {
		body(t, newAgentUsersEnvironment(t, requirePostgresDatabase))
	})
}

// requirePostgresDatabase points the process database at a throwaway schema
// of the test PostgreSQL database.
func requirePostgresDatabase(t *testing.T) {
	t.Helper()
	base := strings.TrimSpace(os.Getenv(usersPostgresDSN))
	if base == "" {
		t.Skip(usersPostgresDSN + " is not set")
	}
	admin, err := gorm.Open(postgres.Open(base), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	adminDB, err := admin.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = adminDB.Close() })
	var databaseName string
	require.NoError(t, admin.Raw("SELECT current_database()").Scan(&databaseName).Error)
	if !strings.Contains(strings.ToLower(databaseName), "test") && os.Getenv("ANIX_TEST_POSTGRES_ALLOW_UNSAFE") != "1" {
		t.Skipf("refusing to run destructive postgres test against database %q", databaseName)
	}
	suffix := make([]byte, 4)
	_, err = rand.Read(suffix)
	require.NoError(t, err)
	schema := "agent_users_" + hex.EncodeToString(suffix)
	require.NoError(t, admin.Exec(`CREATE SCHEMA "`+schema+`"`).Error)
	t.Cleanup(func() { _ = admin.Exec(`DROP SCHEMA IF EXISTS "` + schema + `" CASCADE`).Error })
	require.NoError(t, database.Init(&config.DatabaseConfig{Driver: "postgres", DSN: base + " search_path=" + schema, LogLevel: "silent"}))
}

func newAgentUsersEnvironment(t *testing.T, initDatabase func(t *testing.T)) *agentUsersEnvironment {
	t.Helper()
	cache.InitMemory()
	initDatabase(t)
	db := database.Get()
	requireAutoMigrate(t, &model.Node{}, &model.NodeProtocol{}, &model.Plan{}, &model.User{}, &model.SubscriberChange{}, &model.WireGuardPeer{})
	require.NoError(t, nodesecrets.EnsureSchema(db))

	for _, bound := range []struct {
		value *int
		small int
	}{{&userDeltaPageSize, 2}, {&userDeltaBatch, 3}} {
		previous := *bound.value
		*bound.value = bound.small
		t.Cleanup(func() { *bound.value = previous })
	}
	agentUserFeed.setPoll(20 * time.Millisecond)
	t.Cleanup(func() { agentUserFeed.setPoll(userDeltaPoll) })

	apiKey := "agent-users-test-key"
	hash := sha256.Sum256([]byte(apiKey))
	groupID := uint(7)
	node := model.Node{Name: "agent-users-node", Host: "127.0.0.1", APIKeyHash: hex.EncodeToString(hash[:]), Status: model.NodeStatusOnline, GroupID: &groupID}
	require.NoError(t, db.Create(&node).Error)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	server := grpc.NewServer(grpc.ChainStreamInterceptor(StreamAuthInterceptor("", "")))
	agentv1pb.RegisterAgentControlServiceServer(server, NewAgentControlGRPCServer(NewAgentControlManager()))
	errCh := serveGRPCServerForTest(t, server, listener)
	conn, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() {
		requireClientConnClosed(t, conn)
		stopGRPCServerForTest(t, server, errCh)
		requireDatabaseClosed(t)
	})
	return &agentUsersEnvironment{node: node, apiKey: apiKey, conn: conn, db: db, groupID: groupID}
}

func (e *agentUsersEnvironment) nodeID() uint32 { return uint32(e.node.ID) }

// seedUsers writes users the node serves (ids 1 to served) and one of each
// kind it does not: banned, expired, out of traffic, of another group and
// with no transfer at all. Every row has an e-mail, a password hash and a
// subscription token. It records a change for each.
func (e *agentUsersEnvironment) seedUsers(t *testing.T, served int) {
	t.Helper()
	future := time.Now().Add(time.Hour).Unix()
	past := time.Now().Add(-time.Hour).Unix()
	other := e.groupID + 1
	speed, devices := int64(100), 3
	users := make([]model.User, 0, served+5)
	for id := 1; id <= served; id++ {
		users = append(users, model.User{
			ID: uint(id), Email: "served-" + strconv.Itoa(id) + "@example.test", Password: "$2y$10$hash-" + strconv.Itoa(id),
			Token: "token-" + strconv.Itoa(id), UUID: "uuid-" + strconv.Itoa(id), TransferEnable: 1000, ExpiredAt: &future,
			GroupID: &e.groupID, SpeedLimit: &speed, DeviceLimit: &devices,
		})
	}
	users = append(users,
		model.User{ID: 101, Email: "banned@example.test", Password: "$2y$10$hash-banned", Token: "token-banned", UUID: "uuid-banned", TransferEnable: 1000, GroupID: &e.groupID, Banned: 1},
		model.User{ID: 102, Email: "expired@example.test", Password: "$2y$10$hash-expired", Token: "token-expired", UUID: "uuid-expired", TransferEnable: 1000, GroupID: &e.groupID, ExpiredAt: &past},
		model.User{ID: 103, Email: "exhausted@example.test", Password: "$2y$10$hash-exhausted", Token: "token-exhausted", UUID: "uuid-exhausted", TransferEnable: 10, U: 6, D: 4, GroupID: &e.groupID},
		model.User{ID: 104, Email: "other@example.test", Password: "$2y$10$hash-other", Token: "token-other", UUID: "uuid-other", TransferEnable: 1000, GroupID: &other},
		model.User{ID: 105, Email: "no-transfer@example.test", Password: "$2y$10$hash-none", Token: "token-none", UUID: "uuid-none", GroupID: &e.groupID},
	)
	require.NoError(t, e.db.Create(&users).Error)
	ids := make([]uint, 0, len(users))
	for _, user := range users {
		ids = append(ids, user.ID)
	}
	require.NoError(t, subscriber.RecordChangesTx(e.db, ids, false, time.Now()))
}

// legacyUserIDs is the set the v2board GetUsers RPC gives the node.
func (e *agentUsersEnvironment) legacyUserIDs(t *testing.T) []uint64 {
	t.Helper()
	response, err := NewUserGRPCServer().GetUsers(withNodeCaller(context.Background(), e.nodeID()), &pb.UserListRequest{NodeId: e.nodeID()})
	require.NoError(t, err)
	ids := make([]uint64, 0, len(response.Users))
	for _, user := range response.Users {
		ids = append(ids, uint64(user.Id))
	}
	return ids
}

func (e *agentUsersEnvironment) cursor(t *testing.T) uint64 {
	t.Helper()
	cursor, err := subscriber.Cursor(e.db)
	require.NoError(t, err)
	return cursor
}

// openUsersSession opens a stream whose Hello lists capabilities and carries
// cursor, and returns it with the HelloAck and a cancel.
func (e *agentUsersEnvironment) openUsersSession(t *testing.T, capabilities []*agentv1pb.Capability, cursor uint64) (agentv1pb.AgentControlService_ControlStreamClient, *agentv1pb.HelloAck, context.CancelFunc) {
	t.Helper()
	ctx := metadata.AppendToOutgoingContext(context.Background(), "x-node-id", strconv.FormatUint(uint64(e.node.ID), 10), "x-api-key", e.apiKey)
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	t.Cleanup(cancel)
	stream, err := agentv1pb.NewAgentControlServiceClient(e.conn).ControlStream(ctx)
	require.NoError(t, err)
	hello := validAgentHello(e.nodeID())
	hello.GetHello().Capabilities = capabilities
	hello.GetHello().UsersCursor = cursor
	require.NoError(t, stream.Send(hello))
	message, err := stream.Recv()
	require.NoError(t, err)
	helloAck := message.GetHelloAck()
	require.NotNil(t, helloAck, "got %T", message.Payload)
	return stream, helloAck, cancel
}

func usersCapabilities() []*agentv1pb.Capability {
	return []*agentv1pb.Capability{
		{Name: "agent.ping", Version: agentcontrol.CapabilityVersionV1},
		{Name: agentcontrol.CapabilityUsers, Version: agentcontrol.CapabilityVersionV1},
	}
}

// recvDelta receives the next message, which must carry a UserDelta.
func recvDelta(t *testing.T, stream agentv1pb.AgentControlService_ControlStreamClient) *agentv1pb.UserDelta {
	t.Helper()
	message, err := stream.Recv()
	require.NoError(t, err)
	require.NotNil(t, message.GetUsers(), "got %T", message.Payload)
	assert.NotEmpty(t, message.RequestId)
	return message.GetUsers()
}

// recvPages receives UserDelta messages up to and including the one with
// last_page.
func recvPages(t *testing.T, stream agentv1pb.AgentControlService_ControlStreamClient) []*agentv1pb.UserDelta {
	t.Helper()
	var pages []*agentv1pb.UserDelta
	for {
		delta := recvDelta(t, stream)
		pages = append(pages, delta)
		if delta.LastPage {
			return pages
		}
	}
}

func upsertIDs(pages []*agentv1pb.UserDelta) []uint64 {
	var ids []uint64
	for _, page := range pages {
		for _, user := range page.Upserts {
			ids = append(ids, user.UserId)
		}
	}
	return ids
}

// requireHeartbeatAckNext sends a heartbeat and requires its ack to be the
// next message: nothing was pushed before it.
func requireHeartbeatAckNext(t *testing.T, stream agentv1pb.AgentControlService_ControlStreamClient, nodeID uint32, sessionID string) {
	t.Helper()
	// A delta in flight would have been read by now: give the sender a
	// few polls.
	time.Sleep(100 * time.Millisecond)
	require.NoError(t, stream.Send(&agentv1pb.AgentToControl{
		RequestId: "heartbeat-request", NodeId: nodeID, SentAtUnixMs: time.Now().UnixMilli(),
		Payload: &agentv1pb.AgentToControl_Heartbeat{Heartbeat: &agentv1pb.Heartbeat{SessionId: sessionID, UptimeSeconds: 1}},
	}))
	message, err := stream.Recv()
	require.NoError(t, err)
	require.NotNil(t, message.GetHeartbeatAck(), "got %T", message.Payload)
}

// An Agent without users.v1 gets nothing new: the server lists the
// capability, but no delta is sent, as before A2-4.
func TestAgentControlUsersNotSentWithoutCapability(t *testing.T) {
	environment := newAgentUsersEnvironment(t, func(t *testing.T) { requireInMemoryDatabase(t) })
	environment.seedUsers(t, 3)
	stream, helloAck, _ := environment.openUsersSession(t, []*agentv1pb.Capability{{Name: "agent.ping", Version: "v1"}}, 0)
	assert.True(t, agentcontrol.HasCapabilityVersion(helloAck.ServerCapabilities, agentcontrol.CapabilityUsers, agentcontrol.CapabilityVersionV1))
	assert.False(t, agentcontrol.Negotiated([]*agentv1pb.Capability{{Name: "agent.ping", Version: "v1"}}, helloAck.ServerCapabilities, agentcontrol.CapabilityUsers))
	requireHeartbeatAckNext(t, stream, environment.nodeID(), helloAck.SessionId)
	require.NoError(t, stream.CloseSend())
}

// A cursor of 0, one ahead of the log and one older than the log each get
// the whole user set in pages; the set is the legacy pull's. A cursor the
// log still covers gets the changes after it.
func TestAgentControlUsersResyncAndResume(t *testing.T) {
	forEachUsersDatabase(t, func(t *testing.T, environment *agentUsersEnvironment) {
		environment.seedUsers(t, 5)
		latest := environment.cursor(t)
		legacy := environment.legacyUserIDs(t)
		require.Equal(t, []uint64{1, 2, 3, 4, 5}, legacy)

		for _, test := range []struct {
			name   string
			cursor uint64
		}{{"no cursor", 0}, {"cursor ahead of the log", latest + 10}} {
			t.Run(test.name, func(t *testing.T) {
				stream, helloAck, cancel := environment.openUsersSession(t, usersCapabilities(), test.cursor)
				defer cancel()
				assert.True(t, agentcontrol.Negotiated(usersCapabilities(), helloAck.ServerCapabilities, agentcontrol.CapabilityUsers))
				pages := recvPages(t, stream)
				require.Len(t, pages, 3, "5 users in pages of 2")
				for i, page := range pages {
					assert.True(t, page.Full, "page %d", i)
					assert.Equal(t, i == 2, page.LastPage, "page %d", i)
					assert.Equal(t, latest, page.Cursor, "page %d", i)
					assert.Empty(t, page.RemovedUserIds)
				}
				assert.Equal(t, legacy, upsertIDs(pages), "the deltas and the legacy pull give the node the same users")
				for _, user := range pages[0].Upserts {
					assert.Equal(t, "uuid-"+strconv.FormatUint(user.UserId, 10), user.Uuid)
					assert.Equal(t, int64(100), user.SpeedLimitMbps)
					assert.Equal(t, int32(3), user.DeviceLimit)
					assert.Empty(t, user.ExtraJson)
				}
				requireHeartbeatAckNext(t, stream, environment.nodeID(), helloAck.SessionId)
			})
		}

		// Prune the first three changes: a cursor before them is stale, one
		// just before the oldest remaining row resumes.
		var changes []model.SubscriberChange
		require.NoError(t, environment.db.Order("id").Find(&changes).Error)
		require.Len(t, changes, 10)
		require.NoError(t, environment.db.Where("id <= ?", changes[2].ID).Delete(&model.SubscriberChange{}).Error)

		t.Run("stale cursor", func(t *testing.T) {
			stream, _, cancel := environment.openUsersSession(t, usersCapabilities(), changes[1].ID)
			defer cancel()
			pages := recvPages(t, stream)
			require.Len(t, pages, 3)
			assert.True(t, pages[0].Full)
			assert.Equal(t, legacy, upsertIDs(pages))
		})
		t.Run("known cursor", func(t *testing.T) {
			stream, helloAck, cancel := environment.openUsersSession(t, usersCapabilities(), changes[2].ID)
			defer cancel()
			// Changes 4 to 10 remain: users 4, 5 and the five the node does
			// not serve, in batches of 3.
			first := recvDelta(t, stream)
			assert.False(t, first.Full)
			assert.True(t, first.LastPage)
			assert.Equal(t, changes[5].ID, first.Cursor)
			assert.Equal(t, []uint64{4, 5}, upsertIDs([]*agentv1pb.UserDelta{first}))
			assert.Equal(t, []uint64{101}, first.RemovedUserIds)
			second := recvDelta(t, stream)
			assert.Equal(t, changes[8].ID, second.Cursor)
			assert.Empty(t, second.Upserts)
			assert.Equal(t, []uint64{102, 103, 104}, second.RemovedUserIds)
			third := recvDelta(t, stream)
			assert.Equal(t, latest, third.Cursor)
			assert.Equal(t, []uint64{105}, third.RemovedUserIds)
			requireHeartbeatAckNext(t, stream, environment.nodeID(), helloAck.SessionId)
		})
	})
}

// A connected Agent at the latest cursor gets a delta for each change as the
// log advances: a removal for a user the node stops serving, an upsert for a
// new one, and the batches bounded by userDeltaBatch.
func TestAgentControlUsersFollowTheChangeLog(t *testing.T) {
	forEachUsersDatabase(t, func(t *testing.T, environment *agentUsersEnvironment) {
		environment.seedUsers(t, 3)
		stream, helloAck, cancel := environment.openUsersSession(t, usersCapabilities(), environment.cursor(t))
		defer cancel()
		requireHeartbeatAckNext(t, stream, environment.nodeID(), helloAck.SessionId)

		now := time.Now()
		require.NoError(t, environment.db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(&model.User{}).Where("id = ?", 2).Update("banned", 1).Error; err != nil {
				return err
			}
			if err := tx.Create(&model.User{ID: 9, Email: "new@example.test", Password: "$2y$10$hash-new", Token: "token-new", UUID: "uuid-new", TransferEnable: 1000, GroupID: &environment.groupID}).Error; err != nil {
				return err
			}
			return subscriber.RecordChangesTx(tx, []uint{2, 9}, false, now)
		}))
		delta := recvDelta(t, stream)
		assert.False(t, delta.Full)
		assert.True(t, delta.LastPage)
		assert.Equal(t, environment.cursor(t), delta.Cursor)
		assert.Equal(t, []uint64{9}, upsertIDs([]*agentv1pb.UserDelta{delta}))
		assert.Equal(t, []uint64{2}, delta.RemovedUserIds)
		assert.Equal(t, []uint64{1, 3, 9}, environment.legacyUserIDs(t))

		// Five changes at once, in batches of 3: a user changed twice is
		// sent once per batch.
		require.NoError(t, subscriber.RecordChangesTx(environment.db, []uint{1, 1, 3, 9, 104}, false, now))
		first := recvDelta(t, stream)
		assert.Equal(t, []uint64{1, 3}, upsertIDs([]*agentv1pb.UserDelta{first}))
		assert.Empty(t, first.RemovedUserIds)
		second := recvDelta(t, stream)
		assert.Equal(t, []uint64{9}, upsertIDs([]*agentv1pb.UserDelta{second}))
		assert.Equal(t, []uint64{104}, second.RemovedUserIds)
		assert.Equal(t, environment.cursor(t), second.Cursor)
		requireHeartbeatAckNext(t, stream, environment.nodeID(), helloAck.SessionId)
	})
}

// An Agent that loses the stream in the middle of a resync has no cursor to
// resume from: it reconnects with the one it had and gets the whole set
// again. With the cursor of a completed resync, nothing is resent.
func TestAgentControlUsersReconnectMidResync(t *testing.T) {
	environment := newAgentUsersEnvironment(t, func(t *testing.T) { requireInMemoryDatabase(t) })
	environment.seedUsers(t, 5)

	stream, _, cancel := environment.openUsersSession(t, usersCapabilities(), 0)
	first := recvDelta(t, stream)
	require.True(t, first.Full)
	require.False(t, first.LastPage)
	cancel()

	stream, _, cancel = environment.openUsersSession(t, usersCapabilities(), 0)
	defer cancel()
	pages := recvPages(t, stream)
	require.Len(t, pages, 3)
	assert.Equal(t, []uint64{1, 2, 3, 4, 5}, upsertIDs(pages))
	cancel()

	stream, helloAck, cancel := environment.openUsersSession(t, usersCapabilities(), pages[2].Cursor)
	defer cancel()
	requireHeartbeatAckNext(t, stream, environment.nodeID(), helloAck.SessionId)
}

// A session replaced by a newer one for the same node stops; the new one
// gets the set from its own cursor.
func TestAgentControlUsersReplacedSession(t *testing.T) {
	environment := newAgentUsersEnvironment(t, func(t *testing.T) { requireInMemoryDatabase(t) })
	environment.seedUsers(t, 5)
	first, _, cancelFirst := environment.openUsersSession(t, usersCapabilities(), 0)
	defer cancelFirst()
	recvPages(t, first)
	second, _, cancelSecond := environment.openUsersSession(t, usersCapabilities(), 0)
	defer cancelSecond()
	assert.Equal(t, []uint64{1, 2, 3, 4, 5}, upsertIDs(recvPages(t, second)))
	require.NoError(t, first.Send(&agentv1pb.AgentToControl{
		RequestId: "late-heartbeat", NodeId: environment.nodeID(), SentAtUnixMs: time.Now().UnixMilli(),
		Payload: &agentv1pb.AgentToControl_Heartbeat{Heartbeat: &agentv1pb.Heartbeat{SessionId: "stale"}},
	}))
	_, err := first.Recv()
	require.Error(t, err, "the replaced session ends")
}

// A NodeUser carries the id, the uuid, the limits and the protocol extras,
// never the e-mail, the password hash or the subscription token: neither as
// a field of the message nor as a value of any delta, extras included.
func TestAgentControlUsersCarryNoSecrets(t *testing.T) {
	fields := (&agentv1pb.NodeUser{}).ProtoReflect().Descriptor().Fields()
	names := make([]string, 0, fields.Len())
	for i := 0; i < fields.Len(); i++ {
		names = append(names, string(fields.Get(i).Name()))
	}
	assert.ElementsMatch(t, []string{"user_id", "uuid", "speed_limit_mbps", "device_limit", "extra_json"}, names)

	environment := newAgentUsersEnvironment(t, func(t *testing.T) { requireInMemoryDatabase(t) })
	// A WireGuard protocol fills extra_json with the peer fields.
	settings := `{"cidr":"10.9.0.0/24"}`
	require.NoError(t, environment.db.Create(&model.NodeProtocol{NodeID: environment.node.ID, Type: model.ProtocolWireGuard, Enable: 1, Settings: &settings}).Error)
	environment.seedUsers(t, 3)
	var users []model.User
	require.NoError(t, environment.db.Find(&users).Error)
	require.NotEmpty(t, users)

	stream, _, cancel := environment.openUsersSession(t, usersCapabilities(), 0)
	defer cancel()
	pages := recvPages(t, stream)
	assert.Equal(t, []uint64{1, 2, 3}, upsertIDs(pages))
	for _, page := range pages {
		text := prototext.Format(page)
		for _, user := range users {
			for _, secret := range []string{user.Email, user.Password, user.Token} {
				require.NotEmpty(t, secret)
				assert.NotContains(t, text, secret)
			}
		}
		walkStrings(page.ProtoReflect(), func(value string) {
			assert.NotContains(t, value, "@example.test")
			assert.NotContains(t, value, "$2y$")
		})
		for _, user := range page.Upserts {
			assert.Contains(t, string(user.ExtraJson), `"wireguard_peer_ip":"10.9.0.`)
			assert.Contains(t, string(user.ExtraJson), `"wireguard_public_key"`)
		}
	}
}

// walkStrings visits every string and bytes value of a message, recursively.
func walkStrings(message protoreflect.Message, visit func(string)) {
	message.Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		switch {
		case field.IsList():
			list := value.List()
			for i := 0; i < list.Len(); i++ {
				walkValue(field, list.Get(i), visit)
			}
		case field.IsMap():
			value.Map().Range(func(key protoreflect.MapKey, value protoreflect.Value) bool {
				visit(key.String())
				walkValue(field.MapValue(), value, visit)
				return true
			})
		default:
			walkValue(field, value, visit)
		}
		return true
	})
}

func walkValue(field protoreflect.FieldDescriptor, value protoreflect.Value, visit func(string)) {
	switch field.Kind() {
	case protoreflect.StringKind:
		visit(value.String())
	case protoreflect.BytesKind:
		visit(string(value.Bytes()))
	case protoreflect.MessageKind, protoreflect.GroupKind:
		walkStrings(value.Message(), visit)
	}
}

// The metrics count what was sent.
func TestWriteAgentUsersPrometheus(t *testing.T) {
	environment := newAgentUsersEnvironment(t, func(t *testing.T) { requireInMemoryDatabase(t) })
	environment.seedUsers(t, 1)
	before := agentUserMetrics.fullPages.Load()
	resyncsBefore := agentUserMetrics.resyncs[usersResyncNoCursor].Load()
	stream, helloAck, cancel := environment.openUsersSession(t, usersCapabilities(), 0)
	defer cancel()
	recvPages(t, stream)
	requireHeartbeatAckNext(t, stream, environment.nodeID(), helloAck.SessionId)
	assert.Equal(t, before+1, agentUserMetrics.fullPages.Load())
	assert.Equal(t, resyncsBefore+1, agentUserMetrics.resyncs[usersResyncNoCursor].Load())

	var body strings.Builder
	WriteAgentUsersPrometheus(&body)
	text := body.String()
	for _, line := range []string{
		"# TYPE anixops_agent_user_deltas_sent_total counter",
		"anixops_agent_user_deltas_sent_total{kind=\"full\"} " + strconv.FormatUint(before+1, 10),
		"anixops_agent_user_resyncs_total{reason=\"no_cursor\"} " + strconv.FormatUint(resyncsBefore+1, 10),
		"anixops_agent_user_resyncs_total{reason=\"pruned\"} ",
		"anixops_agent_users_sessions 1\n",
		"anixops_agent_users_cursor_lag 0\n",
	} {
		assert.Contains(t, text, line)
	}
}

// Rows pruned after a connected session's cursor make the next read a full
// resync, as a stale cursor does on connect.
func TestAgentControlUsersPrunedMidSessionResyncs(t *testing.T) {
	environment := newAgentUsersEnvironment(t, func(t *testing.T) { requireInMemoryDatabase(t) })
	environment.seedUsers(t, 3)
	cursor := environment.cursor(t)
	stream, helloAck, cancel := environment.openUsersSession(t, usersCapabilities(), cursor)
	defer cancel()
	requireHeartbeatAckNext(t, stream, environment.nodeID(), helloAck.SessionId)

	pruned := agentUserMetrics.resyncs[usersResyncPruned].Load()
	// Three changes land and the first of them is pruned with everything
	// before it, in one transaction: the session finds a gap after its
	// cursor.
	require.NoError(t, environment.db.Transaction(func(tx *gorm.DB) error {
		if err := subscriber.RecordChangesTx(tx, []uint{1, 2, 3}, false, time.Now()); err != nil {
			return err
		}
		return tx.Where("id <= ?", cursor+1).Delete(&model.SubscriberChange{}).Error
	}))
	pages := recvPages(t, stream)
	require.Len(t, pages, 2)
	assert.True(t, pages[0].Full)
	assert.Equal(t, []uint64{1, 2, 3}, upsertIDs(pages))
	assert.Equal(t, environment.cursor(t), pages[1].Cursor)
	assert.Equal(t, pruned+1, agentUserMetrics.resyncs[usersResyncPruned].Load())
	requireHeartbeatAckNext(t, stream, environment.nodeID(), helloAck.SessionId)
}

// The feed wakes a session only for a change after its cursor; a session
// at the latest change waits.
func TestUserChangeFeedWakesPastCursorOnly(t *testing.T) {
	environment := newAgentUsersEnvironment(t, func(t *testing.T) { requireInMemoryDatabase(t) })
	environment.seedUsers(t, 1)
	latest := environment.cursor(t)
	feed := newUserChangeFeed()
	feed.setPoll(5 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	assert.ErrorIs(t, feed.wait(ctx, latest), context.DeadlineExceeded, "nothing after the cursor")

	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, feed.wait(ctx, latest-1), "a change after the cursor")

	// A second waiter is woken by the first one's read.
	woken := make(chan error, 1)
	go func() { woken <- feed.wait(ctx, latest) }()
	time.Sleep(20 * time.Millisecond)
	require.NoError(t, subscriber.RecordChangesTx(environment.db, []uint{1}, false, time.Now()))
	require.NoError(t, feed.wait(ctx, latest))
	require.NoError(t, <-woken)
	feed.mu.Lock()
	assert.Empty(t, feed.waiters)
	feed.mu.Unlock()
}
