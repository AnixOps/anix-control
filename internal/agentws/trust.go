// Package agentws defines the internal context keys shared by the v2 gateway
// and the kernel-side compatibility adapter for preauthenticated agent sockets.
package agentws

const (
	TrustedContextKey     = "anixops.agent_ws.trusted"
	ForwardNodeContextKey = "anixops.agent_ws.forward_node"
)
