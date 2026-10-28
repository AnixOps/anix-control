// Package network is how the device network model rides the Agent Control
// stream (anixops.network.v1; docs/design/network-model section 6.4).
// Control and the Agent both use it, so they encode and check the same
// bytes:
//
//   - Hello: the Agent lists the capability network.v1
//     (agentcontrol.CapabilityNetwork) with its DeviceCapabilities as
//     protojson in the attribute "device_capabilities" (HelloCapability,
//     DeviceCapabilitiesFromHello).
//   - Desired state: Control sends the device's NetworkPlan as the member
//     "network" of the anixops.nodeconfig document of a ConfigSnapshot
//     (PlanMember, PlanFromNodeConfig), so the plan and the protocol
//     configuration share one config_revision. Only a session that
//     negotiated network.v1 gets the member.
//   - Status: the Agent sends its NetworkStatus as a PackageReport with
//     plugin_id "network", kind "network.status" and version "v1"
//     (StatusReport, IsStatusReport, DecodeStatus).
//
// Check validates a plan on its own (the planner invariants of design
// section 10 that one device's plan can show); Unsupported lists what a
// plan needs that a device does not run. HashOf and Stamp give plans their
// hash and generation as the forward planner does.
//
// Protojson is written with the proto field names (device_ref), as in the
// contract fixtures (contracts/network/v1); readers accept both spellings
// and ignore unknown fields, so a newer Agent's additions do not break an
// older Control.
package network

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	agentv1pb "github.com/AnixOps/anix-control/sdk/api/agent/v1"
	networkv1 "github.com/AnixOps/anix-control/sdk/api/network/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

const (
	// AttributeDeviceCapabilities is the attribute of the Hello's
	// network.v1 capability that holds the DeviceCapabilities as protojson.
	AttributeDeviceCapabilities = "device_capabilities"
	// MaxDeviceCapabilitiesBytes caps that attribute.
	MaxDeviceCapabilitiesBytes = 16 << 10

	// NodeConfigMember is the nodeconfig document member holding the plan.
	NodeConfigMember = "network"

	// StatusPluginID, StatusKind and StatusVersion name a NetworkStatus
	// PackageReport. Control accepts it from a session that negotiated
	// network.v1, not from a plugin release.
	StatusPluginID = "network"
	StatusKind     = "network.status"
	StatusVersion  = "v1"
)

// ErrInvalid wraps every refusal of a capability attribute, plan or status.
var ErrInvalid = errors.New("network wire: invalid")

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

var (
	marshal   = protojson.MarshalOptions{UseProtoNames: true}
	unmarshal = protojson.UnmarshalOptions{DiscardUnknown: true}
)

// HelloCapability answers the network.v1 capability an Agent lists in its
// Hello, with caps in its attribute.
func HelloCapability(caps *networkv1.DeviceCapabilities) (*agentv1pb.Capability, error) {
	if err := CheckDeviceCapabilities(caps, caps.GetDeviceRef()); err != nil {
		return nil, err
	}
	encoded, err := marshal.Marshal(caps)
	if err != nil {
		return nil, fmt.Errorf("network wire: encode device capabilities: %w", err)
	}
	if len(encoded) > MaxDeviceCapabilitiesBytes {
		return nil, invalid("device capabilities exceed %d bytes", MaxDeviceCapabilitiesBytes)
	}
	return &agentv1pb.Capability{
		Name: agentcontrol.CapabilityNetwork, Version: agentcontrol.CapabilityVersionV1,
		Attributes: map[string]string{AttributeDeviceCapabilities: string(encoded)},
	}, nil
}

