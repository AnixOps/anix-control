package main

import (
	"context"
	"log"

	kernelsubscriberv1 "github.com/AnixOps/anix-control/sdk/api/kernelsubscriber/v1"
	"github.com/AnixOps/anix-control/sdk/packagestoresdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/packages/forward/native"
	"google.golang.org/grpc"
	"gorm.io/gorm"
)

// forwardBridge is what the forward host needs from the package bridge.
type forwardBridge interface {
	pluginhostsdk.RouterBridge
	packagestoresdk.Leaser
}

// newForwardService returns the forward host's router. The routes in
// forwardRoutes have a native handler on the adopted forward tables and the
// forward node, runtime settings, user directory and entitlement views;
// such a route serves natively once the kernel sets its mode, and falls
// back to the legacy handler otherwise. The traffic reset of a subscriber
// goes through the kernel's KernelSubscriber over the bridge connection
// (local socket or module listener); a bridge without one leaves it legacy.
// The routes in bridgedRoutes always relay to the legacy handler.
func newForwardService(bridge forwardBridge, leaseID string) (*pluginhostsdk.Router, error) {
	storage := packagestoresdk.SharedOpener(bridge)
	service := &native.Service{Open: func(ctx context.Context) (*gorm.DB, error) {
		store, err := storage(ctx)
		if err != nil {
			return nil, err
		}
		return store.DB.WithContext(ctx), nil
	}}
	if conn, ok := bridge.(interface {
		Conn() grpc.ClientConnInterface
	}); ok && conn.Conn() != nil {
		service.Subscriber = kernelsubscriberv1.NewKernelSubscriberClient(conn.Conn())
	}
	return pluginhostsdk.NewRouter(pluginhostsdk.RouterConfig{
		PackageID: "forward", LeaseID: leaseID, Bridge: bridge, Logf: log.Printf,
		AllowRoute: func(routeID string) bool {
			_, nativeRoute := forwardRoutes[routeID]
			_, bridgedRoute := bridgedRoutes[routeID]
			return nativeRoute || bridgedRoute
		},
		Native: service.Handlers(),
	})
}

// forwardRoutes are the package's compatibility routes with a native
// handler. None of them changes what a node runs or state the kernel keeps
// in memory.
var forwardRoutes = map[string]struct{}{
	// Panel forwards: the lists, and the display order, which no node runs.
	"forward.forward.list.post":               {},
	"forward.admin.forward.list.post":         {},
	"forward.forward.update_order.post":       {},
	"forward.admin.forward.update_order.post": {},
	// Tunnels: the list, creation and deletion of an unused tunnel. A
	// tunnel runs nothing until a forward uses it.
	"forward.admin.tunnel.list.post":   {},
	"forward.admin.tunnel.create.post": {},
	"forward.admin.tunnel.delete.post": {},
	// The tunnels a forward may use, for the caller.
	"forward.tunnel.user.tunnel.post":       {},
	"forward.admin.tunnel.user.tunnel.post": {},
	// Tunnel permissions: granting one, and the list.
	"forward.tunnel.user.assign.post":       {},
	"forward.admin.tunnel.user.assign.post": {},
	"forward.tunnel.user.list.post":         {},
	"forward.admin.tunnel.user.list.post":   {},
	// Reads: a forward's ingress latencies, the node statistics and the
	// caller's legacy rules (their nodes without API tokens).
	"forward.admin.forward.observability.multi_ingress.get": {},
	"forward.admin.forward.stats.get":                       {},
	"forward.user.forward.rules.get":                        {},
	// The administrator's traffic reset (KernelSubscriber.ResetTraffic, or
	// a tunnel permission's traffic).
	"forward.user.reset.post": {},
}

