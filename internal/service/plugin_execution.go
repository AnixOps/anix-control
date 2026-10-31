package service

import "time"

// Agent plugin dispatch is off in this release, and no setting turns it on
// (single-binary design, B23 and section 4.3). The built-in packages carry no
// Agent plugin payload, so there is nothing to dispatch. M4's Agent release
// manifest brings the Agent plugins, and with them this path, again without a
// setting. Until then the constant keeps it unreachable: assignments and
// configuration changes record the desired state and queue no Agent operation.
const (
	// AgentPluginDispatchEnabled: queue and dispatch Agent plugin
	// operations (the kernel operation bridge). False until M4.
	AgentPluginDispatchEnabled = false
)

// AgentPluginWorkerPollInterval is the poll interval of the Agent plugin
// dispatcher, once M4 starts it.
const AgentPluginWorkerPollInterval = 5 * time.Second
