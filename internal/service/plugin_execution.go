package service

import "time"

// Agent plugin dispatch and topology deployment execution are off in this
// release, and no setting turns them on (single-binary design, B23 and
// section 4.3). The built-in packages carry no Agent plugin payload, so
// there is nothing to dispatch; production ran with both off before. M4's
// Agent release manifest brings the Agent plugins, and with them both paths,
// again without a setting. Until then the code paths stay and these constants
// keep them unreachable: the handlers record the desired state and queue no
// Agent operation, apply and rollback of a deployment answer
// topology_execution_disabled, and the start launches neither worker.
const (
	// AgentPluginDispatchEnabled: queue and dispatch Agent plugin
	// operations (the kernel operation bridge). False until M4.
	AgentPluginDispatchEnabled = false
	// TopologyExecutionEnabled: run the topology deployment executor and
	// accept deployment apply and rollback. It needs Agent plugin
	// dispatch. False until M4.
	TopologyExecutionEnabled = false
)

// AgentPluginWorkerPollInterval is the poll interval of the Agent plugin
// dispatcher and the topology executor, once M4 starts them.
const AgentPluginWorkerPollInterval = 5 * time.Second
