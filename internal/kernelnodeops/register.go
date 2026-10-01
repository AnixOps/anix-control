package kernelnodeops

// RegisterNodeOperationExecutors serves the node configuration and agent
// operations of NO-6 (node-ops-service.md section 3.11): node.sync of the
// nodeconfig family, agent.operation of the agents family and
// agent.diagnostic of the diagnose family, all dispatched on the agent
// sessions sources returns. With a nil sources the defaults registered
// with UseAgentStreams and UseWebSocketAgents are read at each operation.
func RegisterNodeOperationExecutors(registry *Registry, sources SourcesFunc) error {
	if sources == nil {
		sources = DefaultAgentSources
	}
	for _, entry := range []struct {
		kind     string
		executor Executor
	}{
		{KindNodeSync, nodeSyncExecutor{sources: sources}},
		{KindAgentOperation, agentOperationExecutor{sources: sources}},
		{KindAgentDiagnostic, agentDiagnosticExecutor{sources: sources}},
	} {
		if err := registry.Register(entry.kind, entry.executor); err != nil {
			return err
		}
	}
	return nil
}
