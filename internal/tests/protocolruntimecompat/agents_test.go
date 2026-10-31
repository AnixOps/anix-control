package protocolruntimecompat

import (
	"context"
	"crypto/rand"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v5/internal/cache"
	"github.com/AnixOps/anix-control/v5/internal/handler"
	"github.com/AnixOps/anix-control/v5/internal/kernelnodeops"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/AnixOps/anix-control/v5/internal/tests/fakeagent"
	"github.com/AnixOps/anix-control/v5/internal/tests/packagecompat"
	"github.com/AnixOps/anix-control/v5/packages/protocol-runtime/native"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// The node synchronization, Agent Control and agent routes act on the
// agents' live connections, which only the kernel holds. Both sides run on
// one kernel database and reach the same agents: scripted Agent Control
// agents (internal/tests/fakeagent) on the process's stream managers, and
// a WebSocket agent on the kernel's agent handler, whose sessions the
// native side reads through the KernelNodeOps session RPCs and executors.
// Each request goes to the legacy handler, then through the gateway to the
// package host; the answers are the same bytes but for what each
// dispatch draws anew (operation ids and revisions, acknowledgement times,
// task ids). The WebSocket fallback of a diagnostic task that its
// acknowledgement does not reach is covered by internal/handler's tests.

const (
	streamKey = "fake-stream-api-key-0123456789"
	configKey = "fake-config-api-key-0123456789"
	quietKey  = "fake-quiet-api-key-0123456789"
)

type agentsFixture struct {
	t       *testing.T
	db      *gorm.DB
	kernel  *packagecompat.Kernel
	agents  *handler.AgentHandler
	control *fakeagent.Control
	// stream, config and quiet are the nodes' ids: an agent on the
	// stream, one that negotiated config.v1, none.
	stream, config, quiet    uint
	streamAgent, configAgent *fakeagent.Agent
}

// agentRoutes are the routes the fixture serves, with their legacy
// handlers.
func (f *agentsFixture) routes() []packagecompat.KernelHostRoute {
	nodes := func(method func(*handler.NodeHandler, *gin.Context)) gin.HandlerFunc {
		return func(c *gin.Context) { method(handler.NewNodeHandler(), c) }
	}
	agents := func(method func(*handler.AgentHandler, *gin.Context)) gin.HandlerFunc {
		return func(c *gin.Context) { method(f.agents, c) }
	}
	routes := []struct {
		method, pattern, routeID string
		legacy                   gin.HandlerFunc
	}{
		{"POST", "/api/v2/admin/nodes/:id/sync", native.SyncRouteID, nodes((*handler.NodeHandler).SyncProtocol)},
		{"GET", "/api/v2/admin/nodes/:id/agent-control", native.AgentControlRouteID, nodes((*handler.NodeHandler).GetAgentControlStatus)},
		{"POST", "/api/v2/admin/nodes/:id/agent-control/operations", native.AgentControlOpsRouteID, nodes((*handler.NodeHandler).DispatchAgentControlOperation)},
		{"GET", "/api/v2/admin/agent/list", native.AgentListRouteID, agents((*handler.AgentHandler).ListAgents)},
		{"GET", "/api/v2/admin/agent/monitor", native.AgentMonitorRouteID, agents((*handler.AgentHandler).GetMonitor)},
		{"POST", "/api/v2/admin/agent/tasks", native.AgentTasksCreateRouteID, agents((*handler.AgentHandler).CreateTask)},
		{"POST", "/api/v2/admin/agent/execute", native.AgentExecuteRouteID, agents((*handler.AgentHandler).ExecuteCommand)},
	}
	hostRoutes := make([]packagecompat.KernelHostRoute, 0, len(routes))
	for _, route := range routes {
		routeID := route.routeID
		hostRoutes = append(hostRoutes, packagecompat.KernelHostRoute{
			Method: route.method, Pattern: route.pattern, RouteID: routeID, Legacy: route.legacy,
			Native: func(nodeOps kernelnodeopsv1.KernelNodeOpsClient) pluginhostsdk.NativeHandler {
				return (&native.Service{
					Open:    func(ctx context.Context) (*gorm.DB, error) { return f.db.WithContext(ctx), nil },
					Leased:  packagecompat.Lease(f.db, "protocol-runtime"),
					NodeOps: nodeOps,
				}).Handlers()[routeID]
			},
		})
	}
	return hostRoutes
}

// forEachAgents runs body with a fixture on each backend.
func forEachAgents(t *testing.T, body func(t *testing.T, f *agentsFixture)) {
	models := append([]any{&model.Node{}, &model.NodeProtocol{}, &model.ForwardNode{}, &model.AgentDiagnosticTask{}, &model.OperationLog{}},
		append(model.KernelNodeOperationModels(), &model.EgressRoute{})...)
	packagecompat.ForEachKernelDatabase(t, models, func(t *testing.T, db *gorm.DB) {
		cache.InitMemory()
		require.NoError(t, service.MigrateKernelModelsForTest(db))
		f := &agentsFixture{t: t, db: db, agents: handler.NewAgentHandler()}
		// The Agent Control managers are the process's: node ids of their
		// own keep this fixture's revisions and operations apart.
		var random [2]byte
		_, err := rand.Read(random[:])
		require.NoError(t, err)
		base := uint(20000) + uint(random[0])<<6 + uint(random[1])%64*4
		f.stream, f.config, f.quiet = base, base+1, base+3
		for id, key := range map[uint]string{f.stream: streamKey, f.config: configKey, f.quiet: quietKey} {
			require.NoError(t, db.Create(&model.Node{ID: id, Name: "node-" + strconv.Itoa(int(id)), Host: "198.51.100.1", Port: 443,
				APIKey: key, APIKeyHash: fakeagent.APIKeyHash(key), CreatedAt: seeded, UpdatedAt: seeded}).Error)
		}
		settings := `{"flow":"xtls-rprx-vision"}`
		require.NoError(t, db.Create(&model.NodeProtocol{NodeID: f.config, Name: "vless", Type: model.ProtocolVLESS, Port: 443, Settings: &settings}).Error)

		f.control = fakeagent.StartControl(t, db)
		// The legacy diagnostic routes read the kernel's default sources,
		// as the server registers them at startup.
		previous := kernelnodeops.DefaultAgentSources()
		kernelnodeops.UseAgentStreams(f.control.Streams)
		t.Cleanup(func() {
			kernelnodeops.UseAgentStreams(previous.Streams)
		})
		sources := func() kernelnodeops.AgentSources {
			return kernelnodeops.AgentSources{Streams: f.control.Streams}
		}
		f.kernel = packagecompat.StartKernel(t, packagecompat.KernelOptions{
			DB: db, PackageID: "protocol-runtime", Capabilities: protocolCapabilities(), Agents: sources, Routes: f.routes(),
		})
		body(t, f)
	})
}

func (f *agentsFixture) node(id uint) agentcontrol.AgentNode {
	return agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: uint32(id)} // #nosec G115 -- a test id.
}

