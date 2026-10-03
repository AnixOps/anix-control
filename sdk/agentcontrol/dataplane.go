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
	// CapabilityDiag: the Agent runs node-side diagnostics (diag.*
	// operations), so Control may choose the node as a vantage. It adds no
	// payload; Control records it on the session.
	CapabilityDiag = "diag"
	// CapabilityPackageReports: the Agent sends PackageReport, the latest
	// observation of a plugin package; Control does not acknowledge it.
	CapabilityPackageReports = "package-reports"
	// CapabilityForward: the node forwards (forward-sdk.md section 8).
	// Control carries the node's NodeForwardState in an
	// anixops.nodeconfig/v2 ConfigSnapshot (so it needs config.v1 too) and
	// accepts the node's NodeForwardReport as a PackageReport (with
	// package-reports.v1). The Agent's Hello lists it with the node's
	// NodeCapabilities in an attribute (sdk/forward/wire).
	CapabilityForward = "forward"
	// CapabilityMaintenance: the Agent sends its maintenance outbox as
	// MaintenanceEvents, and Control answers each batch with
	// MaintenanceAck (maintenance.go; PROTOCOL.md, "Maintenance events").
	CapabilityMaintenance = "maintenance"
)

// DataPlaneCapabilities are the capabilities of the data plane, the ones
// Control lists in HelloAck.server_capabilities when it serves them and the
// Agent's Hello lists them.
var DataPlaneCapabilities = []string{
	CapabilityConfig, CapabilityUsers, CapabilityReports, CapabilityPackageReports, CapabilityForward, CapabilityMaintenance,
}

// Negotiated reports whether a data-plane capability is in use on a session:
// both the Agent's Hello.capabilities and Control's
// HelloAck.server_capabilities list it at CapabilityVersionV1.
func Negotiated(agent, server []*agentv1pb.Capability, name string) bool {
	return HasCapabilityVersion(agent, name, CapabilityVersionV1) &&
		HasCapabilityVersion(server, name, CapabilityVersionV1)
}

// NegotiatedCapabilities lists the data-plane capabilities in use on a
// session (Negotiated), as name.version (config.v1), in the order of
// DataPlaneCapabilities.
func NegotiatedCapabilities(agent, server []*agentv1pb.Capability) []string {
	negotiated := []string{}
	for _, name := range DataPlaneCapabilities {
		if Negotiated(agent, server, name) {
			negotiated = append(negotiated, name+"."+CapabilityVersionV1)
		}
	}
	return negotiated
}
