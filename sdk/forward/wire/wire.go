// Package wire is how forwarding rides the Agent Control stream
// (anix.agent.v1; docs/architecture/forward-sdk.md section 8 and
// sdk/api/agent/v1/PROTOCOL.md, "Forwarding"). Control and the Agent both
// use it, so they encode and check the same bytes:
//
//   - Hello: the Agent lists the capability forward.v1
//     (agentcontrol.CapabilityForward) with its NodeCapabilities as protojson
//     in the attribute "node_capabilities" (HelloCapability,
//     NodeCapabilitiesFromHello).
//   - Desired state: Control sends the node's NodeForwardState in a
//     ConfigSnapshot of format anixops.nodeconfig/v2, the
//     anixops.nodeconfig/v1 document plus a member "forward" holding the
//     state as protojson (StateMember, StateFromNodeConfig). Only a session
//     that negotiated forward.v1 gets v2.
//   - Reports: the Agent sends its NodeForwardReport as a PackageReport with
//     plugin_id "forward", kind "forward.report" and version "v1"
//     (Report, IsReport, DecodeReport).
//
// Protojson is written with the proto field names (node_ref), as in the
// contract fixtures; readers accept both spellings and ignore unknown
// fields, so a newer Agent's additions do not break an older Control.
package wire

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"unicode/utf8"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/validate"
	"google.golang.org/protobuf/encoding/protojson"
)

const (
	// AttributeNodeCapabilities is the attribute of the Hello's forward.v1
	// capability that holds the node's NodeCapabilities as protojson.
	AttributeNodeCapabilities = "node_capabilities"
	// MaxNodeCapabilitiesBytes caps that attribute.
	MaxNodeCapabilitiesBytes = 16 << 10

	// NodeConfigFormat is the ConfigSnapshot format that carries
	// forwarding: anixops.nodeconfig/v1 plus the member NodeConfigMember.
	NodeConfigFormat = "anixops.nodeconfig/v2"
	// NodeConfigMember is the document member holding the NodeForwardState.
	NodeConfigMember = "forward"

	// ReportPluginID, ReportKind and ReportVersion name a forward report's
	// PackageReport. Control accepts it from a session that negotiated
	// forward.v1, not from a plugin release.
	ReportPluginID = "forward"
	ReportKind     = "forward.report"
	ReportVersion  = "v1"

	// MaxEngines caps NodeCapabilities.engines.
	MaxEngines = 8
	// MaxReportEntries caps each of a report's counters, health and errors
	// (the PackageReport payload is capped at 256 KiB anyway).
	MaxReportEntries = 16384
	// MaxEpochBytes caps Counters.counter_epoch.
	MaxEpochBytes = 128
	// MaxTextBytes caps the free-text fields (versions, reasons, error
	// messages); longer ones are refused.
	MaxTextBytes = 1024
)

// ErrInvalid wraps every refusal of a capability attribute or report.
var ErrInvalid = errors.New("forward wire: invalid")

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

