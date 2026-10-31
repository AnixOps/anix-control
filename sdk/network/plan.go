package network

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/netip"
	"sort"

	networkv1 "github.com/AnixOps/anix-control/sdk/api/network/v1"
	"google.golang.org/protobuf/proto"
)

// HashOf answers a plan's hash: the SHA-256 (hex) of its deterministic
// protobuf encoding with generation and hash unset.
func HashOf(plan *networkv1.NetworkPlan) string {
	clone := proto.Clone(plan).(*networkv1.NetworkPlan)
	clone.Generation, clone.Hash = 0, ""
	b, err := proto.MarshalOptions{Deterministic: true}.Marshal(clone)
	if err != nil {
		panic("network: encoding plan: " + err.Error())
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Generation is what Control keeps per device between plans.
type Generation struct {
	Generation uint64
	Hash       string
}

// Stamp sets plan's hash and generation from the previous one: the
// generation stays while the hash is unchanged and grows by one otherwise
// (1 for a device without one).
func Stamp(plan *networkv1.NetworkPlan, previous Generation) Generation {
	hash := HashOf(plan)
	g := previous
	if g.Hash != hash || g.Generation == 0 {
		g = Generation{Generation: g.Generation + 1, Hash: hash}
	}
	plan.Generation, plan.Hash = g.Generation, g.Hash
	return g
}

// Unsupported lists what plan needs that a device with caps does not run:
// each (component, kind, version) missing from caps, and each egress
// routing table or rule priority outside the device's routing range, as
// sorted human-readable lines. A plan with any is refused whole
// (ConfigStatus error network_capability_missing).
func Unsupported(plan *networkv1.NetworkPlan, caps *networkv1.DeviceCapabilities) []string {
	runs := map[string]bool{}
	for _, support := range caps.GetKinds() {
		for _, version := range support.GetVersions() {
			runs[kindKey(support.GetComponent(), support.GetKind(), version)] = true
		}
	}
	missing := map[string]bool{}
	need := func(component networkv1.Component, kind string, version uint32) {
		if version == 0 {
			version = 1
		}
		if key := kindKey(component, kind, version); !runs[key] {
			missing[key] = true
		}
	}
	for _, ingress := range plan.GetIngresses() {
		need(networkv1.Component_COMPONENT_INGRESS, ingress.GetKind(), ingress.GetKindVersion())
	}
	first, last := RoutingRange(caps)
	for _, egress := range plan.GetEgresses() {
		need(networkv1.Component_COMPONENT_EGRESS, egress.GetKind(), egress.GetKindVersion())
		if routing := egress.GetRouting(); routing != nil {
			for name, value := range map[string]uint32{"table": routing.GetTable(), "rule priority": routing.GetRulePriority()} {
				if value != 0 && (value < first || value > last) {
					missing[fmt.Sprintf("egress %s %s %d outside %d–%d", egress.GetId(), name, value, first, last)] = true
				}
			}
		}
	}
	for _, transport := range plan.GetTransports() {
		need(networkv1.Component_COMPONENT_TRANSPORT, transport.GetKind(), transport.GetKindVersion())
	}
	out := make([]string, 0, len(missing))
	for line := range missing {
		out = append(out, line)
	}
	sort.Strings(out)
	return out
}

func kindKey(component networkv1.Component, kind string, version uint32) string {
	return fmt.Sprintf("%s %s v%d", component, kind, version)
}

// CheckStatus bounds a status for the stream's device deviceRef: device_ref
// is deviceRef; plan_hash empty or lowercase hex SHA-256; at most
// MaxComponents components, interfaces and keys; known enums, component ids
// and kind names; addresses that parse; text within MaxTextBytes; at most
// MaxForwardTargets target healths per port forward, each with a host and a
// port.
func CheckStatus(status *networkv1.NetworkStatus, deviceRef string) error {
	if status == nil {
		return invalid("status is required")
	}
	if deviceRef == "" || status.GetDeviceRef() != deviceRef {
		return invalid("status names device %q, not the stream's device %q", status.GetDeviceRef(), deviceRef)
	}
	if hash := status.GetPlanHash(); hash != "" && !hashPattern.MatchString(hash) {
		return invalid("plan_hash is not a lowercase hex SHA-256")
	}
	if len(status.GetComponents()) > MaxComponents || len(status.GetInterfaces()) > MaxComponents || len(status.GetPublicKeys()) > MaxComponents {
		return invalid("more than %d components, interfaces or keys", MaxComponents)
	}
	for i, component := range status.GetComponents() {
		if !knownComponent(component.GetComponent()) {
			return invalid("components[%d]: unknown component", i)
		}
		if !idPattern.MatchString(component.GetId()) {
			return invalid("components[%d]: bad id", i)
		}
		if networkv1.ComponentState_name[int32(component.GetState())] == "" {
			return invalid("components[%d]: unknown state", i)
		}
		if id := component.GetActiveEgressId(); id != "" && !idPattern.MatchString(id) {
			return invalid("components[%d]: bad active_egress_id", i)
		}
		for _, text := range []string{component.GetDetail(), component.GetCarrier()} {
			if err := checkText(text); err != nil {
				return invalid("components[%d]: %v", i, err)
			}
		}
	}
	if len(status.GetPortForwards()) > MaxComponents {
		return invalid("more than %d port forwards", MaxComponents)
	}
	for i, forward := range status.GetPortForwards() {
		if !idPattern.MatchString(forward.GetId()) {
			return invalid("port_forwards[%d]: bad id", i)
		}
		if networkv1.ComponentState_name[int32(forward.GetState())] == "" {
			return invalid("port_forwards[%d]: unknown state", i)
		}
		if err := checkText(forward.GetDetail()); err != nil {
			return invalid("port_forwards[%d]: %v", i, err)
		}
		if len(forward.GetTargets()) > MaxForwardTargets {
			return invalid("port_forwards[%d]: more than %d targets", i, MaxForwardTargets)
		}
		for j, target := range forward.GetTargets() {
			if target.GetHost() == "" || target.GetPort() == 0 || target.GetPort() > MaxPort {
				return invalid("port_forwards[%d]: targets[%d]: host and a port from 1 to %d are required", i, j, MaxPort)
			}
			if err := checkText(target.GetHost()); err != nil {
				return invalid("port_forwards[%d]: targets[%d]: %v", i, j, err)
			}
			if networkv1.TargetHealthState_name[int32(target.GetState())] == "" {
				return invalid("port_forwards[%d]: targets[%d]: unknown state", i, j)
			}
			if target.GetCheckedAtUnixMs() < 0 || target.GetCircuitOpenUntilUnixMs() < 0 {
				return invalid("port_forwards[%d]: targets[%d]: negative time", i, j)
			}
		}
	}
	if err := checkReverseProxyStatus(status.GetReverseProxies()); err != nil {
		return err
	}
	for i, iface := range status.GetInterfaces() {
		if err := checkText(iface.GetName()); err != nil {
			return invalid("interfaces[%d]: %v", i, err)
		}
		if len(iface.GetAddresses()) > 256 {
			return invalid("interfaces[%d]: more than 256 addresses", i)
		}
		for _, address := range append([]string{iface.GetGatewayV4(), iface.GetGatewayV6()}, iface.GetAddresses()...) {
			if address == "" {
				continue
			}
			if _, err := netip.ParsePrefix(address); err != nil {
				if _, err := netip.ParseAddr(address); err != nil {
					return invalid("interfaces[%d]: %q is not an address or prefix", i, address)
				}
			}
		}
	}
	for i, key := range status.GetPublicKeys() {
		if !kindPattern.MatchString(key.GetKind()) || key.GetKey() == "" {
			return invalid("public_keys[%d]: needs a kind and a key", i)
		}
		if err := checkKey(key.GetKey()); err != nil {
			return invalid("public_keys[%d]: %v", i, err)
		}
	}
	return nil
}