// connectStream connects the scripted stream agents.
func (f *agentsFixture) connectStream(script fakeagent.Script) {
	f.t.Helper()
	f.streamAgent = fakeagent.ConnectEnrolled(f.t, f.control, f.node(f.stream), script)
	configScript := script
	configScript.Capabilities = append(append([]string(nil), fakeagent.DefaultCapabilities...), agentcontrol.CapabilityConfig)
	f.configAgent = fakeagent.ConnectEnrolled(f.t, f.control, f.node(f.config), configScript)
}

// compare sends c to the legacy handler, then through the gateway to the
// package host, and requires the same answer; the host answered itself.
func (f *agentsFixture) compare(t *testing.T, method, pattern string, c packagecompat.Case) packagecompat.Result {
	t.Helper()
	c.Principal = admin
	var legacyHandler gin.HandlerFunc
	var routeID string
	for _, route := range f.routes() {
		if route.Method == method && route.Pattern == pattern {
			legacyHandler, routeID = route.Legacy, route.RouteID
		}
	}
	require.NotNil(t, legacyHandler, "%s %s", method, pattern)
	before := f.kernel.LegacyCalls(routeID)
	legacy := packagecompat.ServeLegacy(t, method, pattern, c, legacyHandler)
	native := f.kernel.Serve(t, method, pattern, c)
	require.Equal(t, before, f.kernel.LegacyCalls(routeID), "the package host answered from the legacy handler: %s", native.Body)
	packagecompat.RequireSame(t, c, legacy, native)
	return native
}

func id(value uint) string { return strconv.Itoa(int(value)) }

