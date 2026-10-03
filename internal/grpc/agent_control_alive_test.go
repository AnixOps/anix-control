package grpc

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	"github.com/AnixOps/anix-control/v4/internal/cache"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// stubAliveCounts replaces the alive count source, the sharing window and
// the send interval for one test, and gives the shared counts a fresh
// start. set changes what the source answers.
func stubAliveCounts(t *testing.T, interval, refresh time.Duration) (set func(map[uint64]int, error)) {
	t.Helper()
	var mu sync.Mutex
	counts, failure := map[uint64]int{}, error(nil)
	previousSource, previousInterval, previousRefresh, previousShared := aliveCountSource, aliveListInterval, aliveListRefresh, sharedAliveCounts
	aliveCountSource = func() (map[uint64]int, error) {
		mu.Lock()
		defer mu.Unlock()
		copied := make(map[uint64]int, len(counts))
		for id, count := range counts {
			copied[id] = count
		}
		return copied, failure
	}
	aliveListInterval, aliveListRefresh, sharedAliveCounts = interval, refresh, &aliveCounts{}
	t.Cleanup(func() {
		aliveCountSource, aliveListInterval, aliveListRefresh, sharedAliveCounts = previousSource, previousInterval, previousRefresh, previousShared
	})
	return func(next map[uint64]int, err error) {
		mu.Lock()
		defer mu.Unlock()
		counts, failure = next, err
	}
}

func aliveCapabilities() []*agentv1pb.Capability {
	return []*agentv1pb.Capability{
		{Name: "agent.ping", Version: agentcontrol.CapabilityVersionV1},
		{Name: agentcontrol.CapabilityAlive, Version: agentcontrol.CapabilityVersionV1},
	}
}

// expectAliveList reads the next message, which must be an AliveList.
func expectAliveList(t *testing.T, stream agentv1pb.AgentControlService_ControlStreamClient) *agentv1pb.AliveList {
	t.Helper()
	message, err := stream.Recv()
	require.NoError(t, err)
	list := message.GetAliveList()
	require.NotNil(t, list, "got %T", message.Payload)
	assert.NotEmpty(t, message.RequestId)
	return list
}

func aliveEntries(list *agentv1pb.AliveList) map[uint64]uint32 {
	entries := map[uint64]uint32{}
	for _, entry := range list.GetEntries() {
		entries[entry.GetUserId()] = entry.GetAliveCount()
	}
	return entries
}

// alive.v1 is offered to an agent that lists it, for proxy nodes only, and
// a session sends the list at once and again only when the counts change.
func TestAgentControlAliveListOnTheStream(t *testing.T) {
	set := stubAliveCounts(t, 20*time.Millisecond, 0)
	set(map[uint64]int{9: 2, 7: 1, 8: 0}, nil)
	environment := newAgentControlTestEnvironment(t)
	server := NewAgentControlGRPCServer(NewAgentControlManager())
	proxy := agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: 1}
	assert.True(t, agentcontrol.HasCapabilityVersion(server.serverCapabilities(proxy, aliveCapabilities()), agentcontrol.CapabilityAlive, agentcontrol.CapabilityVersionV1))
	assert.False(t, agentcontrol.HasCapabilityVersion(server.serverCapabilities(agentcontrol.AgentNode{Kind: agentcontrol.NodeKindForward, ID: 1}, aliveCapabilities()),
		agentcontrol.CapabilityAlive, agentcontrol.CapabilityVersionV1), "forward nodes have no users")
	assert.False(t, agentcontrol.HasCapabilityVersion(server.serverCapabilities(proxy, reportCapabilities()), agentcontrol.CapabilityAlive, agentcontrol.CapabilityVersionV1),
		"only an agent that lists it")

	stream, helloAck := openReportsSession(t, environment, aliveCapabilities())
	require.True(t, agentcontrol.Negotiated(aliveCapabilities(), helloAck.ServerCapabilities, agentcontrol.CapabilityAlive))
	first := expectAliveList(t, stream)
	assert.Equal(t, uint64(1), first.Revision)
	assert.True(t, first.LastPage)
	assert.NotZero(t, first.ComputedAtUnixMs)
	require.Len(t, first.Entries, 2, "users without a device online are left out")
	assert.Equal(t, uint64(7), first.Entries[0].UserId, "in user id order")
	assert.Equal(t, map[uint64]uint32{7: 1, 9: 2}, aliveEntries(first))

	set(map[uint64]int{7: 3}, nil)
	second := expectAliveList(t, stream)
	assert.Equal(t, uint64(2), second.Revision)
	assert.Equal(t, map[uint64]uint32{7: 3}, aliveEntries(second))

	// Everyone went offline: an empty list replaces the last one.
	set(map[uint64]int{}, nil)
	third := expectAliveList(t, stream)
	assert.Equal(t, uint64(3), third.Revision)
	assert.True(t, third.LastPage)
	assert.Empty(t, third.Entries)
	require.NoError(t, stream.CloseSend())
}

// A session without alive.v1 never gets a list.
func TestAgentControlAliveListNotSentWithoutTheCapability(t *testing.T) {
	set := stubAliveCounts(t, 10*time.Millisecond, 0)
	set(map[uint64]int{7: 1}, nil)
	environment := newAgentControlTestEnvironment(t)
	stream, helloAck := openReportsSession(t, environment, reportCapabilities())
	assert.False(t, agentcontrol.HasCapability(helloAck.ServerCapabilities, agentcontrol.CapabilityAlive))
	time.Sleep(50 * time.Millisecond)
	nodeID := uint32(environment.node.ID)
	require.NoError(t, stream.Send(heartbeatMessage("heartbeat-1", nodeID, helloAck.SessionId)))
	message, err := stream.Recv()
	require.NoError(t, err)
	assert.NotNil(t, message.GetHeartbeatAck(), "the first message after HelloAck is the heartbeat's answer, got %T", message.Payload)
	require.NoError(t, stream.CloseSend())
}