// DeviceCapabilitiesFromHello finds network.v1 among a Hello's capabilities
// and decodes its device capabilities for the stream's device deviceRef.
// listed is false when the Hello does not list network.v1. An error means it
// is listed but its attribute is missing, oversize or malformed, or names
// another device: Control then does not serve network.v1 on the session.
func DeviceCapabilitiesFromHello(capabilities []*agentv1pb.Capability, deviceRef string) (caps *networkv1.DeviceCapabilities, listed bool, err error) {
	var capability *agentv1pb.Capability
	for _, c := range capabilities {
		if c.GetName() == agentcontrol.CapabilityNetwork && c.GetVersion() == agentcontrol.CapabilityVersionV1 {
			capability = c
			break
		}
	}
	if capability == nil {
		return nil, false, nil
	}
	raw, ok := capability.GetAttributes()[AttributeDeviceCapabilities]
	if !ok || raw == "" {
		return nil, true, invalid("network.v1 has no %s attribute", AttributeDeviceCapabilities)
	}
	if len(raw) > MaxDeviceCapabilitiesBytes {
		return nil, true, invalid("%s exceeds %d bytes", AttributeDeviceCapabilities, MaxDeviceCapabilitiesBytes)
	}
	caps = &networkv1.DeviceCapabilities{}
	if err := unmarshal.Unmarshal([]byte(raw), caps); err != nil {
		return nil, true, invalid("%s is not a DeviceCapabilities: %v", AttributeDeviceCapabilities, err)
	}
	if err := CheckDeviceCapabilities(caps, deviceRef); err != nil {
		return nil, true, err
	}
	if caps.GetDeviceRef() == "" {
		caps.DeviceRef = deviceRef
	}
	return caps, true, nil
}

// PlanMember answers plan as the value of the nodeconfig member "network":
// its protojson with proto field names, decoded into generic JSON values,
// so encoding/json writes it with sorted keys and the document's bytes (and
// so its hash) depend only on the plan.
func PlanMember(plan *networkv1.NetworkPlan) (map[string]any, error) {
	encoded, err := marshal.Marshal(plan)
	if err != nil {
		return nil, fmt.Errorf("network wire: encode plan: %w", err)
	}
	member := map[string]any{}
	if err := json.Unmarshal(encoded, &member); err != nil {
		return nil, fmt.Errorf("network wire: decode plan: %w", err)
	}
	return member, nil
}

// PlanFromNodeConfig answers the NetworkPlan of a ConfigSnapshot's
// document, for the Agent. found is false for a document without the
// member: the Agent then runs without a plan, as before network.v1.
func PlanFromNodeConfig(configJSON []byte) (plan *networkv1.NetworkPlan, found bool, err error) {
	var document map[string]json.RawMessage
	if err := json.Unmarshal(configJSON, &document); err != nil {
		return nil, false, invalid("node configuration is not a JSON object: %v", err)
	}
	raw, ok := document[NodeConfigMember]
	if !ok {
		return nil, false, nil
	}
	plan, err = DecodePlan(raw)
	return plan, true, err
}

// DecodePlan decodes a plan's protojson (a contract fixture, the member).
func DecodePlan(raw []byte) (*networkv1.NetworkPlan, error) {
	plan := &networkv1.NetworkPlan{}
	if err := unmarshal.Unmarshal(raw, plan); err != nil {
		return nil, invalid("member %q is not a NetworkPlan: %v", NodeConfigMember, err)
	}
	return plan, nil
}

// StatusReport answers the PackageReport that carries status.
func StatusReport(status *networkv1.NetworkStatus) (*agentv1pb.PackageReport, error) {
	if err := CheckStatus(status, status.GetDeviceRef()); err != nil {
		return nil, err
	}
	payload, err := marshal.Marshal(status)
	if err != nil {
		return nil, fmt.Errorf("network wire: encode status: %w", err)
	}
	if err := agentcontrol.ValidatePackageReportPayloadSize(payload); err != nil {
		return nil, invalid("%v", err)
	}
	return &agentv1pb.PackageReport{
		PluginId: StatusPluginID, Kind: StatusKind, Version: StatusVersion,
		PayloadJson: payload, ObservedAtUnixMs: status.GetObservedAtUnixMs(),
	}, nil
}

// IsStatusReport tells whether a PackageReport is a network status:
// plugin_id "network" and kind "network.status".
func IsStatusReport(report *agentv1pb.PackageReport) bool {
	return report.GetPluginId() == StatusPluginID && report.GetKind() == StatusKind
}

// DecodeStatus decodes a status report's payload and checks it for the
// stream's device deviceRef (CheckStatus).
func DecodeStatus(payload []byte, deviceRef string) (*networkv1.NetworkStatus, error) {
	if err := agentcontrol.ValidatePackageReportPayloadSize(payload); err != nil {
		return nil, invalid("%v", err)
	}
	status := &networkv1.NetworkStatus{}
	if err := unmarshal.Unmarshal(payload, status); err != nil {
		return nil, invalid("payload is not a NetworkStatus: %v", err)
	}
	if err := CheckStatus(status, deviceRef); err != nil {
		return nil, err
	}
	return status, nil
}