func TestNodeSyncParity(t *testing.T) {
	forEachAgents(t, func(t *testing.T, f *agentsFixture) {
		pattern := "/api/v2/admin/nodes/:id/sync"
		path := func(node string) string { return "/api/v2/admin/nodes/" + node + "/sync" }
		f.compare(t, "POST", pattern, packagecompat.Case{Name: "a node on the legacy transports", Path: path(id(f.quiet))})
		f.compare(t, "POST", pattern, packagecompat.Case{Name: "an unknown node", Path: path("99")})
		f.compare(t, "POST", pattern, packagecompat.Case{Name: "not a number", Path: path("x")})

		f.connectStream(fakeagent.Script{})
		f.compare(t, "POST", pattern, packagecompat.Case{Name: "node.reload on the stream", Path: path(id(f.stream)),
			Mask: []string{"data.operation_id", "data.revision", "data.ack.operation_id", "data.ack.revision", "data.ack.accepted_at_unix_ms"}})
		// The snapshot carries the stored configuration: the second sync
		// stores the same, at the same revision.
		f.compare(t, "POST", pattern, packagecompat.Case{Name: "a configuration snapshot", Path: path(id(f.config))})

		f.streamAgent.Disconnect()
		require.Eventually(t, func() bool { _, connected := f.control.Streams.Session(f.node(f.stream)); return !connected }, 5*time.Second, 10*time.Millisecond)
		f.compare(t, "POST", pattern, packagecompat.Case{Name: "a node whose agent left", Path: path(id(f.stream))})
	})
}

func TestNodeSyncRefusedParity(t *testing.T) {
	forEachAgents(t, func(t *testing.T, f *agentsFixture) {
		f.connectStream(fakeagent.Script{Ack: func(*agentv1pb.DesiredOperation) (bool, string) { return false, "busy" }})
		f.compare(t, "POST", "/api/v2/admin/nodes/:id/sync", packagecompat.Case{Name: "the agent refuses", Path: "/api/v2/admin/nodes/" + id(f.stream) + "/sync",
			Mask: []string{"data.operation_id", "data.revision", "data.ack.operation_id", "data.ack.revision", "data.ack.accepted_at_unix_ms"}})
	})
}

func TestAgentControlParity(t *testing.T) {
	forEachAgents(t, func(t *testing.T, f *agentsFixture) {
		status := "/api/v2/admin/nodes/:id/agent-control"
		statusPath := func(node string) string { return "/api/v2/admin/nodes/" + node + "/agent-control" }
		operations := "/api/v2/admin/nodes/:id/agent-control/operations"
		operationsPath := func(node string) string { return "/api/v2/admin/nodes/" + node + "/agent-control/operations" }
		dispatched := []string{
			"data.operation.operation_id", "data.operation.revision", "data.operation.deadline_unix_ms",
			"data.ack.operation_id", "data.ack.revision", "data.ack.accepted_at_unix_ms",
		}

		f.compare(t, "GET", status, packagecompat.Case{Name: "not connected", Path: statusPath(id(f.quiet))})
		f.compare(t, "GET", status, packagecompat.Case{Name: "an unknown node", Path: statusPath("99")})
		f.compare(t, "GET", status, packagecompat.Case{Name: "not a number", Path: statusPath("x")})
		f.compare(t, "POST", operations, packagecompat.Case{Name: "a node without a stream", Path: operationsPath(id(f.quiet)), Body: []byte(`{"kind":"agent.ping"}`)})

		f.connectStream(fakeagent.Script{Capabilities: []string{"agent.control", "operation.cancel", "agent.ping", "node.reload"}})
		f.compare(t, "GET", status, packagecompat.Case{Name: "connected, nothing observed", Path: statusPath(id(f.stream))})
		f.compare(t, "POST", operations, packagecompat.Case{Name: "a ping with a payload", Path: operationsPath(id(f.stream)),
			Body: []byte(`{"kind":" agent.ping ","payload":{"source":"admin","n":2},"timeout_seconds":5}`), Mask: dispatched})
		f.compare(t, "POST", operations, packagecompat.Case{Name: "the default timeout", Path: operationsPath(id(f.stream)),
			Body: []byte(`{"kind":"node.reload","timeout_seconds":-3}`), Mask: dispatched})
		// Both sides observed a terminal state of their own operation:
		// they differ in the operation, as each dispatch does.
		require.Eventually(t, func() bool { _, ok := f.control.Streams.Observed(f.node(f.stream)); return ok }, 5*time.Second, 10*time.Millisecond)
		f.compare(t, "GET", status, packagecompat.Case{Name: "connected, an operation observed", Path: statusPath(id(f.stream)),
			Mask: []string{"data.connection.last_seen", "data.connection.desired_revision", "data.connection.observed_revision"}})
		f.compare(t, "POST", operations, packagecompat.Case{Name: "an operation the agent does not advertise", Path: operationsPath(id(f.stream)),
			Body: []byte(`{"kind":"users.reload"}`)})
		f.compare(t, "POST", operations, packagecompat.Case{Name: "an operation the route does not send", Path: operationsPath(id(f.stream)),
			Body: []byte(`{"kind":"agent.upgrade"}`)})
		f.compare(t, "POST", operations, packagecompat.Case{Name: "no kind", Path: operationsPath(id(f.stream)), Body: []byte(`{}`)})
		f.compare(t, "POST", operations, packagecompat.Case{Name: "not JSON", Path: operationsPath(id(f.stream)), Body: []byte(`{"kind":`)})
		f.compare(t, "POST", operations, packagecompat.Case{Name: "an unknown node", Path: operationsPath("99"), Body: []byte(`{"kind":"agent.ping"}`)})
		f.compare(t, "POST", operations, packagecompat.Case{Name: "not a number", Path: operationsPath("x"), Body: []byte(`{"kind":"agent.ping"}`)})
	})
}