// The counts are read once per refresh window for all sessions; a failed
// read keeps the last counts, and the generation moves only on a change.
func TestAliveCountsSharedAndStable(t *testing.T) {
	set := stubAliveCounts(t, time.Minute, time.Minute)
	counts := &aliveCounts{}
	set(nil, errors.New("cache down"))
	_, ok := counts.current(time.Now())
	assert.False(t, ok, "no counts yet")

	set(map[uint64]int{7: 1, 0: 4, 8: -1}, nil)
	now := time.Now()
	first, ok := counts.current(now)
	require.True(t, ok)
	assert.Equal(t, uint64(1), first.generation)
	assert.Len(t, first.entries, 1, "user 0 and negative counts are dropped")

	set(map[uint64]int{7: 2}, nil)
	cached, _ := counts.current(now.Add(time.Second))
	assert.Equal(t, first.generation, cached.generation, "within the refresh window nothing is read")

	changed, _ := counts.current(now.Add(2 * time.Minute))
	assert.Equal(t, uint64(2), changed.generation)
	assert.Equal(t, uint32(2), changed.entries[0].AliveCount)

	same, _ := counts.current(now.Add(4 * time.Minute))
	assert.Equal(t, changed.generation, same.generation, "the same counts keep the generation")

	set(nil, errors.New("cache down"))
	kept, ok := counts.current(now.Add(6 * time.Minute))
	assert.True(t, ok)
	assert.Equal(t, changed.generation, kept.generation)
}

// A list larger than a page is split, every page with the revision and
// only the last with last_page; the pages join to the list.
func TestSendAliveListPages(t *testing.T) {
	previous := aliveListPageSize
	aliveListPageSize = 2
	t.Cleanup(func() { aliveListPageSize = previous })
	stream := &recordingControlStream{}
	connection := &AgentControlConnection{NodeID: 3, stream: stream}
	entries := []*agentv1pb.UserAlive{{UserId: 1, AliveCount: 1}, {UserId: 2, AliveCount: 1}, {UserId: 3, AliveCount: 2}, {UserId: 4, AliveCount: 1}, {UserId: 5, AliveCount: 9}}
	require.NoError(t, sendAliveList(connection, 4, aliveSnapshot{generation: 1, computedAt: time.Now(), entries: entries}))
	sent := stream.messages()
	require.Len(t, sent, 3)
	var joined []*agentv1pb.UserAlive
	for index, message := range sent {
		list := message.GetAliveList()
		assert.Equal(t, uint32(3), message.NodeId)
		assert.Equal(t, uint64(4), list.Revision)
		assert.Equal(t, index == len(sent)-1, list.LastPage)
		joined = append(joined, list.Entries...)
	}
	assert.Equal(t, entries, joined)

	empty := &recordingControlStream{}
	require.NoError(t, sendAliveList(&AgentControlConnection{NodeID: 3, stream: empty}, 1, aliveSnapshot{generation: 1}))
	require.Len(t, empty.messages(), 1)
	assert.True(t, empty.messages()[0].GetAliveList().LastPage)

	failing := &recordingControlStream{err: status.Error(codes.Unavailable, "gone")}
	assert.Error(t, sendAliveList(&AgentControlConnection{NodeID: 3, stream: failing}, 1, aliveSnapshot{generation: 1, entries: entries}))
}

// The alive list counts what UpdateOnlineStatus stores, per user across
// nodes: the source of UniProxy alivelist and of alive.v1.
func TestUserAliveCountsFromTheOnlineSets(t *testing.T) {
	cache.InitMemory()
	servers := service.NewServerService()
	require.NoError(t, servers.UpdateOnlineStatus(model.ServerType("vmess"), 1, map[uint][]string{7: {"203.0.113.1", "203.0.113.2"}, 8: {"203.0.113.3"}}))
	require.NoError(t, servers.UpdateOnlineStatus(model.ServerType(""), 2, map[uint][]string{7: {"198.51.100.1"}}))
	counts, err := service.UserAliveCounts()
	require.NoError(t, err)
	assert.Equal(t, map[uint64]int{7: 3, 8: 1}, counts)
	legacy, err := servers.GetAllUsersOnlineCount()
	require.NoError(t, err)
	assert.Equal(t, map[string]int{"7": 3, "8": 1}, legacy, "UniProxy alivelist answers the same counts")
}

// recordingControlStream is a server stream that records what is sent, or
// fails every send with err.
type recordingControlStream struct {
	grpc.ServerStream
	mu   sync.Mutex
	sent []*agentv1pb.ControlToAgent
	err  error
}

func (s *recordingControlStream) Send(message *agentv1pb.ControlToAgent) error {
	if s.err != nil {
		return s.err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, message)
	return nil
}

func (s *recordingControlStream) Recv() (*agentv1pb.AgentToControl, error) { return nil, io.EOF }

func (s *recordingControlStream) Context() context.Context { return context.Background() }

func (s *recordingControlStream) messages() []*agentv1pb.ControlToAgent {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*agentv1pb.ControlToAgent(nil), s.sent...)
}