// bridgedRoutes are the package's compatibility routes without a native
// handler; they always relay to the kernel's legacy handler.
var bridgedRoutes = map[string]struct{}{
	// Forward nodes and Ansible machines are rows of v2_forward_node, which
	// holds each node's API token. The token authenticates the node's agent
	// (WebSocket, gRPC and REST), so the table is a protected kernel table
	// no package may adopt; the answers show the tokens to administrators,
	// the kernel's gost manager keeps each node's address and token in
	// memory, the checks dial the node, the statistics sync calls the
	// node's gost metrics, and the Ansible list writes the machine tag.
	"forward.admin.forward.nodes.get":                           {},
	"forward.admin.forward.nodes.post":                          {},
	"forward.admin.forward.nodes.id.get":                        {},
	"forward.admin.forward.nodes.id.put":                        {},
	"forward.admin.forward.nodes.id.delete":                     {},
	"forward.admin.forward.nodes.id.check.post":                 {},
	"forward.admin.forward.nodes.id.toggle.post":                {},
	"forward.admin.forward.nodes.id.sync_stats.post":            {},
	"forward.admin.forward.ansible_machines.get":                {},
	"forward.admin.forward.ansible_machines.post":               {},
	"forward.admin.forward.ansible_machines.id.get":             {},
	"forward.admin.forward.ansible_machines.id.put":             {},
	"forward.admin.forward.ansible_machines.id.delete":          {},
	"forward.admin.forward.ansible_machines.id.check.post":      {},
	"forward.admin.forward.ansible_machines.id.toggle.post":     {},
	"forward.admin.forward.ansible_machines.id.sync_stats.post": {},
	// Panel forward changes apply the forward on its node (NodeX, a local
	// Ansible job or a clean agent job, whose payload carries the node's
	// API token) and record the runtime result; the port bindings and the
	// quota checks belong to the same flow. The diagnoses dial the
	// targets and nodes from Control, and the backend sync re-applies every
	// active forward.
	"forward.forward.create.post":             {},
	"forward.admin.forward.create.post":       {},
	"forward.forward.update.post":             {},
	"forward.admin.forward.update.post":       {},
	"forward.forward.delete.post":             {},
	"forward.admin.forward.delete.post":       {},
	"forward.forward.force_delete.post":       {},
	"forward.admin.forward.force_delete.post": {},
	"forward.forward.pause.post":              {},
	"forward.admin.forward.pause.post":        {},
	"forward.forward.resume.post":             {},
	"forward.admin.forward.resume.post":       {},
	"forward.forward.diagnose.post":           {},
	"forward.admin.forward.diagnose.post":     {},
	"forward.admin.tunnel.diagnose.post":      {},
	"forward.admin.forward.sync_backend.post": {},
	// A tunnel update rebuilds its forwards' port bindings and re-applies
	// them on their node when the protocol, listen addresses or interface
	// change.
	"forward.admin.tunnel.update.post": {},
	// Removing a permission deletes its forwards from their node first;
	// updating one pauses its forwards when it lapses and re-applies them
	// when the speed limit changes.
	"forward.tunnel.user.remove.post":       {},
	"forward.admin.tunnel.user.remove.post": {},
	"forward.tunnel.user.update.post":       {},
	"forward.admin.tunnel.user.update.post": {},
	// Legacy rules: every change is pushed to NodeX with the nodes' API
	// tokens. The administrator's answers embed the full node rows, tokens
	// included, which kapi_forward_node_v1 does not show. The agents' rule
	// list authenticates a forward node by its API token.
	"forward.admin.forward.rules.get":            {},
	"forward.admin.forward.rules.post":           {},
	"forward.admin.forward.rules.id.get":         {},
	"forward.admin.forward.rules.id.put":         {},
	"forward.admin.forward.rules.id.delete":      {},
	"forward.admin.forward.rules.id.toggle.post": {},
	"forward.user.forward.rules.post":            {},
	"forward.forward.agent.rules.get":            {},
	// Runtime status and diagnosis read the protected v2_system_config
	// (NodeX address and token, Ansible settings), call NodeX and inspect
	// files on Control's disk. The job list shows job payloads, which carry
	// node API tokens (v2_forward_runtime_job is protected).
	"forward.admin.forward.runtime.jobs.get":   {},
	"forward.admin.forward.runtime.status.get": {},
	"forward.admin.forward.runtime.doctor.get": {},
	"forward.admin.forward.local.status.get":   {},
	"forward.admin.forward.local.doctor.get":   {},
	// The target catalog, latency trend and topology also read the proxy
	// nodes of v2_node (status, parent, load), the proxy-node package's
	// table, which no kernel view shows yet.
	"forward.admin.forward.observability.targets.get":  {},
	"forward.admin.forward.observability.trend.get":    {},
	"forward.admin.forward.observability.topology.get": {},
	// Clean agents: v2_forward_clean_agent holds each agent's token, which
	// authenticates it (a protected table). Registration, heartbeat and
	// report authenticate an agent, claim runtime jobs and record their
	// results and traffic; the install script is built from the request's
	// host or Control's configured public URL.
	"forward.admin.forward.agents.get":            {},
	"forward.admin.forward.agents.post":           {},
	"forward.admin.forward.agents.id.revoke.post": {},
	"forward.forward_agent.install_sh.get":        {},
	"forward.forward_agent.register.post":         {},
	"forward.forward_agent.heartbeat.post":        {},
	"forward.forward_agent.report.post":           {},
	// Flow accounting: the forward's counters, the subscriber's traffic
	// (subscriber.RecordTrafficTx) and the permission's traffic change in
	// one kernel transaction under a per-forward lock in Control's memory,
	// and a subscriber or permission that runs out pauses its forwards on
	// their nodes.
	"forward.internal.forward.traffic.upload.post":   {},
	"forward.internal.forward.traffic.report.post":   {},
	"forward.internal.forward.traffic.snapshot.post": {},
}
