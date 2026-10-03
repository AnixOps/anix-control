package kernelforward

import (
	"errors"
	"testing"
	"time"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/validate"
	"github.com/AnixOps/anix-control/sdk/forward/wire"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
)

// The planner behind the kernel's tables: a route's lifecycle moves only
// the generations of the nodes it changes, allocations stick, a deleted
// route's port is held for the grace period, and every change reaches the
// state listeners after it committed.
func TestPlanLifecycle(t *testing.T)         { runPlanLifecycle(t, openSQLite(t)) }
func TestPostgresPlanLifecycle(t *testing.T) { runPlanLifecycle(t, openPostgres(t)) }

func runPlanLifecycle(t *testing.T, db *gorm.DB) {
	f := newFixture(t, db)
	listener := listen(t)
	// The two Hellos planned the empty states of the two nodes.
	assert.EqualValues(t, 1, f.state(entry).GetGeneration())
	assert.EqualValues(t, 1, f.state(exit).GetGeneration())
	assert.Empty(t, f.state(entry).GetHops())

	route := f.create("create-1", twoHop(0))
	require.Len(t, route.GetId(), 26)
	assert.EqualValues(t, 1, route.GetRevision())
	assert.Equal(t, start.UnixMilli(), route.GetCreatedAtUnixMs())
	assert.ElementsMatch(t, []string{"forward-11", "forward-12"}, listener.take())

	entryState, exitState := f.state(entry), f.state(exit)
	assert.EqualValues(t, 2, entryState.GetGeneration())
	assert.EqualValues(t, 2, exitState.GetGeneration())
	require.Len(t, entryState.GetHops(), 1)
	require.Len(t, exitState.GetHops(), 1)
	entryHop, exitHop := entryState.GetHops()[0], exitState.GetHops()[0]
	assert.EqualValues(t, DefaultPortFirst, entryHop.GetListen().GetPort(), "the lowest free port of the default range")
	assert.EqualValues(t, DefaultPortFirst, exitHop.GetListen().GetPort())
	require.Len(t, entryHop.GetUpstreams(), 1)
	assert.Equal(t, "192.0.2.12", entryHop.GetUpstreams()[0].GetAddress())
	assert.Equal(t, []string{"192.0.2.11"}, exitHop.GetIngressSources())
	assert.NotZero(t, entryHop.GetMark())
	assert.Len(t, entryState.GetStateHash(), 64)

	allocations := f.allocations()
	require.Len(t, allocations, 2)
	for _, allocation := range allocations {
		assert.Equal(t, route.GetId(), allocation.RouteID)
		assert.EqualValues(t, DefaultPortFirst, allocation.Port)
		assert.NotZero(t, allocation.Mark)
		assert.Nil(t, allocation.ReleasedAt)
	}

	// The configuration member carries the stamped state.
	negotiated, member, err := NodeConfigMember(db, entry)
	require.NoError(t, err)
	require.True(t, negotiated)
	assert.Equal(t, "2", member["generation"])
	assert.Equal(t, "forward-11", member["node_ref"])

	// A target change touches the exit only; the ports stick.
	f.clock.Advance(time.Minute)
	changed := proto.Clone(route).(*forwardv1.Route)
	changed.Targets = append(changed.Targets, &forwardv1.Target{Host: "198.51.100.11", Port: 443})
	updated, err := f.service.UpdateRoute(f.ctx, "update-1", changed, 1)
	require.NoError(t, err)
	assert.EqualValues(t, 2, updated.GetRevision())
	assert.Equal(t, route.GetCreatedAtUnixMs(), updated.GetCreatedAtUnixMs())
	assert.Equal(t, f.clock.Now().UnixMilli(), updated.GetUpdatedAtUnixMs())
	assert.Equal(t, []string{"forward-12"}, listener.take())
	assert.EqualValues(t, 2, f.state(entry).GetGeneration())
	assert.EqualValues(t, 3, f.state(exit).GetGeneration())
	assert.Len(t, f.state(exit).GetHops()[0].GetUpstreams(), 2)
	assert.Equal(t, allocations, withoutTimes(f.allocations(), allocations))

	// Re-planning identical input bumps nothing.
	outcome, err := f.service.Replan(f.ctx, "test")
	require.NoError(t, err)
	assert.False(t, outcome.Refused)
	assert.Empty(t, outcome.Changed)
	assert.Empty(t, listener.take())

	// Delete: both nodes get an empty state, the ports are held.
	require.NoError(t, f.service.DeleteRoute(f.ctx, "delete-1", route.GetId()))
	assert.ElementsMatch(t, []string{"forward-11", "forward-12"}, listener.take())
	assert.Empty(t, f.state(entry).GetHops())
	assert.EqualValues(t, 3, f.state(entry).GetGeneration())
	for _, allocation := range f.allocations() {
		require.NotNil(t, allocation.ReleasedAt)
	}
	_, err = f.service.GetRoute(f.ctx, route.GetId())
	require.ErrorIs(t, err, ErrNotFound)

	// Within the grace period the port is taken...
	_, err = f.service.CreateRoute(f.ctx, "create-2", twoHop(DefaultPortFirst))
	var refusal *RefusedError
	require.ErrorAs(t, err, &refusal)
	assert.True(t, refusal.Precondition)
	assert.Equal(t, validate.CodePortInUse, refusal.Violations[0].Code)
	assert.Empty(t, refusal.Violations[0].RouteID, "the new route has no id yet")
	// ...and an allocated port moves past it.
	other := f.create("create-3", twoHop(0))
	assert.EqualValues(t, DefaultPortFirst+1, f.state(entry).GetHops()[0].GetListen().GetPort())
	// After the grace period the port is free again.
	f.clock.Advance(ReleaseGrace + time.Second)
	again := f.create("create-4", twoHop(DefaultPortFirst))
	assert.NotEqual(t, other.GetId(), again.GetId())
	var held int64
	require.NoError(t, db.Model(&model.KernelForwardAllocation{}).Where("released_at IS NOT NULL").Count(&held).Error)
	assert.Zero(t, held, "the next plan dropped allocations past their grace period")
	status, err := f.service.PlanStatus(f.ctx)
	require.NoError(t, err)
	assert.False(t, status.Refused)
	assert.Positive(t, status.Revision)
}