func TestAgentListAndMonitorParity(t *testing.T) {
	forEachAgents(t, func(t *testing.T, f *agentsFixture) {
		monitor := "/api/v2/admin/agent/monitor"
		f.compare(t, "GET", "/api/v2/admin/agent/list", packagecompat.Case{Name: "no agent", Path: "/api/v2/admin/agent/list"})
		f.connectStream(fakeagent.Script{})
		// The agents' hello traffic moves their last_seen; both sides must
		// read the settled value.
		time.Sleep(500 * time.Millisecond)
		answer := f.compare(t, "GET", "/api/v2/admin/agent/list", packagecompat.Case{Name: "one stream agent", Path: "/api/v2/admin/agent/list"})
		require.NotContains(t, string(answer.Body), streamKey)

		// Monitor snapshots came from the retired REST channel.
		f.compare(t, "GET", monitor, packagecompat.Case{Name: "no snapshot", Path: monitor + "?node_id=" + id(f.stream)})
		f.compare(t, "GET", monitor, packagecompat.Case{Name: "an unknown node", Path: monitor + "?node_id=99"})
		f.compare(t, "GET", monitor, packagecompat.Case{Name: "node zero", Path: monitor + "?node_id=0"})
		f.compare(t, "GET", monitor, packagecompat.Case{Name: "no node", Path: monitor})
		f.compare(t, "GET", monitor, packagecompat.Case{Name: "not a number", Path: monitor + "?node_id=x"})
	})
}

func TestAgentTasksParity(t *testing.T) {
	forEachAgents(t, func(t *testing.T, f *agentsFixture) {
		tasks, execute := "/api/v2/admin/agent/tasks", "/api/v2/admin/agent/execute"
		sent := []string{"data.task_id", "data.message_id", "data.data.id", "data.data.task_id", "data.data.created_at", "data.data.timestamp"}
		onStream := append(append([]string(nil), sent...), "data.ack.operation_id", "data.ack.revision", "data.ack.accepted_at_unix_ms")
		body := func(node uint, action, params string) []byte {
			return []byte(fmt.Sprintf(`{"node_id":%d,"type":"diagnostic","action":%q,"params":%s,"timeout":20}`, node, action, params))
		}

		f.compare(t, "POST", tasks, packagecompat.Case{Name: "an offline node", Path: tasks, Body: body(f.quiet, "service_status", `{"service":"anix-agent"}`)})
		f.compare(t, "POST", tasks, packagecompat.Case{Name: "a type that is not diagnostic", Path: tasks, Body: []byte(`{"node_id":1,"type":"shell","action":"service_status"}`)})
		f.compare(t, "POST", tasks, packagecompat.Case{Name: "no action", Path: tasks, Body: []byte(`{"node_id":1,"type":"diagnostic"}`)})
		f.compare(t, "POST", tasks, packagecompat.Case{Name: "not JSON", Path: tasks, Body: []byte(`{"node_id":`)})
		f.compare(t, "POST", execute, packagecompat.Case{Name: "execute: no node", Path: execute, Body: []byte(`{"action":"service_status"}`)})

		f.connectStream(fakeagent.Script{})
		f.compare(t, "POST", tasks, packagecompat.Case{Name: "on the stream", Path: tasks, Body: body(f.stream, "service_status", `{"service":"anix-agent","extra":1}`), Mask: onStream})
		f.compare(t, "POST", execute, packagecompat.Case{Name: "execute on the stream", Path: execute,
			Body: []byte(fmt.Sprintf(`{"node_id":%d,"action":"log_tail","params":{"service":"anix-agent","lines":5000}}`, f.stream)), Mask: onStream})
		f.compare(t, "POST", tasks, packagecompat.Case{Name: "an action off the whitelist", Path: tasks, Body: body(f.stream, "rm -rf /", `{}`)})
		f.compare(t, "POST", tasks, packagecompat.Case{Name: "parameters the whitelist refuses", Path: tasks, Body: body(f.stream, "service_status", `{"service":"sshd"}`)})
	})
}
