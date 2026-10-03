package agentcontrol

import (
	"errors"

	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
)

// Machine-readable codes of the data plane's answers (PROTOCOL.md, "Error
// codes"). The text fields (ReportAck.error, MaintenanceEventResult.error,
// ConfigStatus.error) stay for people; an Agent decides on the code. A code
// is lowercase words joined by "_", at most 64 bytes, and a later Control
// may add codes: an Agent treats an unknown refusal code as a refusal, the
// way it treated a non-empty error before codes existed.

// ErrorCodeCapabilityNotNegotiated ends a stream that sent a data-plane
// payload whose capability the session did not negotiate
// (InvalidArgument, in the MetadataErrorCode trailer).
const ErrorCodeCapabilityNotNegotiated = "agent_capability_not_negotiated"

// ReportAck.error_code (reports.v1).
const (
	// ReportErrorCodeBatchIDInvalid: no batch_id, or one over 128 bytes.
	ReportErrorCodeBatchIDInvalid = "report_batch_id_invalid"
	// ReportErrorCodeInvalid: an entry is malformed (an empty entry, a
	// user_id of 0, fields_json that is not JSON).
	ReportErrorCodeInvalid = "report_invalid"
	// ReportErrorCodeCounterOverflow: bytes beyond the 64-bit counter range.
	ReportErrorCodeCounterOverflow = "report_counter_overflow"
	// ReportErrorCodeNodeGone: the stream's node no longer exists.
	ReportErrorCodeNodeGone = "report_node_gone"
	// ReportErrorCodeUnavailable: Control could not record the batch now
	// (its database failed). applied is false and error empty; the Agent
	// keeps the batch and resends it after ReportAck.retry_after_ms. Sent
	// only to an Agent that negotiated ReportsAttributeTransientAck.
	ReportErrorCodeUnavailable = "report_unavailable"
)

// ReportsAttributeTransientAck is the Capability attribute of reports.v1
// with which an Agent asks for a ReportAck of ReportErrorCodeUnavailable
// when Control cannot record a batch now, instead of no answer. Its value
// is ReportsTransientAckV1. Control echoes the attribute on its reports.v1
// in HelloAck.server_capabilities when it sends such answers; without it,
// it sends none (an Agent that did not ask would drop the batch).
const (
	ReportsAttributeTransientAck = "transient_ack"
	ReportsTransientAckV1        = "v1"
)

// MaintenanceEventResult.error_code (maintenance.v1).
const (
	// MaintenanceErrorCodeSchemaUnsupported: the batch's version is not
	// MaintenanceSchemaV1.
	MaintenanceErrorCodeSchemaUnsupported = "maintenance_schema_unsupported"
	// MaintenanceErrorCodeBatchTooLarge: more than MaxMaintenanceBatchEvents
	// events, or more than MaxMaintenanceBatchBytes.
	MaintenanceErrorCodeBatchTooLarge = "maintenance_batch_too_large"
	// MaintenanceErrorCodeEventInvalid: the event is not a valid
	// anixops.maintenance/v1 event (ParseMaintenanceEvent).
	MaintenanceErrorCodeEventInvalid = "maintenance_event_invalid"
	// MaintenanceErrorCodeWrongNode: the event's node_id is not the
	// stream's node.
	MaintenanceErrorCodeWrongNode = "maintenance_event_wrong_node"
	// MaintenanceErrorCodeNodeGone: the stream's node no longer exists.
	MaintenanceErrorCodeNodeGone = "maintenance_node_gone"
	// MaintenanceErrorCodeUnavailable: Control could not store the event
	// now; persisted is false and error empty, and the Agent sends it again
	// after MaintenanceEventResult.retry_after_ms.
	MaintenanceErrorCodeUnavailable = "maintenance_unavailable"
)

// ConfigStatus.error_code (config.v1), set by the Agent when applied is
// false.
const (
	// ConfigErrorCodeFormatUnsupported: the snapshot's format is one the
	// Agent does not know.
	ConfigErrorCodeFormatUnsupported = "config_format_unsupported"
	// ConfigErrorCodeHashMismatch: config_hash is not the SHA-256 of
	// config_json.
	ConfigErrorCodeHashMismatch = "config_hash_mismatch"
	// ConfigErrorCodeInvalid: the document does not parse or fails the
	// Agent's checks.
	ConfigErrorCodeInvalid = "config_invalid"
	// ConfigErrorCodeApplyFailed: the Agent could not run the configuration
	// (a core or driver refused it).
	ConfigErrorCodeApplyFailed = "config_apply_failed"
)

// ConfigErrorCodes are the ConfigStatus codes this contract defines.
var ConfigErrorCodes = []string{
	ConfigErrorCodeFormatUnsupported, ConfigErrorCodeHashMismatch, ConfigErrorCodeInvalid, ConfigErrorCodeApplyFailed,
}

// AgentArtifacts refusals (artifacts.v1), in the MetadataErrorCode trailer.
// They are the codes of the HTTP plugin download's JSON body.
const (
	// ErrorCodePluginReleaseAddressInvalid (InvalidArgument): the request
	// lacks a plugin_id, version, 64-hex sha256 or positive size.
	ErrorCodePluginReleaseAddressInvalid = "invalid_plugin_release_address"
	// ErrorCodePluginReleaseAddressMismatch (InvalidArgument): the address
	// is not the release's verified content.
	ErrorCodePluginReleaseAddressMismatch = "plugin_release_address_mismatch"
	// ErrorCodePluginReleaseNotAssigned (PermissionDenied): the
	// certificate's node has no enabled assignment of the release, or the
	// release is not an official enabled Agent release.
	ErrorCodePluginReleaseNotAssigned = "plugin_release_not_assigned"
	// ErrorCodePluginReleaseNotFound (NotFound): the assigned release or its
	// artifact is gone.
	ErrorCodePluginReleaseNotFound = "plugin_release_not_found"
	// ErrorCodePluginReleaseIntegrityFailed (FailedPrecondition): the
	// stored release no longer verifies.
	ErrorCodePluginReleaseIntegrityFailed = "plugin_release_integrity_failed"
	// ErrorCodePluginReleaseBusy (ResourceExhausted): the node has as many
	// downloads running as Control allows; retry later.
	ErrorCodePluginReleaseBusy = "plugin_release_download_busy"
)

// ErrMaintenanceEventWrongNode reports an event of another node; it also
// matches ErrInvalidMaintenanceEvent.
var ErrMaintenanceEventWrongNode = errors.New("maintenance event of another node")

// TransientReportAcks reports whether a session sends ReportAck of
// ReportErrorCodeUnavailable: both the Agent's and Control's reports.v1
// carry ReportsAttributeTransientAck at ReportsTransientAckV1.
func TransientReportAcks(agent, server []*agentv1pb.Capability) bool {
	return hasTransientAck(agent) && hasTransientAck(server)
}

func hasTransientAck(capabilities []*agentv1pb.Capability) bool {
	for _, capability := range capabilities {
		if capability != nil && capability.Name == CapabilityReports && capability.Version == CapabilityVersionV1 &&
			capability.Attributes[ReportsAttributeTransientAck] == ReportsTransientAckV1 {
			return true
		}
	}
	return false
}