var (
	marshal   = protojson.MarshalOptions{UseProtoNames: true}
	unmarshal = protojson.UnmarshalOptions{DiscardUnknown: true}
	// routeIDPattern: the route ids the drivers accept (1 to 64 ASCII
	// letters and digits); Control's are ULIDs.
	routeIDPattern = regexp.MustCompile(`^[0-9A-Za-z]{1,64}$`)
	hashPattern    = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// HelloCapability answers the forward.v1 capability an Agent lists in its
// Hello, with caps in its attribute.
func HelloCapability(caps *forwardv1.NodeCapabilities) (*agentv1pb.Capability, error) {
	if err := CheckNodeCapabilities(caps, caps.GetNodeRef()); err != nil {
		return nil, err
	}
	encoded, err := marshal.Marshal(caps)
	if err != nil {
		return nil, fmt.Errorf("forward wire: encode node capabilities: %w", err)
	}
	if len(encoded) > MaxNodeCapabilitiesBytes {
		return nil, invalid("node capabilities exceed %d bytes", MaxNodeCapabilitiesBytes)
	}
	return &agentv1pb.Capability{
		Name: agentcontrol.CapabilityForward, Version: agentcontrol.CapabilityVersionV1,
		Attributes: map[string]string{AttributeNodeCapabilities: string(encoded)},
	}, nil
}

// NodeCapabilitiesFromHello finds forward.v1 among a Hello's capabilities
// and decodes its node capabilities for the stream's node nodeRef. listed
// is false when the Hello does not list forward.v1. An error means it is
// listed but its attribute is missing, oversize or malformed, or names
// another node: Control then does not serve forward.v1 on the session.
func NodeCapabilitiesFromHello(capabilities []*agentv1pb.Capability, nodeRef string) (caps *forwardv1.NodeCapabilities, listed bool, err error) {
	var capability *agentv1pb.Capability
	for _, c := range capabilities {
		if c.GetName() == agentcontrol.CapabilityForward && c.GetVersion() == agentcontrol.CapabilityVersionV1 {
			capability = c
			break
		}
	}
	if capability == nil {
		return nil, false, nil
	}
	raw, ok := capability.GetAttributes()[AttributeNodeCapabilities]
	if !ok || raw == "" {
		return nil, true, invalid("forward.v1 has no %s attribute", AttributeNodeCapabilities)
	}
	if len(raw) > MaxNodeCapabilitiesBytes {
		return nil, true, invalid("%s exceeds %d bytes", AttributeNodeCapabilities, MaxNodeCapabilitiesBytes)
	}
	caps = &forwardv1.NodeCapabilities{}
	if err := unmarshal.Unmarshal([]byte(raw), caps); err != nil {
		return nil, true, invalid("%s is not a NodeCapabilities: %v", AttributeNodeCapabilities, err)
	}
	if err := CheckNodeCapabilities(caps, nodeRef); err != nil {
		return nil, true, err
	}
	if caps.GetNodeRef() == "" {
		caps.NodeRef = nodeRef
	}
	return caps, true, nil
}

// CheckNodeCapabilities bounds caps: node_ref empty or nodeRef, at most
// MaxEngines engines, each a known engine at most once, enums the contract
// defines, and text within MaxTextBytes.
func CheckNodeCapabilities(caps *forwardv1.NodeCapabilities, nodeRef string) error {
	if caps == nil {
		return invalid("node capabilities are required")
	}
	if caps.GetNodeRef() != "" && caps.GetNodeRef() != nodeRef {
		return invalid("node capabilities name %q, not the stream's node %q", caps.GetNodeRef(), nodeRef)
	}
	if len(caps.GetEngines()) > MaxEngines {
		return invalid("more than %d engines", MaxEngines)
	}
	for _, text := range []string{caps.GetKernelVersion(), caps.GetCgroup(), caps.GetAgentVersion()} {
		if err := checkText(text); err != nil {
			return err
		}
	}
	seen := map[forwardv1.Engine]bool{}
	for i, engine := range caps.GetEngines() {
		e := engine.GetEngine()
		if e == forwardv1.Engine_ENGINE_UNSPECIFIED || forwardv1.Engine_name[int32(e)] == "" {
			return invalid("engines[%d]: unknown engine %d", i, e)
		}
		if seen[e] {
			return invalid("engines[%d]: %s listed twice", i, e)
		}
		seen[e] = true
		if err := checkText(engine.GetVersion()); err != nil {
			return err
		}
		if err := checkText(engine.GetUnavailableReason()); err != nil {
			return err
		}
		if len(engine.GetStrategies()) > len(forwardv1.BalanceStrategy_name) || len(engine.GetLinkSecurities()) > len(forwardv1.LinkSecurity_name) {
			return invalid("engines[%d]: too many strategies or link securities", i)
		}
		for _, s := range engine.GetStrategies() {
			if forwardv1.BalanceStrategy_name[int32(s)] == "" {
				return invalid("engines[%d]: unknown strategy %d", i, s)
			}
		}
		for _, s := range engine.GetLinkSecurities() {
			if forwardv1.LinkSecurity_name[int32(s)] == "" {
				return invalid("engines[%d]: unknown link security %d", i, s)
			}
		}
	}
	return nil
}

func checkText(text string) error {
	if len(text) > MaxTextBytes {
		return invalid("a text field exceeds %d bytes", MaxTextBytes)
	}
	if !utf8.ValidString(text) {
		return invalid("a text field is not UTF-8")
	}
	return nil
}

// StateMember answers state as the value of the nodeconfig/v2 member
// "forward": its protojson with proto field names, decoded into generic
// JSON values, so encoding/json writes it with sorted keys and the
// document's bytes (and so its hash) depend only on the state.
func StateMember(state *forwardv1.NodeForwardState) (map[string]any, error) {
	encoded, err := marshal.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("forward wire: encode state: %w", err)
	}
	member := map[string]any{}
	if err := json.Unmarshal(encoded, &member); err != nil {
		return nil, fmt.Errorf("forward wire: decode state: %w", err)
	}
	return member, nil
}