// withoutTimes answers rows with the update times of want, for comparing
// allocations across plans.
func withoutTimes(rows, want []model.KernelForwardAllocation) []model.KernelForwardAllocation {
	for i := range rows {
		if i < len(want) {
			rows[i].UpdatedAt = want[i].UpdatedAt
		}
	}
	return rows
}

// Route writes: validation, the request ledger, revisions and the
// classification of refusals.
func TestRouteWrites(t *testing.T)         { runRouteWrites(t, openSQLite(t)) }
func TestPostgresRouteWrites(t *testing.T) { runRouteWrites(t, openPostgres(t)) }

func runRouteWrites(t *testing.T, db *gorm.DB) {
	f := newFixture(t, db)
	route := f.create("create-1", twoHop(31000))

	// A retry answers the recorded route and stores nothing new.
	replayed, err := f.service.CreateRoute(f.ctx, "create-1", twoHop(31000))
	require.NoError(t, err)
	assert.True(t, proto.Equal(route, replayed))
	var count int64
	require.NoError(t, db.Model(&model.KernelForwardRoute{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
	// The same request id for another request conflicts.
	_, err = f.service.CreateRoute(f.ctx, "create-1", twoHop(31001))
	require.ErrorIs(t, err, ErrRequestConflict)

	for name, call := range map[string]func() error{
		"no request id": func() error { _, err := f.service.CreateRoute(f.ctx, " ", twoHop(0)); return err },
		"long request id": func() error {
			_, err := f.service.CreateRoute(f.ctx, string(make([]byte, MaxRequestID+1)), twoHop(0))
			return err
		},
		"no route": func() error { _, err := f.service.CreateRoute(f.ctx, "x", nil); return err },
		"id on create": func() error {
			r := twoHop(0)
			r.Id = route.GetId()
			_, err := f.service.CreateRoute(f.ctx, "x", r)
			return err
		},
		"update without id": func() error { _, err := f.service.UpdateRoute(f.ctx, "x", twoHop(0), 1); return err },
		"update without revision": func() error {
			_, err := f.service.UpdateRoute(f.ctx, "x", route, 0)
			return err
		},
		"delete without id": func() error { return f.service.DeleteRoute(f.ctx, "x", "") },
		"plan without route": func() error {
			_, err := f.service.PlanRoute(f.ctx, &forwardv1.PlanRouteRequest{})
			return err
		},
	} {
		require.ErrorIs(t, call(), ErrInvalidRequest, name)
	}

	// A malformed route is INVALID_ARGUMENT, with its fields.
	bad := twoHop(0)
	bad.Owner = "root"
	bad.Targets[0].Port = 0
	_, err = f.service.CreateRoute(f.ctx, "bad", bad)
	var refusal *RefusedError
	require.ErrorAs(t, err, &refusal)
	assert.False(t, refusal.Precondition)
	fields := map[string]bool{}
	for _, v := range refusal.ProtoViolations() {
		fields[v.GetField()] = true
	}
	assert.True(t, fields["owner"] && fields["targets[0].port"], fields)
	// An expired route cannot be created.
	expired := twoHop(0)
	expired.Limits = &forwardv1.Limits{ExpiresAtUnixMs: start.Add(-time.Hour).UnixMilli()}
	_, err = f.service.CreateRoute(f.ctx, "expired", expired)
	require.ErrorAs(t, err, &refusal)
	assert.Equal(t, validate.CodeExpired, refusal.Violations[0].Code)
	// A node the inventory does not have, or a disabled one, is a
	// precondition.
	for _, ref := range []string{"forward-99", "forward-13"} {
		unknown := twoHop(0)
		unknown.Hops[1].NodeRefs = []string{ref}
		_, err = f.service.CreateRoute(f.ctx, "unknown-"+ref, unknown)
		require.ErrorAs(t, err, &refusal, ref)
		assert.True(t, refusal.Precondition, ref)
	}
	// A refused request is not recorded: a fixed retry applies.
	fixed := f.create("bad", twoHop(0))
	assert.NotEmpty(t, fixed.GetId())

	// Updates: not found, stale revision, retry.
	missing := proto.Clone(route).(*forwardv1.Route)
	missing.Id = "01JF1A000000000000000000ZZ"
	_, err = f.service.UpdateRoute(f.ctx, "u-missing", missing, 1)
	require.ErrorIs(t, err, ErrNotFound)
	_, err = f.service.UpdateRoute(f.ctx, "u-stale", route, 7)
	require.ErrorIs(t, err, ErrRevisionMismatch)
	renamed := proto.Clone(route).(*forwardv1.Route)
	renamed.Name = "renamed"
	first, err := f.service.UpdateRoute(f.ctx, "u-1", renamed, 1)
	require.NoError(t, err)
	retried, err := f.service.UpdateRoute(f.ctx, "u-1", renamed, 1)
	require.NoError(t, err)
	assert.True(t, proto.Equal(first, retried))
	got, err := f.service.GetRoute(f.ctx, route.GetId())
	require.NoError(t, err)
	assert.Equal(t, "renamed", got.GetName())
	assert.EqualValues(t, 2, got.GetRevision())
	// An update that breaks the plan is refused and changes nothing.
	broken := proto.Clone(got).(*forwardv1.Route)
	broken.Hops[0].NodeRefs = []string{"forward-99"}
	_, err = f.service.UpdateRoute(f.ctx, "u-broken", broken, 2)
	require.ErrorAs(t, err, &refusal)
	assert.Equal(t, route.GetId(), refusal.Violations[0].RouteID)
	got, err = f.service.GetRoute(f.ctx, route.GetId())
	require.NoError(t, err)
	assert.EqualValues(t, 2, got.GetRevision())

	// Deletes: not found, retry.
	require.ErrorIs(t, f.service.DeleteRoute(f.ctx, "d-missing", missing.GetId()), ErrNotFound)
	require.NoError(t, f.service.DeleteRoute(f.ctx, "d-1", fixed.GetId()))
	require.NoError(t, f.service.DeleteRoute(f.ctx, "d-1", fixed.GetId()), "a retry answers the recorded delete")

	// The request ledger forgets after its retention.
	f.clock.Advance(RequestRetention + time.Hour)
	require.NoError(t, f.service.Maintain(f.ctx))
	require.NoError(t, db.Model(&model.KernelForwardRequest{}).Count(&count).Error)
	assert.Zero(t, count)
}

// An inventory change that breaks a stored route refuses the whole plan:
// every node keeps its state, the status says why, and restoring the node
// plans again.
func TestInventoryReplan(t *testing.T)         { runInventoryReplan(t, openSQLite(t)) }
func TestPostgresInventoryReplan(t *testing.T) { runInventoryReplan(t, openPostgres(t)) }

func runInventoryReplan(t *testing.T, db *gorm.DB) {
	f := newFixture(t, db)
	route := f.create("create-1", twoHop(0))
	before := f.state(exit)

	// The same capabilities do not replan.
	_, replanned, err := f.service.RecordHello(f.ctx, exit, nftCaps(), "4.2.1")
	require.NoError(t, err)
	assert.False(t, replanned)

	outcome := f.hello(exit, nftCaps(forwardv1.Engine_ENGINE_GOST))
	assert.True(t, outcome.Refused)
	assert.Equal(t, validate.CodeEngineNotAdvertised, outcome.Violations[0].Code)
	assert.True(t, proto.Equal(before, f.state(exit)), "a refused plan keeps every state")
	status, err := f.service.PlanStatus(f.ctx)
	require.NoError(t, err)
	assert.True(t, status.Refused)
	assert.Equal(t, "hello", status.Reason)
	require.NotEmpty(t, status.Violations)
	assert.Equal(t, route.GetId(), status.Violations[0].GetRouteId())

	// Any route write is refused meanwhile, naming the broken route.
	_, err = f.service.CreateRoute(f.ctx, "create-2", twoHop(0))
	var refusal *RefusedError
	require.ErrorAs(t, err, &refusal)
	assert.True(t, refusal.Precondition)
	routeIDs := map[string]bool{}
	for _, v := range refusal.Violations {
		routeIDs[v.RouteID] = true
	}
	assert.True(t, routeIDs[route.GetId()], "the stored route is named: %v", routeIDs)
	assert.True(t, routeIDs[""], "the new route's own violations name no route: %v", routeIDs)

	outcome = f.hello(exit, nftCaps(forwardv1.Engine_ENGINE_NFTABLES, forwardv1.Engine_ENGINE_GOST))
	assert.False(t, outcome.Refused)
	status, err = f.service.PlanStatus(f.ctx)
	require.NoError(t, err)
	assert.False(t, status.Refused)

	// A Hello without forward.v1 clears the node's flag: its
	// configuration falls back to anixops.nodeconfig/v1.
	_, _, err = f.service.RecordHello(f.ctx, exit, nil, "4.1.0")
	require.NoError(t, err)
	negotiated, member, err := NodeConfigMember(db, exit)
	require.NoError(t, err)
	assert.False(t, negotiated)
	assert.Nil(t, member)
	nodes, err := f.service.Nodes(f.ctx)
	require.NoError(t, err)
	byRef := map[string]NodeStatus{}
	for _, node := range nodes {
		byRef[node.NodeRef] = node
	}
	assert.False(t, byRef["forward-12"].Negotiated)
	assert.NotNil(t, byRef["forward-12"].Info, "the node stays in the inventory with its capabilities")
	assert.Equal(t, []uint32{22, 7000, 7001}, byRef["forward-11"].ReservedPorts)

	// A node that never negotiated has no member.
	negotiated, _, err = NodeConfigMember(db, proxy)
	require.NoError(t, err)
	assert.False(t, negotiated)
}

// Node settings join the inventory and steer allocation; a proxy node
// with a DNS host has no address until one is set.
func TestNodeSettings(t *testing.T)         { runNodeSettings(t, openSQLite(t)) }
func TestPostgresNodeSettings(t *testing.T) { runNodeSettings(t, openPostgres(t)) }

func runNodeSettings(t *testing.T, db *gorm.DB) {
	f := newFixture(t, db)
	for name, settings := range map[string]NodeSettings{
		"half range":   {PortFirst: 100},
		"upside down":  {PortFirst: 200, PortLast: 100},
		"beyond 65535": {PortFirst: 100, PortLast: 70000},
		"port 0":       {ReservedPorts: []uint32{0}},
		"dns address":  {Addresses: []string{"node.example.com"}},
		"empty label":  {Labels: map[string]string{"": "x"}},
		"many ports":   {ReservedPorts: make([]uint32, MaxReservedPorts+1)},
	} {
		_, err := f.service.SetNodeSettings(f.ctx, exit, settings)
		require.ErrorIs(t, err, ErrInvalidRequest, name)
	}
	_, err := f.service.SetNodeSettings(f.ctx, proxyMissing, NodeSettings{})
	require.ErrorIs(t, err, ErrNodeNotFound)

	_, err = f.service.SetNodeSettings(f.ctx, exit, NodeSettings{PortFirst: 40000, PortLast: 40010, ReservedPorts: []uint32{40000}, Labels: map[string]string{"link": "iepl"}})
	require.NoError(t, err)
	f.create("create-1", twoHop(0))
	assert.EqualValues(t, 40001, f.state(exit).GetHops()[0].GetListen().GetPort())

	// The proxy node joins with settings only; its DNS host is no address,
	// but its reported server IP is.
	_, err = f.service.SetNodeSettings(f.ctx, proxy, NodeSettings{})
	require.NoError(t, err)
	nodes, err := f.service.Nodes(f.ctx)
	require.NoError(t, err)
	var proxyStatus NodeStatus
	for _, node := range nodes {
		if node.NodeRef == "proxy-21" {
			proxyStatus = node
		}
	}
	require.NotNil(t, proxyStatus.Info)
	assert.Empty(t, proxyStatus.Info.GetAddresses())
	assert.Empty(t, proxyStatus.Info.GetEngines(), "no Hello, no engines")
	assert.Equal(t, []uint32{22, 443, 30443}, proxyStatus.ReservedPorts)
	serverIP := "203.0.113.21"
	require.NoError(t, db.Model(&model.Node{}).Where("id = ?", 21).Update("server_ip", serverIP).Error)
	nodes, err = f.service.Nodes(f.ctx)
	require.NoError(t, err)
	for _, node := range nodes {
		if node.NodeRef == "proxy-21" {
			assert.Equal(t, []string{serverIP}, node.Info.GetAddresses())
		}
	}
}

// PlanRoute previews without storing anything; ListRoutes pages and
// filters.
func TestPlanRouteAndList(t *testing.T)         { runPlanRouteAndList(t, openSQLite(t)) }
func TestPostgresPlanRouteAndList(t *testing.T) { runPlanRouteAndList(t, openPostgres(t)) }

func runPlanRouteAndList(t *testing.T, db *gorm.DB) {
	f := newFixture(t, db)
	stored := f.create("create-1", twoHop(0))
	preview, err := f.service.PlanRoute(f.ctx, &forwardv1.PlanRouteRequest{Route: twoHop(0)})
	require.NoError(t, err)
	require.Empty(t, preview.GetViolations())
	require.Len(t, preview.GetStates(), 2)
	assert.EqualValues(t, DefaultPortFirst+1, preview.GetStates()[0].GetHops()[0].GetListen().GetPort(), "the stored route's port is taken")
	// The stored route keeps its own ports in a preview of itself.
	self, err := f.service.PlanRoute(f.ctx, &forwardv1.PlanRouteRequest{Route: stored})
	require.NoError(t, err)
	assert.EqualValues(t, DefaultPortFirst, self.GetStates()[0].GetHops()[0].GetListen().GetPort())
	var count int64
	require.NoError(t, db.Model(&model.KernelForwardRoute{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
	// Violations are answered, not raised.
	bad, err := f.service.PlanRoute(f.ctx, &forwardv1.PlanRouteRequest{Route: &forwardv1.Route{Owner: "admin"}})
	require.NoError(t, err)
	assert.NotEmpty(t, bad.GetViolations())

	user := twoHop(0)
	user.Owner = "user:7"
	userRoute := f.create("create-2", user)
	third := f.create("create-3", twoHop(0))
	routes, next, err := f.service.ListRoutes(f.ctx, ListOptions{PageSize: 2})
	require.NoError(t, err)
	require.Len(t, routes, 2)
	require.NotEmpty(t, next)
	rest, next, err := f.service.ListRoutes(f.ctx, ListOptions{PageSize: 2, PageToken: next})
	require.NoError(t, err)
	require.Len(t, rest, 1)
	assert.Empty(t, next)
	ids := []string{routes[0].GetId(), routes[1].GetId(), rest[0].GetId()}
	assert.ElementsMatch(t, []string{stored.GetId(), userRoute.GetId(), third.GetId()}, ids)
	owned, _, err := f.service.ListRoutes(f.ctx, ListOptions{Owner: "user:7"})
	require.NoError(t, err)
	require.Len(t, owned, 1)
	assert.Equal(t, userRoute.GetId(), owned[0].GetId())
	onNode, _, err := f.service.ListRoutes(f.ctx, ListOptions{NodeRef: "forward-12", PageSize: 5000})
	require.NoError(t, err)
	assert.Len(t, onNode, 3)
	none, _, err := f.service.ListRoutes(f.ctx, ListOptions{NodeRef: "forward-13"})
	require.NoError(t, err)
	assert.Empty(t, none)
}

// The node configuration member is the stamped state, and generation 0
// before the node's first plan.
func TestNodeConfigMember(t *testing.T) {
	db := openSQLite(t)
	f := newFixture(t, db)
	f.create("create-1", twoHop(0))
	_, member, err := NodeConfigMember(db, entry)
	require.NoError(t, err)
	state, found, err := wire.StateFromNodeConfig(wire.NodeConfigFormat, mustJSON(t, map[string]any{wire.NodeConfigMember: member}))
	require.NoError(t, err)
	require.True(t, found)
	assert.True(t, proto.Equal(f.state(entry), state))

	// A node whose first plan was refused has no state yet.
	require.NoError(t, db.Where("node_ref = ?", "forward-12").Delete(&model.KernelForwardNodeState{}).Error)
	_, member, err = NodeConfigMember(db, exit)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"node_ref": "forward-12"}, member)

	// A database without the tables carries none.
	bare := openBare(t)
	negotiated, member, err := NodeConfigMember(bare, entry)
	require.NoError(t, err)
	assert.False(t, negotiated)
	assert.Nil(t, member)
	_, _, err = (&Service{DB: bare}).RecordHello(f.ctx, entry, nil, "4.1.0")
	require.NoError(t, err, "nothing to clear")
}

func TestServiceWithoutDatabase(t *testing.T) {
	var service *Service
	_, err := service.GetRoute(t.Context(), "x")
	require.Error(t, err)
	_, err = (&Service{}).Replan(t.Context(), "x")
	require.Error(t, err)
	require.Error(t, (&Service{}).Maintain(t.Context()))
	assert.Equal(t, "default", (&Service{}).cluster())
	assert.False(t, New(nil).now().IsZero())
	assert.Equal(t, "forward route refused", (&RefusedError{}).Error())
	assert.False(t, errors.Is(ErrNotFound, ErrInvalidRequest))
}
