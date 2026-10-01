package service

// The KernelNodeOps operation families (node-ops-service.md section 3.2).
// Each lets an official package submit one family's node operations; any
// one of them lets it read, watch and cancel the operations it owns.
const (
	// CapabilityNodeOpsForward: ApplyForward, ApplyTunnel,
	// SyncForwardBackend, ApplyLegacyRule.
	CapabilityNodeOpsForward = "kernel.nodeops.forward.v1"
	// CapabilityNodeOpsNodeConfig: SyncNode, RetireNode, RetireProtocol,
	// PutSecretDocument and ValidateNodeConfig.
	CapabilityNodeOpsNodeConfig = "kernel.nodeops.nodeconfig.v1"
	// CapabilityNodeOpsDiagnose: CheckEndpoints, CollectNodeStats,
	// DiagnoseForward, DiagnoseTunnel, RunAgentDiagnostic.
	CapabilityNodeOpsDiagnose = "kernel.nodeops.diagnose.v1"
	// CapabilityNodeOpsAgents: AgentControlOperation and the agent session
	// reads.
	CapabilityNodeOpsAgents = "kernel.nodeops.agents.v1"
	// CapabilityNodeOpsCredentials: IssueCredential, RevokeCredential,
	// IssueRegistrationKey, RevokeRegistrationKey, IssueCleanAgent.
	CapabilityNodeOpsCredentials = "kernel.nodeops.credentials.v1" // #nosec G101 -- a capability name, not a credential.
)

// NodeOpsCapabilities are the five KernelNodeOps family capabilities, in
// the order of the contract's OperationFamily enum.
func NodeOpsCapabilities() []string {
	return []string{
		CapabilityNodeOpsForward, CapabilityNodeOpsNodeConfig, CapabilityNodeOpsDiagnose,
		CapabilityNodeOpsAgents, CapabilityNodeOpsCredentials,
	}
}

func isNodeOpsCapability(capability string) bool {
	for _, known := range NodeOpsCapabilities() {
		if capability == known {
			return true
		}
	}
	return false
}