// StateFromNodeConfig answers the NodeForwardState of a ConfigSnapshot,
// for the Agent. found is false for a format other than
// anixops.nodeconfig/v2 or a document without the member. A generation of
// 0 means Control has no state for the node yet: the Agent keeps what it
// runs.
func StateFromNodeConfig(format string, configJSON []byte) (state *forwardv1.NodeForwardState, found bool, err error) {
	if format != NodeConfigFormat {
		return nil, false, nil
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(configJSON, &document); err != nil {
		return nil, false, invalid("node configuration is not a JSON object: %v", err)
	}
	raw, ok := document[NodeConfigMember]
	if !ok {
		return nil, false, nil
	}
	state = &forwardv1.NodeForwardState{}
	if err := unmarshal.Unmarshal(raw, state); err != nil {
		return nil, true, invalid("member %q is not a NodeForwardState: %v", NodeConfigMember, err)
	}
	return state, true, nil
}

// Report answers the PackageReport that carries report.
func Report(report *forwardv1.NodeForwardReport) (*agentv1pb.PackageReport, error) {
	if err := CheckReport(report, report.GetNodeRef()); err != nil {
		return nil, err
	}
	payload, err := marshal.Marshal(report)
	if err != nil {
		return nil, fmt.Errorf("forward wire: encode report: %w", err)
	}
	if err := agentcontrol.ValidatePackageReportPayloadSize(payload); err != nil {
		return nil, invalid("%v", err)
	}
	return &agentv1pb.PackageReport{
		PluginId: ReportPluginID, Kind: ReportKind, Version: ReportVersion,
		PayloadJson: payload, ObservedAtUnixMs: report.GetObservedAtUnixMs(),
	}, nil
}

// IsReport tells whether a PackageReport is a forward report: plugin_id
// "forward" and kind "forward.report".
func IsReport(report *agentv1pb.PackageReport) bool {
	return report.GetPluginId() == ReportPluginID && report.GetKind() == ReportKind
}

// DecodeReport decodes a forward report's payload and checks it for the
// stream's node nodeRef (CheckReport).
func DecodeReport(payload []byte, nodeRef string) (*forwardv1.NodeForwardReport, error) {
	if err := agentcontrol.ValidatePackageReportPayloadSize(payload); err != nil {
		return nil, invalid("%v", err)
	}
	report := &forwardv1.NodeForwardReport{}
	if err := unmarshal.Unmarshal(payload, report); err != nil {
		return nil, invalid("payload is not a NodeForwardReport: %v", err)
	}
	if err := CheckReport(report, nodeRef); err != nil {
		return nil, err
	}
	return report, nil
}

// CheckReport bounds a report for the stream's node nodeRef: node_ref is
// nodeRef; state_hash empty or lowercase hex SHA-256; at most
// MaxReportEntries counters, health entries and errors; route ids of 1 to
// 64 ASCII letters and digits; hop indexes below validate.MaxHops; a
// counter's node_ref empty or nodeRef, its counter_epoch 1 to
// MaxEpochBytes bytes, and no (route, hop, epoch) twice; known enums;
// text within MaxTextBytes.
func CheckReport(report *forwardv1.NodeForwardReport, nodeRef string) error {
	if report == nil {
		return invalid("report is required")
	}
	if nodeRef == "" || report.GetNodeRef() != nodeRef {
		return invalid("report names node %q, not the stream's node %q", report.GetNodeRef(), nodeRef)
	}
	if hash := report.GetStateHash(); hash != "" && !hashPattern.MatchString(hash) {
		return invalid("state_hash is not a lowercase hex SHA-256")
	}
	if len(report.GetCounters()) > MaxReportEntries || len(report.GetHealth()) > MaxReportEntries || len(report.GetErrors()) > MaxReportEntries {
		return invalid("more than %d counters, health entries or errors", MaxReportEntries)
	}
	type counterKey struct {
		route string
		hop   uint32
		epoch string
	}
	seen := map[counterKey]bool{}
	for i, counter := range report.GetCounters() {
		if err := checkHop(counter.GetRouteId(), counter.GetHopIndex()); err != nil {
			return invalid("counters[%d]: %v", i, err)
		}
		if counter.GetNodeRef() != "" && counter.GetNodeRef() != nodeRef {
			return invalid("counters[%d]: names node %q", i, counter.GetNodeRef())
		}
		epoch := counter.GetCounterEpoch()
		if epoch == "" || len(epoch) > MaxEpochBytes || !utf8.ValidString(epoch) {
			return invalid("counters[%d]: counter_epoch must be 1 to %d bytes of UTF-8", i, MaxEpochBytes)
		}
		key := counterKey{counter.GetRouteId(), counter.GetHopIndex(), epoch}
		if seen[key] {
			return invalid("counters[%d]: %s hop %d epoch %q is listed twice", i, key.route, key.hop, key.epoch)
		}
		seen[key] = true
	}
	for i, hopError := range report.GetErrors() {
		if err := checkHop(hopError.GetRouteId(), hopError.GetHopIndex()); err != nil {
			return invalid("errors[%d]: %v", i, err)
		}
		if forwardv1.Engine_name[int32(hopError.GetEngine())] == "" {
			return invalid("errors[%d]: unknown engine", i)
		}
		if err := checkText(hopError.GetMessage()); err != nil {
			return invalid("errors[%d]: %v", i, err)
		}
	}
	for i, health := range report.GetHealth() {
		if err := checkHop(health.GetRouteId(), health.GetHopIndex()); err != nil {
			return invalid("health[%d]: %v", i, err)
		}
		if len(health.GetAddress()) > validate.MaxHostnameBytes || !utf8.ValidString(health.GetAddress()) || health.GetPort() > validate.MaxPort {
			return invalid("health[%d]: bad upstream address or port", i)
		}
		if forwardv1.HealthState_name[int32(health.GetState())] == "" {
			return invalid("health[%d]: unknown state", i)
		}
	}
	return nil
}

func checkHop(routeID string, hopIndex uint32) error {
	if !routeIDPattern.MatchString(routeID) {
		return errors.New("route_id must be 1 to 64 ASCII letters and digits")
	}
	if hopIndex >= validate.MaxHops {
		return fmt.Errorf("hop_index %d is not below %d", hopIndex, validate.MaxHops)
	}
	return nil
}
