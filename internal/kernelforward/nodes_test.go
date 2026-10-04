package kernelforward

import (
	"context"
	"errors"
	"testing"
	"time"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/validate"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

func TestNodeRegistry(t *testing.T)         { runNodeRegistry(t, openSQLite(t)) }
func TestPostgresNodeRegistry(t *testing.T) { runNodeRegistry(t, openPostgres(t)) }

func summaryByRef(t *testing.T, nodes []*forwardv1.NodeSummary, ref string) *forwardv1.NodeSummary {
	t.Helper()
	for _, node := range nodes {
		if node.GetNodeRef() == ref {
			return node
		}
	}
	t.Fatalf("%s not listed", ref)
	return nil
}

func runNodeRegistry(t *testing.T, db *gorm.DB) {
	f := newFixture(t, db)
	ctx := f.ctx

	// The inventory: every forward node row, and the proxy nodes with
	// forwarding settings or a forward.v1 Hello.
	nodes, err := f.service.ListNodes(ctx, NodeFilter{})
	require.NoError(t, err)
	refs := make([]string, 0, len(nodes))
	for _, node := range nodes {
		refs = append(refs, node.GetNodeRef())
	}
	assert.Equal(t, []string{"forward-11", "forward-12", "forward-13"}, refs)
	entrySummary := summaryByRef(t, nodes, "forward-11")
	assert.True(t, entrySummary.GetInInventory())
	assert.True(t, entrySummary.GetNegotiated())
	assert.Equal(t, []string{"192.0.2.11"}, entrySummary.GetInfo().GetAddresses())
	assert.Equal(t, []uint32{22, 7000, 7001}, entrySummary.GetReservedPorts())
	assert.Equal(t, forwardv1.NodeTransport_NODE_TRANSPORT_AGENT, entrySummary.GetRecord().GetTransport())
	assert.False(t, summaryByRef(t, nodes, "forward-13").GetInInventory(), "a disabled node is left out")

	// A proxy node joins with settings.
	answer, err := f.service.SetNodeSettingsAnswer(ctx, "proxy-21", NodeSettings{PortFirst: 40000, PortLast: 40100, Addresses: []string{"203.0.113.21"}, Labels: map[string]string{"link": "iepl"}})
	require.NoError(t, err)
	assert.Empty(t, answer.GetViolations())
	assert.True(t, answer.GetNode().GetInInventory())
	assert.Equal(t, "proxy", answer.GetNode().GetKind())
	assert.Equal(t, uint32(40000), answer.GetNode().GetSettings().GetPortRange().GetFirst())
	assert.Equal(t, []uint32{22, 443, 30443}, answer.GetNode().GetReservedPorts())
	proxies, err := f.service.ListNodes(ctx, NodeFilter{Kind: "proxy"})
	require.NoError(t, err)
	require.Len(t, proxies, 1)
	_, err = f.service.SetNodeSettingsAnswer(ctx, "proxy-99", NodeSettings{})
	assert.ErrorIs(t, err, ErrNodeNotFound)
	_, err = f.service.SetNodeSettingsAnswer(ctx, "proxy-21", NodeSettings{PortFirst: 5})
	assert.ErrorIs(t, err, ErrInvalidRequest)
	_, err = f.service.ListNodes(ctx, NodeFilter{Kind: "router"})
	assert.ErrorIs(t, err, ErrInvalidRequest)

	// Create: Control assigns the id; an Ansible machine is a tagged relay.
	created, err := f.service.CreateForwardNode(ctx, "node-1", &forwardv1.ForwardNodeRecord{
		Name: "sg-relay", Host: "192.0.2.31", Transport: forwardv1.NodeTransport_NODE_TRANSPORT_ANSIBLE, Port: 22,
	}, &forwardv1.NodeSettings{PortRange: &forwardv1.PortRange{First: 31000, Last: 31999}})
	require.NoError(t, err)
	assert.Equal(t, "relay", created.GetRecord().GetRole())
	assert.Equal(t, forwardv1.NodeTransport_NODE_TRANSPORT_ANSIBLE, created.GetRecord().GetTransport())
	assert.True(t, created.GetEnabled())
	assert.True(t, created.GetInInventory(), "settings put a new node in the inventory")
	var row model.ForwardNode
	require.NoError(t, db.First(&row, created.GetRecord().GetId()).Error)
	assert.Empty(t, row.APIToken, "an Ansible machine has no credential")
	assert.Contains(t, row.Tags, ansibleTag)

	again, err := f.service.CreateForwardNode(ctx, "node-1", &forwardv1.ForwardNodeRecord{
		Name: "sg-relay", Host: "192.0.2.31", Transport: forwardv1.NodeTransport_NODE_TRANSPORT_ANSIBLE, Port: 22,
	}, &forwardv1.NodeSettings{PortRange: &forwardv1.PortRange{First: 31000, Last: 31999}})
	require.NoError(t, err)
	assert.Equal(t, created.GetNodeRef(), again.GetNodeRef(), "a retry answers the recorded node")
	_, err = f.service.CreateForwardNode(ctx, "node-1", &forwardv1.ForwardNodeRecord{Name: "other", Host: "192.0.2.32"}, nil)
	assert.ErrorIs(t, err, ErrRequestConflict)

	agentNode, err := f.service.CreateForwardNode(ctx, "node-2", &forwardv1.ForwardNodeRecord{Name: "jp-exit", Host: "192.0.2.41", Role: "exit"}, nil)
	require.NoError(t, err)
	assert.False(t, agentNode.GetInInventory(), "a node without settings joins with its Agent's Hello")
	var agentRow model.ForwardNode
	require.NoError(t, db.First(&agentRow, agentNode.GetRecord().GetId()).Error)
	assert.Len(t, agentRow.APIToken, 32, "an Agent node keeps a legacy credential no answer carries")

	for _, bad := range []*forwardv1.ForwardNodeRecord{
		{Host: "192.0.2.1"},
		{Name: "x"},
		{Name: "x", Host: "a b"},
		{Name: "x", Host: "h", Role: "entry"},
		{Name: "x", Host: "h", Port: 70000},
		{Name: "x", Host: "h", Role: "exit", Transport: forwardv1.NodeTransport_NODE_TRANSPORT_ANSIBLE},
		{Id: 5, Name: "x", Host: "h"},
	} {
		_, err := f.service.CreateForwardNode(ctx, "bad", bad, nil)
		assert.ErrorIs(t, err, ErrInvalidRequest, "%v", bad)
	}
	ansibleOnly, err := f.service.ListNodes(ctx, NodeFilter{Transport: forwardv1.NodeTransport_NODE_TRANSPORT_ANSIBLE})
	require.NoError(t, err)
	require.Len(t, ansibleOnly, 1)
	assert.Equal(t, created.GetNodeRef(), ansibleOnly[0].GetNodeRef())

	// A node a route uses can be neither disabled nor deleted.
	route := f.create("route-1", twoHop(0))
	exitRecord := summaryByRef(t, mustList(t, f.service), "forward-12").GetRecord()
	exitRecord.Enabled = false
	_, err = f.service.UpdateForwardNode(ctx, "node-3", exitRecord)
	var refusal *RefusedError
	require.True(t, errors.As(err, &refusal))
	assert.True(t, refusal.Precondition)
	require.Len(t, refusal.Violations, 1)
	assert.Equal(t, route.GetId(), refusal.Violations[0].RouteID)
	assert.Equal(t, validate.CodeNodeInUse, refusal.Violations[0].Code)
	assert.Equal(t, "hops[1].node_refs[0]", refusal.Violations[0].Field)
	err = f.service.DeleteForwardNode(ctx, "node-4", 12)
	require.True(t, errors.As(err, &refusal))

	// Other edits replan: a new host moves the node's address.
	exitRecord.Enabled = true
	exitRecord.Host = "192.0.2.112"
	updated, err := f.service.UpdateForwardNode(ctx, "node-5", exitRecord)
	require.NoError(t, err)
	assert.Empty(t, updated.GetViolations())
	assert.Equal(t, []string{"192.0.2.112"}, updated.GetNode().GetInfo().GetAddresses())
	entryState := f.state(entry)
	require.Len(t, entryState.GetHops(), 1)
	assert.Equal(t, "192.0.2.112", entryState.GetHops()[0].GetUpstreams()[0].GetAddress())

	// GetNode answers the desired state and the latest report.
	_, err = f.service.RecordReport(ctx, exit, &forwardv1.NodeForwardReport{
		NodeRef: "forward-12", Generation: f.state(exit).GetGeneration(), StateHash: f.state(exit).GetStateHash(), Applied: true,
		ObservedAtUnixMs: f.clock.Now().UnixMilli(),
	}, f.clock.Now(), f.clock.Now())
	require.NoError(t, err)
	detail, err := f.service.GetNode(ctx, "forward-12")
	require.NoError(t, err)
	assert.Equal(t, detail.GetState().GetGeneration(), detail.GetNode().GetDesiredGeneration())
	assert.Equal(t, uint32(1), detail.GetNode().GetDesiredHops())
	assert.True(t, detail.GetNode().GetReported())
	assert.True(t, detail.GetNode().GetApplied())
	assert.Equal(t, detail.GetNode().GetDesiredGeneration(), detail.GetNode().GetReportedGeneration())
	assert.NotNil(t, detail.GetReport())
	_, err = f.service.GetNode(ctx, "forward-404")
	assert.ErrorIs(t, err, ErrNodeNotFound)
	_, err = f.service.GetNode(ctx, "router-1")
	assert.ErrorIs(t, err, ErrInvalidRequest)

	// Delete removes the node and its inventory rows once no route uses it.
	require.NoError(t, f.service.DeleteRoute(ctx, "route-del", route.GetId()))
	require.NoError(t, f.service.DeleteForwardNode(ctx, "node-6", 12))
	require.NoError(t, f.service.DeleteForwardNode(ctx, "node-6", 12), "a retry answers the recorded delete")
	var count int64
	require.NoError(t, db.Model(&model.KernelForwardNode{}).Where("node_ref = ?", "forward-12").Count(&count).Error)
	assert.Zero(t, count)
	err = f.service.DeleteForwardNode(ctx, "node-7", 12)
	assert.ErrorIs(t, err, ErrNodeNotFound)
}

func mustList(t *testing.T, svc *Service) []*forwardv1.NodeSummary {
	t.Helper()
	nodes, err := svc.ListNodes(context.Background(), NodeFilter{})
	require.NoError(t, err)
	return nodes
}

func TestTrafficBuckets(t *testing.T)         { runTrafficBuckets(t, openSQLite(t)) }
func TestPostgresTrafficBuckets(t *testing.T) { runTrafficBuckets(t, openPostgres(t)) }

func runTrafficBuckets(t *testing.T, db *gorm.DB) {
	f := newFixture(t, db)
	route := f.create("route-1", twoHop(0))
	report := func(up uint64) {
		state := f.state(entry)
		_, err := f.service.RecordReport(f.ctx, entry, &forwardv1.NodeForwardReport{
			NodeRef: "forward-11", Generation: state.GetGeneration(), StateHash: state.GetStateHash(), Applied: true,
			Counters:         []*forwardv1.Counters{{RouteId: route.GetId(), NodeRef: "forward-11", UpBytes: up, DownBytes: 2 * up, CounterEpoch: "e1"}},
			ObservedAtUnixMs: f.clock.Now().UnixMilli(),
		}, f.clock.Now(), f.clock.Now())
		require.NoError(t, err)
	}
	report(100)
	f.clock.Advance(time.Hour)
	report(250)
	buckets, truncated, err := f.service.TrafficBuckets(f.ctx, route.GetId(), "", start.Add(-time.Hour), start.Add(3*time.Hour))
	require.NoError(t, err)
	assert.False(t, truncated)
	require.Len(t, buckets, 2)
	assert.Equal(t, uint64(100), buckets[0].GetUpBytes())
	assert.Equal(t, uint64(150), buckets[1].GetUpBytes())
	assert.Equal(t, uint64(300), buckets[1].GetDownBytes())
	assert.Equal(t, start.Truncate(time.Hour).UnixMilli(), buckets[0].GetHourStartUnixMs())
	byNode, _, err := f.service.TrafficBuckets(f.ctx, "", "forward-12", start.Add(-time.Hour), start.Add(3*time.Hour))
	require.NoError(t, err)
	assert.Empty(t, byNode)
	defaults, _, err := f.service.TrafficBuckets(f.ctx, "", "", time.Time{}, time.Time{})
	require.NoError(t, err)
	assert.Len(t, defaults, 2, "the default window is the last 24 hours")
	_, _, err = f.service.TrafficBuckets(f.ctx, "", "", start, start.Add(32*24*time.Hour))
	assert.ErrorIs(t, err, ErrInvalidRequest)

	enforced, err := f.service.Enforcement(f.ctx, []string{route.GetId()})
	require.NoError(t, err)
	assert.Empty(t, enforced)
}

// TestServerNodeMethods covers the new ForwardControl methods' codes and
// details.
func TestServerNodeMethods(t *testing.T) {
	f := newFixture(t, openSQLite(t))
	server := (&Server{Service: f.service, Authorizer: grants{service.CapabilityForward: true}}).For(host)
	ctx := f.ctx

	listed, err := server.ListNodes(ctx, &forwardv1.ListNodesRequest{Kind: "forward"})
	require.NoError(t, err)
	assert.Len(t, listed.GetNodes(), 3)
	_, err = server.GetNode(ctx, &forwardv1.GetNodeRequest{NodeRef: "forward-77"})
	assert.Equal(t, codes.NotFound, status.Code(err))
	_, err = server.SetNodeSettings(ctx, &forwardv1.SetNodeSettingsRequest{NodeRef: "forward-11", Settings: &forwardv1.NodeSettings{Addresses: []string{"not-ip"}}})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	created, err := server.CreateForwardNode(ctx, &forwardv1.CreateForwardNodeRequest{RequestId: "n1", Node: &forwardv1.ForwardNodeRecord{Name: "n", Host: "192.0.2.9"}})
	require.NoError(t, err)
	assert.Equal(t, "forward", created.GetNode().GetKind())

	route := f.create("route-1", twoHop(0))
	_, err = server.DeleteForwardNode(ctx, &forwardv1.DeleteForwardNodeRequest{RequestId: "d1", Id: 11})
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
	details := status.Convert(err).Details()
	require.Len(t, details, 1)
	answer, ok := details[0].(*forwardv1.DeleteForwardNodeResponse)
	require.True(t, ok)
	require.Len(t, answer.GetViolations(), 1)
	assert.Equal(t, route.GetId(), answer.GetViolations()[0].GetRouteId())
	assert.Equal(t, "node_in_use", answer.GetViolations()[0].GetCode())

	got, err := server.GetRoute(ctx, &forwardv1.GetRouteRequest{RouteId: route.GetId()})
	require.NoError(t, err)
	assert.Empty(t, got.GetEnforced())
	traffic, err := server.GetTraffic(ctx, &forwardv1.GetTrafficRequest{RouteId: route.GetId()})
	require.NoError(t, err)
	assert.Empty(t, traffic.GetBuckets())
	_, err = server.GetTraffic(ctx, &forwardv1.GetTrafficRequest{SinceUnixMs: 10, UntilUnixMs: 5})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}
