package grpc

import (
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v5/internal/cache"
	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/stretchr/testify/require"
)

// Everything that reads last_check_at asks whether a node was seen within 5
// minutes (the stats' active nodes, the subscription's online nodes, the node
// list). The write interval is a minute: three heartbeats of the 20 second
// interval per write, and four writes inside the window.
func TestAgentLivenessWriteIntervalIsFarInsideTheOnlineWindow(t *testing.T) {
	const onlineWindow = 5 * time.Minute
	require.Equal(t, 60*time.Second, AgentLivenessWriteInterval)
	require.LessOrEqual(t, 4*AgentLivenessWriteInterval, onlineWindow)
	require.Greater(t, AgentLivenessWriteInterval, time.Duration(defaultAgentHeartbeatIntervalSeconds)*time.Second,
		"a heartbeat inside the interval is what the throttle drops")
}

type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time { return c.now }

func TestLivenessThrottleAllowsOneWritePerNodeAndInterval(t *testing.T) {
	clock := &fakeClock{now: time.Unix(1_800_000_000, 0)}
	throttle := newLivenessThrottle(AgentLivenessWriteInterval)
	throttle.now = clock.Now
	proxy := agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: 7}
	forward := agentcontrol.AgentNode{Kind: agentcontrol.NodeKindForward, ID: 7}

	require.True(t, throttle.due(proxy), "the first touch writes")
	require.False(t, throttle.due(proxy))
	require.True(t, throttle.due(forward), "a forward node of the same id is another node")
	clock.now = clock.now.Add(AgentLivenessWriteInterval - time.Second)
	require.False(t, throttle.due(proxy))
	clock.now = clock.now.Add(time.Second)
	require.True(t, throttle.due(proxy), "the interval is over")
	require.False(t, throttle.due(proxy))

	throttle.forget(proxy)
	require.True(t, throttle.due(proxy), "a failed write is retried at once")

	clock.now = clock.now.Add(10 * time.Second)
	throttle.written(proxy)
	clock.now = clock.now.Add(AgentLivenessWriteInterval - time.Second)
	require.False(t, throttle.due(proxy), "a write of another kind counts")

	clock.now = clock.now.Add(-time.Hour)
	require.True(t, throttle.due(proxy), "a clock that went back never holds a node out")

	var none *livenessThrottle
	require.True(t, none.due(proxy), "no throttle writes every touch")
	require.True(t, (&livenessThrottle{}).due(proxy))
}

func TestLivenessThrottleForgetsOldEntriesPastItsMemoryLimit(t *testing.T) {
	clock := &fakeClock{now: time.Unix(1_800_000_000, 0)}
	throttle := newLivenessThrottle(time.Minute)
	throttle.now = clock.Now
	for id := uint32(1); id <= livenessMemoryLimit; id++ {
		throttle.due(agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: id})
	}
	clock.now = clock.now.Add(2 * time.Minute)
	require.True(t, throttle.due(agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: livenessMemoryLimit + 1}))
	require.Len(t, throttle.last, 1, "the entries older than the interval were dropped")
}

// touchNode writes a node's liveness once per interval, and a stream's Hello
// always writes.
func TestTouchNodeWritesTheNodeOncePerInterval(t *testing.T) {
	cache.InitMemory()
	requireInMemoryDatabase(t)
	t.Cleanup(func() { requireDatabaseClosed(t) })
	requireAutoMigrate(t, &model.Node{}, &model.ForwardNode{})
	db := database.GetDB()
	require.NoError(t, db.Create(&model.Node{ID: 7, Name: "proxy", Host: "127.0.0.1", APIKey: "k7", Status: model.NodeStatusOffline}).Error)
	require.NoError(t, db.Create(&model.ForwardNode{ID: 7, Name: "forward", Host: "127.0.0.2"}).Error)

	clock := &fakeClock{now: time.Unix(1_800_000_000, 0)}
	server := &AgentControlGRPCServer{nodeService: service.NewNodeService(), liveness: newLivenessThrottle(AgentLivenessWriteInterval)}
	server.liveness.now = clock.Now
	proxy := agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: 7}
	forward := agentcontrol.AgentNode{Kind: agentcontrol.NodeKindForward, ID: 7}

	// What the node row says: seen (a time), and online.
	proxyRow := func() model.Node {
		var node model.Node
		require.NoError(t, db.First(&node, 7).Error)
		return node
	}
	forwardRow := func() model.ForwardNode {
		var node model.ForwardNode
		require.NoError(t, db.First(&node, 7).Error)
		return node
	}
	// Make the next write visible: park the columns on a sentinel.
	reset := func() {
		require.NoError(t, db.Model(&model.Node{}).Where("id = 7").Updates(map[string]any{"last_check_at": 123, "status": model.NodeStatusOffline}).Error)
		require.NoError(t, db.Model(&model.ForwardNode{}).Where("id = 7").Updates(map[string]any{"last_check": time.Unix(123, 0), "status": 0}).Error)
	}

	require.NoError(t, server.touchNode(proxy))
	require.NoError(t, server.touchNode(forward))
	require.NotEqual(t, int64(123), *proxyRow().LastCheckAt)
	require.Equal(t, model.NodeStatusOnline, proxyRow().Status)
	require.Equal(t, model.ForwardNodeStatusOnline, forwardRow().Status)

	reset()
	clock.now = clock.now.Add(20 * time.Second)
	require.NoError(t, server.touchNode(proxy))
	require.NoError(t, server.touchNode(forward))
	require.Equal(t, int64(123), *proxyRow().LastCheckAt, "a heartbeat inside the interval writes nothing")
	require.Equal(t, 0, forwardRow().Status)

	require.NoError(t, server.touchNodeNow(proxy))
	require.NotEqual(t, int64(123), *proxyRow().LastCheckAt, "a Hello always writes")

	reset()
	clock.now = clock.now.Add(AgentLivenessWriteInterval)
	require.NoError(t, server.touchNode(proxy))
	require.NoError(t, server.touchNode(forward))
	require.NotEqual(t, int64(123), *proxyRow().LastCheckAt, "the next interval writes")
	require.Equal(t, model.ForwardNodeStatusOnline, forwardRow().Status)

	// A server without a throttle writes every touch.
	bare := &AgentControlGRPCServer{nodeService: service.NewNodeService()}
	reset()
	require.NoError(t, bare.touchNode(proxy))
	require.NotEqual(t, int64(123), *proxyRow().LastCheckAt)
}

// On a real stream: the Hello writes the node, the heartbeats of the
// following seconds do not.
func TestStreamHeartbeatsInsideTheIntervalDoNotWriteTheNode(t *testing.T) {
	environment := newReportsTestEnvironment(t)
	db := database.GetDB()
	lastCheck := func() int64 {
		var node model.Node
		require.NoError(t, db.First(&node, environment.node.ID).Error)
		require.NotNil(t, node.LastCheckAt)
		return *node.LastCheckAt
	}

	stream, ack := openReportsSession(t, environment, reportCapabilities())
	require.NotEqual(t, int64(123), lastCheck(), "the Hello recorded the node as seen")

	// Park the column on a sentinel: a write would replace it.
	require.NoError(t, db.Model(&model.Node{}).Where("id = ?", environment.node.ID).Update("last_check_at", 123).Error)
	for i := 0; i < 3; i++ {
		expectHeartbeatAck(t, stream, uint32(environment.node.ID), ack.SessionId, "heartbeat-"+string(rune('a'+i)))
	}
	require.Equal(t, int64(123), lastCheck(), "heartbeats inside the interval write nothing")
}
