package grpc

// NewMetadataAuthControlServer is NewAgentControlGRPCServer for the
// external test package's plaintext cross-repository fixtures: the node
// named by x-node-id is authenticated as if its client certificate had been
// verified (metadataIdentity).
func NewMetadataAuthControlServer(manager *AgentControlManager) *AgentControlGRPCServer {
	return newTestAgentControlServer(manager)
}
