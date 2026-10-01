package agentcontrol

import agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"

// Data-plane capabilities of the control stream (PROTOCOL.md, "Data plane").
// Each is advertised at CapabilityVersionV1, so the protocol writes them
// config.v1, users.v1 and reports.v1. An Agent lists the ones it implements in
// Hello.capabilities; Control lists the ones it serves in
// HelloAck.server_capabilities. A feature is in use on a session only when
// both list it (Negotiated), and neither side sends its payloads otherwise.
const (
	// CapabilityConfig: Control sends ConfigSnapshot and the Agent answers
	// ConfigStatus; Hello.config_revision is read.
	CapabilityConfig = "config"
	// CapabilityUsers: Control sends UserDelta; Hello.users_cursor is read.
	CapabilityUsers = "users"
	// CapabilityReports: the Agent sends TrafficReport, LogBatch and
	// NodeStatus, and Control answers reports with ReportAck.
	CapabilityReports = "reports"
)

// Negotiated reports whether a data-plane capability is in use on a session:
// both the Agent's Hello.capabilities and Control's
// HelloAck.server_capabilities list it at CapabilityVersionV1.
func Negotiated(agent, server []*agentv1pb.Capability, name string) bool {
	return HasCapabilityVersion(agent, name, CapabilityVersionV1) &&
		HasCapabilityVersion(server, name, CapabilityVersionV1)
}
