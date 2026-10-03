package validate

import (
	"fmt"
	"strings"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
)

// Code says which rule a Violation breaks, so a UI or API client can react
// to it (highlight a field, translate the message) without parsing text.
// Codes are stable: a new rule gets a new code, an existing code keeps its
// meaning.
type Code string

const (
	// CodeRequired: a field that must be set is empty or unspecified.
	CodeRequired Code = "required"
	// CodeInvalidFormat: the value's syntax is wrong (address, host name,
	// node reference, ULID, label, path).
	CodeInvalidFormat Code = "invalid_format"
	// CodeInvalidEnum: an enum value the contract does not define.
	CodeInvalidEnum Code = "invalid_enum"
	// CodeOutOfRange: a number outside its allowed range.
	CodeOutOfRange Code = "out_of_range"
	// CodeTooMany: a list or map longer than its cap.
	CodeTooMany Code = "too_many"
	// CodeTooLong: a string longer than its cap.
	CodeTooLong Code = "too_long"
	// CodeTooLarge: the encoded route is larger than MaxRouteBytes.
	CodeTooLarge Code = "too_large"
	// CodeDuplicate: a node, target or other entry that appears twice.
	CodeDuplicate Code = "duplicate"
	// CodeInvalidRole: a hop's role does not match its place in the chain.
	CodeInvalidRole Code = "invalid_role"
	// CodeEngineNotEnabled: ENGINE_ANIXOPS before it is enabled (v4.3).
	CodeEngineNotEnabled Code = "engine_not_enabled"
	// CodeLinkUnsupported: the dialling or the listening hop's engine cannot
	// carry the link (forward-sdk.md section 4.2).
	CodeLinkUnsupported Code = "link_unsupported"
	// CodeNotApplicable: a field that means nothing in its context.
	CodeNotApplicable Code = "not_applicable"
	// CodeRequiresSingleNode: dial_address on a hop with several nodes.
	CodeRequiresSingleNode Code = "requires_single_node"
	// CodeForbidden: the owner may not use this setting
	// (TARGET_POLICY_ALLOW_PRIVATE on a user's route).
	CodeForbidden Code = "forbidden"
	// CodeTargetNotAllowed: the target policy refuses the target.
	CodeTargetNotAllowed Code = "target_not_allowed"
	// CodeInvalidRelation: two fields that contradict each other.
	CodeInvalidRelation Code = "invalid_relation"
	// CodeUnusedHops: DIRECT_MODE_FORCED on a route with later hops.
	CodeUnusedHops Code = "unused_hops"
	// CodeExpired: expires_at is not in the future when the route is
	// created.
	CodeExpired Code = "expired"
	// CodeUnknownNode: a node reference that is not in the inventory.
	CodeUnknownNode Code = "unknown_node"
	// CodeEngineNotAdvertised: a node that does not advertise the hop's
	// engine.
	CodeEngineNotAdvertised Code = "engine_not_advertised"
	// CodeEngineUnavailable: a node that advertises the hop's engine as
	// unavailable.
	CodeEngineUnavailable Code = "engine_unavailable"
	// CodeCapabilityMissing: a node's engine lacks a feature the route
	// needs (link security, strategy, UDP, IPv6, a limit).
	CodeCapabilityMissing Code = "capability_missing"
	// CodePortOutOfRange: a port outside the node's allocation range.
	CodePortOutOfRange Code = "port_out_of_range"
	// CodePortReserved: a port reserved on the node (SSH, the Agent's own
	// ports, a per-node list).
	CodePortReserved Code = "port_reserved"
)

// Violation is one reason a route is refused.
type Violation struct {
	// Field is a path into the route with the contract's field names
	// ("hops[1].ingress.security", `labels["k"]`); empty for the route as a
	// whole.
	Field   string
	Code    Code
	Message string
}

func (v Violation) Error() string {
	if v.Field == "" {
		return fmt.Sprintf("%s (%s)", v.Message, v.Code)
	}
	return fmt.Sprintf("%s: %s (%s)", v.Field, v.Message, v.Code)
}

// ToProto converts the violation to the contract.
func (v Violation) ToProto() *forwardv1.Violation {
	return &forwardv1.Violation{Field: v.Field, Message: v.Message, Code: string(v.Code)}
}

// Violations are every reason a route is refused, in rule order. Empty
// means the route is valid.
type Violations []Violation

func (vs Violations) Error() string {
	parts := make([]string, len(vs))
	for i, v := range vs {
		parts[i] = v.Error()
	}
	return strings.Join(parts, "; ")
}

// Err answers nil for no violations, else vs as an error.
func (vs Violations) Err() error {
	if len(vs) == 0 {
		return nil
	}
	return vs
}

// Has reports whether vs holds a violation of field with code.
func (vs Violations) Has(field string, code Code) bool {
	for _, v := range vs {
		if v.Field == field && v.Code == code {
			return true
		}
	}
	return false
}

// ToProto converts the violations to the contract.
func (vs Violations) ToProto() []*forwardv1.Violation {
	out := make([]*forwardv1.Violation, len(vs))
	for i, v := range vs {
		out[i] = v.ToProto()
	}
	return out
}

// collector gathers violations.
type collector struct {
	out Violations
}

func (c *collector) add(field string, code Code, format string, args ...any) {
	c.out = append(c.out, Violation{Field: field, Code: code, Message: fmt.Sprintf(format, args...)})
}

func hopField(i int, name string) string {
	return fmt.Sprintf("hops[%d].%s", i, name)
}

func targetField(i int, name string) string {
	return fmt.Sprintf("targets[%d].%s", i, name)
}

func hopNodeField(i, j int) string {
	return fmt.Sprintf("hops[%d].node_refs[%d]", i, j)
}
