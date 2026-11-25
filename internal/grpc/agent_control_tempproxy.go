package grpc

import (
	"context"
	"log/slog"
	"math"
	"sync"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v5/internal/tempproxy"
)

// configTriggerTemporaryProxy: an administrator opened or closed a
// temporary proxy of the node (internal/tempproxy).
const configTriggerTemporaryProxy = "temporary_proxy"

var (
	temporaryProxyOnce     sync.Once
	temporaryProxyMu       sync.Mutex
	temporaryProxyManagers = map[*AgentControlManager]bool{}
)

// followTemporaryProxies makes opening or closing a proxy node's temporary
// proxy push the node's configuration at once on the managers' sessions,
// instead of at the next refresh. An expired proxy leaves the
// configuration at the next refresh; the node's kernel ended it on time
// already.
func followTemporaryProxies(proxy *AgentControlManager) {
	if proxy == nil {
		return
	}
	temporaryProxyMu.Lock()
	temporaryProxyManagers[proxy] = true
	temporaryProxyMu.Unlock()
	temporaryProxyOnce.Do(func() { tempproxy.OnChange(pushTemporaryProxies) })
}

// pushTemporaryProxies pushes the configuration of the proxy node nodeID on
// every manager's session that negotiated config.v1.
func pushTemporaryProxies(nodeID uint) {
	// Node ids are uint32 on the stream; a larger one has no session.
	if nodeID > math.MaxUint32 {
		return
	}
	node := agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: uint32(nodeID)}
	temporaryProxyMu.Lock()
	var targets []*AgentControlConnection
	for manager := range temporaryProxyManagers {
		manager.mu.RLock()
		connection := manager.connections[node.ID]
		manager.mu.RUnlock()
		if connection != nil && connection.configNegotiated {
			targets = append(targets, connection)
		}
	}
	temporaryProxyMu.Unlock()
	for _, connection := range targets {
		go func(connection *AgentControlConnection) {
			ctx, cancel := context.WithTimeout(context.WithoutCancel(connection.stream.Context()), forwardHelloTimeout)
			defer cancel()
			if err := pushDesiredConfig(ctx, connection, node, configTriggerTemporaryProxy); err != nil {
				slog.Debug("agent config: the push after a temporary proxy change failed", "component", "agent-control", "node", node.String(), "error", err)
			}
		}(connection)
	}
}
